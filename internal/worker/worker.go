// Package worker implements the queue processing loop for Ralph.
// It takes plans from the pending queue, creates worktrees, runs the iteration loop,
// and handles completion (PR or merge).
package worker

import (
	"context"
	"errors"
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
	"github.com/arvesolland/ralph/internal/worktree"
)

// Common errors returned by Worker operations.
var (
	// ErrQueueEmpty is returned when there are no plans to process.
	ErrQueueEmpty = errors.New("no pending plans in queue")

	// ErrInterrupted is returned when the worker is interrupted by signal.
	ErrInterrupted = errors.New("interrupted by signal")
)

// DefaultPollInterval is the default time to wait between queue checks when empty.
const DefaultPollInterval = 30 * time.Second

// DefaultMaxIterations is the default maximum number of iterations per plan.
const DefaultMaxIterations = 30

// Worker processes plans from the queue.
type Worker struct {
	// queue is the plan queue manager
	queue *plan.Queue

	// config is the loaded configuration
	config *config.Config

	// configDir is the path to the .ralph directory
	configDir string

	// worktreeManager manages worktree creation/removal
	worktreeManager *worktree.WorktreeManager

	// git is the git interface for the main repository
	git git.Git

	// mainWorktreePath is the path to the main repository worktree
	mainWorktreePath string

	// runner executes Claude CLI
	runner runner.Runner

	// promptBuilder builds prompts from templates
	promptBuilder *prompt.Builder

	// pollInterval is the time to wait between queue checks when empty
	pollInterval time.Duration

	// maxIterations is the maximum iterations per plan
	maxIterations int

	// completionMode is "pr" or "merge"
	completionMode string

	// onPlanStart is called when a plan starts processing
	onPlanStart func(p *plan.Plan)

	// onPlanComplete is called when a plan completes successfully
	onPlanComplete func(p *plan.Plan, result *runner.LoopResult)

	// onPlanError is called when a plan fails
	onPlanError func(p *plan.Plan, err error)

	// onBlocker is called when a blocker is detected
	onBlocker func(p *plan.Plan, blocker *runner.Blocker)
}

// WorkerConfig holds configuration for creating a Worker.
type WorkerConfig struct {
	// Queue is the plan queue manager
	Queue *plan.Queue

	// Config is the loaded configuration
	Config *config.Config

	// ConfigDir is the path to the .ralph directory
	ConfigDir string

	// WorktreeManager manages worktree creation/removal
	WorktreeManager *worktree.WorktreeManager

	// Git is the git interface for the main repository
	Git git.Git

	// MainWorktreePath is the path to the main repository worktree
	MainWorktreePath string

	// Runner executes Claude CLI
	Runner runner.Runner

	// PromptBuilder builds prompts from templates
	PromptBuilder *prompt.Builder

	// PollInterval is the time to wait between queue checks when empty
	PollInterval time.Duration

	// MaxIterations is the maximum iterations per plan
	MaxIterations int

	// CompletionMode is "pr" or "merge"
	CompletionMode string

	// Callbacks
	OnPlanStart    func(p *plan.Plan)
	OnPlanComplete func(p *plan.Plan, result *runner.LoopResult)
	OnPlanError    func(p *plan.Plan, err error)
	OnBlocker      func(p *plan.Plan, blocker *runner.Blocker)
}

// NewWorker creates a new Worker with the given configuration.
func NewWorker(cfg WorkerConfig) *Worker {
	pollInterval := cfg.PollInterval
	if pollInterval == 0 {
		pollInterval = DefaultPollInterval
	}

	maxIterations := cfg.MaxIterations
	if maxIterations == 0 {
		maxIterations = DefaultMaxIterations
	}

	completionMode := cfg.CompletionMode
	if completionMode == "" {
		completionMode = "pr"
	}

	return &Worker{
		queue:            cfg.Queue,
		config:           cfg.Config,
		configDir:        cfg.ConfigDir,
		worktreeManager:  cfg.WorktreeManager,
		git:              cfg.Git,
		mainWorktreePath: cfg.MainWorktreePath,
		runner:           cfg.Runner,
		promptBuilder:    cfg.PromptBuilder,
		pollInterval:     pollInterval,
		maxIterations:    maxIterations,
		completionMode:   completionMode,
		onPlanStart:      cfg.OnPlanStart,
		onPlanComplete:   cfg.OnPlanComplete,
		onPlanError:      cfg.OnPlanError,
		onBlocker:        cfg.OnBlocker,
	}
}

// Run processes plans from the queue continuously until interrupted.
// It polls for new plans when the queue is empty.
func (w *Worker) Run(ctx context.Context) error {
	log.Info("Worker started, polling interval: %v", w.pollInterval)

	// Set up interrupt handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigCh:
			log.Warn("Received signal %v, finishing current work...", sig)
			cancel()
		case <-ctx.Done():
		}
	}()

	for {
		// Check for cancellation
		select {
		case <-ctx.Done():
			log.Info("Worker stopping due to context cancellation")
			return ctx.Err()
		default:
		}

		// Try to process a plan
		err := w.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, ErrQueueEmpty) {
				// No plans available, wait and poll again
				log.Debug("Queue empty, waiting %v before next check", w.pollInterval)
				select {
				case <-ctx.Done():
					log.Info("Worker stopping while waiting")
					return ctx.Err()
				case <-time.After(w.pollInterval):
					continue
				}
			}

			if errors.Is(err, context.Canceled) || errors.Is(err, ErrInterrupted) {
				log.Info("Worker interrupted")
				return err
			}

			// Log error but continue processing
			log.Error("Error processing plan: %v", err)
			// Wait a bit before retrying to avoid tight error loops
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// RunOnce processes a single plan from the queue and returns.
// Returns ErrQueueEmpty if no plans are pending.
func (w *Worker) RunOnce(ctx context.Context) error {
	// Check if there's already a current plan
	currentPlan, err := w.queue.Current()
	if err != nil {
		return fmt.Errorf("checking current queue: %w", err)
	}

	var p *plan.Plan

	if currentPlan != nil {
		// Resume the current plan
		log.Info("Resuming current plan: %s", currentPlan.Name)
		p = currentPlan
	} else {
		// Get next pending plan
		pending, err := w.queue.Pending()
		if err != nil {
			return fmt.Errorf("listing pending plans: %w", err)
		}

		if len(pending) == 0 {
			return ErrQueueEmpty
		}

		// Take the first pending plan
		p = pending[0]

		// Activate it (move to current/)
		log.Info("Activating plan: %s", p.Name)
		if err := w.queue.Activate(p); err != nil {
			return fmt.Errorf("activating plan: %w", err)
		}
	}

	// Process the plan
	return w.processPlan(ctx, p)
}

// processPlan handles the full lifecycle of a single plan:
// create worktree → sync files → run hooks → run loop → sync back → complete
func (w *Worker) processPlan(ctx context.Context, p *plan.Plan) error {
	// Notify start
	if w.onPlanStart != nil {
		w.onPlanStart(p)
	}

	// Create or get existing worktree
	wt, err := w.ensureWorktree(p)
	if err != nil {
		w.notifyError(p, err)
		return fmt.Errorf("ensuring worktree: %w", err)
	}

	// Sync files to worktree
	if err := worktree.SyncToWorktree(p, wt.Path, w.config, w.mainWorktreePath); err != nil {
		w.notifyError(p, err)
		return fmt.Errorf("syncing to worktree: %w", err)
	}

	// Run init hooks (only for newly created worktrees)
	// We track this by checking if context.json exists
	ctxPath := runner.ContextPath(wt.Path)
	if _, err := os.Stat(ctxPath); os.IsNotExist(err) {
		log.Info("Running worktree init hooks...")
		hookResult, hookErr := worktree.RunInitHooks(wt.Path, w.config, w.mainWorktreePath)
		if hookErr != nil {
			log.Warn("Init hooks failed: %v", hookErr)
			// Continue anyway - hooks are optional
		} else if hookResult != nil {
			log.Debug("Init hooks completed via method: %s", hookResult.Method)
		}
	}

	// Set up git for the worktree
	wtGit := git.NewGit(wt.Path)

	// Load or create execution context
	execCtx, err := w.loadOrCreateContext(p, wt.Path)
	if err != nil {
		w.notifyError(p, err)
		return fmt.Errorf("loading context: %w", err)
	}

	// Create the iteration loop
	loop := runner.NewIterationLoop(runner.LoopConfig{
		Plan:          p,
		Context:       execCtx,
		Config:        w.config,
		Runner:        w.runner,
		Git:           wtGit,
		PromptBuilder: w.promptBuilder,
		WorktreePath:  wt.Path,
		OnBlocker: func(blocker *runner.Blocker) {
			if w.onBlocker != nil {
				w.onBlocker(p, blocker)
			}
		},
	})

	// Run the iteration loop
	log.Info("Starting iteration loop for plan: %s", p.Name)
	result := loop.Run(ctx)

	// Sync files back from worktree
	if syncErr := worktree.SyncFromWorktree(p, wt.Path, w.mainWorktreePath); syncErr != nil {
		log.Error("Failed to sync from worktree: %v", syncErr)
		// Continue to handle completion
	}

	// Handle result
	if result.Error != nil {
		// Check if it's a cancellation
		if errors.Is(result.Error, context.Canceled) {
			log.Info("Plan processing interrupted")
			return ErrInterrupted
		}

		w.notifyError(p, result.Error)
		return result.Error
	}

	if result.Completed {
		// Plan completed successfully
		return w.completePlan(ctx, p, wt, result)
	}

	// Plan didn't complete (max iterations or blocker)
	if result.FinalBlocker != nil {
		log.Warn("Plan blocked: %s", result.FinalBlocker.Description)
	}

	// Notify completion (even if not verified complete)
	if w.onPlanComplete != nil {
		w.onPlanComplete(p, result)
	}

	return nil
}

// ensureWorktree creates a worktree for the plan if it doesn't exist.
func (w *Worker) ensureWorktree(p *plan.Plan) (*worktree.Worktree, error) {
	// Check if worktree already exists
	existing, err := w.worktreeManager.Get(p)
	if err != nil {
		return nil, fmt.Errorf("checking existing worktree: %w", err)
	}

	if existing != nil {
		log.Debug("Using existing worktree: %s", existing.Path)
		return existing, nil
	}

	// Create new worktree
	log.Info("Creating worktree for branch: %s", p.Branch)
	wt, err := w.worktreeManager.Create(p)
	if err != nil {
		return nil, fmt.Errorf("creating worktree: %w", err)
	}

	log.Success("Worktree created: %s", wt.Path)
	return wt, nil
}

// loadOrCreateContext loads existing context or creates new one.
func (w *Worker) loadOrCreateContext(p *plan.Plan, worktreePath string) (*runner.Context, error) {
	ctxPath := runner.ContextPath(worktreePath)

	// Try to load existing context
	execCtx, err := runner.LoadContext(ctxPath)
	if err == nil {
		log.Debug("Loaded existing context at iteration %d", execCtx.Iteration)
		return execCtx, nil
	}

	// Check if it's a "not exist" error (using errors.Is to handle wrapped errors)
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("loading context: %w", err)
	}

	// Create new context
	baseBranch := w.config.Git.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	// Compute plan file path relative to worktree
	planRelPath, _ := filepath.Rel(w.mainWorktreePath, p.Path)
	if planRelPath == "" {
		planRelPath = filepath.Join("plans", "current", filepath.Base(p.Path))
	}

	execCtx = runner.NewContext(p, baseBranch, w.maxIterations)
	execCtx.PlanFile = planRelPath

	// Save the new context
	if err := runner.SaveContext(execCtx, ctxPath); err != nil {
		return nil, fmt.Errorf("saving context: %w", err)
	}

	log.Debug("Created new execution context")
	return execCtx, nil
}

// completePlan handles plan completion (archive, PR/merge, cleanup).
func (w *Worker) completePlan(ctx context.Context, p *plan.Plan, wt *worktree.Worktree, result *runner.LoopResult) error {
	log.Success("Plan completed: %s", p.Name)

	// Notify completion
	if w.onPlanComplete != nil {
		w.onPlanComplete(p, result)
	}

	// Archive the plan (move to complete/)
	if err := w.queue.Complete(p); err != nil {
		log.Error("Failed to archive plan: %v", err)
		// Continue with cleanup
	}

	// Clean up worktree
	log.Info("Cleaning up worktree...")
	if err := w.worktreeManager.Remove(p, false); err != nil {
		log.Warn("Failed to remove worktree: %v", err)
		// Non-fatal
	}

	return nil
}

// notifyError calls the error callback if set.
func (w *Worker) notifyError(p *plan.Plan, err error) {
	if w.onPlanError != nil {
		w.onPlanError(p, err)
	}
}
