// Package cli provides the command-line interface for ralph.
package cli

import (
	"context"
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
	"github.com/arvesolland/ralph/internal/logfile"
	"github.com/arvesolland/ralph/internal/process"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/arvesolland/ralph/internal/worker"
	"github.com/arvesolland/ralph/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	workerOnce         bool
	workerPRMode       bool
	workerMergeMode    bool
	workerBranchMode   bool
	workerInterval     time.Duration
	workerMaxIter      int
	workerSync         bool
	workerSyncInterval time.Duration
	workerPush         bool
	workerTimeout      time.Duration
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Process plans from the Board queue",
	Long: `Run the worker loop to process plans from the Board queue.

The worker will:
1. Poll Board for ready plans
2. Activate the first ready plan
3. Create a git worktree for the plan's branch
4. Run the iteration loop until completion or max iterations
5. On completion: create PR (default), merge directly, or push branch only
6. Clean up the worktree
7. Repeat for the next ready plan

With --once, it processes a single plan and exits.
Without --once, it runs continuously, polling for new plans.

With --sync, it pulls from remote before each queue check, enabling
a "push-to-deploy" workflow.

Example:
  ralph worker                    # continuous mode
  ralph worker --once             # single plan mode
  ralph worker --merge            # merge directly instead of creating PR
  ralph worker --branch           # push to branch only, no PR
  ralph worker --sync             # pull from remote before each check`,
	RunE: runWorker,
}

func init() {
	rootCmd.AddCommand(workerCmd)

	workerCmd.Flags().BoolVar(&workerOnce, "once", false, "process one plan and exit")
	workerCmd.Flags().BoolVar(&workerPRMode, "pr", false, "use PR mode for completion (default)")
	workerCmd.Flags().BoolVar(&workerMergeMode, "merge", false, "use merge mode for completion")
	workerCmd.Flags().BoolVar(&workerBranchMode, "branch", false, "push to branch only, no PR or merge")
	workerCmd.Flags().DurationVar(&workerInterval, "interval", worker.DefaultPollInterval, "poll interval when queue is empty")
	workerCmd.Flags().IntVar(&workerMaxIter, "max", worker.DefaultMaxIterations, "maximum iterations per plan")
	workerCmd.Flags().BoolVar(&workerSync, "sync", false, "pull from remote before each queue check")
	workerCmd.Flags().DurationVar(&workerSyncInterval, "sync-interval", 0, "minimum time between syncs (e.g., 60s)")
	workerCmd.Flags().BoolVar(&workerPush, "push", false, "push to remote after each iteration (prevents work loss on spot instances)")
	workerCmd.Flags().DurationVar(&workerTimeout, "timeout", 0, "timeout per iteration (e.g., 90m, 2h) (default from config or 60m)")
}

func runWorker(cmd *cobra.Command, args []string) error {
	// Determine completion mode
	completionMode := "pr"
	if workerMergeMode {
		completionMode = "merge"
	}
	if workerBranchMode {
		completionMode = "branch"
	}

	// Load configuration
	cfg, err := config.LoadWithDefaults(GetConfigPath())
	if err != nil {
		log.Warn("Failed to load config, using defaults: %v", err)
		cfg = config.Defaults()
	}

	// If completion mode not set via flags, use config
	if !workerMergeMode && !workerPRMode && !workerBranchMode && cfg.Completion.Mode != "" {
		completionMode = cfg.Completion.Mode
	}

	// Determine project slug
	projectSlug := cfg.Board.ProjectSlug
	if projectSlug == "" {
		return fmt.Errorf("board.project_slug not configured; run 'ralph init' or set it in .ralph/config.yaml")
	}

	// Create Board client
	boardClient := cfg.BoardClient()

	// Determine sync settings (flags take precedence over config)
	syncEnabled := workerSync
	syncInterval := workerSyncInterval

	if !cmd.Flags().Changed("sync") && cfg.Worker.Sync {
		syncEnabled = cfg.Worker.Sync
	}
	if !cmd.Flags().Changed("sync-interval") && cfg.Worker.SyncInterval != "" {
		if parsed, parseErr := time.ParseDuration(cfg.Worker.SyncInterval); parseErr == nil {
			syncInterval = parsed
		} else {
			log.Warn("Invalid worker.sync_interval in config: %v", parseErr)
		}
	}

	// Determine iteration timeout (flag > config > default)
	iterationTimeout := workerTimeout
	if iterationTimeout == 0 && cfg.Runner.IterationTimeout != "" {
		if parsed, parseErr := time.ParseDuration(cfg.Runner.IterationTimeout); parseErr == nil {
			iterationTimeout = parsed
		} else {
			log.Warn("Invalid runner.iteration_timeout in config: %v", parseErr)
		}
	}
	// 0 means WorkerConfig will use runner.IterationTimeout default

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

	// Set up log file (tee stdout/stderr to file)
	var currentLogFile string
	if !noLogFile {
		logsDir := filepath.Join(configDir, "logs")
		lf, err := logfile.New(logfile.Options{
			LogDir:     logsDir,
			Prefix:     logfile.WorkerPrefix(),
			CustomPath: logFilePath,
		})
		if err != nil {
			log.Warn("Failed to create log file: %v", err)
		} else {
			defer lf.Close()
			currentLogFile = lf.Path()
			log.Info("Log file: %s", currentLogFile)
		}
	}

	// Register with Board process registry
	procRegistry := process.New(boardClient, "worker", currentLogFile)
	procRegistry.Register()
	defer procRegistry.Deregister()

	// Ensure worktrees directory exists
	if err := os.MkdirAll(worktreesDir, 0755); err != nil {
		return fmt.Errorf("creating worktrees directory: %w", err)
	}

	// Initialize worktree manager
	wtManager, err := worktree.NewManager(g, worktreesDir)
	if err != nil {
		return fmt.Errorf("initializing worktree manager: %w", err)
	}

	// Create adapter to satisfy worker.WorktreeManager interface
	wtAdapter := &worktreeAdapter{manager: wtManager}

	// Initialize prompt builder
	promptsDir := filepath.Join(configDir, "prompts")
	promptBuilder := prompt.NewBuilder(cfg, configDir, promptsDir)

	// Create Claude runner
	claudeRunner := runner.NewCLIRunner()

	// Create worker
	w := worker.NewWorker(worker.WorkerConfig{
		Board:            boardClient,
		ProjectSlug:      projectSlug,
		Config:           cfg,
		ConfigDir:        configDir,
		WorktreeManager:  wtAdapter,
		Git:              g,
		MainWorktreePath: mainWorktreePath,
		Runner:           claudeRunner,
		PromptBuilder:    promptBuilder,
		PollInterval:     workerInterval,
		MaxIterations:    workerMaxIter,
		CompletionMode:   completionMode,
		SyncEnabled:      syncEnabled,
		SyncInterval:     syncInterval,
		PushAfterIteration: workerPush,
		IterationTimeout:   iterationTimeout,
		ProcessRegistry:    procRegistry,
		OnPlanStart: func(info *worker.PlanInfo) {
			log.Success("=== Starting plan: %s ===", info.Name)
			log.Info("Branch: %s", info.Branch)
		},
		OnPlanComplete: func(info *worker.PlanInfo, result *runner.LoopResult) {
			log.Success("=== Plan complete: %s ===", info.Name)
			log.Info("Iterations: %d", result.Iterations)
			if result.Completed {
				log.Success("Verified complete!")
			}
		},
		OnPlanError: func(info *worker.PlanInfo, err error) {
			log.Error("=== Plan error: %s ===", info.Name)
			log.Error("Error: %v", err)
		},
		OnBlocker: func(info *worker.PlanInfo, blocker *runner.Blocker) {
			log.Warn("=== Blocker detected in %s ===", info.Name)
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
		procRegistry.Deregister()
		cancel()
	}()

	// Set up Slack notifications
	cleanupNotifications := w.SetupNotifications(ctx)
	defer cleanupNotifications()

	// Run the worker
	log.Info("Worker starting...")
	log.Info("Project: %s", projectSlug)
	log.Info("Completion mode: %s", completionMode)
	log.Info("Poll interval: %v", workerInterval)
	log.Info("Max iterations: %d", workerMaxIter)
	if syncEnabled {
		if syncInterval > 0 {
			log.Info("Sync enabled (interval: %v)", syncInterval)
		} else {
			log.Info("Sync enabled (every check)")
		}
	}
	if workerPush {
		log.Info("Push after iteration: enabled")
	}

	if workerOnce {
		// Sync before processing if enabled
		if syncEnabled {
			log.Info("Syncing with remote...")
			if err := g.PullRebase(); err != nil {
				log.Warn("Failed to sync with remote: %v (continuing with local state)", err)
			}
		}

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

// worktreeAdapter adapts worktree.WorktreeManager to the worker.WorktreeManager interface.
type worktreeAdapter struct {
	manager *worktree.WorktreeManager
}

func (a *worktreeAdapter) Create(name, branch string) (string, error) {
	info := worktree.PlanInfo{Name: name, Branch: branch}
	wt, err := a.manager.Create(info)
	if err != nil {
		return "", err
	}
	return wt.Path, nil
}

func (a *worktreeAdapter) Get(name string) (string, error) {
	// We need the branch to construct the PlanInfo. Use name as branch
	// since the worker passes branchToWorktreeName(branch) as name.
	// Reconstruct branch from name by reversing the transformation.
	branch := nameToWorktreeBranch(name)
	info := worktree.PlanInfo{Name: name, Branch: branch}
	wt, err := a.manager.Get(info)
	if err != nil {
		return "", err
	}
	if wt == nil {
		return "", nil
	}
	return wt.Path, nil
}

func (a *worktreeAdapter) Remove(name string, deleteBranch bool) error {
	branch := nameToWorktreeBranch(name)
	info := worktree.PlanInfo{Name: name, Branch: branch}
	return a.manager.Remove(info, deleteBranch)
}

func (a *worktreeAdapter) Exists(name string) bool {
	branch := nameToWorktreeBranch(name)
	info := worktree.PlanInfo{Name: name, Branch: branch}
	return a.manager.Exists(info)
}

// nameToWorktreeBranch reverses the branchToWorktreeName transformation.
// branchToWorktreeName replaces "/" with "-", so we reverse: "feat-my-feature" -> "feat/my-feature".
// Since the worktree.WorktreeManager.Path uses strings.TrimPrefix(branch, "feat/"),
// the name passed here IS the directory name (which is the branch without "feat/" prefix).
// So the branch is "feat/" + name.
func nameToWorktreeBranch(name string) string {
	// The worker's branchToWorktreeName does: strings.ReplaceAll(branch, "/", "-")
	// So "feat/my-feature" becomes "feat-my-feature"
	// We need to get back from "feat-my-feature" to "feat/my-feature"
	// Since the worktree manager uses the branch to build the path
	// via strings.TrimPrefix(branch, "feat/"), the simplest approach
	// is to reconstruct: if name starts with "feat-", it was "feat/..."
	if strings.HasPrefix(name, "feat-") {
		return "feat/" + name[5:]
	}
	return name
}
