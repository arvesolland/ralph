// Package cli provides the command-line interface for ralph.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/notify"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/spf13/cobra"
)

var maxIterations int
var iterationTimeout time.Duration

var runCmd = &cobra.Command{
	Use:   "run <plan-file>",
	Short: "Run the iteration loop on a plan",
	Long: `Execute the iteration loop on a specified plan file.

The iteration loop will:
1. Build a prompt from the plan and context
2. Execute Claude to work on the plan
3. Check for completion markers
4. Verify completion with a secondary model (Haiku)
5. Commit changes after each iteration
6. Repeat until plan is complete or max iterations reached

Example:
  ralph run plans/current/my-feature.md
  ralph run plans/pending/fix-bug.md --max 50`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().IntVar(&maxIterations, "max", runner.DefaultMaxIterations, "maximum iterations before stopping")
	runCmd.Flags().DurationVar(&iterationTimeout, "timeout", runner.IterationTimeout, "timeout per iteration (e.g., 60m, 2h)")
}

func runRun(cmd *cobra.Command, args []string) error {
	planPath := args[0]

	// Validate plan file exists
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		return fmt.Errorf("plan file does not exist: %s", planPath)
	}

	// Make path absolute
	absPlanPath, err := filepath.Abs(planPath)
	if err != nil {
		return fmt.Errorf("resolving plan path: %w", err)
	}

	// Load the plan
	p, err := plan.Load(absPlanPath)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}

	// Auto-migrate flat files to bundles
	if !p.IsBundle() {
		log.Info("Migrating flat file to bundle: %s", p.Name)
		if err := plan.MigrateToBundle(p); err != nil {
			return fmt.Errorf("migrating to bundle: %w", err)
		}
		log.Info("Created bundle at: %s", p.BundleDir)
	}

	log.Info("Running plan: %s", p.Name)
	log.Info("Branch: %s", p.Branch)
	log.Info("Max iterations: %d", maxIterations)
	log.Info("Iteration timeout: %v", iterationTimeout)

	// Load configuration
	cfg, err := config.LoadWithDefaults(GetConfigPath())
	if err != nil {
		log.Warn("Failed to load config, using defaults: %v", err)
		cfg = config.Defaults()
	}

	// Check for API key - warn if verification won't work
	if !runner.IsAPIKeyAvailable() {
		log.Warn("ANTHROPIC_API_KEY not set - plan verification will be skipped")
		log.Warn("Plans may be marked complete without AI verification")
		fmt.Print("Continue without verification? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			log.Info("Set ANTHROPIC_API_KEY and try again")
			return nil
		}
	}

	// Determine worktree path (current directory for now - worker will handle actual worktree)
	worktreePath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Initialize git
	g := git.NewGit(worktreePath)

	// Verify we're in a git repo
	_, err = g.RepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Get current branch for context
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Remove stale context.json if it exists (prevents agent confusion from leftover state)
	ctxPath := runner.ContextPath(worktreePath)
	if err := os.Remove(ctxPath); err != nil && !os.IsNotExist(err) {
		log.Warn("Failed to remove stale context file: %v", err)
	}

	// Create fresh execution context with paths relative to working directory
	execCtx := runner.NewContext(p, cfg.Git.BaseBranch, maxIterations, worktreePath)
	execCtx.FeatureBranch = currentBranch // Override with actual current branch

	// Save the fresh context so the agent can read it
	if err := runner.SaveContext(execCtx, ctxPath); err != nil {
		return fmt.Errorf("saving context: %w", err)
	}

	// Initialize prompt builder
	configDir := filepath.Dir(GetConfigPath())
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

	// Send start notification
	if cfg.Slack.NotifyStart {
		if err := notifier.Start(p); err != nil {
			log.Debug("Failed to send start notification: %v", err)
		}
	}

	// Create CLI runner
	claudeRunner := runner.NewCLIRunner()

	// Create iteration loop
	loop := runner.NewIterationLoop(runner.LoopConfig{
		Plan:             p,
		Context:          execCtx,
		Config:           cfg,
		Runner:           claudeRunner,
		Git:              g,
		PromptBuilder:    promptBuilder,
		WorktreePath:     worktreePath,
		IterationTimeout: iterationTimeout,
		OnIteration: func(iteration int, result *runner.Result) {
			log.Info("Iteration %d/%d complete", iteration, maxIterations)
			if result.IsComplete {
				log.Info("Completion marker detected")
			}
			// Update Slack progress
			notifier.UpdateProgress(p, &notify.ProgressStatus{
				Iteration:     iteration,
				MaxIterations: maxIterations,
				Phase:         notify.PhaseRunning,
			})
			// Send iteration notification if configured
			if cfg.Slack.NotifyIteration {
				if err := notifier.Iteration(p, iteration, maxIterations); err != nil {
					log.Debug("Failed to send iteration notification: %v", err)
				}
			}
		},
		OnBlocker: func(blocker *runner.Blocker) {
			log.Warn("Blocker detected: %s", blocker.Description)
			if blocker.Action != "" {
				log.Info("Action required: %s", blocker.Action)
			}
			// Send blocker notification
			notifier.UpdateProgress(p, &notify.ProgressStatus{
				Iteration:     execCtx.Iteration,
				MaxIterations: maxIterations,
				Phase:         notify.PhaseBlocked,
				Message:       blocker.Description,
			})
			if cfg.Slack.NotifyBlocker {
				if err := notifier.Blocker(p, blocker); err != nil {
					log.Debug("Failed to send blocker notification: %v", err)
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
	fmt.Printf("Iterations completed: %d/%d\n", result.Iterations, maxIterations)

	if result.Completed {
		log.Success("Plan completed successfully!")
		// Send completion notification
		notifier.UpdateProgress(p, &notify.ProgressStatus{
			Iteration:     result.Iterations,
			MaxIterations: maxIterations,
			Phase:         notify.PhaseComplete,
		})
		if cfg.Slack.NotifyComplete {
			notifier.Complete(p, "")
		}
		return nil
	}

	if result.Error != nil {
		// Send error notification
		if cfg.Slack.NotifyError {
			notifier.Error(p, result.Error)
		}
		notifier.UpdateProgress(p, &notify.ProgressStatus{
			Iteration:     result.Iterations,
			MaxIterations: maxIterations,
			Phase:         notify.PhaseError,
			Message:       result.Error.Error(),
		})

		if errors.Is(result.Error, context.Canceled) {
			log.Warn("Execution interrupted by user")
			return nil // Exit 0 on user interruption
		}
		return fmt.Errorf("execution failed: %w", result.Error)
	}

	if result.FinalBlocker != nil {
		log.Warn("Execution stopped on blocker: %s", result.FinalBlocker.Description)
		return nil // Exit 0 - blockers are not failures
	}

	return fmt.Errorf("plan not completed after %d iterations", result.Iterations)
}
