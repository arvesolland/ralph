// Package cli provides the command-line interface for ralph.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/arvesolland/ralph/internal/worker"
	"github.com/arvesolland/ralph/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	workerOnce        bool
	workerPRMode      bool
	workerMergeMode   bool
	workerInterval    time.Duration
	workerMaxIter     int
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Process plans from the queue",
	Long: `Run the worker loop to process plans from the pending queue.

The worker will:
1. Take the first plan from pending/ and move it to current/
2. Create a git worktree for the plan's branch
3. Run the iteration loop until completion or max iterations
4. On completion: create PR (default) or merge directly
5. Move the plan to complete/ and clean up the worktree
6. Repeat for the next pending plan

With --once, it processes a single plan and exits.
Without --once, it runs continuously, polling for new plans.

Example:
  ralph worker           # continuous mode
  ralph worker --once    # single plan mode
  ralph worker --merge   # merge directly instead of creating PR`,
	RunE: runWorker,
}

func init() {
	rootCmd.AddCommand(workerCmd)

	workerCmd.Flags().BoolVar(&workerOnce, "once", false, "process one plan and exit")
	workerCmd.Flags().BoolVar(&workerPRMode, "pr", false, "use PR mode for completion (default)")
	workerCmd.Flags().BoolVar(&workerMergeMode, "merge", false, "use merge mode for completion")
	workerCmd.Flags().DurationVar(&workerInterval, "interval", worker.DefaultPollInterval, "poll interval when queue is empty")
	workerCmd.Flags().IntVar(&workerMaxIter, "max", worker.DefaultMaxIterations, "maximum iterations per plan")
}

func runWorker(cmd *cobra.Command, args []string) error {
	// Determine completion mode
	completionMode := "pr"
	if workerMergeMode {
		completionMode = "merge"
	}
	// --pr is default, so --merge takes precedence if both are set

	// Load configuration
	cfg, err := config.LoadWithDefaults(GetConfigPath())
	if err != nil {
		log.Warn("Failed to load config, using defaults: %v", err)
		cfg = config.Defaults()
	}

	// If completion mode not set via flags, use config
	if !workerMergeMode && !workerPRMode && cfg.Completion.Mode != "" {
		completionMode = cfg.Completion.Mode
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
	plansDir := filepath.Join(repoRoot, "plans")
	worktreesDir := filepath.Join(configDir, "worktrees")

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Join(plansDir, "pending"), 0755); err != nil {
		return fmt.Errorf("creating plans/pending: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(plansDir, "current"), 0755); err != nil {
		return fmt.Errorf("creating plans/current: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(plansDir, "complete"), 0755); err != nil {
		return fmt.Errorf("creating plans/complete: %w", err)
	}
	if err := os.MkdirAll(worktreesDir, 0755); err != nil {
		return fmt.Errorf("creating worktrees directory: %w", err)
	}

	// Initialize queue
	queue := plan.NewQueue(plansDir)

	// Initialize worktree manager
	wtManager, err := worktree.NewManager(g, worktreesDir)
	if err != nil {
		return fmt.Errorf("initializing worktree manager: %w", err)
	}

	// Initialize prompt builder
	promptsDir := filepath.Join(configDir, "prompts")
	promptBuilder := prompt.NewBuilder(cfg, configDir, promptsDir)

	// Create Claude runner
	claudeRunner := runner.NewCLIRunner()

	// Create worker
	w := worker.NewWorker(worker.WorkerConfig{
		Queue:            queue,
		Config:           cfg,
		ConfigDir:        configDir,
		WorktreeManager:  wtManager,
		Git:              g,
		MainWorktreePath: mainWorktreePath,
		Runner:           claudeRunner,
		PromptBuilder:    promptBuilder,
		PollInterval:     workerInterval,
		MaxIterations:    workerMaxIter,
		CompletionMode:   completionMode,
		OnPlanStart: func(p *plan.Plan) {
			log.Success("=== Starting plan: %s ===", p.Name)
			log.Info("Branch: %s", p.Branch)
		},
		OnPlanComplete: func(p *plan.Plan, result *runner.LoopResult) {
			log.Success("=== Plan complete: %s ===", p.Name)
			log.Info("Iterations: %d", result.Iterations)
			if result.Completed {
				log.Success("Verified complete!")
			}
		},
		OnPlanError: func(p *plan.Plan, err error) {
			log.Error("=== Plan error: %s ===", p.Name)
			log.Error("Error: %v", err)
		},
		OnBlocker: func(p *plan.Plan, blocker *runner.Blocker) {
			log.Warn("=== Blocker detected in %s ===", p.Name)
			log.Warn("Description: %s", blocker.Description)
			if blocker.Action != "" {
				log.Info("Action required: %s", blocker.Action)
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

	// Run the worker
	log.Info("Worker starting...")
	log.Info("Completion mode: %s", completionMode)
	log.Info("Poll interval: %v", workerInterval)
	log.Info("Max iterations: %d", workerMaxIter)

	if workerOnce {
		// Process one plan and exit
		err := w.RunOnce(ctx)
		if err != nil {
			if err == worker.ErrQueueEmpty {
				log.Info("No pending plans in queue")
				return nil
			}
			if err == context.Canceled {
				log.Warn("Worker interrupted")
				return nil
			}
			return fmt.Errorf("worker error: %w", err)
		}
		return nil
	}

	// Run continuously
	err = w.Run(ctx)
	if err != nil {
		if err == context.Canceled {
			log.Info("Worker stopped")
			return nil
		}
		return fmt.Errorf("worker error: %w", err)
	}

	return nil
}
