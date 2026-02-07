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

	// Create a test plan with checked boxes (simulates agent having updated plan)
	planContent := `# Test Plan

**Status:** complete

## Tasks

- [x] Task 1
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

	// Create a test plan directly in current/ with checked boxes (simulates agent having updated plan)
	planContent := `# Test Plan

**Status:** complete

## Tasks

- [x] Task 1
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

func TestWorker_RunOnce_RecoversPlanWithExistingPR(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// This test simulates a plan that was interrupted after PR creation but before completion
	// The worker should detect the existing PR and auto-complete the plan

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

	// Create a test plan directly in current/ (simulates interrupted completion)
	planContent := `# Test Plan

**Status:** complete

## Tasks

- [x] Task 1
`
	planPath := filepath.Join(queueDir, "current", "test-recovery.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Initial commit
	if err := g.Add("plans/current/test-recovery.md"); err != nil {
		t.Fatalf("Failed to add plan: %v", err)
	}
	if err := g.Commit("Initial commit"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create a feature branch (simulates what would happen during processing)
	if err := g.CreateBranch("feat/test-recovery"); err != nil {
		t.Fatalf("Failed to create branch: %v", err)
	}

	// Mock gh CLI to return a PR URL
	// We'll do this by creating a fake gh script in a temp bin directory
	binDir := filepath.Join(tmpDir, "bin")
	os.MkdirAll(binDir, 0755)

	ghScript := filepath.Join(binDir, "gh")
	ghContent := `#!/bin/bash
if [ "$1" = "pr" ] && [ "$2" = "list" ] && [ "$4" = "feat/test-recovery" ]; then
  echo "https://github.com/test/test/pull/123"
  exit 0
fi
exit 1
`
	if err := os.WriteFile(ghScript, []byte(ghContent), 0755); err != nil {
		t.Fatalf("Failed to create gh mock: %v", err)
	}

	// Update PATH to include our bin directory
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", binDir+":"+oldPath)
	defer os.Setenv("PATH", oldPath)

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

	// Create a mock runner (shouldn't be called since we detect existing PR)
	mockRunner := &MockRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			t.Error("Runner should not be called when PR exists")
			return nil, nil
		},
	}

	cfg := config.Defaults()
	cfg.Git.BaseBranch = "main"

	builder := prompt.NewBuilder(cfg, tmpDir, "")

	var planStartCalled bool
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
		CompletionMode:   "pr", // Enable PR mode so recovery check runs
		OnPlanStart: func(p *plan.Plan) {
			planStartCalled = true
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	// Verify plan was NOT started (recovered instead)
	if planStartCalled {
		t.Error("OnPlanStart should not be called when plan is recovered")
	}

	// Verify plan moved to complete/
	currentPlan, err = queue.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if currentPlan != nil {
		t.Error("Expected plan to be removed from current/ after recovery")
	}

	// Verify plan is in complete/
	completed, err := os.ReadDir(filepath.Join(queueDir, "complete"))
	if err != nil {
		t.Fatalf("Failed to read complete dir: %v", err)
	}
	if len(completed) == 0 {
		t.Error("Expected plan to be moved to complete/")
	}
}

func TestWorker_RunOnce_RecoversPlanFromPending(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// This test simulates a plan in pending/ that already has a PR
	// This happens with worktree isolation: plan stays in pending/ on main branch
	// even after PR is created from the worktree

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

	// Create a test plan in PENDING (not current!) - simulates worktree isolation
	planContent := `# Test Plan

**Status:** complete

## Tasks

- [x] Task 1
`
	planPath := filepath.Join(queueDir, "pending", "test-pending-recovery.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Initial commit
	if err := g.Add("plans/pending/test-pending-recovery.md"); err != nil {
		t.Fatalf("Failed to add plan: %v", err)
	}
	if err := g.Commit("Initial commit"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create a feature branch (simulates what happened in a previous worktree run)
	if err := g.CreateBranch("feat/test-pending-recovery"); err != nil {
		t.Fatalf("Failed to create branch: %v", err)
	}

	// Mock gh CLI to return a PR URL for this branch
	binDir := filepath.Join(tmpDir, "bin")
	os.MkdirAll(binDir, 0755)

	ghScript := filepath.Join(binDir, "gh")
	ghContent := `#!/bin/bash
if [ "$1" = "pr" ] && [ "$2" = "list" ] && [ "$4" = "feat/test-pending-recovery" ]; then
  echo "https://github.com/test/test/pull/456"
  exit 0
fi
exit 1
`
	if err := os.WriteFile(ghScript, []byte(ghContent), 0755); err != nil {
		t.Fatalf("Failed to create gh mock: %v", err)
	}

	// Update PATH to include our bin directory
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", binDir+":"+oldPath)
	defer os.Setenv("PATH", oldPath)

	// Create worker manager
	worktreesDir := filepath.Join(tmpDir, ".ralph", "worktrees")
	os.MkdirAll(worktreesDir, 0755)

	manager, err := worktree.NewManager(g, worktreesDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	queue := plan.NewQueue(queueDir)

	// Verify NO current plan (it's in pending)
	currentPlan, err := queue.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if currentPlan != nil {
		t.Fatal("Expected no current plan")
	}

	// Verify pending plan exists
	pending, err := queue.Pending()
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending plan, got %d", len(pending))
	}

	// Create a mock runner (shouldn't be called since we detect existing PR)
	mockRunner := &MockRunner{
		RunFunc: func(ctx context.Context, p string, opts runner.Options) (*runner.Result, error) {
			t.Error("Runner should not be called when PR already exists")
			return nil, nil
		},
	}

	cfg := config.Defaults()
	cfg.Git.BaseBranch = "main"

	builder := prompt.NewBuilder(cfg, tmpDir, "")

	var planStartCalled bool
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
		CompletionMode:   "pr",
		OnPlanStart: func(p *plan.Plan) {
			planStartCalled = true
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	// Verify plan was NOT started (recovered instead)
	if planStartCalled {
		t.Error("OnPlanStart should not be called when PR already exists")
	}

	// Verify no plan in current/ (was activated then immediately completed)
	currentPlan, err = queue.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if currentPlan != nil {
		t.Error("Expected no current plan after recovery")
	}

	// Verify plan is in complete/
	completed, err := os.ReadDir(filepath.Join(queueDir, "complete"))
	if err != nil {
		t.Fatalf("Failed to read complete dir: %v", err)
	}
	if len(completed) == 0 {
		t.Error("Expected plan to be moved to complete/")
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
	mu             sync.Mutex
	StartCalls     int
	CompleteCalls  int
	BlockerCalls   int
	ErrorCalls     int
	IterationCalls int
	ProgressCalls  int
	LastPRURL      string
	LastBlocker    *runner.Blocker
	LastError      error
	LastProgress   *notify.ProgressStatus
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

func (m *MockNotifier) UpdateProgress(p *plan.Plan, status *notify.ProgressStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ProgressCalls++
	m.LastProgress = status
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

	notifier := notify.NewNotifier(cfg, nil)

	// Should return SlackNotifier
	if _, ok := notifier.(*notify.SlackNotifier); !ok {
		t.Error("Expected SlackNotifier when bot token is configured")
	}
}

func TestNewNotifier_WithWebhook(t *testing.T) {
	cfg := config.Defaults()
	cfg.Slack.WebhookURL = "https://hooks.slack.com/services/test"

	notifier := notify.NewNotifier(cfg, nil)

	// Should return WebhookNotifier
	if _, ok := notifier.(*notify.WebhookNotifier); !ok {
		t.Error("Expected WebhookNotifier when webhook is configured")
	}
}

func TestNewNotifier_NoConfig(t *testing.T) {
	notifier := notify.NewNotifier(nil, nil)

	// Should return NoopNotifier
	if _, ok := notifier.(*notify.NoopNotifier); !ok {
		t.Error("Expected NoopNotifier when config is nil")
	}
}

func TestNewNotifier_NoSlackConfig(t *testing.T) {
	cfg := config.Defaults()
	// No Slack config set

	notifier := notify.NewNotifier(cfg, nil)

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

func TestWorker_LoadOrCreateContext_StaleContext(t *testing.T) {
	// When a worktree is reused with a different plan, the existing context.json
	// should be detected as stale and recreated.
	tmpDir := t.TempDir()

	cfg := config.Defaults()
	cfg.Git.BaseBranch = "main"

	w := &Worker{
		config:        cfg,
		maxIterations: 10,
	}

	// Create a context.json for "old-plan"
	ralphDir := filepath.Join(tmpDir, ".ralph")
	os.MkdirAll(ralphDir, 0755)
	ctxPath := filepath.Join(ralphDir, "context.json")

	oldCtx := &runner.Context{
		PlanFile:      "plans/current/old-plan/plan.md",
		PlanDir:       "plans/current/old-plan",
		FeatureBranch: "feat/old-plan",
		BaseBranch:    "main",
		Iteration:     5,
		MaxIterations: 10,
	}
	if err := runner.SaveContext(oldCtx, ctxPath); err != nil {
		t.Fatalf("Failed to save old context: %v", err)
	}

	// Create a new plan bundle for "new-plan"
	bundlePath := filepath.Join(tmpDir, "plans", "current", "new-plan")
	os.MkdirAll(bundlePath, 0755)
	planPath := filepath.Join(bundlePath, "plan.md")
	os.WriteFile(planPath, []byte("# Plan: New\n## Tasks\n- [ ] Task 1\n"), 0644)

	p, err := plan.Load(bundlePath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	// loadOrCreateContext should detect stale context and create a fresh one
	execCtx, err := w.loadOrCreateContext(p, tmpDir)
	if err != nil {
		t.Fatalf("loadOrCreateContext() error = %v", err)
	}

	// Should be fresh context (iteration 1, matching new-plan)
	if execCtx.Iteration != 1 {
		t.Errorf("Iteration = %d, want 1 (fresh context)", execCtx.Iteration)
	}
	if filepath.Base(execCtx.PlanDir) != "new-plan" {
		t.Errorf("PlanDir base = %q, want %q", filepath.Base(execCtx.PlanDir), "new-plan")
	}
}

func TestWorker_LoadOrCreateContext_MatchingContext(t *testing.T) {
	// When context.json matches the current plan, it should be reused.
	tmpDir := t.TempDir()

	cfg := config.Defaults()
	cfg.Git.BaseBranch = "main"

	w := &Worker{
		config:        cfg,
		maxIterations: 10,
	}

	// Create a context.json for "my-plan" at iteration 3
	ralphDir := filepath.Join(tmpDir, ".ralph")
	os.MkdirAll(ralphDir, 0755)
	ctxPath := filepath.Join(ralphDir, "context.json")

	existingCtx := &runner.Context{
		PlanFile:      "plans/current/my-plan/plan.md",
		PlanDir:       "plans/current/my-plan",
		FeatureBranch: "feat/my-plan",
		BaseBranch:    "main",
		Iteration:     3,
		MaxIterations: 10,
	}
	if err := runner.SaveContext(existingCtx, ctxPath); err != nil {
		t.Fatalf("Failed to save context: %v", err)
	}

	// Create a plan bundle with the same name
	bundlePath := filepath.Join(tmpDir, "plans", "current", "my-plan")
	os.MkdirAll(bundlePath, 0755)
	planPath := filepath.Join(bundlePath, "plan.md")
	os.WriteFile(planPath, []byte("# Plan: My\n## Tasks\n- [ ] Task 1\n"), 0644)

	p, err := plan.Load(bundlePath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	// loadOrCreateContext should reuse the existing context
	execCtx, err := w.loadOrCreateContext(p, tmpDir)
	if err != nil {
		t.Fatalf("loadOrCreateContext() error = %v", err)
	}

	// Should reuse existing context (iteration 3)
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

	// Verify notifier was created
	if w.notifier == nil {
		t.Error("Expected notifier to be created")
	}

	// Should be WebhookNotifier since we configured webhook
	if _, ok := w.notifier.(*notify.WebhookNotifier); !ok {
		t.Error("Expected WebhookNotifier")
	}
}
