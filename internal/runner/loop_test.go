package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/prompt"
)

// MockRunner implements Runner for testing.
type MockRunner struct {
	Responses     []MockResponse
	responseIndex int
	RecordedOpts  []Options
}

type MockResponse struct {
	Output      string
	TextContent string
	IsComplete  bool
	Blocker     *Blocker
	Error       error
}

func (m *MockRunner) Run(ctx context.Context, prompt string, opts Options) (*Result, error) {
	m.RecordedOpts = append(m.RecordedOpts, opts)

	if m.responseIndex >= len(m.Responses) {
		return &Result{}, nil
	}

	resp := m.Responses[m.responseIndex]
	m.responseIndex++

	if resp.Error != nil {
		return nil, resp.Error
	}

	return &Result{
		Output:      resp.Output,
		TextContent: resp.TextContent,
		IsComplete:  resp.IsComplete,
		Blocker:     resp.Blocker,
		Duration:    100 * time.Millisecond,
	}, nil
}

func TestIterationLoop_Run_MaxIterations(t *testing.T) {
	// Set up temp directories
	tempDir := t.TempDir()
	planDir := filepath.Join(tempDir, "plans", "current")
	os.MkdirAll(planDir, 0755)

	// Create a simple test plan
	planPath := filepath.Join(planDir, "test-plan.md")
	planContent := `# Plan: Test
**Status:** open
## Tasks
- [ ] Task 1
- [ ] Task 2
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	// Initialize git repo
	gitRepo := setupTestGitRepo(t, tempDir)

	// Load the plan
	p, err := plan.Load(planPath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	// Create context with max 2 iterations
	ctx := NewContext(p, "main", 2)

	// Mock runner that never completes
	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Working on task 1..."},
			{TextContent: "Working on task 2..."},
			{TextContent: "Still working..."}, // This won't be reached
		},
	}

	// Create loop
	loop := NewIterationLoop(LoopConfig{
		Plan:             p,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	// Run loop
	result := loop.Run(context.Background())

	// Should reach max iterations
	if result.Completed {
		t.Error("Expected loop to not complete")
	}
	if result.Error == nil {
		t.Error("Expected max iterations error")
	}
	if !strings.Contains(result.Error.Error(), "max iterations") {
		t.Errorf("Expected max iterations error, got: %v", result.Error)
	}
	if result.Iterations != 2 {
		t.Errorf("Expected 2 iterations, got %d", result.Iterations)
	}
}

func TestIterationLoop_Run_CompletesSuccessfully(t *testing.T) {
	tempDir := t.TempDir()
	planDir := filepath.Join(tempDir, "plans", "current")
	os.MkdirAll(planDir, 0755)

	planPath := filepath.Join(planDir, "test-plan.md")
	planContent := `# Plan: Test
**Status:** open
## Tasks
- [ ] Task 1
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	gitRepo := setupTestGitRepo(t, tempDir)

	p, err := plan.Load(planPath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	ctx := NewContext(p, "main", 10)

	// Mock runner that completes on iteration 3
	// First two iterations: normal work
	// Third iteration: completion marker
	// Verification: YES
	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Working on task 1..."},
			{TextContent: "Almost done..."},
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
			{TextContent: "YES", IsComplete: false}, // Verification response
		},
	}

	loop := NewIterationLoop(LoopConfig{
		Plan:             p,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	result := loop.Run(context.Background())

	if !result.Completed {
		t.Errorf("Expected loop to complete, error: %v", result.Error)
	}
	if result.Iterations != 3 {
		t.Errorf("Expected 3 iterations, got %d", result.Iterations)
	}
}

func TestIterationLoop_Run_HandlesBlocker(t *testing.T) {
	tempDir := t.TempDir()
	planDir := filepath.Join(tempDir, "plans", "current")
	os.MkdirAll(planDir, 0755)

	planPath := filepath.Join(planDir, "test-plan.md")
	planContent := `# Plan: Test
**Status:** open
## Tasks
- [ ] Task 1
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	gitRepo := setupTestGitRepo(t, tempDir)

	p, err := plan.Load(planPath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	ctx := NewContext(p, "main", 2)

	blocker := &Blocker{
		Description: "Need API key",
		Action:      "Provide API key in config",
		Resume:      "Will continue after key is set",
		Hash:        "abc12345",
	}

	var blockerCallbackCalled bool
	var receivedBlocker *Blocker

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Found a blocker", Blocker: blocker},
			{TextContent: "Still blocked"},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		Plan:             p,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
		OnBlocker: func(b *Blocker) {
			blockerCallbackCalled = true
			receivedBlocker = b
		},
	})

	result := loop.Run(context.Background())

	// Should continue after blocker but eventually hit max iterations
	if blockerCallbackCalled == false {
		t.Error("Expected blocker callback to be called")
	}
	if receivedBlocker == nil || receivedBlocker.Hash != "abc12345" {
		t.Error("Expected correct blocker to be passed to callback")
	}
	if result.FinalBlocker == nil {
		t.Error("Expected final blocker to be set")
	}
}

func TestIterationLoop_Run_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	planDir := filepath.Join(tempDir, "plans", "current")
	os.MkdirAll(planDir, 0755)

	planPath := filepath.Join(planDir, "test-plan.md")
	planContent := `# Plan: Test
**Status:** open
## Tasks
- [ ] Task 1
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	gitRepo := setupTestGitRepo(t, tempDir)

	p, err := plan.Load(planPath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	ctx := NewContext(p, "main", 100)

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Working..."},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		Plan:             p,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	// Create a context that cancels quickly
	cancelCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := loop.Run(cancelCtx)

	if result.Error != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", result.Error)
	}
}

func TestIterationLoop_Run_OnIterationCallback(t *testing.T) {
	tempDir := t.TempDir()
	planDir := filepath.Join(tempDir, "plans", "current")
	os.MkdirAll(planDir, 0755)

	planPath := filepath.Join(planDir, "test-plan.md")
	planContent := `# Plan: Test
**Status:** open
## Tasks
- [ ] Task 1
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	gitRepo := setupTestGitRepo(t, tempDir)

	p, err := plan.Load(planPath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	ctx := NewContext(p, "main", 3)

	var iterations []int
	var results []*Result

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Iteration 1"},
			{TextContent: "Iteration 2"},
			{TextContent: "Iteration 3"},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		Plan:             p,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
		OnIteration: func(iteration int, result *Result) {
			iterations = append(iterations, iteration)
			results = append(results, result)
		},
	})

	loop.Run(context.Background())

	if len(iterations) != 3 {
		t.Errorf("Expected 3 iteration callbacks, got %d", len(iterations))
	}
	for i, iter := range iterations {
		if iter != i+1 {
			t.Errorf("Expected iteration %d at index %d, got %d", i+1, i, iter)
		}
	}
}

func TestIterationLoop_Run_VerificationFails(t *testing.T) {
	tempDir := t.TempDir()
	planDir := filepath.Join(tempDir, "plans", "current")
	os.MkdirAll(planDir, 0755)

	planPath := filepath.Join(planDir, "test-plan.md")
	planContent := `# Plan: Test
**Status:** open
## Tasks
- [ ] Task 1
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	gitRepo := setupTestGitRepo(t, tempDir)

	p, err := plan.Load(planPath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	ctx := NewContext(p, "main", 3)

	// Mock runner: first iteration claims complete, verification fails, continues
	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
			{TextContent: "NO: Task 1 is still unchecked"}, // Verification response
			{TextContent: "Working more..."},
			{TextContent: "Still working..."},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		Plan:             p,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	result := loop.Run(context.Background())

	// Should NOT complete since verification failed
	if result.Completed {
		t.Error("Expected loop to not complete after verification failure")
	}
	// Should hit max iterations
	if result.Error == nil || !strings.Contains(result.Error.Error(), "max iterations") {
		t.Errorf("Expected max iterations error, got: %v", result.Error)
	}

	// Check that feedback file was written
	feedbackPath := plan.FeedbackPath(p)
	content, err := os.ReadFile(feedbackPath)
	if err == nil && !strings.Contains(string(content), "Task 1 is still unchecked") {
		t.Log("Feedback file content:", string(content))
	}
}

func TestNewIterationLoop_DefaultTimeout(t *testing.T) {
	loop := NewIterationLoop(LoopConfig{})

	if loop.iterationTimeout != IterationTimeout {
		t.Errorf("Expected default timeout %v, got %v", IterationTimeout, loop.iterationTimeout)
	}
}

func TestNewIterationLoop_CustomTimeout(t *testing.T) {
	customTimeout := 5 * time.Minute
	loop := NewIterationLoop(LoopConfig{
		IterationTimeout: customTimeout,
	})

	if loop.iterationTimeout != customTimeout {
		t.Errorf("Expected custom timeout %v, got %v", customTimeout, loop.iterationTimeout)
	}
}

// setupTestGitRepo creates a git repo for testing.
func setupTestGitRepo(t *testing.T, dir string) git.Git {
	t.Helper()

	gitRepo := git.NewGit(dir)

	// Initialize git repo
	cmd := "git init && git config user.email test@test.com && git config user.name Test && git commit --allow-empty -m 'initial'"
	if err := runShellCommand(dir, cmd); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	return gitRepo
}

// runShellCommand runs a shell command in the given directory.
func runShellCommand(dir, cmd string) error {
	c := exec.Command("sh", "-c", cmd)
	c.Dir = dir
	return c.Run()
}
