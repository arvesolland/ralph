// Package worker implements the queue processing loop for Ralph.
// It polls Board for ready plans, creates worktrees, runs the iteration loop,
// and handles completion (PR or merge).
package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/arvesolland/ralph/internal/board"
	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/notify"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/retry"
	"github.com/arvesolland/ralph/internal/runner"
)

// Default constants for worker configuration.
const (
	DefaultPollInterval  = 30 * time.Second
	DefaultMaxIterations = 200
)

// DefaultStatusRetryConfig is the retry config for critical Board status updates.
var DefaultStatusRetryConfig = retry.RetryConfig{
	MaxRetries:   5,
	InitialDelay: 5 * time.Second,
	MaxDelay:     60 * time.Second,
	JitterFactor: 0.25,
}

// Sentinel errors returned by the worker.
var (
	ErrQueueEmpty  = errors.New("no pending plans in queue")
	ErrInterrupted = errors.New("interrupted by signal")
)

// PlanInfo holds the minimal info needed for notifications and logging.
type PlanInfo struct {
	ID     int
	Name   string
	Branch string
}

// WorktreeManager provides an interface for worktree lifecycle management.
// The concrete implementation is injected by the CLI layer.
type WorktreeManager interface {
	Create(name, branch string) (string, error) // returns worktree path
	Get(name string) (string, error)            // returns path or "" if not found
	Remove(name string, deleteBranch bool) error
	Exists(name string) bool
}

// WorkerConfig holds all configuration for creating a new Worker.
type WorkerConfig struct {
	Board              board.Board
	ProjectSlug        string
	Config             *config.Config
	ConfigDir          string
	WorktreeManager    WorktreeManager
	Git                git.Git
	MainWorktreePath   string
	Runner             runner.Runner
	PromptBuilder      *prompt.Builder
	Notifier           notify.Notifier
	PollInterval       time.Duration
	MaxIterations      int
	CompletionMode     string
	SyncEnabled        bool
	SyncInterval       time.Duration
	PushAfterIteration bool
	IterationTimeout   time.Duration
	StatusRetryConfig  *retry.RetryConfig // retry config for Board status updates (default: aggressive)

	// Callbacks
	OnPlanStart    func(info *PlanInfo)
	OnPlanComplete func(info *PlanInfo, result *runner.LoopResult)
	OnPlanError    func(info *PlanInfo, err error)
	OnBlocker      func(info *PlanInfo, blocker *runner.Blocker)
}

// Worker processes plans from the Board queue.
type Worker struct {
	board              board.Board
	projectSlug        string
	config             *config.Config
	configDir          string
	worktreeManager    WorktreeManager
	git                git.Git
	mainWorktreePath   string
	runner             runner.Runner
	promptBuilder      *prompt.Builder
	notifier           notify.Notifier
	pollInterval       time.Duration
	maxIterations      int
	completionMode     string
	syncEnabled        bool
	syncInterval       time.Duration
	lastSyncTime       time.Time
	pushAfterIteration bool
	iterationTimeout   time.Duration
	statusRetryConfig  retry.RetryConfig

	// Callbacks
	onPlanStart    func(info *PlanInfo)
	onPlanComplete func(info *PlanInfo, result *runner.LoopResult)
	onPlanError    func(info *PlanInfo, err error)
	onBlocker      func(info *PlanInfo, blocker *runner.Blocker)
}

// NewWorker creates a new Worker with the given configuration.
func NewWorker(cfg WorkerConfig) *Worker {
	w := &Worker{
		board:              cfg.Board,
		projectSlug:        cfg.ProjectSlug,
		config:             cfg.Config,
		configDir:          cfg.ConfigDir,
		worktreeManager:    cfg.WorktreeManager,
		git:                cfg.Git,
		mainWorktreePath:   cfg.MainWorktreePath,
		runner:             cfg.Runner,
		promptBuilder:      cfg.PromptBuilder,
		notifier:           cfg.Notifier,
		pollInterval:       cfg.PollInterval,
		maxIterations:      cfg.MaxIterations,
		completionMode:     cfg.CompletionMode,
		syncEnabled:        cfg.SyncEnabled,
		syncInterval:       cfg.SyncInterval,
		pushAfterIteration: cfg.PushAfterIteration,
		iterationTimeout:   cfg.IterationTimeout,
		onPlanStart:        cfg.OnPlanStart,
		onPlanComplete:     cfg.OnPlanComplete,
		onPlanError:        cfg.OnPlanError,
		onBlocker:          cfg.OnBlocker,
	}

	// Apply defaults
	if w.pollInterval == 0 {
		w.pollInterval = DefaultPollInterval
	}
	if w.maxIterations == 0 {
		w.maxIterations = DefaultMaxIterations
	}
	if w.completionMode == "" {
		w.completionMode = "pr"
	}
	if w.iterationTimeout == 0 {
		w.iterationTimeout = runner.IterationTimeout
	}
	if w.notifier == nil {
		w.notifier = &notify.NoopNotifier{}
	}
	if cfg.StatusRetryConfig != nil {
		w.statusRetryConfig = *cfg.StatusRetryConfig
	} else {
		w.statusRetryConfig = DefaultStatusRetryConfig
	}

	return w
}

// Run starts the continuous worker loop, processing plans until the context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	log.Info("Worker started (poll interval: %v, max iterations: %d, completion: %s)",
		w.pollInterval, w.maxIterations, w.completionMode)

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	for {
		select {
		case <-ctx.Done():
			log.Info("Worker context cancelled, shutting down")
			return ctx.Err()
		case sig := <-sigCh:
			log.Info("Worker received signal: %v", sig)
			return ErrInterrupted
		default:
		}

		// Sync from remote if enabled
		w.syncFromRemote()

		// Try to process one plan
		err := w.RunOnce(ctx)
		if err == ErrQueueEmpty {
			log.Debug("Queue empty, waiting %v before next poll", w.pollInterval)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case sig := <-sigCh:
				log.Info("Worker received signal: %v", sig)
				return ErrInterrupted
			case <-time.After(w.pollInterval):
				continue
			}
		}
		if err != nil {
			log.Error("Worker error: %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case sig := <-sigCh:
				log.Info("Worker received signal: %v", sig)
				return ErrInterrupted
			case <-time.After(w.pollInterval):
				continue
			}
		}
	}
}

// RunOnce attempts to process a single plan from the queue.
// Returns ErrQueueEmpty if no plans are available.
func (w *Worker) RunOnce(ctx context.Context) error {
	// Check for an active plan (resume scenario)
	agentCtx, err := w.board.ProjectContext(w.projectSlug)
	if err != nil {
		return fmt.Errorf("fetching project context: %w", err)
	}

	var plan *board.Plan

	// Check if there's an active plan to resume
	if agentCtx.Plan.ID > 0 && agentCtx.Plan.Status == board.PlanStatusActive {
		planCopy := agentCtx.Plan
		plan = &planCopy
		log.Info("Resuming active plan #%d: %s", plan.ID, plan.Title)
	} else {
		// Check for ready plans
		readyPlans, err := w.board.ListPlans(w.projectSlug, board.PlanStatusReady)
		if err != nil {
			return fmt.Errorf("listing ready plans: %w", err)
		}

		if len(readyPlans) == 0 {
			return ErrQueueEmpty
		}

		// Take the first ready plan
		plan = &readyPlans[0]
		log.Info("Activating plan #%d: %s", plan.ID, plan.Title)

		// Transition to active
		updated, err := w.board.UpdatePlanStatus(plan.ID, board.PlanStatusActive)
		if err != nil {
			return fmt.Errorf("activating plan #%d: %w", plan.ID, err)
		}
		plan = updated
	}

	// Resolve branch name (uses FeatureBranch or generates from title)
	branch := plan.BranchName()

	// Process the plan
	info := &PlanInfo{
		ID:     plan.ID,
		Name:   plan.Title,
		Branch: branch,
	}

	return w.processPlan(ctx, plan, info)
}

// processPlan executes the full lifecycle of a plan: setup, iterate, complete.
func (w *Worker) processPlan(ctx context.Context, plan *board.Plan, info *PlanInfo) error {
	log.Info("Processing plan: %s (branch: %s)", info.Name, info.Branch)

	// Send start notification (seeds thread from Board if available, saves to Board after creation)
	w.sendStartNotification(plan, info)

	// Call start callback
	if w.onPlanStart != nil {
		w.onPlanStart(info)
	}

	// Ensure worktree exists
	worktreePath, err := w.ensureWorktree(plan)
	if err != nil {
		w.notifyError(info, err)
		if w.onPlanError != nil {
			w.onPlanError(info, err)
		}
		return fmt.Errorf("ensuring worktree: %w", err)
	}

	log.Info("Using worktree: %s", worktreePath)

	// Create or load execution context
	execCtx, err := w.loadOrCreateContext(plan, worktreePath)
	if err != nil {
		w.notifyError(info, err)
		if w.onPlanError != nil {
			w.onPlanError(info, err)
		}
		return fmt.Errorf("loading context: %w", err)
	}

	// Create the iteration loop
	loop := runner.NewIterationLoop(runner.LoopConfig{
		Board:            w.board,
		PlanID:           plan.ID,
		ProjectSlug:      w.projectSlug,
		Context:          execCtx,
		Config:           w.config,
		Runner:           w.runner,
		Git:              git.NewGit(worktreePath),
		PromptBuilder:    w.promptBuilder,
		WorktreePath:     worktreePath,
		IterationTimeout: w.iterationTimeout,
		OnIteration: func(iteration int, result *runner.Result) {
			// Update living status card with task stats
			progress := &notify.ProgressStatus{
				Iteration:     iteration,
				MaxIterations: w.maxIterations,
				Phase:         notify.PhaseRunning,
			}
			if agentCtx, statsErr := w.board.PlanContext(plan.ID); statsErr == nil {
				progress.TasksDone = agentCtx.Stats.Done + agentCtx.Stats.Skipped
				progress.TasksTotal = agentCtx.Stats.TotalTasks
			}
			_ = w.notifier.UpdateProgress(toNotifyPlanInfo(info), progress)
			w.sendIterationNotification(info, iteration, w.maxIterations)
		},
		OnBlocker: func(blocker *runner.Blocker) {
			w.sendBlockerNotification(info, blocker)
			if w.onBlocker != nil {
				w.onBlocker(info, blocker)
			}
		},
		OnAfterCommit: func() {
			if w.pushAfterIteration {
				wtGit := git.NewGit(worktreePath)
				if err := wtGit.PushWithUpstream("origin", info.Branch); err != nil {
					log.Warn("Failed to push after iteration: %v", err)
				}
			}
		},
	})

	// Run the iteration loop
	loopResult := loop.Run(ctx)

	// Handle result
	if loopResult.Completed {
		log.Success("Plan completed: %s", info.Name)

		// Complete the plan
		if err := w.completePlan(plan, info, worktreePath); err != nil {
			log.Error("Failed to complete plan: %v", err)
			w.notifyError(info, err)
			if w.onPlanError != nil {
				w.onPlanError(info, err)
			}
			return fmt.Errorf("completing plan: %w", err)
		}

		// Call complete callback
		if w.onPlanComplete != nil {
			w.onPlanComplete(info, loopResult)
		}

		return nil
	}

	// Plan didn't complete (max iterations or error)
	if loopResult.Error != nil {
		log.Error("Plan failed: %s - %v", info.Name, loopResult.Error)

		// Mark plan as blocked in Board (with retry)
		blockedRetrier := retry.NewRetrier(w.statusRetryConfig)
		if retryErr := blockedRetrier.Do(func() error {
			_, err := w.board.UpdatePlanStatus(plan.ID, board.PlanStatusBlocked)
			return err
		}); retryErr != nil {
			log.Error("Failed to set plan status to blocked after %d retries: %v", blockedRetrier.Attempts(), retryErr)
			log.Error("Manual recovery required: board-cli plan status %d --status blocked", plan.ID)
		}

		w.notifyError(info, loopResult.Error)
		if w.onPlanError != nil {
			w.onPlanError(info, loopResult.Error)
		}

		return loopResult.Error
	}

	return nil
}

// ensureWorktree creates or reuses a worktree for the plan.
func (w *Worker) ensureWorktree(plan *board.Plan) (string, error) {
	if w.worktreeManager == nil {
		// No worktree manager, use main worktree
		return w.mainWorktreePath, nil
	}

	// Use plan feature branch as worktree name
	branch := plan.BranchName()
	name := branchToWorktreeName(branch)

	// Check if worktree already exists
	if w.worktreeManager.Exists(name) {
		path, err := w.worktreeManager.Get(name)
		if err != nil {
			return "", fmt.Errorf("getting existing worktree: %w", err)
		}
		log.Debug("Reusing existing worktree: %s", path)
		return path, nil
	}

	// Create new worktree
	path, err := w.worktreeManager.Create(name, branch)
	if err != nil {
		return "", fmt.Errorf("creating worktree: %w", err)
	}

	log.Info("Created worktree: %s (branch: %s)", path, branch)
	return path, nil
}

// loadOrCreateContext loads an existing context or creates a new one.
func (w *Worker) loadOrCreateContext(plan *board.Plan, worktreePath string) (*runner.Context, error) {
	ctxPath := runner.ContextPath(worktreePath)

	// Try to load existing context
	execCtx, err := runner.LoadContext(ctxPath)
	if err == nil {
		// Check if context matches the current plan
		if execCtx.PlanID == plan.ID {
			log.Debug("Resuming context at iteration %d", execCtx.Iteration)
			return execCtx, nil
		}
		log.Warn("Stale context (plan ID %d vs %d), creating fresh context", execCtx.PlanID, plan.ID)
	}

	// Create fresh context
	baseBranch := "main"
	if w.config != nil && w.config.Git.BaseBranch != "" {
		baseBranch = w.config.Git.BaseBranch
	}

	execCtx = runner.NewContext(plan.ID, plan.BranchName(), baseBranch, w.maxIterations)

	// Save the new context
	if err := runner.SaveContext(execCtx, ctxPath); err != nil {
		return nil, fmt.Errorf("saving context: %w", err)
	}

	return execCtx, nil
}

// completePlan handles the completion workflow (PR creation or merge).
func (w *Worker) completePlan(plan *board.Plan, info *PlanInfo, worktreePath string) error {
	var prURL string

	switch w.completionMode {
	case "pr":
		url, err := w.completePR(plan, info, worktreePath)
		if err != nil {
			return err
		}
		prURL = url

	case "merge":
		baseBranch := "main"
		if w.config != nil && w.config.Git.BaseBranch != "" {
			baseBranch = w.config.Git.BaseBranch
		}
		if err := CompleteMerge(info.Branch, baseBranch, w.git); err != nil {
			return fmt.Errorf("merge completion: %w", err)
		}

	case "branch":
		baseBranch := "main"
		if w.config != nil && w.config.Git.BaseBranch != "" {
			baseBranch = w.config.Git.BaseBranch
		}
		wtGit := git.NewGit(worktreePath)
		if err := CompleteBranch(info.Branch, baseBranch, wtGit); err != nil {
			return fmt.Errorf("branch completion: %w", err)
		}
	}

	// Update Board status to complete with retry (critical path)
	r := retry.NewRetrier(w.statusRetryConfig)
	statusUpdateFailed := false
	if err := r.Do(func() error {
		_, err := w.board.UpdatePlanStatus(plan.ID, board.PlanStatusComplete)
		return err
	}); err != nil {
		statusUpdateFailed = true
		log.Error("CRITICAL: Failed to set plan status to complete in Board after %d retries: %v", r.Attempts(), err)
		log.Error("Manual recovery required: board-cli plan status %d --status complete", plan.ID)
	}

	// Skip worktree cleanup if Board status update failed (preserve work for manual recovery)
	if statusUpdateFailed {
		log.Warn("Skipping worktree cleanup because Board status update failed (preserving work for recovery)")
	} else if w.worktreeManager != nil {
		name := branchToWorktreeName(info.Branch)
		if err := w.worktreeManager.Remove(name, false); err != nil {
			log.Warn("Failed to remove worktree: %v", err)
		}
	}

	// Send completion notification
	w.sendCompleteNotification(info, prURL)

	return nil
}

// completePR pushes the branch and creates a PR.
func (w *Worker) completePR(plan *board.Plan, info *PlanInfo, worktreePath string) (string, error) {
	wtGit := git.NewGit(worktreePath)

	// Push branch
	if err := pushBranch(wtGit, info.Branch); err != nil {
		return "", fmt.Errorf("pushing branch: %w", err)
	}

	// Create PR
	prURL, err := createPRSimple(plan.Title, info.Branch, worktreePath, w.runner, w.promptBuilder)
	if err != nil {
		log.Error("Failed to create PR: %v", err)
		logManualPRInstructionsSimple(info.Name, info.Branch)
		return "", nil // Non-fatal; branch is pushed
	}

	log.Success("PR created: %s", prURL)
	return prURL, nil
}

// syncFromRemote pulls latest changes from the remote if sync is enabled.
func (w *Worker) syncFromRemote() {
	if !w.syncEnabled {
		return
	}

	if time.Since(w.lastSyncTime) < w.syncInterval {
		return
	}

	log.Debug("Syncing from remote...")
	if err := w.git.Pull(); err != nil {
		log.Warn("Failed to sync from remote: %v", err)
	}
	w.lastSyncTime = time.Now()
}

// SetupNotifications creates and configures the notifier based on worker config.
// Returns a cleanup function that should be called on shutdown.
func (w *Worker) SetupNotifications(ctx context.Context) func() {
	tracker, err := notify.NewThreadTracker(notify.ThreadTrackerPath(w.configDir))
	if err != nil {
		log.Warn("Failed to create thread tracker: %v", err)
		tracker = nil
	}
	w.notifier = notify.NewNotifier(w.config, tracker)
	return func() {
		// ThreadTracker auto-saves on Set/Delete; no explicit save needed.
	}
}

// toNotifyPlanInfo converts a worker PlanInfo to a notify.PlanInfo.
func toNotifyPlanInfo(info *PlanInfo) notify.PlanInfo {
	return notify.PlanInfo{Name: info.Name, Branch: info.Branch}
}

// toNotifyBlocker converts a runner.Blocker to a notify.Blocker.
func toNotifyBlocker(b *runner.Blocker) *notify.Blocker {
	if b == nil {
		return nil
	}
	return &notify.Blocker{
		Content:     b.Content,
		Description: b.Description,
		Action:      b.Action,
		Resume:      b.Resume,
		Hash:        b.Hash,
	}
}

// Notification methods using PlanInfo.

func (w *Worker) sendStartNotification(plan *board.Plan, info *PlanInfo) {
	if w.config == nil || !w.config.Slack.NotifyStart {
		return
	}

	np := toNotifyPlanInfo(info)

	// If Board has a Slack thread URL, seed the thread tracker so we resume the existing thread.
	if plan.SlackThreadURL != "" {
		channelID, threadTS, err := notify.ParseSlackThreadURL(plan.SlackThreadURL)
		if err == nil {
			if sn, ok := w.notifier.(*notify.SlackNotifier); ok && sn != nil {
				_ = sn.SeedThread(info.Name, channelID, threadTS)
				log.Info("Resumed Slack thread from Board: %s", plan.SlackThreadURL)

				// Update the living status card instead of creating a new thread
				progress := &notify.ProgressStatus{
					Iteration:     0,
					MaxIterations: 0,
					Phase:         notify.PhaseInitializing,
				}
				_ = w.notifier.UpdateProgress(np, progress)
				return
			}
		} else {
			log.Warn("Failed to parse Slack thread URL from Board: %v", err)
		}
	}

	// No existing thread — create a new one via Start()
	if err := w.notifier.Start(np); err != nil {
		log.Warn("Failed to send start notification: %v", err)
		return
	}

	// After Start(), save the new thread URL back to Board
	if sn, ok := w.notifier.(*notify.SlackNotifier); ok && sn != nil {
		threadURL := sn.GetThreadURL(info.Name)
		if threadURL != "" {
			if _, err := w.board.UpdatePlan(plan.ID, map[string]string{"slack-thread-url": threadURL}); err != nil {
				log.Warn("Failed to save Slack thread URL to Board: %v", err)
			} else {
				log.Info("Saved Slack thread URL to Board: %s", threadURL)
			}
		}
	}
}

func (w *Worker) sendCompleteNotification(info *PlanInfo, prURL string) {
	if w.config == nil || !w.config.Slack.NotifyComplete {
		return
	}
	if err := w.notifier.Complete(toNotifyPlanInfo(info), prURL); err != nil {
		log.Warn("Failed to send complete notification: %v", err)
	}
}

func (w *Worker) sendBlockerNotification(info *PlanInfo, blocker *runner.Blocker) {
	if w.config == nil || !w.config.Slack.NotifyBlocker {
		return
	}
	if err := w.notifier.BlockerNotify(toNotifyPlanInfo(info), toNotifyBlocker(blocker)); err != nil {
		log.Warn("Failed to send blocker notification: %v", err)
	}
}

func (w *Worker) notifyError(info *PlanInfo, err error) {
	if w.config == nil || !w.config.Slack.NotifyError {
		return
	}
	if notifyErr := w.notifier.Error(toNotifyPlanInfo(info), err); notifyErr != nil {
		log.Warn("Failed to send error notification: %v", notifyErr)
	}
}

func (w *Worker) sendIterationNotification(info *PlanInfo, iteration, maxIterations int) {
	if w.config == nil || !w.config.Slack.NotifyIteration {
		return
	}
	if err := w.notifier.Iteration(toNotifyPlanInfo(info), iteration, maxIterations); err != nil {
		log.Warn("Failed to send iteration notification: %v", err)
	}
}

// branchToWorktreeName converts a branch name to a safe worktree directory name.
func branchToWorktreeName(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}
