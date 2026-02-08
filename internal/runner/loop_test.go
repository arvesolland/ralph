package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/atm"
	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
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
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	// Create context with max 2 iterations
	ctx := NewContext(1, "feat/test", "main", 2)

	// Mock runner that never completes
	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Working on task 1..."},
			{TextContent: "Working on task 2..."},
			{TextContent: "Still working..."}, // Won't be reached
		},
	}

	loop := NewIterationLoop(LoopConfig{
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	result := loop.Run(context.Background())

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
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(1, "feat/test", "main", 10)

	// No ATM client configured, so completion marker is trusted directly
	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Working on task 1..."},
			{TextContent: "Almost done..."},
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
		},
	}

	loop := NewIterationLoop(LoopConfig{
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
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(1, "feat/test", "main", 2)

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

	if !blockerCallbackCalled {
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
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(1, "feat/test", "main", 100)

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Working..."},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	cancelCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := loop.Run(cancelCtx)

	if result.Error != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", result.Error)
	}
}

func TestIterationLoop_Run_OnIterationCallback(t *testing.T) {
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(1, "feat/test", "main", 3)

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

func TestIterationLoop_CompletesWithNoATM(t *testing.T) {
	// Without ATM client, completion marker should be trusted directly
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(1, "feat/test", "main", 10)

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		// No ATM client
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
		t.Errorf("Expected completion with no ATM, error: %v", result.Error)
	}
	if result.Iterations != 1 {
		t.Errorf("Expected 1 iteration, got %d", result.Iterations)
	}
	// Only 1 runner call (the iteration) - no separate verification call
	if len(mockRunner.RecordedOpts) != 1 {
		t.Errorf("Expected 1 runner call, got %d", len(mockRunner.RecordedOpts))
	}
}

func TestIterationLoop_ATMContextFailure_HardError(t *testing.T) {
	// PlanContextText failure should be a hard error that terminates the iteration
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 10)

	mockATM := atm.NewMockATM()
	mockATM.PlanContextTextFunc = func(planID int) (string, error) {
		return "", fmt.Errorf("connection refused")
	}

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Should not reach this"},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		ATM:              mockATM,
		PlanID:           42,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	result := loop.Run(context.Background())

	if result.Completed {
		t.Error("Expected loop to not complete")
	}
	if result.Error == nil {
		t.Fatal("Expected error from ATM context failure")
	}
	if !strings.Contains(result.Error.Error(), "fetching ATM plan context") {
		t.Errorf("Expected ATM context error, got: %v", result.Error)
	}
	// Runner should not have been called since PlanContextText failed first
	if len(mockRunner.RecordedOpts) != 0 {
		t.Errorf("Expected 0 runner calls, got %d", len(mockRunner.RecordedOpts))
	}
}

func TestIterationLoop_ATMCompletionCheck_AllDone(t *testing.T) {
	// When ATM stats show all tasks done, loop should complete
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 10)

	mockATM := atm.NewMockATM()
	mockATM.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Plan Context\nAll tasks listed here.", nil
	}
	mockATM.PlanContextFunc = func(planID int) (*atm.AgentContext, error) {
		return &atm.AgentContext{
			Stats: atm.Stats{
				TotalTasks: 3,
				Done:       2,
				Skipped:    1,
			},
		}, nil
	}

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "All done! <promise>COMPLETE</promise>", IsComplete: true},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		ATM:              mockATM,
		PlanID:           42,
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
		t.Errorf("Expected completion, got error: %v", result.Error)
	}
	if result.Iterations != 1 {
		t.Errorf("Expected 1 iteration, got %d", result.Iterations)
	}
}

func TestIterationLoop_ATMCompletionCheck_NotDone(t *testing.T) {
	// When ATM stats show tasks remain, should be a false completion and AddFeedback called
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 2)

	mockATM := atm.NewMockATM()
	mockATM.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Plan Context", nil
	}
	mockATM.PlanContextFunc = func(planID int) (*atm.AgentContext, error) {
		return &atm.AgentContext{
			Stats: atm.Stats{
				TotalTasks: 5,
				Done:       2,
				Skipped:    0,
			},
		}, nil
	}

	var feedbackCalls []string
	mockATM.AddFeedbackFunc = func(planID int, author, body string) (*atm.Feedback, error) {
		feedbackCalls = append(feedbackCalls, body)
		return &atm.Feedback{}, nil
	}

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
			{TextContent: "Still working..."},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		ATM:              mockATM,
		PlanID:           42,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	result := loop.Run(context.Background())

	if result.Completed {
		t.Error("Expected loop to not complete since tasks remain")
	}
	// Should have called AddFeedback with a descriptive message
	if len(feedbackCalls) != 1 {
		t.Fatalf("Expected 1 AddFeedback call, got %d", len(feedbackCalls))
	}
	if !strings.Contains(feedbackCalls[0], "false completion attempt 1/5") {
		t.Errorf("Expected false completion feedback, got: %s", feedbackCalls[0])
	}
}

func TestIterationLoop_FalseCompletionCircuitBreaker(t *testing.T) {
	// After MaxFalseCompletions (5) consecutive false completions, loop should halt
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 100)

	mockATM := atm.NewMockATM()
	mockATM.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Plan", nil
	}
	// Always report incomplete
	mockATM.PlanContextFunc = func(planID int) (*atm.AgentContext, error) {
		return &atm.AgentContext{
			Stats: atm.Stats{TotalTasks: 3, Done: 1},
		}, nil
	}

	// Every iteration claims completion
	responses := make([]MockResponse, MaxFalseCompletions)
	for i := range responses {
		responses[i] = MockResponse{TextContent: "Done!", IsComplete: true}
	}
	mockRunner := &MockRunner{Responses: responses}

	loop := NewIterationLoop(LoopConfig{
		ATM:              mockATM,
		PlanID:           42,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	result := loop.Run(context.Background())

	if result.Completed {
		t.Error("Expected loop to NOT complete (circuit breaker)")
	}
	if result.Error == nil {
		t.Fatal("Expected error from circuit breaker")
	}
	if !strings.Contains(result.Error.Error(), "claimed completion 5 times") {
		t.Errorf("Expected circuit breaker error, got: %v", result.Error)
	}
	if result.Iterations != MaxFalseCompletions {
		t.Errorf("Expected %d iterations, got %d", MaxFalseCompletions, result.Iterations)
	}
}

func TestIterationLoop_ATMCompletionCheck_Unreachable(t *testing.T) {
	// When PlanContext fails all 3 retries, should fail-closed (not trust completion marker)
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 2)

	mockATM := atm.NewMockATM()
	mockATM.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Plan", nil
	}
	// PlanContext always fails (unreachable)
	planContextCalls := 0
	mockATM.PlanContextFunc = func(planID int) (*atm.AgentContext, error) {
		planContextCalls++
		return nil, fmt.Errorf("connection timeout")
	}

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
			{TextContent: "Working..."},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		ATM:              mockATM,
		PlanID:           42,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	// Use a context that will cancel during the retry backoff to speed up the test
	cancelCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	result := loop.Run(cancelCtx)

	// Should NOT complete (fail-closed: ATM unreachable means don't trust marker)
	if result.Completed {
		t.Error("Expected loop to not complete when ATM is unreachable (fail-closed)")
	}
	// PlanContext should have been called multiple times (retries)
	if planContextCalls < 2 {
		t.Errorf("Expected multiple PlanContext retry calls, got %d", planContextCalls)
	}
}

func TestIterationLoop_ATMProgressTracking(t *testing.T) {
	// AddProgress should be called after each iteration; failure is non-fatal
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 2)

	mockATM := atm.NewMockATM()
	mockATM.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Plan", nil
	}

	var progressCalls []string
	mockATM.AddProgressFunc = func(planID int, author, body string) (*atm.Progress, error) {
		progressCalls = append(progressCalls, body)
		return nil, fmt.Errorf("progress write failed") // Non-fatal error
	}

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Working on task 1..."},
			{TextContent: "Working on task 2..."},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		ATM:              mockATM,
		PlanID:           42,
		Context:          ctx,
		Config:           config.Defaults(),
		Runner:           mockRunner,
		Git:              gitRepo,
		PromptBuilder:    prompt.NewBuilder(config.Defaults(), "", ""),
		WorktreePath:     tempDir,
		IterationTimeout: 1 * time.Second,
	})

	result := loop.Run(context.Background())

	// Should reach max iterations (not fail due to progress error)
	if result.Error == nil || !strings.Contains(result.Error.Error(), "max iterations") {
		t.Errorf("Expected max iterations error (progress failure is non-fatal), got: %v", result.Error)
	}
	// AddProgress should have been called for each iteration
	if len(progressCalls) != 2 {
		t.Errorf("Expected 2 AddProgress calls, got %d", len(progressCalls))
	}
	if !strings.Contains(progressCalls[0], "Iteration 1") {
		t.Errorf("Expected iteration info in progress body, got: %s", progressCalls[0])
	}
}

// setupTestGitRepo creates a git repo for testing.
func setupTestGitRepo(t *testing.T, dir string) git.Git {
	t.Helper()

	gitRepo := git.NewGit(dir)

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
	c.Stderr = os.Stderr
	return c.Run()
}
