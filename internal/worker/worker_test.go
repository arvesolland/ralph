package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/board"
	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/notify"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/retry"
	"github.com/arvesolland/ralph/internal/runner"
)

// MockBoardRunner implements runner.Runner for testing.
type MockBoardRunner struct {
	RunFunc func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error)
	calls   int
}

func (m *MockBoardRunner) Run(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
	m.calls++
	if m.RunFunc != nil {
		return m.RunFunc(ctx, p, opts)
	}
	if m.calls >= 2 {
		return &runner.Result{
			Output:      `{"type":"assistant","message":{"content":[{"type":"text","text":"Done"}]}}`,
			TextContent: "Done\n<promise>COMPLETE</promise>",
			Duration:    time.Second,
			Attempts:    1,
			IsComplete:  true,
		}, nil
	}
	return &runner.Result{
		Output:      `{"type":"assistant","message":{"content":[{"type":"text","text":"Working..."}]}}`,
		TextContent: "Working...",
		Duration:    time.Second,
		Attempts:    1,
	}, nil
}

func TestNewWorker(t *testing.T) {
	cfg := WorkerConfig{
		Config:           config.Defaults(),
		ConfigDir:        "/tmp/.ralph",
		MainWorktreePath: "/tmp",
		PollInterval:     10 * time.Second,
		MaxIterations:    5,
		CompletionMode:   "merge",
	}

	w := NewWorker(cfg)

	if w.pollInterval != 10*time.Second {
		t.Errorf("pollInterval = %v, want %v", w.pollInterval, 10*time.Second)
	}

	if w.maxIterations != 5 {
		t.Errorf("maxIterations = %d, want %d", w.maxIterations, 5)
	}

	if w.completionMode != "merge" {
		t.Errorf("completionMode = %q, want %q", w.completionMode, "merge")
	}
}

func TestNewWorker_Defaults(t *testing.T) {
	cfg := WorkerConfig{
		Config:           config.Defaults(),
		MainWorktreePath: "/tmp",
	}

	w := NewWorker(cfg)

	if w.pollInterval != DefaultPollInterval {
		t.Errorf("pollInterval = %v, want %v", w.pollInterval, DefaultPollInterval)
	}

	if w.maxIterations != DefaultMaxIterations {
		t.Errorf("maxIterations = %d, want %d", w.maxIterations, DefaultMaxIterations)
	}

	if w.completionMode != "pr" {
		t.Errorf("completionMode = %q, want %q", w.completionMode, "pr")
	}
}

func TestConstants(t *testing.T) {
	if DefaultPollInterval != 30*time.Second {
		t.Errorf("DefaultPollInterval = %v, want %v", DefaultPollInterval, 30*time.Second)
	}

	if DefaultMaxIterations != 200 {
		t.Errorf("DefaultMaxIterations = %d, want %d", DefaultMaxIterations, 200)
	}
}

func TestErrors(t *testing.T) {
	if ErrQueueEmpty.Error() != "no pending plans in queue" {
		t.Errorf("ErrQueueEmpty message unexpected: %q", ErrQueueEmpty.Error())
	}

	if ErrInterrupted.Error() != "interrupted by signal" {
		t.Errorf("ErrInterrupted message unexpected: %q", ErrInterrupted.Error())
	}
}

func TestNewWorker_WithNotifier(t *testing.T) {
	mockNotifier := &MockNotifier{}

	cfg := WorkerConfig{
		Config:           config.Defaults(),
		MainWorktreePath: "/tmp",
		Notifier:         mockNotifier,
	}

	w := NewWorker(cfg)

	if w.notifier != mockNotifier {
		t.Error("Expected notifier to be set")
	}
}

func TestNewWorker_DefaultNotifier(t *testing.T) {
	cfg := WorkerConfig{
		Config:           config.Defaults(),
		MainWorktreePath: "/tmp",
	}

	w := NewWorker(cfg)

	if _, ok := w.notifier.(*notify.NoopNotifier); !ok {
		t.Error("Expected notifier to be NoopNotifier when not provided")
	}
}

func TestWorker_SendNotifications(t *testing.T) {
	mockNotifier := &MockNotifier{}

	cfg := config.Defaults()
	cfg.Slack.NotifyStart = true
	cfg.Slack.NotifyComplete = true
	cfg.Slack.NotifyError = true
	cfg.Slack.NotifyBlocker = true
	cfg.Slack.NotifyIteration = true

	w := &Worker{
		config:   cfg,
		notifier: mockNotifier,
	}

	testInfo := &PlanInfo{ID: 1, Name: "test", Branch: "feat/test"}
	testPlan := &board.Plan{ID: 1, Title: "test"}

	w.sendStartNotification(testPlan, testInfo)
	if mockNotifier.StartCalls != 1 {
		t.Errorf("StartCalls = %d, want 1", mockNotifier.StartCalls)
	}

	w.sendCompleteNotification(testInfo, "https://github.com/test/pr/1")
	if mockNotifier.CompleteCalls != 1 {
		t.Errorf("CompleteCalls = %d, want 1", mockNotifier.CompleteCalls)
	}
	if mockNotifier.LastPRURL != "https://github.com/test/pr/1" {
		t.Errorf("LastPRURL = %q, want %q", mockNotifier.LastPRURL, "https://github.com/test/pr/1")
	}

	blocker := &runner.Blocker{Description: "Test blocker", Hash: "abc123"}
	w.sendBlockerNotification(testInfo, blocker)
	if mockNotifier.BlockerCalls != 1 {
		t.Errorf("BlockerCalls = %d, want 1", mockNotifier.BlockerCalls)
	}

	testErr := ErrGHNotInstalled
	w.notifyError(testInfo, testErr)
	if mockNotifier.ErrorCalls != 1 {
		t.Errorf("ErrorCalls = %d, want 1", mockNotifier.ErrorCalls)
	}

	w.sendIterationNotification(testInfo, 5, 10)
	if mockNotifier.IterationCalls != 1 {
		t.Errorf("IterationCalls = %d, want 1", mockNotifier.IterationCalls)
	}
}

func TestWorker_SendNotifications_Disabled(t *testing.T) {
	mockNotifier := &MockNotifier{}

	cfg := config.Defaults()
	cfg.Slack.NotifyStart = false
	cfg.Slack.NotifyComplete = false
	cfg.Slack.NotifyError = false
	cfg.Slack.NotifyBlocker = false
	cfg.Slack.NotifyIteration = false

	w := &Worker{
		config:   cfg,
		notifier: mockNotifier,
	}

	testInfo := &PlanInfo{ID: 1, Name: "test", Branch: "feat/test"}
	testPlan := &board.Plan{ID: 1, Title: "test"}

	w.sendStartNotification(testPlan, testInfo)
	w.sendCompleteNotification(testInfo, "")
	w.sendBlockerNotification(testInfo, &runner.Blocker{})
	w.notifyError(testInfo, ErrGHNotInstalled)
	w.sendIterationNotification(testInfo, 1, 10)

	if mockNotifier.StartCalls != 0 {
		t.Errorf("StartCalls = %d, want 0", mockNotifier.StartCalls)
	}
	if mockNotifier.CompleteCalls != 0 {
		t.Errorf("CompleteCalls = %d, want 0", mockNotifier.CompleteCalls)
	}
	if mockNotifier.BlockerCalls != 0 {
		t.Errorf("BlockerCalls = %d, want 0", mockNotifier.BlockerCalls)
	}
	if mockNotifier.ErrorCalls != 0 {
		t.Errorf("ErrorCalls = %d, want 0", mockNotifier.ErrorCalls)
	}
	if mockNotifier.IterationCalls != 0 {
		t.Errorf("IterationCalls = %d, want 0", mockNotifier.IterationCalls)
	}
}

func TestWorker_SendNotifications_NilConfig(t *testing.T) {
	mockNotifier := &MockNotifier{}

	w := &Worker{
		config:   nil,
		notifier: mockNotifier,
	}

	testInfo := &PlanInfo{ID: 1, Name: "test", Branch: "feat/test"}
	testPlan := &board.Plan{ID: 1, Title: "test"}

	// Should not panic with nil config
	w.sendStartNotification(testPlan, testInfo)
	w.sendCompleteNotification(testInfo, "")
	w.sendBlockerNotification(testInfo, &runner.Blocker{})
	w.notifyError(testInfo, ErrGHNotInstalled)
	w.sendIterationNotification(testInfo, 1, 10)

	if mockNotifier.StartCalls != 0 || mockNotifier.CompleteCalls != 0 ||
		mockNotifier.BlockerCalls != 0 || mockNotifier.ErrorCalls != 0 ||
		mockNotifier.IterationCalls != 0 {
		t.Error("Expected no notification calls with nil config")
	}
}

func TestWorker_LoadOrCreateContext_StaleContext(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.Defaults()
	cfg.Git.BaseBranch = "main"

	w := &Worker{
		config:        cfg,
		maxIterations: 10,
	}

	// Create a context.json for plan ID 100
	ralphDir := filepath.Join(tmpDir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	ctxPath := filepath.Join(ralphDir, "context.json")

	oldCtx := &runner.Context{
		PlanID:        100,
		FeatureBranch: "feat/old-plan",
		BaseBranch:    "main",
		Iteration:     5,
		MaxIterations: 10,
	}
	if err := runner.SaveContext(oldCtx, ctxPath); err != nil {
		t.Fatalf("Failed to save old context: %v", err)
	}

	// Create a plan with different ID
	plan := &board.Plan{ID: 200, FeatureBranch: "feat/new-plan"}

	execCtx, err := w.loadOrCreateContext(plan, tmpDir)
	if err != nil {
		t.Fatalf("loadOrCreateContext() error = %v", err)
	}

	// Should be fresh context (iteration 1, matching new plan)
	if execCtx.Iteration != 1 {
		t.Errorf("Iteration = %d, want 1 (fresh context)", execCtx.Iteration)
	}
	if execCtx.PlanID != 200 {
		t.Errorf("PlanID = %d, want %d", execCtx.PlanID, 200)
	}
}

func TestWorker_LoadOrCreateContext_MatchingContext(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.Defaults()
	cfg.Git.BaseBranch = "main"

	w := &Worker{
		config:        cfg,
		maxIterations: 10,
	}

	// Create a context.json for plan ID 42
	ralphDir := filepath.Join(tmpDir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	ctxPath := filepath.Join(ralphDir, "context.json")

	existingCtx := &runner.Context{
		PlanID:        42,
		FeatureBranch: "feat/my-plan",
		BaseBranch:    "main",
		Iteration:     3,
		MaxIterations: 10,
	}
	if err := runner.SaveContext(existingCtx, ctxPath); err != nil {
		t.Fatalf("Failed to save context: %v", err)
	}

	// Create a plan with matching ID
	plan := &board.Plan{ID: 42, FeatureBranch: "feat/my-plan"}

	execCtx, err := w.loadOrCreateContext(plan, tmpDir)
	if err != nil {
		t.Fatalf("loadOrCreateContext() error = %v", err)
	}

	if execCtx.Iteration != 3 {
		t.Errorf("Iteration = %d, want 3 (reused context)", execCtx.Iteration)
	}
}

func TestWorker_SetupNotifications(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".ralph")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	cfg := config.Defaults()
	cfg.Slack.WebhookURL = "https://hooks.slack.com/services/test"
	cfg.Slack.NotifyStart = true

	w := &Worker{
		config:           cfg,
		configDir:        configDir,
		mainWorktreePath: tmpDir,
	}

	ctx := context.Background()
	cleanup := w.SetupNotifications(ctx)
	defer cleanup()

	if w.notifier == nil {
		t.Error("Expected notifier to be created")
	}

	if _, ok := w.notifier.(*notify.WebhookNotifier); !ok {
		t.Error("Expected WebhookNotifier")
	}
}

func TestToNotifyBlocker(t *testing.T) {
	t.Run("nil blocker", func(t *testing.T) {
		result := toNotifyBlocker(nil)
		if result != nil {
			t.Error("Expected nil for nil blocker")
		}
	})

	t.Run("with blocker", func(t *testing.T) {
		blocker := &runner.Blocker{
			Content:     "full content",
			Description: "desc",
			Action:      "do something",
			Resume:      "will resume",
			Hash:        "abc123",
		}
		result := toNotifyBlocker(blocker)
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result.Description != "desc" {
			t.Errorf("Description = %q, want %q", result.Description, "desc")
		}
		if result.Hash != "abc123" {
			t.Errorf("Hash = %q, want %q", result.Hash, "abc123")
		}
	})
}

// MockNotifier implements notify.Notifier for testing.
type MockNotifier struct {
	StartCalls     int
	CompleteCalls  int
	BlockerCalls   int
	ErrorCalls     int
	IterationCalls int
	ProgressCalls  int
	LastPRURL      string
	LastBlocker    *notify.Blocker
	LastError      error
	LastProgress   *notify.ProgressStatus
}

func (m *MockNotifier) Start(p notify.PlanInfo) error {
	m.StartCalls++
	return nil
}

func (m *MockNotifier) Complete(p notify.PlanInfo, prURL string) error {
	m.CompleteCalls++
	m.LastPRURL = prURL
	return nil
}

func (m *MockNotifier) BlockerNotify(p notify.PlanInfo, blocker *notify.Blocker) error {
	m.BlockerCalls++
	m.LastBlocker = blocker
	return nil
}

func (m *MockNotifier) Error(p notify.PlanInfo, err error) error {
	m.ErrorCalls++
	m.LastError = err
	return nil
}

func (m *MockNotifier) Iteration(p notify.PlanInfo, iteration, maxIterations int) error {
	m.IterationCalls++
	return nil
}

func (m *MockNotifier) UpdateProgress(p notify.PlanInfo, status *notify.ProgressStatus) error {
	m.ProgressCalls++
	m.LastProgress = status
	return nil
}
func (m *MockNotifier) Flush() {}

// MockWorktreeManager implements WorktreeManager for testing.
type MockWorktreeManager struct {
	worktrees   map[string]string // name -> path
	RemoveCalls int               // tracks number of Remove calls
}

func newMockWorktreeManager() *MockWorktreeManager {
	return &MockWorktreeManager{worktrees: make(map[string]string)}
}

func (m *MockWorktreeManager) Create(name, branch string) (string, error) {
	// Return pre-registered path or error
	if path, ok := m.worktrees[name]; ok {
		return path, nil
	}
	return "", errors.New("worktree not pre-registered in mock")
}

func (m *MockWorktreeManager) Get(name string) (string, error) {
	if path, ok := m.worktrees[name]; ok {
		return path, nil
	}
	return "", errors.New("worktree not found")
}

func (m *MockWorktreeManager) Remove(name string, deleteBranch bool) error {
	m.RemoveCalls++
	delete(m.worktrees, name)
	return nil
}

func (m *MockWorktreeManager) Exists(name string) bool {
	_, ok := m.worktrees[name]
	return ok
}

// Register pre-sets a worktree name to a given path so Create/Get return it.
func (m *MockWorktreeManager) Register(name, path string) {
	m.worktrees[name] = path
}

// setupWorkerTestGitRepo creates a git repo with a local bare remote and a feature branch.
// This allows push operations to succeed in the completion workflow.
func setupWorkerTestGitRepo(t *testing.T, dir, featureBranch string) git.Git {
	t.Helper()

	// Create a bare repo to act as remote
	bareDir := filepath.Join(filepath.Dir(dir), "bare-"+branchToWorktreeName(featureBranch)+".git")
	if err := os.MkdirAll(bareDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = bareDir
	bareInit.Stderr = os.Stderr
	if err := bareInit.Run(); err != nil {
		t.Fatalf("Failed to init bare repo: %v", err)
	}

	// Init worktree repo, create feature branch, and set up remote
	initCmd := "git init && git config user.email test@test.com && git config user.name Test && " +
		"git commit --allow-empty -m 'initial' && " +
		"git checkout -b " + featureBranch + " && " +
		"git commit --allow-empty -m 'feature' && " +
		"git remote add origin " + bareDir + " && " +
		"git push -u origin " + featureBranch
	cmd := exec.Command("sh", "-c", initCmd)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}
	return git.NewGit(dir)
}

func TestWorker_RunOnce_ActivatesPlan(t *testing.T) {
	// ProjectContext returns no active plan, ListPlans returns one ready plan
	// → UpdatePlanStatus("active") should be called, plan gets processed
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	setupWorkerTestGitRepo(t, wtDir, "feat/test")

	mockBoard := board.NewMockBoard()

	// No active plan in project context
	mockBoard.ProjectContextFunc = func(slug string) (*board.AgentContext, error) {
		return &board.AgentContext{
			Plan: board.Plan{ID: 0}, // No active plan
		}, nil
	}

	// One ready plan
	mockBoard.ListPlansFunc = func(projectSlug, status string) ([]board.Plan, error) {
		if status == board.PlanStatusReady {
			return []board.Plan{
				{ID: 10, Title: "Test Plan", FeatureBranch: "feat/test", Status: board.PlanStatusReady},
			}, nil
		}
		return nil, nil
	}

	// Track UpdatePlanStatus calls
	var statusUpdates []struct{ ID int; Status string }
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		statusUpdates = append(statusUpdates, struct{ ID int; Status string }{id, status})
		return &board.Plan{ID: id, Title: "Test Plan", FeatureBranch: "feat/test", Status: status}, nil
	}
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Context", nil
	}

	// Runner completes immediately
	mockRunner := &MockBoardRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			return &runner.Result{
				TextContent: "Done\n<promise>COMPLETE</promise>",
				IsComplete:  true,
				Duration:    100 * time.Millisecond,
			}, nil
		},
	}

	// Board says all tasks done for completion check
	mockBoard.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			Stats: board.Stats{TotalTasks: 1, Done: 1},
		}, nil
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-test", wtDir)

	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		Board:            mockBoard,
		ProjectSlug:      "test-project",
		Config:           cfg,
		WorktreeManager:  wtMgr,
		Git:              git.NewGit(tmpDir),
		MainWorktreePath: tmpDir,
		Runner:           mockRunner,
		PromptBuilder:    prompt.NewBuilder(cfg, "", ""),
		MaxIterations:    5,
		CompletionMode:   "branch",
	})

	err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	// Should have activated the plan
	foundActivate := false
	for _, u := range statusUpdates {
		if u.ID == 10 && u.Status == board.PlanStatusActive {
			foundActivate = true
		}
	}
	if !foundActivate {
		t.Errorf("Expected UpdatePlanStatus(10, 'active'), got updates: %v", statusUpdates)
	}
}

func TestWorker_RunOnce_ResumesActivePlan(t *testing.T) {
	// ProjectContext returns an active plan → resumes without calling ListPlans
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	setupWorkerTestGitRepo(t, wtDir, "feat/active")

	mockBoard := board.NewMockBoard()

	// Active plan in project context
	mockBoard.ProjectContextFunc = func(slug string) (*board.AgentContext, error) {
		return &board.AgentContext{
			Plan: board.Plan{ID: 20, Title: "Active Plan", FeatureBranch: "feat/active", Status: board.PlanStatusActive},
		}, nil
	}

	// ListPlans should NOT be called
	listPlansCalled := false
	mockBoard.ListPlansFunc = func(projectSlug, status string) ([]board.Plan, error) {
		listPlansCalled = true
		return nil, nil
	}

	var statusUpdates []string
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		statusUpdates = append(statusUpdates, status)
		return &board.Plan{ID: id, Title: "Active Plan", FeatureBranch: "feat/active", Status: status}, nil
	}
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Context", nil
	}
	mockBoard.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			Stats: board.Stats{TotalTasks: 1, Done: 1},
		}, nil
	}

	mockRunner := &MockBoardRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			return &runner.Result{
				TextContent: "Done\n<promise>COMPLETE</promise>",
				IsComplete:  true,
				Duration:    100 * time.Millisecond,
			}, nil
		},
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-active", wtDir)

	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		Board:            mockBoard,
		ProjectSlug:      "test-project",
		Config:           cfg,
		WorktreeManager:  wtMgr,
		Git:              git.NewGit(tmpDir),
		MainWorktreePath: tmpDir,
		Runner:           mockRunner,
		PromptBuilder:    prompt.NewBuilder(cfg, "", ""),
		MaxIterations:    5,
		CompletionMode:   "branch",
	})

	err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	// ListPlans should NOT have been called (active plan was found)
	if listPlansCalled {
		t.Error("Expected ListPlans to NOT be called when resuming active plan")
	}
}

func TestWorker_RunOnce_EmptyQueue(t *testing.T) {
	// No active plan, no ready plans → ErrQueueEmpty
	mockBoard := board.NewMockBoard()

	mockBoard.ProjectContextFunc = func(slug string) (*board.AgentContext, error) {
		return &board.AgentContext{
			Plan: board.Plan{ID: 0},
		}, nil
	}
	mockBoard.ListPlansFunc = func(projectSlug, status string) ([]board.Plan, error) {
		return []board.Plan{}, nil
	}

	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		Board:            mockBoard,
		ProjectSlug:      "test-project",
		Config:           cfg,
		MainWorktreePath: "/tmp",
	})

	err := w.RunOnce(context.Background())
	if !errors.Is(err, ErrQueueEmpty) {
		t.Errorf("Expected ErrQueueEmpty, got: %v", err)
	}
}

func TestWorker_RunOnce_CompletionFlow(t *testing.T) {
	// Verifies the full status transition: ready → active → complete
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	setupWorkerTestGitRepo(t, wtDir, "feat/complete")

	mockBoard := board.NewMockBoard()

	mockBoard.ProjectContextFunc = func(slug string) (*board.AgentContext, error) {
		return &board.AgentContext{Plan: board.Plan{ID: 0}}, nil
	}
	mockBoard.ListPlansFunc = func(projectSlug, status string) ([]board.Plan, error) {
		if status == board.PlanStatusReady {
			return []board.Plan{
				{ID: 30, Title: "Completion Test", FeatureBranch: "feat/complete", Status: board.PlanStatusReady},
			}, nil
		}
		return nil, nil
	}

	// Track all status transitions
	var transitions []struct{ ID int; Status string }
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		transitions = append(transitions, struct{ ID int; Status string }{id, status})
		return &board.Plan{ID: id, Title: "Completion Test", FeatureBranch: "feat/complete", Status: status}, nil
	}
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Context", nil
	}
	mockBoard.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			Stats: board.Stats{TotalTasks: 1, Done: 1},
		}, nil
	}

	mockRunner := &MockBoardRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			return &runner.Result{
				TextContent: "Done\n<promise>COMPLETE</promise>",
				IsComplete:  true,
				Duration:    100 * time.Millisecond,
			}, nil
		},
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-complete", wtDir)

	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		Board:            mockBoard,
		ProjectSlug:      "test-project",
		Config:           cfg,
		WorktreeManager:  wtMgr,
		Git:              git.NewGit(tmpDir),
		MainWorktreePath: tmpDir,
		Runner:           mockRunner,
		PromptBuilder:    prompt.NewBuilder(cfg, "", ""),
		MaxIterations:    5,
		CompletionMode:   "branch",
	})

	err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	// Verify transitions: first active, then complete
	if len(transitions) < 2 {
		t.Fatalf("Expected at least 2 status transitions, got %d: %v", len(transitions), transitions)
	}

	if transitions[0].Status != board.PlanStatusActive {
		t.Errorf("Expected first transition to 'active', got %q", transitions[0].Status)
	}

	// Last transition should be complete
	last := transitions[len(transitions)-1]
	if last.Status != board.PlanStatusComplete {
		t.Errorf("Expected last transition to 'complete', got %q", last.Status)
	}
}

// fastRetryConfig returns a retry config with minimal delays for testing.
func fastRetryConfig() retry.RetryConfig {
	return retry.RetryConfig{
		MaxRetries:   5,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		JitterFactor: 0,
	}
}

func TestCompletePlan_RetriesStatusUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	setupWorkerTestGitRepo(t, wtDir, "feat/retry-test")

	mockBoard := board.NewMockBoard()

	// Fail twice with a retryable error, then succeed
	updateCalls := 0
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		updateCalls++
		if status == board.PlanStatusComplete && updateCalls <= 2 {
			return nil, fmt.Errorf("connection refused")
		}
		return &board.Plan{ID: id, Status: status}, nil
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-retry-test", wtDir)

	cfg := config.Defaults()
	fastCfg := fastRetryConfig()
	w := &Worker{
		board:             mockBoard,
		config:            cfg,
		worktreeManager:   wtMgr,
		notifier:          &notify.NoopNotifier{},
		completionMode:    "branch",
		git:               git.NewGit(tmpDir),
		statusRetryConfig: fastCfg,
	}

	plan := &board.Plan{ID: 42, FeatureBranch: "feat/retry-test"}
	info := &PlanInfo{ID: 42, Name: "Retry Test", Branch: "feat/retry-test"}

	err := w.completePlan(plan, info, wtDir)
	if err != nil {
		t.Fatalf("completePlan() error = %v", err)
	}

	// Should have retried (3 calls: 2 failures + 1 success)
	if updateCalls < 3 {
		t.Errorf("Expected at least 3 UpdatePlanStatus calls (with retries), got %d", updateCalls)
	}

	// Worktree should have been cleaned up (status update eventually succeeded)
	if wtMgr.RemoveCalls != 1 {
		t.Errorf("Expected 1 worktree Remove call, got %d", wtMgr.RemoveCalls)
	}
}

func TestCompletePlan_SkipsWorktreeCleanupOnStatusFailure(t *testing.T) {
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	setupWorkerTestGitRepo(t, wtDir, "feat/skip-cleanup")

	mockBoard := board.NewMockBoard()

	// Always fail with a retryable error
	updateCalls := 0
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		updateCalls++
		if status == board.PlanStatusComplete {
			return nil, fmt.Errorf("connection refused")
		}
		return &board.Plan{ID: id, Status: status}, nil
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-skip-cleanup", wtDir)

	cfg := config.Defaults()
	fastCfg := fastRetryConfig()
	w := &Worker{
		board:             mockBoard,
		config:            cfg,
		worktreeManager:   wtMgr,
		notifier:          &notify.NoopNotifier{},
		completionMode:    "branch",
		git:               git.NewGit(tmpDir),
		statusRetryConfig: fastCfg,
	}

	plan := &board.Plan{ID: 43, FeatureBranch: "feat/skip-cleanup"}
	info := &PlanInfo{ID: 43, Name: "Skip Cleanup", Branch: "feat/skip-cleanup"}

	// completePlan should return nil even when Board status update fails
	err := w.completePlan(plan, info, wtDir)
	if err != nil {
		t.Fatalf("completePlan() should return nil when only Board status update fails, got: %v", err)
	}

	// Worktree should NOT have been cleaned up
	if wtMgr.RemoveCalls != 0 {
		t.Errorf("Expected 0 worktree Remove calls (cleanup should be skipped), got %d", wtMgr.RemoveCalls)
	}

	// Should have attempted retries (initial + 5 retries = 6 calls)
	if updateCalls != 6 {
		t.Errorf("Expected 6 UpdatePlanStatus calls (1 initial + 5 retries), got %d", updateCalls)
	}
}

func TestCompletePlan_NonRetryableErrorFailsFast(t *testing.T) {
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	setupWorkerTestGitRepo(t, wtDir, "feat/non-retryable")

	mockBoard := board.NewMockBoard()

	// Fail with a non-retryable error (e.g., 404 not found)
	updateCalls := 0
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		updateCalls++
		if status == board.PlanStatusComplete {
			return nil, fmt.Errorf("not found: plan 99")
		}
		return &board.Plan{ID: id, Status: status}, nil
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-non-retryable", wtDir)

	cfg := config.Defaults()
	fastCfg := fastRetryConfig()
	w := &Worker{
		board:             mockBoard,
		config:            cfg,
		worktreeManager:   wtMgr,
		notifier:          &notify.NoopNotifier{},
		completionMode:    "branch",
		git:               git.NewGit(tmpDir),
		statusRetryConfig: fastCfg,
	}

	plan := &board.Plan{ID: 99, FeatureBranch: "feat/non-retryable"}
	info := &PlanInfo{ID: 99, Name: "Non-Retryable", Branch: "feat/non-retryable"}

	err := w.completePlan(plan, info, wtDir)
	if err != nil {
		t.Fatalf("completePlan() should return nil even on non-retryable error, got: %v", err)
	}

	// Non-retryable error should fail after 1 attempt (no retries)
	if updateCalls != 1 {
		t.Errorf("Expected 1 UpdatePlanStatus call (no retries for non-retryable), got %d", updateCalls)
	}

	// Worktree should NOT be cleaned up (status update failed)
	if wtMgr.RemoveCalls != 0 {
		t.Errorf("Expected 0 worktree Remove calls, got %d", wtMgr.RemoveCalls)
	}
}

func TestProcessPlan_KnowledgeExtraction(t *testing.T) {
	// Verifies that the OnBeforeIteration callback extracts lessons from Board
	// feedback and writes them to .claude/learnings.md in the worktree.
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	setupWorkerTestGitRepo(t, wtDir, "feat/knowledge-test")

	// Create a CLAUDE.md in the worktree so EnsureReference can add to it
	claudeMDPath := filepath.Join(wtDir, "CLAUDE.md")
	if err := os.WriteFile(claudeMDPath, []byte("# Project\n\nSome instructions.\n"), 0644); err != nil {
		t.Fatalf("Failed to write CLAUDE.md: %v", err)
	}

	mockBoard := board.NewMockBoard()

	// PlanContextText for prompt building
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Context", nil
	}

	// PlanContext returns human feedback for knowledge extraction
	mockBoard.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			Stats: board.Stats{TotalTasks: 1, Done: 1},
			RecentFeedback: []board.Feedback{
				{
					PlanID:    planID,
					Author:    "arve",
					Body:      "Always use structured logging instead of fmt.Println",
					CreatedAt: "2026-02-25",
				},
				{
					PlanID:    planID,
					Author:    "ralph", // system — should be excluded
					Body:      "iteration 1 completed",
					CreatedAt: "2026-02-25",
				},
			},
		}, nil
	}

	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		return &board.Plan{ID: id, Title: "Knowledge Test", FeatureBranch: "feat/knowledge-test", Status: status}, nil
	}

	mockRunner := &MockBoardRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			return &runner.Result{
				TextContent: "Done\n<promise>COMPLETE</promise>",
				IsComplete:  true,
				Duration:    100 * time.Millisecond,
			}, nil
		},
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-knowledge-test", wtDir)

	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		Board:            mockBoard,
		ProjectSlug:      "test-project",
		Config:           cfg,
		WorktreeManager:  wtMgr,
		Git:              git.NewGit(tmpDir),
		MainWorktreePath: tmpDir,
		Runner:           mockRunner,
		PromptBuilder:    prompt.NewBuilder(cfg, "", ""),
		MaxIterations:    5,
		CompletionMode:   "branch",
	})

	plan := &board.Plan{ID: 60, Title: "Knowledge Test", FeatureBranch: "feat/knowledge-test"}
	info := &PlanInfo{ID: 60, Name: "Knowledge Test", Branch: "feat/knowledge-test"}

	err := w.processPlan(context.Background(), plan, info)
	if err != nil {
		t.Fatalf("processPlan() error = %v", err)
	}

	// Verify .claude/learnings.md was created with the human feedback lesson
	learningsPath := filepath.Join(wtDir, ".claude", "learnings.md")
	content, err := os.ReadFile(learningsPath)
	if err != nil {
		t.Fatalf("Failed to read learnings file: %v", err)
	}

	learnings := string(content)
	if !strings.Contains(learnings, "Always use structured logging") {
		t.Errorf("Learnings file should contain human feedback lesson, got:\n%s", learnings)
	}
	if strings.Contains(learnings, "iteration 1 completed") {
		t.Error("Learnings file should NOT contain system-authored (ralph) feedback")
	}

	// Verify CLAUDE.md now references the learnings file
	claudeContent, err := os.ReadFile(claudeMDPath)
	if err != nil {
		t.Fatalf("Failed to read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(claudeContent), "Operational Learnings") {
		t.Errorf("CLAUDE.md should reference learnings file, got:\n%s", string(claudeContent))
	}
}

func TestProcessPlan_BlockedStatusUsesRetry(t *testing.T) {
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	setupWorkerTestGitRepo(t, wtDir, "feat/blocked-retry")

	mockBoard := board.NewMockBoard()

	// PlanContextText for prompt building
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Context", nil
	}
	// PlanContext for stats
	mockBoard.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			Stats: board.Stats{TotalTasks: 2, Done: 0},
		}, nil
	}

	// Track blocked status update calls
	blockedUpdateCalls := 0
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		if status == board.PlanStatusBlocked {
			blockedUpdateCalls++
			if blockedUpdateCalls <= 2 {
				return nil, fmt.Errorf("connection refused")
			}
		}
		return &board.Plan{ID: id, Status: status}, nil
	}

	// Runner returns an error to trigger the blocked path
	mockRunner := &MockBoardRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			return nil, fmt.Errorf("claude failed")
		},
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-blocked-retry", wtDir)

	cfg := config.Defaults()
	fastCfg := fastRetryConfig()
	w := NewWorker(WorkerConfig{
		Board:             mockBoard,
		ProjectSlug:       "test-project",
		Config:            cfg,
		WorktreeManager:   wtMgr,
		Git:               git.NewGit(tmpDir),
		MainWorktreePath:  tmpDir,
		Runner:            mockRunner,
		PromptBuilder:     prompt.NewBuilder(cfg, "", ""),
		MaxIterations:     1,
		CompletionMode:    "branch",
		StatusRetryConfig: &fastCfg,
	})

	plan := &board.Plan{ID: 50, Title: "Blocked Retry Test", FeatureBranch: "feat/blocked-retry"}
	info := &PlanInfo{ID: 50, Name: "Blocked Retry Test", Branch: "feat/blocked-retry"}

	// processPlan should return the loop error
	err := w.processPlan(context.Background(), plan, info)
	if err == nil {
		t.Fatal("processPlan() should return error when loop fails")
	}

	// Should have retried the blocked status update (3 calls: 2 failures + 1 success)
	if blockedUpdateCalls < 3 {
		t.Errorf("Expected at least 3 blocked UpdatePlanStatus calls (with retries), got %d", blockedUpdateCalls)
	}
}

func TestCompletePlan_StatusUpdateBeforeGitOps(t *testing.T) {
	// Verifies that Board status transitions to complete BEFORE git operations.
	// Uses an ordered log to track the sequence of calls.
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	setupWorkerTestGitRepo(t, wtDir, "feat/order-test")

	mockBoard := board.NewMockBoard()

	var callOrder []string
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		callOrder = append(callOrder, fmt.Sprintf("UpdatePlanStatus:%s", status))
		return &board.Plan{ID: id, Status: status, FeatureBranch: "feat/order-test"}, nil
	}
	mockBoard.AddProgressFunc = func(planID int, author, body string) (*board.Progress, error) {
		callOrder = append(callOrder, "AddProgress")
		return nil, nil
	}

	// Use a mock git that records when push happens
	mockGit := &mockGitForCompletion{
		pushError: nil,
		workDir:   wtDir,
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-order-test", wtDir)

	cfg := config.Defaults()
	fastCfg := fastRetryConfig()
	w := &Worker{
		board:             mockBoard,
		config:            cfg,
		worktreeManager:   wtMgr,
		notifier:          &notify.NoopNotifier{},
		completionMode:    "branch",
		git:               mockGit,
		statusRetryConfig: fastCfg,
	}

	plan := &board.Plan{ID: 70, FeatureBranch: "feat/order-test"}
	info := &PlanInfo{ID: 70, Name: "Order Test", Branch: "feat/order-test"}

	err := w.completePlan(plan, info, wtDir)
	if err != nil {
		t.Fatalf("completePlan() error = %v", err)
	}

	// Verify ordering: status update must come first
	if len(callOrder) < 2 {
		t.Fatalf("Expected at least 2 calls, got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "UpdatePlanStatus:complete" {
		t.Errorf("First call should be UpdatePlanStatus:complete, got %q", callOrder[0])
	}
	if callOrder[1] != "AddProgress" {
		t.Errorf("Second call should be AddProgress, got %q", callOrder[1])
	}
}

func TestCompletePlan_GitFailureAfterStatusUpdate(t *testing.T) {
	// Verifies that git operation failure after successful status update
	// does NOT cause completePlan to return an error. The plan stays complete
	// in Board and the worker moves on.
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	// Don't set up a git repo — push will fail, simulating git failure

	mockBoard := board.NewMockBoard()

	var statusUpdated bool
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		if status == board.PlanStatusComplete {
			statusUpdated = true
		}
		return &board.Plan{ID: id, Status: status, FeatureBranch: "feat/git-fail"}, nil
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-git-fail", wtDir)

	cfg := config.Defaults()
	fastCfg := fastRetryConfig()

	// Test merge mode — checkout will fail because no git repo
	w := &Worker{
		board:             mockBoard,
		config:            cfg,
		worktreeManager:   wtMgr,
		notifier:          &notify.NoopNotifier{},
		completionMode:    "merge",
		git:               git.NewGit(wtDir), // no actual git repo → merge will fail
		statusRetryConfig: fastCfg,
	}

	plan := &board.Plan{ID: 80, FeatureBranch: "feat/git-fail"}
	info := &PlanInfo{ID: 80, Name: "Git Fail", Branch: "feat/git-fail"}

	// completePlan should return nil even when git operations fail
	err := w.completePlan(plan, info, wtDir)
	if err != nil {
		t.Fatalf("completePlan() should return nil when git fails after status update, got: %v", err)
	}

	// Board status should have been updated to complete
	if !statusUpdated {
		t.Error("Expected Board status to be updated to complete")
	}

	// Worktree should still be cleaned up (status succeeded)
	if wtMgr.RemoveCalls != 1 {
		t.Errorf("Expected 1 worktree Remove call, got %d", wtMgr.RemoveCalls)
	}
}

func TestCompletePlan_WorkerMovesOnAfterGitFailure(t *testing.T) {
	// End-to-end test: verifies that processPlan returns nil (worker moves on)
	// even when git operations fail during completion.
	tmpDir := t.TempDir()
	wtDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	// Minimal git repo for runner context but no remote (push will fail)
	initCmd := "git init && git config user.email test@test.com && git config user.name Test && " +
		"git commit --allow-empty -m 'initial' && " +
		"git checkout -b feat/moveon-test"
	cmd := exec.Command("sh", "-c", initCmd)
	cmd.Dir = wtDir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	mockBoard := board.NewMockBoard()
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Context", nil
	}
	mockBoard.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			Stats: board.Stats{TotalTasks: 1, Done: 1},
		}, nil
	}
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		return &board.Plan{ID: id, Title: "Move On Test", FeatureBranch: "feat/moveon-test", Status: status}, nil
	}

	mockRunner := &MockBoardRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			return &runner.Result{
				TextContent: "Done\n<promise>COMPLETE</promise>",
				IsComplete:  true,
				Duration:    100 * time.Millisecond,
			}, nil
		},
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-moveon-test", wtDir)

	var completeCalled bool
	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		Board:            mockBoard,
		ProjectSlug:      "test-project",
		Config:           cfg,
		WorktreeManager:  wtMgr,
		Git:              git.NewGit(tmpDir),
		MainWorktreePath: tmpDir,
		Runner:           mockRunner,
		PromptBuilder:    prompt.NewBuilder(cfg, "", ""),
		MaxIterations:    5,
		CompletionMode:   "branch", // push will fail — no remote
		OnPlanComplete: func(info *PlanInfo, result *runner.LoopResult) {
			completeCalled = true
		},
	})

	plan := &board.Plan{ID: 90, Title: "Move On Test", FeatureBranch: "feat/moveon-test"}
	info := &PlanInfo{ID: 90, Name: "Move On Test", Branch: "feat/moveon-test"}

	// processPlan should return nil (worker moves on to next plan)
	err := w.processPlan(context.Background(), plan, info)
	if err != nil {
		t.Fatalf("processPlan() should return nil when git fails after completion, got: %v", err)
	}

	// onPlanComplete callback should have been called
	if !completeCalled {
		t.Error("Expected onPlanComplete callback to be called")
	}
}

func TestWorker_Run_NonRetryableError_SkipsRetry(t *testing.T) {
	// When RunOnce returns a non-retryable error, the worker should NOT wait
	// and retry. It should move on immediately (continue to next iteration).
	mockBoard := board.NewMockBoard()

	callCount := 0
	mockBoard.ProjectContextFunc = func(slug string) (*board.AgentContext, error) {
		callCount++
		if callCount == 1 {
			// First call: return a non-retryable error (unauthorized)
			return nil, fmt.Errorf("unauthorized: invalid API token")
		}
		// Second call: return empty queue so worker stops
		return &board.AgentContext{Plan: board.Plan{ID: 0}}, nil
	}
	mockBoard.ListPlansFunc = func(projectSlug, status string) ([]board.Plan, error) {
		return []board.Plan{}, nil
	}

	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		Board:            mockBoard,
		ProjectSlug:      "test-project",
		Config:           cfg,
		MainWorktreePath: "/tmp",
		PollInterval:     100 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := w.Run(ctx)
	// Should eventually return context deadline exceeded (after empty queue polling)
	if err != context.DeadlineExceeded {
		// Worker may return ErrInterrupted or context error; both acceptable
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Logf("Run() returned: %v (expected context timeout)", err)
		}
	}

	// The key assertion: we made at least 2 calls, meaning the worker moved on
	// from the non-retryable error without blocking forever
	if callCount < 2 {
		t.Errorf("Expected at least 2 ProjectContext calls (non-retryable error should be skipped), got %d", callCount)
	}
}

func TestWorker_Run_RetryableError_Retries(t *testing.T) {
	// When RunOnce returns a retryable error, the worker should wait and retry.
	mockBoard := board.NewMockBoard()

	callCount := 0
	mockBoard.ProjectContextFunc = func(slug string) (*board.AgentContext, error) {
		callCount++
		if callCount <= 2 {
			// First two calls: return a retryable error (connection refused)
			return nil, fmt.Errorf("connection refused")
		}
		// Third call: return empty queue
		return &board.AgentContext{Plan: board.Plan{ID: 0}}, nil
	}
	mockBoard.ListPlansFunc = func(projectSlug, status string) ([]board.Plan, error) {
		return []board.Plan{}, nil
	}

	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		Board:            mockBoard,
		ProjectSlug:      "test-project",
		Config:           cfg,
		MainWorktreePath: "/tmp",
		PollInterval:     50 * time.Millisecond, // Short poll for test speed
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	// Should have retried and eventually gotten to the empty queue
	if callCount < 3 {
		t.Errorf("Expected at least 3 ProjectContext calls (retryable errors should be retried), got %d", callCount)
	}
}

func TestWorker_Run_ConsecutiveErrors_CircuitBreaker(t *testing.T) {
	// After DefaultMaxConsecutiveErrs identical errors, the worker should
	// stop retrying and move on.
	mockBoard := board.NewMockBoard()

	callCount := 0
	mockBoard.ProjectContextFunc = func(slug string) (*board.AgentContext, error) {
		callCount++
		if callCount <= DefaultMaxConsecutiveErrs+1 {
			// Return the same retryable error repeatedly
			return nil, fmt.Errorf("connection refused: same error")
		}
		// After circuit breaker trips, return empty queue
		return &board.AgentContext{Plan: board.Plan{ID: 0}}, nil
	}
	mockBoard.ListPlansFunc = func(projectSlug, status string) ([]board.Plan, error) {
		return []board.Plan{}, nil
	}

	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		Board:            mockBoard,
		ProjectSlug:      "test-project",
		Config:           cfg,
		MainWorktreePath: "/tmp",
		PollInterval:     50 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	// Should have called at least DefaultMaxConsecutiveErrs+1 times
	// (the circuit breaker trips after the Nth identical error)
	if callCount < DefaultMaxConsecutiveErrs+1 {
		t.Errorf("Expected at least %d calls before circuit breaker, got %d", DefaultMaxConsecutiveErrs+1, callCount)
	}
}

func TestProcessPlan_BlocksPlanOnWorktreeError(t *testing.T) {
	// When worktree setup fails, processPlan should transition the plan to blocked.
	mockBoard := board.NewMockBoard()

	var blockedPlanID int
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		if status == board.PlanStatusBlocked {
			blockedPlanID = id
		}
		return &board.Plan{ID: id, Status: status}, nil
	}

	// Worktree manager that always fails
	failingWtMgr := &failingWorktreeManager{}

	cfg := config.Defaults()
	fastCfg := fastRetryConfig()
	w := NewWorker(WorkerConfig{
		Board:             mockBoard,
		ProjectSlug:       "test-project",
		Config:            cfg,
		WorktreeManager:   failingWtMgr,
		MainWorktreePath:  "/tmp",
		StatusRetryConfig: &fastCfg,
	})

	plan := &board.Plan{ID: 55, Title: "Worktree Fail", FeatureBranch: "feat/wt-fail"}
	info := &PlanInfo{ID: 55, Name: "Worktree Fail", Branch: "feat/wt-fail"}

	err := w.processPlan(context.Background(), plan, info)
	if err == nil {
		t.Fatal("processPlan() should return error when worktree setup fails")
	}

	if blockedPlanID != 55 {
		t.Errorf("Expected plan #55 to be blocked, got plan #%d", blockedPlanID)
	}
}

// failingWorktreeManager always fails to create worktrees.
type failingWorktreeManager struct{}

func (f *failingWorktreeManager) Create(name, branch string) (string, error) {
	return "", fmt.Errorf("worktree creation failed: branch not found")
}

func (f *failingWorktreeManager) Get(name string) (string, error) {
	return "", fmt.Errorf("worktree not found")
}

func (f *failingWorktreeManager) Remove(name string, deleteBranch bool) error {
	return nil
}

func (f *failingWorktreeManager) Exists(name string) bool {
	return false
}

func TestBlockPlanOnError(t *testing.T) {
	mockBoard := board.NewMockBoard()

	var blockedIDs []int
	mockBoard.UpdatePlanStatusFunc = func(id int, status string) (*board.Plan, error) {
		if status == board.PlanStatusBlocked {
			blockedIDs = append(blockedIDs, id)
		}
		return &board.Plan{ID: id, Status: status}, nil
	}

	cfg := config.Defaults()
	fastCfg := fastRetryConfig()
	w := &Worker{
		board:             mockBoard,
		config:            cfg,
		notifier:          &notify.NoopNotifier{},
		statusRetryConfig: fastCfg,
	}

	w.blockPlanOnError(42, fmt.Errorf("test error"))

	if len(blockedIDs) != 1 || blockedIDs[0] != 42 {
		t.Errorf("Expected plan #42 to be blocked, got %v", blockedIDs)
	}
}

