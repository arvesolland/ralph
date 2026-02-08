package worker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/atm"
	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/notify"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/runner"
)

// MockATMRunner implements runner.Runner for testing.
type MockATMRunner struct {
	RunFunc func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error)
	calls   int
}

func (m *MockATMRunner) Run(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
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

	w.sendStartNotification(testInfo)
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

	w.sendStartNotification(testInfo)
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

	// Should not panic with nil config
	w.sendStartNotification(testInfo)
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
	os.MkdirAll(ralphDir, 0755)
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
	plan := &atm.Plan{ID: 200, FeatureBranch: "feat/new-plan"}

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
	os.MkdirAll(ralphDir, 0755)
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
	plan := &atm.Plan{ID: 42, FeatureBranch: "feat/my-plan"}

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
	os.MkdirAll(configDir, 0755)

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

// MockWorktreeManager implements WorktreeManager for testing.
type MockWorktreeManager struct {
	worktrees map[string]string // name -> path
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
	os.MkdirAll(bareDir, 0755)
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
	os.MkdirAll(wtDir, 0755)
	setupWorkerTestGitRepo(t, wtDir, "feat/test")

	mockATM := atm.NewMockATM()

	// No active plan in project context
	mockATM.ProjectContextFunc = func(slug string) (*atm.AgentContext, error) {
		return &atm.AgentContext{
			Plan: atm.Plan{ID: 0}, // No active plan
		}, nil
	}

	// One ready plan
	mockATM.ListPlansFunc = func(projectSlug, status string) ([]atm.Plan, error) {
		if status == atm.PlanStatusReady {
			return []atm.Plan{
				{ID: 10, Title: "Test Plan", FeatureBranch: "feat/test", Status: atm.PlanStatusReady},
			}, nil
		}
		return nil, nil
	}

	// Track UpdatePlanStatus calls
	var statusUpdates []struct{ ID int; Status string }
	mockATM.UpdatePlanStatusFunc = func(id int, status string) (*atm.Plan, error) {
		statusUpdates = append(statusUpdates, struct{ ID int; Status string }{id, status})
		return &atm.Plan{ID: id, Title: "Test Plan", FeatureBranch: "feat/test", Status: status}, nil
	}
	mockATM.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Context", nil
	}

	// Runner completes immediately
	mockRunner := &MockATMRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			return &runner.Result{
				TextContent: "Done\n<promise>COMPLETE</promise>",
				IsComplete:  true,
				Duration:    100 * time.Millisecond,
			}, nil
		},
	}

	// ATM says all tasks done for completion check
	mockATM.PlanContextFunc = func(planID int) (*atm.AgentContext, error) {
		return &atm.AgentContext{
			Stats: atm.Stats{TotalTasks: 1, Done: 1},
		}, nil
	}

	wtMgr := newMockWorktreeManager()
	wtMgr.Register("feat-test", wtDir)

	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		ATM:              mockATM,
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
		if u.ID == 10 && u.Status == atm.PlanStatusActive {
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
	os.MkdirAll(wtDir, 0755)
	setupWorkerTestGitRepo(t, wtDir, "feat/active")

	mockATM := atm.NewMockATM()

	// Active plan in project context
	mockATM.ProjectContextFunc = func(slug string) (*atm.AgentContext, error) {
		return &atm.AgentContext{
			Plan: atm.Plan{ID: 20, Title: "Active Plan", FeatureBranch: "feat/active", Status: atm.PlanStatusActive},
		}, nil
	}

	// ListPlans should NOT be called
	listPlansCalled := false
	mockATM.ListPlansFunc = func(projectSlug, status string) ([]atm.Plan, error) {
		listPlansCalled = true
		return nil, nil
	}

	var statusUpdates []string
	mockATM.UpdatePlanStatusFunc = func(id int, status string) (*atm.Plan, error) {
		statusUpdates = append(statusUpdates, status)
		return &atm.Plan{ID: id, Title: "Active Plan", FeatureBranch: "feat/active", Status: status}, nil
	}
	mockATM.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Context", nil
	}
	mockATM.PlanContextFunc = func(planID int) (*atm.AgentContext, error) {
		return &atm.AgentContext{
			Stats: atm.Stats{TotalTasks: 1, Done: 1},
		}, nil
	}

	mockRunner := &MockATMRunner{
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
		ATM:              mockATM,
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
	mockATM := atm.NewMockATM()

	mockATM.ProjectContextFunc = func(slug string) (*atm.AgentContext, error) {
		return &atm.AgentContext{
			Plan: atm.Plan{ID: 0},
		}, nil
	}
	mockATM.ListPlansFunc = func(projectSlug, status string) ([]atm.Plan, error) {
		return []atm.Plan{}, nil
	}

	cfg := config.Defaults()
	w := NewWorker(WorkerConfig{
		ATM:              mockATM,
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
	os.MkdirAll(wtDir, 0755)
	setupWorkerTestGitRepo(t, wtDir, "feat/complete")

	mockATM := atm.NewMockATM()

	mockATM.ProjectContextFunc = func(slug string) (*atm.AgentContext, error) {
		return &atm.AgentContext{Plan: atm.Plan{ID: 0}}, nil
	}
	mockATM.ListPlansFunc = func(projectSlug, status string) ([]atm.Plan, error) {
		if status == atm.PlanStatusReady {
			return []atm.Plan{
				{ID: 30, Title: "Completion Test", FeatureBranch: "feat/complete", Status: atm.PlanStatusReady},
			}, nil
		}
		return nil, nil
	}

	// Track all status transitions
	var transitions []struct{ ID int; Status string }
	mockATM.UpdatePlanStatusFunc = func(id int, status string) (*atm.Plan, error) {
		transitions = append(transitions, struct{ ID int; Status string }{id, status})
		return &atm.Plan{ID: id, Title: "Completion Test", FeatureBranch: "feat/complete", Status: status}, nil
	}
	mockATM.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Context", nil
	}
	mockATM.PlanContextFunc = func(planID int) (*atm.AgentContext, error) {
		return &atm.AgentContext{
			Stats: atm.Stats{TotalTasks: 1, Done: 1},
		}, nil
	}

	mockRunner := &MockATMRunner{
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
		ATM:              mockATM,
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

	if transitions[0].Status != atm.PlanStatusActive {
		t.Errorf("Expected first transition to 'active', got %q", transitions[0].Status)
	}

	// Last transition should be complete
	last := transitions[len(transitions)-1]
	if last.Status != atm.PlanStatusComplete {
		t.Errorf("Expected last transition to 'complete', got %q", last.Status)
	}
}

