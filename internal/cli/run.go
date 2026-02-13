// Package cli provides the command-line interface for ralph.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/arvesolland/ralph/internal/board"
	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/notify"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/arvesolland/ralph/internal/worker"
	"github.com/arvesolland/ralph/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	runPlanID         int
	runMaxIterations  int
	runCompletionMode string
	runPush           bool
	runTimeout        time.Duration
)

var runCmd = &cobra.Command{
	Use:   "run --plan <plan-id>",
	Short: "Run the iteration loop on a plan",
	Long: `Execute the iteration loop on a specified Board plan.

The iteration loop will:
1. Fetch plan details from Board
2. Set plan status to active
3. Create/reuse a git worktree for the plan's branch
4. Build a prompt from Board context
5. Execute Claude to work on the plan
6. Check for completion via Board task stats
7. Commit changes after each iteration
8. Repeat until plan is complete or max iterations reached
9. On completion: create PR (default), merge, or push branch only

Example:
  ralph run --plan 42
  ralph run --plan 42 --max 100
  ralph run --plan 42 --completion-mode merge`,
	RunE: runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().IntVar(&runPlanID, "plan", 0, "Board plan ID (required)")
	runCmd.Flags().IntVar(&runMaxIterations, "max", runner.DefaultMaxIterations, "maximum iterations before stopping")
	runCmd.Flags().StringVar(&runCompletionMode, "completion-mode", "", "completion mode: pr, merge, or branch (default from config)")
	runCmd.Flags().BoolVar(&runPush, "push", false, "push to remote after each iteration")
	runCmd.Flags().DurationVar(&runTimeout, "timeout", 0, "timeout per iteration (e.g., 90m, 2h) (default from config or 60m)")
	_ = runCmd.MarkFlagRequired("plan")
}

func runRun(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.LoadWithDefaults(GetConfigPath())
	if err != nil {
		log.Warn("Failed to load config, using defaults: %v", err)
		cfg = config.Defaults()
	}

	// Create Board client
	boardClient := cfg.BoardClient()

	// Determine project slug
	projectSlug := cfg.Board.ProjectSlug
	if projectSlug == "" {
		return fmt.Errorf("board.project_slug not configured; run 'ralph init' or set it in .ralph/config.yaml")
	}

	// Determine completion mode
	completionMode := runCompletionMode
	if completionMode == "" {
		completionMode = cfg.Completion.Mode
	}
	if completionMode == "" {
		completionMode = "pr"
	}

	// Determine iteration timeout (flag > config > default)
	iterationTimeout := runTimeout
	if iterationTimeout == 0 && cfg.Runner.IterationTimeout != "" {
		if parsed, parseErr := time.ParseDuration(cfg.Runner.IterationTimeout); parseErr == nil {
			iterationTimeout = parsed
		} else {
			log.Warn("Invalid runner.iteration_timeout in config: %v", parseErr)
		}
	}
	if iterationTimeout == 0 {
		iterationTimeout = runner.IterationTimeout
	}

	// Fetch plan details
	plan, err := boardClient.GetPlan(runPlanID)
	if err != nil {
		return fmt.Errorf("fetching plan #%d: %w", runPlanID, err)
	}

	// Resolve branch name (uses branch or generates from title)
	branch := plan.BranchName()

	log.Info("Plan: %s (ID: %d, status: %s)", plan.Title, plan.ID, plan.Status)
	log.Info("Branch: %s", branch)
	log.Info("Max iterations: %d", runMaxIterations)
	log.Info("Iteration timeout: %v", iterationTimeout)
	log.Info("Completion mode: %s", completionMode)

	// Set plan status to active if not already
	if plan.Status != board.PlanStatusActive {
		updated, err := boardClient.UpdatePlanStatus(plan.ID, board.PlanStatusActive)
		if err != nil {
			return fmt.Errorf("activating plan #%d: %w", plan.ID, err)
		}
		plan = updated
		log.Info("Plan status set to active")
	}

	// Get working directory (main worktree)
	mainWorktreePath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Initialize git
	g := git.NewGit(mainWorktreePath)

	// Verify we're in a git repo
	repoRoot, err := g.RepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Set up paths
	configDir := filepath.Join(repoRoot, ".ralph")
	worktreesDir := filepath.Join(configDir, "worktrees")

	// Ensure worktrees directory exists
	if err := os.MkdirAll(worktreesDir, 0755); err != nil {
		return fmt.Errorf("creating worktrees directory: %w", err)
	}

	// Initialize worktree manager and create/get worktree
	wtManager, err := worktree.NewManager(g, worktreesDir)
	if err != nil {
		return fmt.Errorf("initializing worktree manager: %w", err)
	}

	wtInfo := worktree.PlanInfo{
		Name:   plan.Title,
		Branch: branch,
	}

	var worktreePath string
	if wtManager.Exists(wtInfo) {
		wt, err := wtManager.Get(wtInfo)
		if err != nil {
			return fmt.Errorf("getting existing worktree: %w", err)
		}
		if wt != nil {
			worktreePath = wt.Path
			log.Info("Reusing existing worktree: %s", worktreePath)
		}
	}

	if worktreePath == "" {
		wt, err := wtManager.Create(wtInfo)
		if err != nil {
			return fmt.Errorf("creating worktree: %w", err)
		}
		worktreePath = wt.Path
		log.Info("Created worktree: %s", worktreePath)
	}

	// Create or load execution context
	ctxPath := runner.ContextPath(worktreePath)
	var execCtx *runner.Context

	existingCtx, err := runner.LoadContext(ctxPath)
	if err == nil && existingCtx.PlanID == plan.ID {
		execCtx = existingCtx
		log.Info("Resuming at iteration %d", execCtx.Iteration)
	} else {
		execCtx = runner.NewContext(plan.ID, branch, cfg.Git.BaseBranch, runMaxIterations)
		if err := runner.SaveContext(execCtx, ctxPath); err != nil {
			return fmt.Errorf("saving context: %w", err)
		}
	}

	// Initialize prompt builder
	promptsDir := filepath.Join(configDir, "prompts")
	promptBuilder := prompt.NewBuilder(cfg, configDir, promptsDir)

	// Set up Slack notifications
	var notifier notify.Notifier = &notify.NoopNotifier{}
	trackerPath := notify.ThreadTrackerPath(configDir)
	tracker, err := notify.NewThreadTracker(trackerPath)
	if err != nil {
		log.Warn("Failed to create thread tracker: %v", err)
	} else {
		notifier = notify.NewNotifier(cfg, tracker)
	}

	// Start Socket Mode bot for thread reply tracking (eyes reaction + feedback)
	if cfg.Slack.Channel != "" && tracker != nil {
		bot := notify.StartBotIfConfigured(context.Background(), tracker, configDir, cfg.Slack.Channel)
		if bot != nil {
			log.Info("Socket Mode bot started for thread reply tracking")
			defer bot.Stop()
		}
	}

	// Build PlanInfo for notifications
	planInfo := &worker.PlanInfo{
		ID:     plan.ID,
		Name:   plan.Title,
		Branch: branch,
	}

	// Send start notification (seed thread from Board if available)
	npi := notify.PlanInfo{Name: planInfo.Name, Branch: planInfo.Branch}
	if cfg.Slack.NotifyStart {
		threadSeeded := false

		// If Board has a Slack thread URL, seed the tracker so we resume the existing thread
		if plan.SlackThreadURL != "" {
			channelID, threadTS, parseErr := notify.ParseSlackThreadURL(plan.SlackThreadURL)
			if parseErr == nil {
				if sn, ok := notifier.(*notify.SlackNotifier); ok && sn != nil {
					_ = sn.SeedThread(npi.Name, npi.Branch, channelID, threadTS)
					log.Info("Resumed Slack thread from Board: %s", plan.SlackThreadURL)
					threadSeeded = true
				}
			} else {
				log.Warn("Failed to parse Slack thread URL from Board: %v", parseErr)
			}
		}

		if !threadSeeded {
			// No existing thread — create a new one
			if err := notifier.Start(npi); err != nil {
				log.Debug("Failed to send start notification: %v", err)
			}

			// Save the new thread URL back to Board
			if sn, ok := notifier.(*notify.SlackNotifier); ok && sn != nil {
				threadURL := sn.GetThreadURL(npi.Name)
				if threadURL != "" {
					if _, err := boardClient.UpdatePlan(plan.ID, map[string]string{"slack-thread-url": threadURL}); err != nil {
						log.Warn("Failed to save Slack thread URL to Board: %v", err)
					} else {
						log.Info("Saved Slack thread URL to Board: %s", threadURL)
					}
				}
			}
		}

		// Update the status card with task stats from Board
		progress := &notify.ProgressStatus{
			Iteration:     0,
			MaxIterations: runMaxIterations,
			Phase:         notify.PhaseInitializing,
		}
		if agentCtx, statsErr := boardClient.PlanContext(plan.ID); statsErr == nil {
			progress.TasksDone = agentCtx.Stats.Done + agentCtx.Stats.Skipped
			progress.TasksTotal = agentCtx.Stats.TotalTasks
		}
		_ = notifier.UpdateProgress(npi, progress)
	}

	// Create Claude runner
	claudeRunner := runner.NewCLIRunner()

	// Create iteration loop
	wtGit := git.NewGit(worktreePath)
	loop := runner.NewIterationLoop(runner.LoopConfig{
		Board:            boardClient,
		PlanID:           plan.ID,
		ProjectSlug:      projectSlug,
		Context:          execCtx,
		Config:           cfg,
		Runner:           claudeRunner,
		Git:              wtGit,
		PromptBuilder:    promptBuilder,
		WorktreePath:     worktreePath,
		IterationTimeout: iterationTimeout,
		OnIteration: func(iteration int, result *runner.Result) {
			log.Info("Iteration %d/%d complete", iteration, runMaxIterations)
			if result.IsComplete {
				log.Info("Completion marker detected")
			}
			// Fetch Board task stats for progress display
			progress := &notify.ProgressStatus{
				Iteration:     iteration,
				MaxIterations: runMaxIterations,
				Phase:         notify.PhaseRunning,
			}
			if agentCtx, err := boardClient.PlanContext(plan.ID); err == nil {
				progress.TasksDone = agentCtx.Stats.Done + agentCtx.Stats.Skipped
				progress.TasksTotal = agentCtx.Stats.TotalTasks
				log.Info("Tasks: %d/%d done", progress.TasksDone, progress.TasksTotal)
			} else {
				log.Warn("Failed to fetch Board task stats for Slack progress: %v", err)
			}
			_ = notifier.UpdateProgress(npi, progress)
			if cfg.Slack.NotifyIteration {
				if err := notifier.Iteration(npi, iteration, runMaxIterations); err != nil {
					log.Debug("Failed to send iteration notification: %v", err)
				}
			}
		},
		OnBlocker: func(blocker *runner.Blocker) {
			log.Warn("Blocker detected: %s", blocker.Description)
			if blocker.Action != "" {
				log.Info("Action required: %s", blocker.Action)
			}
			blockerProgress := &notify.ProgressStatus{
				Iteration:     execCtx.Iteration,
				MaxIterations: runMaxIterations,
				Phase:         notify.PhaseBlocked,
				Message:       blocker.Description,
			}
			if agentCtx, err := boardClient.PlanContext(plan.ID); err == nil {
				blockerProgress.TasksDone = agentCtx.Stats.Done + agentCtx.Stats.Skipped
				blockerProgress.TasksTotal = agentCtx.Stats.TotalTasks
			}
			_ = notifier.UpdateProgress(npi, blockerProgress)
			if cfg.Slack.NotifyBlocker {
				if err := notifier.BlockerNotify(npi, &notify.Blocker{
					Content:     blocker.Content,
					Description: blocker.Description,
					Action:      blocker.Action,
					Resume:      blocker.Resume,
					Hash:        blocker.Hash,
				}); err != nil {
					log.Debug("Failed to send blocker notification: %v", err)
				}
			}
		},
		OnAfterCommit: func() {
			if runPush {
				if err := wtGit.PushWithUpstream("origin", branch); err != nil {
					log.Warn("Failed to push after iteration: %v", err)
				}
			}
		},
	})

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Warn("Received signal %v, stopping after current iteration...", sig)
		cancel()
	}()

	// Run the iteration loop
	result := loop.Run(ctx)

	// Report results
	fmt.Println()
	fmt.Println("==============================")
	fmt.Printf("Iterations completed: %d/%d\n", result.Iterations, runMaxIterations)

	if result.Completed {
		log.Success("Plan completed successfully!")

		// Handle completion (PR/merge/branch)
		prURL, err := handleCompletion(plan, completionMode, worktreePath, cfg, wtGit, g, claudeRunner, promptBuilder)
		if err != nil {
			log.Error("Completion failed: %v", err)
			return fmt.Errorf("completion: %w", err)
		}

		// Update Board with PR URL
		if prURL != "" {
			if _, err := boardClient.UpdatePlan(plan.ID, map[string]string{"pr-url": prURL}); err != nil {
				log.Warn("Failed to update Board with PR URL: %v", err)
			}
		}

		// Update Board status
		if _, err := boardClient.UpdatePlanStatus(plan.ID, board.PlanStatusComplete); err != nil {
			log.Error("Failed to update Board status: %v", err)
		}

		// Clean up worktree
		if err := wtManager.Remove(wtInfo, false); err != nil {
			log.Warn("Failed to remove worktree: %v", err)
		}

		// Send completion notification
		completeProgress := &notify.ProgressStatus{
			Iteration:     result.Iterations,
			MaxIterations: runMaxIterations,
			Phase:         notify.PhaseComplete,
		}
		if agentCtx, err := boardClient.PlanContext(plan.ID); err == nil {
			completeProgress.TasksDone = agentCtx.Stats.Done + agentCtx.Stats.Skipped
			completeProgress.TasksTotal = agentCtx.Stats.TotalTasks
		}
		_ = notifier.UpdateProgress(npi, completeProgress)
		if cfg.Slack.NotifyComplete {
			_ = notifier.Complete(npi, prURL)
		}
		notifier.Flush()
		return nil
	}

	if result.Error != nil {
		// Send error notification
		if cfg.Slack.NotifyError {
			_ = notifier.Error(npi, result.Error)
		}
		_ = notifier.UpdateProgress(npi, &notify.ProgressStatus{
			Iteration:     result.Iterations,
			MaxIterations: runMaxIterations,
			Phase:         notify.PhaseError,
			Message:       result.Error.Error(),
		})

		notifier.Flush()
		if errors.Is(result.Error, context.Canceled) {
			log.Warn("Execution interrupted by user")
			return nil
		}
		return fmt.Errorf("execution failed: %w", result.Error)
	}

	if result.FinalBlocker != nil {
		log.Warn("Execution stopped on blocker: %s", result.FinalBlocker.Description)
		return nil
	}

	return fmt.Errorf("plan not completed after %d iterations", result.Iterations)
}

// handleCompletion handles the plan completion workflow based on mode.
// Returns the PR URL (if applicable) and any error.
func handleCompletion(plan *board.Plan, mode, worktreePath string, cfg *config.Config, wtGit, mainGit git.Git, _ runner.Runner, _ *prompt.Builder) (string, error) {
	baseBranch := cfg.Git.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	featureBranch := plan.BranchName()

	switch mode {
	case "pr":
		// Push and create PR
		if err := wtGit.PushWithUpstream("origin", featureBranch); err != nil {
			return "", fmt.Errorf("pushing branch: %w", err)
		}
		prURL, err := createPR(plan.Title, worktreePath)
		if err != nil {
			log.Warn("Failed to create PR: %v", err)
			log.Warn("Branch has been pushed. Create PR manually for branch: %s", featureBranch)
			return "", nil
		}
		log.Success("PR created: %s", prURL)
		return prURL, nil

	case "merge":
		if err := worker.CompleteMerge(featureBranch, baseBranch, mainGit); err != nil {
			return "", fmt.Errorf("merge completion: %w", err)
		}

	case "branch":
		if err := worker.CompleteBranch(featureBranch, baseBranch, wtGit); err != nil {
			return "", fmt.Errorf("branch completion: %w", err)
		}

	default:
		return "", fmt.Errorf("unknown completion mode: %s", mode)
	}

	return "", nil
}

// createPR creates a pull request using the gh CLI.
func createPR(title, workDir string) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh CLI not installed")
	}

	prTitle := fmt.Sprintf("feat: %s", title)
	body := fmt.Sprintf("## Summary\n\nImplements: %s\n\n---\n\nGenerated by [Ralph](https://github.com/arvesolland/ralph)", title)

	cmd := exec.Command("gh", "pr", "create", "--title", prTitle, "--body", body)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errOutput := stderr.String()
		if strings.Contains(errOutput, "already exists") {
			// PR already exists - get its URL
			viewCmd := exec.Command("gh", "pr", "view", "--json", "url", "-q", ".url")
			viewCmd.Dir = workDir
			var viewOut bytes.Buffer
			viewCmd.Stdout = &viewOut
			if viewErr := viewCmd.Run(); viewErr == nil {
				return strings.TrimSpace(viewOut.String()), nil
			}
		}
		return "", fmt.Errorf("gh pr create: %s: %w", errOutput, err)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = strings.TrimSpace(stderr.String())
	}
	return output, nil
}
