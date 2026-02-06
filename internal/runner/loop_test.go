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
	"github.com/arvesolland/ralph/internal/state"
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

	// Create context with max 2 iterations (paths relative to tempDir)
	ctx := NewContext(p, "main", 2, tempDir)

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
	// Plan has checked boxes - simulates agent having updated the plan before claiming completion
	planContent := `# Plan: Test
**Status:** complete
## Tasks
- [x] Task 1
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	gitRepo := setupTestGitRepo(t, tempDir)

	p, err := plan.Load(planPath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	ctx := NewContext(p, "main", 10, tempDir)

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

	ctx := NewContext(p, "main", 2, tempDir)

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

	ctx := NewContext(p, "main", 100, tempDir)

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

	ctx := NewContext(p, "main", 3, tempDir)

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

	ctx := NewContext(p, "main", 3, tempDir)

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

func TestIterationLoop_CriteriaGatedCompletion(t *testing.T) {
	// Set up a plan bundle with state.yaml where all tasks are done.
	// Completion should succeed via criteria gate WITHOUT LLM verification.
	tempDir := t.TempDir()
	bundlePath := filepath.Join(tempDir, "plans", "current", "test-plan")
	os.MkdirAll(bundlePath, 0755)

	planPath := filepath.Join(bundlePath, "plan.md")
	planContent := `# Plan: Test
**Status:** active
## Tasks
### T1: Do something
**Status:** complete
**Done when:**
- [x] Something is done
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	// Create state.yaml with all tasks done
	now := time.Now()
	st := &state.PlanState{
		ID:     "test-plan",
		Title:  "Test",
		Status: state.PlanStatusActive,
		Tasks: []state.TaskState{
			{
				ID:     "T1",
				Title:  "Do something",
				Status: state.TaskStatusDone,
				Criteria: []state.Criterion{
					{Text: "Something is done", Done: true, DoneAt: &now},
				},
			},
		},
	}
	if err := state.SaveState(st, bundlePath); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	gitRepo := setupTestGitRepo(t, tempDir)

	p, err := plan.Load(bundlePath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	ctx := NewContext(p, "main", 10, tempDir)

	// Mock runner: only one call needed (the iteration that claims complete).
	// NO verification call should happen (criteria gate handles it).
	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
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
		t.Errorf("Expected criteria-gated completion to succeed, error: %v", result.Error)
	}
	if result.Iterations != 1 {
		t.Errorf("Expected 1 iteration, got %d", result.Iterations)
	}
	// Only 1 runner call (the iteration itself) — no verification LLM call
	if len(mockRunner.RecordedOpts) != 1 {
		t.Errorf("Expected 1 runner call (no LLM verification), got %d", len(mockRunner.RecordedOpts))
	}
}

func TestIterationLoop_CriteriaGatedCompletion_NotAllDone(t *testing.T) {
	// Set up a plan bundle with state.yaml where NOT all tasks are done.
	// Agent claims complete, state.yaml incomplete, LLM fallback also says NO.
	tempDir := t.TempDir()
	bundlePath := filepath.Join(tempDir, "plans", "current", "test-plan")
	os.MkdirAll(bundlePath, 0755)

	planPath := filepath.Join(bundlePath, "plan.md")
	planContent := `# Plan: Test
**Status:** active
## Tasks
### T1: Do something
**Status:** complete
**Done when:**
- [x] First thing
### T2: Do another thing
**Status:** open
**Done when:**
- [ ] Second thing
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	// Create state.yaml with T1 done, T2 still todo
	now := time.Now()
	st := &state.PlanState{
		ID:     "test-plan",
		Title:  "Test",
		Status: state.PlanStatusActive,
		Tasks: []state.TaskState{
			{
				ID:     "T1",
				Title:  "Do something",
				Status: state.TaskStatusDone,
				Criteria: []state.Criterion{
					{Text: "First thing", Done: true, DoneAt: &now},
				},
			},
			{
				ID:     "T2",
				Title:  "Do another thing",
				Status: state.TaskStatusTodo,
				Criteria: []state.Criterion{
					{Text: "Second thing", Done: false},
				},
			},
		},
	}
	if err := state.SaveState(st, bundlePath); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	gitRepo := setupTestGitRepo(t, tempDir)

	p, err := plan.Load(bundlePath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	ctx := NewContext(p, "main", 2, tempDir)

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
			// LLM verification: Verify() does programmatic checkbox check first.
			// Plan has unchecked boxes so checkCheckboxes() short-circuits before LLM call.
			// The NO response is returned by the programmatic check, not a runner call.
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

	// Should NOT complete — both state.yaml and LLM verification say incomplete
	if result.Completed {
		t.Error("Expected verification to reject incomplete plan")
	}
	// Should hit max iterations
	if result.Error == nil || !strings.Contains(result.Error.Error(), "max iterations") {
		t.Errorf("Expected max iterations error, got: %v", result.Error)
	}
	// 2 runner calls: iteration + next iteration (LLM verify was short-circuited by checkbox check)
	if len(mockRunner.RecordedOpts) != 2 {
		t.Errorf("Expected 2 runner calls (iteration + iteration; LLM verify short-circuited), got %d", len(mockRunner.RecordedOpts))
	}

	// Check that combined feedback was written
	feedbackPath := plan.FeedbackPath(p)
	content, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Logf("Could not read feedback file: %v", err)
	} else if !strings.Contains(string(content), "state.yaml also shows") {
		t.Errorf("Expected combined feedback with state.yaml info, got: %s", string(content))
	}
}

func TestIterationLoop_StateYamlFallbackToLLM(t *testing.T) {
	// Plan bundle with state.yaml where T2 is still todo, but plan.md has all
	// checkboxes checked (agent did the work but couldn't update state.yaml).
	// Agent claims COMPLETE → state.yaml incomplete → LLM fallback says YES → loop completes.
	tempDir := t.TempDir()
	bundlePath := filepath.Join(tempDir, "plans", "current", "test-plan")
	os.MkdirAll(bundlePath, 0755)

	planPath := filepath.Join(bundlePath, "plan.md")
	planContent := `# Plan: Test
**Status:** complete
## Tasks
### T1: Do something
**Status:** complete
**Done when:**
- [x] First thing
### T2: Do another thing
**Status:** complete
**Done when:**
- [x] Second thing
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	// Create state.yaml with T1 done, T2 still todo (stale)
	now := time.Now()
	st := &state.PlanState{
		ID:     "test-plan",
		Title:  "Test",
		Status: state.PlanStatusActive,
		Tasks: []state.TaskState{
			{
				ID:     "T1",
				Title:  "Do something",
				Status: state.TaskStatusDone,
				Criteria: []state.Criterion{
					{Text: "First thing", Done: true, DoneAt: &now},
				},
			},
			{
				ID:     "T2",
				Title:  "Do another thing",
				Status: state.TaskStatusTodo,
				Criteria: []state.Criterion{
					{Text: "Second thing", Done: false},
				},
			},
		},
	}
	if err := state.SaveState(st, bundlePath); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	gitRepo := setupTestGitRepo(t, tempDir)

	p, err := plan.Load(bundlePath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	ctx := NewContext(p, "main", 10, tempDir)

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
			{TextContent: "YES"}, // LLM verification fallback says complete
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

	// Should complete — LLM overrides stale state.yaml
	if !result.Completed {
		t.Errorf("Expected LLM fallback to verify completion, error: %v", result.Error)
	}
	if result.Iterations != 1 {
		t.Errorf("Expected 1 iteration, got %d", result.Iterations)
	}
	// 2 runner calls: iteration + LLM verify fallback
	if len(mockRunner.RecordedOpts) != 2 {
		t.Errorf("Expected 2 runner calls (iteration + LLM verify), got %d", len(mockRunner.RecordedOpts))
	}
}

func TestIterationLoop_FallbackLLMVerification(t *testing.T) {
	// Plan WITHOUT state.yaml should fall back to LLM verification (existing behavior).
	tempDir := t.TempDir()
	planDir := filepath.Join(tempDir, "plans", "current")
	os.MkdirAll(planDir, 0755)

	// Flat file plan (not a bundle) — no state.yaml
	planPath := filepath.Join(planDir, "test-plan.md")
	planContent := `# Plan: Test
**Status:** complete
## Tasks
- [x] Task 1
`
	os.WriteFile(planPath, []byte(planContent), 0644)

	gitRepo := setupTestGitRepo(t, tempDir)

	p, err := plan.Load(planPath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	ctx := NewContext(p, "main", 10, tempDir)

	// Iteration + LLM verification call
	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
			{TextContent: "YES"}, // LLM verification response
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
		t.Errorf("Expected LLM verification to pass, error: %v", result.Error)
	}
	// Should have 2 runner calls: 1 iteration + 1 LLM verification
	if len(mockRunner.RecordedOpts) != 2 {
		t.Errorf("Expected 2 runner calls (iteration + LLM verify), got %d", len(mockRunner.RecordedOpts))
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
