package worker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/notify"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/arvesolland/ralph/internal/worktree"
)

// MockRunner implements runner.Runner for testing.
type MockRunner struct {
	RunFunc func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error)
	calls   int
}

func (m *MockRunner) Run(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
	m.calls++
	if m.RunFunc != nil {
		return m.RunFunc(ctx, p, opts)
	}
	// Default: return a complete result after a few calls
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
		Queue:            plan.NewQueue("/tmp"),
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
		Queue:            plan.NewQueue("/tmp"),
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

func TestWorker_RunOnce_QueueEmpty(t *testing.T) {
	// Create temp directory for queue
	tmpDir := t.TempDir()
	queueDir := filepath.Join(tmpDir, "plans")
	os.MkdirAll(filepath.Join(queueDir, "pending"), 0755)
	os.MkdirAll(filepath.Join(queueDir, "current"), 0755)
	os.MkdirAll(filepath.Join(queueDir, "complete"), 0755)

	queue := plan.NewQueue(queueDir)

	w := NewWorker(WorkerConfig{
		Queue:            queue,
		Config:           config.Defaults(),
		MainWorktreePath: tmpDir,
	})

	ctx := context.Background()
	err := w.RunOnce(ctx)

	if err != ErrQueueEmpty {
		t.Errorf("RunOnce() error = %v, want %v", err, ErrQueueEmpty)
	}
}

func TestWorker_RunOnce_ActivatesPlan(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create temp directory with git repo
	tmpDir := t.TempDir()

	// Initialize git repo
	g := git.NewGit(tmpDir)
	if err := runGitInit(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create queue structure
	queueDir := filepath.Join(tmpDir, "plans")
	os.MkdirAll(filepath.Join(queueDir, "pending"), 0755)
	os.MkdirAll(filepath.Join(queueDir, "current"), 0755)
	os.MkdirAll(filepath.Join(queueDir, "complete"), 0755)

	// Create a test plan
	planContent := `# Test Plan

**Status:** pending

## Tasks

- [ ] Task 1
`
	planPath := filepath.Join(queueDir, "pending", "test-plan.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Initial commit
	if err := g.Add("plans/pending/test-plan.md"); err != nil {
		t.Fatalf("Failed to add plan: %v", err)
	}
	if err := g.Commit("Initial commit"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create worker manager
	worktreesDir := filepath.Join(tmpDir, ".ralph", "worktrees")
	os.MkdirAll(worktreesDir, 0755)

	manager, err := worktree.NewManager(g, worktreesDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	queue := plan.NewQueue(queueDir)

	// Create a mock runner that immediately completes
	mockRunner := &MockRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			// Check if this is a verification call (uses Print mode)
			if opts.Print {
				return &runner.Result{
					Output:      "YES",
					TextContent: "YES",
					Duration:    time.Second,
					Attempts:    1,
				}, nil
			}
			return &runner.Result{
				Output:      "Done",
				TextContent: "Done\n<promise>COMPLETE</promise>",
				Duration:    time.Second,
				IsComplete:  true,
			}, nil
		},
	}

	cfg := config.Defaults()
	cfg.Git.BaseBranch = "main"

	builder := prompt.NewBuilder(cfg, tmpDir, "")

	// Track callbacks
	var planStarted, planCompleted bool

	w := NewWorker(WorkerConfig{
		Queue:            queue,
		Config:           cfg,
		ConfigDir:        filepath.Join(tmpDir, ".ralph"),
		WorktreeManager:  manager,
		Git:              g,
		MainWorktreePath: tmpDir,
		Runner:           mockRunner,
		PromptBuilder:    builder,
		MaxIterations:    3,
		OnPlanStart: func(p *plan.Plan) {
			planStarted = true
		},
		OnPlanComplete: func(p *plan.Plan, result *runner.LoopResult) {
			planCompleted = true
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	// Verify plan was moved from pending to complete
	pending, _ := queue.Pending()
	if len(pending) != 0 {
		t.Errorf("Pending count = %d, want 0", len(pending))
	}

	// Check callbacks were called
	if !planStarted {
		t.Error("OnPlanStart was not called")
	}
	if !planCompleted {
		t.Error("OnPlanComplete was not called")
	}
}

func TestWorker_Run_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	queueDir := filepath.Join(tmpDir, "plans")
	os.MkdirAll(filepath.Join(queueDir, "pending"), 0755)
	os.MkdirAll(filepath.Join(queueDir, "current"), 0755)

	queue := plan.NewQueue(queueDir)

	w := NewWorker(WorkerConfig{
		Queue:            queue,
		Config:           config.Defaults(),
		MainWorktreePath: tmpDir,
		PollInterval:     100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	err := w.Run(ctx)

	if err != context.Canceled {
		t.Errorf("Run() error = %v, want %v", err, context.Canceled)
	}
}

func TestWorker_RunOnce_ResumesCurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create temp directory with git repo
	tmpDir := t.TempDir()

	// Initialize git repo
	g := git.NewGit(tmpDir)
	if err := runGitInit(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create queue structure
	queueDir := filepath.Join(tmpDir, "plans")
	os.MkdirAll(filepath.Join(queueDir, "pending"), 0755)
	os.MkdirAll(filepath.Join(queueDir, "current"), 0755)
	os.MkdirAll(filepath.Join(queueDir, "complete"), 0755)

	// Create a test plan directly in current/
	planContent := `# Test Plan

**Status:** pending

## Tasks

- [ ] Task 1
`
	planPath := filepath.Join(queueDir, "current", "test-plan.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Initial commit
	if err := g.Add("plans/current/test-plan.md"); err != nil {
		t.Fatalf("Failed to add plan: %v", err)
	}
	if err := g.Commit("Initial commit"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create worker manager
	worktreesDir := filepath.Join(tmpDir, ".ralph", "worktrees")
	os.MkdirAll(worktreesDir, 0755)

	manager, err := worktree.NewManager(g, worktreesDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	queue := plan.NewQueue(queueDir)

	// Verify current plan exists
	currentPlan, err := queue.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if currentPlan == nil {
		t.Fatal("Expected current plan to exist")
	}

	// Create a mock runner that immediately completes
	mockRunner := &MockRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			// Check if this is a verification call (uses Print mode)
			if opts.Print {
				return &runner.Result{
					Output:      "YES",
					TextContent: "YES",
					Duration:    time.Second,
					Attempts:    1,
				}, nil
			}
			return &runner.Result{
				Output:      "Done",
				TextContent: "Done\n<promise>COMPLETE</promise>",
				Duration:    time.Second,
				IsComplete:  true,
			}, nil
		},
	}

	cfg := config.Defaults()
	cfg.Git.BaseBranch = "main"

	builder := prompt.NewBuilder(cfg, tmpDir, "")

	var resumedPlan string
	w := NewWorker(WorkerConfig{
		Queue:            queue,
		Config:           cfg,
		ConfigDir:        filepath.Join(tmpDir, ".ralph"),
		WorktreeManager:  manager,
		Git:              g,
		MainWorktreePath: tmpDir,
		Runner:           mockRunner,
		PromptBuilder:    builder,
		MaxIterations:    3,
		OnPlanStart: func(p *plan.Plan) {
			resumedPlan = p.Name
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if resumedPlan != "test-plan" {
		t.Errorf("Resumed plan = %q, want %q", resumedPlan, "test-plan")
	}
}

func TestConstants(t *testing.T) {
	if DefaultPollInterval != 30*time.Second {
		t.Errorf("DefaultPollInterval = %v, want %v", DefaultPollInterval, 30*time.Second)
	}

	if DefaultMaxIterations != 30 {
		t.Errorf("DefaultMaxIterations = %d, want %d", DefaultMaxIterations, 30)
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

// Helper function to initialize a git repository.
func runGitInit(dir string) error {
	g := git.NewGit(dir)

	// Create initial file
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0644); err != nil {
		return err
	}

	// Git init
	cmd := gitCommand(dir, "init", "-b", "main")
	if err := cmd.Run(); err != nil {
		return err
	}

	// Configure user for commits
	cmd = gitCommand(dir, "config", "user.email", "test@test.com")
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = gitCommand(dir, "config", "user.name", "Test User")
	if err := cmd.Run(); err != nil {
		return err
	}

	// Initial commit
	if err := g.Add("README.md"); err != nil {
		return err
	}
	return g.Commit("Initial commit")
}

func gitCommand(dir string, args ...string) *execCommand {
	return &execCommand{
		dir:  dir,
		args: args,
	}
}

type execCommand struct {
	dir  string
	args []string
}

func (c *execCommand) Run() error {
	cmd := exec.Command("git", c.args...)
	cmd.Dir = c.dir
	return cmd.Run()
}

// MockNotifier implements notify.Notifier for testing.
type MockNotifier struct {
	mu           sync.Mutex
	StartCalls   int
	CompleteCalls int
	BlockerCalls int
	ErrorCalls   int
	IterationCalls int
	LastPRURL    string
	LastBlocker  *runner.Blocker
	LastError    error
}

func (m *MockNotifier) Start(p *plan.Plan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StartCalls++
	return nil
}

func (m *MockNotifier) Complete(p *plan.Plan, prURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CompleteCalls++
	m.LastPRURL = prURL
	return nil
}

func (m *MockNotifier) Blocker(p *plan.Plan, blocker *runner.Blocker) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlockerCalls++
	m.LastBlocker = blocker
	return nil
}

func (m *MockNotifier) Error(p *plan.Plan, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrorCalls++
	m.LastError = err
	return nil
}

func (m *MockNotifier) Iteration(p *plan.Plan, iteration, maxIterations int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.IterationCalls++
	return nil
}

func TestNewWorker_WithNotifier(t *testing.T) {
	mockNotifier := &MockNotifier{}

	cfg := WorkerConfig{
		Queue:            plan.NewQueue("/tmp"),
		Config:           config.Defaults(),
		MainWorktreePath: "/tmp",
		Notifier:         mockNotifier,
	}

	w := NewWorker(cfg)

	// Verify notifier is set
	if w.notifier != mockNotifier {
		t.Error("Expected notifier to be set")
	}
}

func TestNewWorker_DefaultNotifier(t *testing.T) {
	cfg := WorkerConfig{
		Queue:            plan.NewQueue("/tmp"),
		Config:           config.Defaults(),
		MainWorktreePath: "/tmp",
	}

	w := NewWorker(cfg)

	// Verify notifier is NoopNotifier when not provided
	if _, ok := w.notifier.(*notify.NoopNotifier); !ok {
		t.Error("Expected notifier to be NoopNotifier when not provided")
	}
}

func TestNewNotifier_WithBotToken(t *testing.T) {
	cfg := config.Defaults()
	cfg.Slack.BotToken = "xoxb-test-token"
	cfg.Slack.Channel = "C12345"

	notifier := NewNotifier(cfg, nil)

	// Should return SlackNotifier
	if _, ok := notifier.(*notify.SlackNotifier); !ok {
		t.Error("Expected SlackNotifier when bot token is configured")
	}
}

func TestNewNotifier_WithWebhook(t *testing.T) {
	cfg := config.Defaults()
	cfg.Slack.WebhookURL = "https://hooks.slack.com/services/test"

	notifier := NewNotifier(cfg, nil)

	// Should return WebhookNotifier
	if _, ok := notifier.(*notify.WebhookNotifier); !ok {
		t.Error("Expected WebhookNotifier when webhook is configured")
	}
}

func TestNewNotifier_NoConfig(t *testing.T) {
	notifier := NewNotifier(nil, nil)

	// Should return NoopNotifier
	if _, ok := notifier.(*notify.NoopNotifier); !ok {
		t.Error("Expected NoopNotifier when config is nil")
	}
}

func TestNewNotifier_NoSlackConfig(t *testing.T) {
	cfg := config.Defaults()
	// No Slack config set

	notifier := NewNotifier(cfg, nil)

	// Should return NoopNotifier
	if _, ok := notifier.(*notify.NoopNotifier); !ok {
		t.Error("Expected NoopNotifier when no Slack is configured")
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

	testPlan := &plan.Plan{Name: "test", Branch: "feat/test"}

	// Test sendStartNotification
	w.sendStartNotification(testPlan)
	if mockNotifier.StartCalls != 1 {
		t.Errorf("StartCalls = %d, want 1", mockNotifier.StartCalls)
	}

	// Test sendCompleteNotification
	w.sendCompleteNotification(testPlan, "https://github.com/test/pr/1")
	if mockNotifier.CompleteCalls != 1 {
		t.Errorf("CompleteCalls = %d, want 1", mockNotifier.CompleteCalls)
	}
	if mockNotifier.LastPRURL != "https://github.com/test/pr/1" {
		t.Errorf("LastPRURL = %q, want %q", mockNotifier.LastPRURL, "https://github.com/test/pr/1")
	}

	// Test sendBlockerNotification
	blocker := &runner.Blocker{Description: "Test blocker"}
	w.sendBlockerNotification(testPlan, blocker)
	if mockNotifier.BlockerCalls != 1 {
		t.Errorf("BlockerCalls = %d, want 1", mockNotifier.BlockerCalls)
	}
	if mockNotifier.LastBlocker != blocker {
		t.Error("LastBlocker not set correctly")
	}

	// Test notifyError
	testErr := ErrGHNotInstalled
	w.notifyError(testPlan, testErr)
	if mockNotifier.ErrorCalls != 1 {
		t.Errorf("ErrorCalls = %d, want 1", mockNotifier.ErrorCalls)
	}

	// Test sendIterationNotification
	w.sendIterationNotification(testPlan, 5, 10)
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

	testPlan := &plan.Plan{Name: "test", Branch: "feat/test"}

	// All notifications should be skipped when disabled
	w.sendStartNotification(testPlan)
	w.sendCompleteNotification(testPlan, "")
	w.sendBlockerNotification(testPlan, &runner.Blocker{})
	w.notifyError(testPlan, ErrGHNotInstalled)
	w.sendIterationNotification(testPlan, 1, 10)

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
		config:   nil, // nil config
		notifier: mockNotifier,
	}

	testPlan := &plan.Plan{Name: "test", Branch: "feat/test"}

	// Should not panic with nil config
	w.sendStartNotification(testPlan)
	w.sendCompleteNotification(testPlan, "")
	w.sendBlockerNotification(testPlan, &runner.Blocker{})
	w.notifyError(testPlan, ErrGHNotInstalled)
	w.sendIterationNotification(testPlan, 1, 10)

	// No calls should be made
	if mockNotifier.StartCalls != 0 || mockNotifier.CompleteCalls != 0 ||
		mockNotifier.BlockerCalls != 0 || mockNotifier.ErrorCalls != 0 ||
		mockNotifier.IterationCalls != 0 {
		t.Error("Expected no notification calls with nil config")
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

	// Verify notifier was created
	if w.notifier == nil {
		t.Error("Expected notifier to be created")
	}

	// Should be WebhookNotifier since we configured webhook
	if _, ok := w.notifier.(*notify.WebhookNotifier); !ok {
		t.Error("Expected WebhookNotifier")
	}
}
