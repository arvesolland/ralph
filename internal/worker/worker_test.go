package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

