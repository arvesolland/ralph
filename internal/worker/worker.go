// Package worker implements the queue processing loop for Ralph.
// It polls ATM for ready plans, creates worktrees, runs the iteration loop,
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

	"github.com/arvesolland/ralph/internal/atm"
	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/notify"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/runner"
)

// Default constants for worker configuration.
const (
	DefaultPollInterval  = 30 * time.Second
	DefaultMaxIterations = 200
)

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
	ATM                atm.ATM
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

	// Callbacks
	OnPlanStart    func(info *PlanInfo)
	OnPlanComplete func(info *PlanInfo, result *runner.LoopResult)
	OnPlanError    func(info *PlanInfo, err error)
	OnBlocker      func(info *PlanInfo, blocker *runner.Blocker)
}

// Worker processes plans from the ATM queue.
type Worker struct {
	atm                atm.ATM
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

	// Callbacks
	onPlanStart    func(info *PlanInfo)
	onPlanComplete func(info *PlanInfo, result *runner.LoopResult)
	onPlanError    func(info *PlanInfo, err error)
	onBlocker      func(info *PlanInfo, blocker *runner.Blocker)
}

// NewWorker creates a new Worker with the given configuration.
func NewWorker(cfg WorkerConfig) *Worker {
	w := &Worker{
		atm:                cfg.ATM,
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
	if w.notifier == nil {
		w.notifier = &notify.NoopNotifier{}
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
			// Continue running after errors
		}
	}
}

// RunOnce attempts to process a single plan from the queue.
// Returns ErrQueueEmpty if no plans are available.
func (w *Worker) RunOnce(ctx context.Context) error {
	// Check for an active plan (resume scenario)
	agentCtx, err := w.atm.ProjectContext(w.projectSlug)
	if err != nil {
		return fmt.Errorf("fetching project context: %w", err)
	}

	var plan *atm.Plan

	// Check if there's an active plan to resume
	if agentCtx.Plan.ID > 0 && agentCtx.Plan.Status == atm.PlanStatusActive {
		planCopy := agentCtx.Plan
		plan = &planCopy
		log.Info("Resuming active plan #%d: %s", plan.ID, plan.Title)
	} else {
		// Check for ready plans
		readyPlans, err := w.atm.ListPlans(w.projectSlug, atm.PlanStatusReady)
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
		updated, err := w.atm.UpdatePlanStatus(plan.ID, atm.PlanStatusActive)
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
func (w *Worker) processPlan(ctx context.Context, plan *atm.Plan, info *PlanInfo) error {
	log.Info("Processing plan: %s (branch: %s)", info.Name, info.Branch)

	// Send start notification
	w.sendStartNotification(info)

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
		ATM:              w.atm,
		PlanID:           plan.ID,
		ProjectSlug:      w.projectSlug,
		Context:          execCtx,
		Config:           w.config,
		Runner:           w.runner,
		Git:              git.NewGit(worktreePath),
		PromptBuilder:    w.promptBuilder,
		WorktreePath:     worktreePath,
		IterationTimeout: runner.IterationTimeout,
		OnIteration: func(iteration int, result *runner.Result) {
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

		// Mark plan as blocked in ATM
		if _, err := w.atm.UpdatePlanStatus(plan.ID, atm.PlanStatusBlocked); err != nil {
			log.Error("Failed to set plan status to blocked: %v", err)
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
func (w *Worker) ensureWorktree(plan *atm.Plan) (string, error) {
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
func (w *Worker) loadOrCreateContext(plan *atm.Plan, worktreePath string) (*runner.Context, error) {
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
func (w *Worker) completePlan(plan *atm.Plan, info *PlanInfo, worktreePath string) error {
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

	// Update ATM status to complete
	if _, err := w.atm.UpdatePlanStatus(plan.ID, atm.PlanStatusComplete); err != nil {
		log.Error("Failed to set plan status to complete in ATM: %v", err)
	}

	// Clean up worktree
	if w.worktreeManager != nil {
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
func (w *Worker) completePR(plan *atm.Plan, info *PlanInfo, worktreePath string) (string, error) {
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
	tracker, err := notify.NewThreadTracker(w.configDir)
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

func (w *Worker) sendStartNotification(info *PlanInfo) {
	if w.config == nil || !w.config.Slack.NotifyStart {
		return
	}
	if err := w.notifier.Start(toNotifyPlanInfo(info)); err != nil {
		log.Warn("Failed to send start notification: %v", err)
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
