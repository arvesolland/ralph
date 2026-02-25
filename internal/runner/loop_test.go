package runner

import (
	"context"
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

	// No Board client configured, so completion marker is trusted directly
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

func TestIterationLoop_CompletesWithNoBoard(t *testing.T) {
	// Without Board client, completion marker should be trusted directly
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(1, "feat/test", "main", 10)

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		// No Board client
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
		t.Errorf("Expected completion with no Board, error: %v", result.Error)
	}
	if result.Iterations != 1 {
		t.Errorf("Expected 1 iteration, got %d", result.Iterations)
	}
	// Only 1 runner call (the iteration) - no separate verification call
	if len(mockRunner.RecordedOpts) != 1 {
		t.Errorf("Expected 1 runner call, got %d", len(mockRunner.RecordedOpts))
	}
}

func TestIterationLoop_BoardContextFailure_HardError(t *testing.T) {
	// PlanContextText failure should be a hard error that terminates the iteration (Board)
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 10)

	mockBoard := board.NewMockBoard()
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "", fmt.Errorf("connection refused")
	}

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Should not reach this"},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		Board:              mockBoard,
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
		t.Fatal("Expected error from Board context failure")
	}
	if !strings.Contains(result.Error.Error(), "fetching Board plan context") {
		t.Errorf("Expected Board context error, got: %v", result.Error)
	}
	// Runner should not have been called since PlanContextText failed first
	if len(mockRunner.RecordedOpts) != 0 {
		t.Errorf("Expected 0 runner calls, got %d", len(mockRunner.RecordedOpts))
	}
}

func TestIterationLoop_BoardCompletionCheck_AllDone(t *testing.T) {
	// When Board stats show all tasks done, loop should complete
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 10)

	mockBoard := board.NewMockBoard()
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Plan Context\nAll tasks listed here.", nil
	}
	mockBoard.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			Stats: board.Stats{
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
		Board:              mockBoard,
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

func TestIterationLoop_BoardCompletionCheck_NotDone(t *testing.T) {
	// When Board stats show tasks remain, should be a false completion and AddFeedback called
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 2)

	mockBoard := board.NewMockBoard()
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Plan Context", nil
	}
	mockBoard.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			Stats: board.Stats{
				TotalTasks: 5,
				Done:       2,
				Skipped:    0,
			},
		}, nil
	}

	var feedbackCalls []string
	mockBoard.AddFeedbackFunc = func(planID int, author, body string) (*board.Feedback, error) {
		feedbackCalls = append(feedbackCalls, body)
		return &board.Feedback{}, nil
	}

	mockRunner := &MockRunner{
		Responses: []MockResponse{
			{TextContent: "Done! <promise>COMPLETE</promise>", IsComplete: true},
			{TextContent: "Still working..."},
		},
	}

	loop := NewIterationLoop(LoopConfig{
		Board:              mockBoard,
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

	mockBoard := board.NewMockBoard()
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Plan", nil
	}
	// Always report incomplete
	mockBoard.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			Stats: board.Stats{TotalTasks: 3, Done: 1},
		}, nil
	}

	// Every iteration claims completion
	responses := make([]MockResponse, MaxFalseCompletions)
	for i := range responses {
		responses[i] = MockResponse{TextContent: "Done!", IsComplete: true}
	}
	mockRunner := &MockRunner{Responses: responses}

	loop := NewIterationLoop(LoopConfig{
		Board:              mockBoard,
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

func TestIterationLoop_BoardCompletionCheck_Unreachable(t *testing.T) {
	// When PlanContext fails all 3 retries, should fail-closed (not trust completion marker)
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 2)

	mockBoard := board.NewMockBoard()
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Plan", nil
	}
	// PlanContext always fails (unreachable)
	planContextCalls := 0
	mockBoard.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
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
		Board:              mockBoard,
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

	// Should NOT complete (fail-closed: Board unreachable means don't trust marker)
	if result.Completed {
		t.Error("Expected loop to not complete when Board is unreachable (fail-closed)")
	}
	// PlanContext should have been called multiple times (retries)
	if planContextCalls < 2 {
		t.Errorf("Expected multiple PlanContext retry calls, got %d", planContextCalls)
	}
}

func TestIterationLoop_BoardProgressTracking(t *testing.T) {
	// AddProgress should be called after each iteration; failure is non-fatal
	tempDir := t.TempDir()
	gitRepo := setupTestGitRepo(t, tempDir)

	ctx := NewContext(42, "feat/test", "main", 2)

	mockBoard := board.NewMockBoard()
	mockBoard.PlanContextTextFunc = func(planID int) (string, error) {
		return "# Plan", nil
	}

	var progressCalls []string
	mockBoard.AddProgressFunc = func(planID int, author, body string) (*board.Progress, error) {
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
		Board:              mockBoard,
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
	if !strings.Contains(progressCalls[0], "[iter 1]") {
		t.Errorf("Expected iteration info in progress body, got: %s", progressCalls[0])
	}
}

// setupOverrideFile creates the .ralph/steering/ directory and writes content to override.md.
func setupOverrideFile(t *testing.T, dir string, content string) {
	t.Helper()
	steeringPath := filepath.Join(dir, steeringDir)
	if err := os.MkdirAll(steeringPath, 0o755); err != nil {
		t.Fatalf("Failed to create steering directory: %v", err)
	}
	overridePath := filepath.Join(steeringPath, overrideFilename)
	if err := os.WriteFile(overridePath, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write override file: %v", err)
	}
}

func TestReadAndConsumeOverride_NoFile(t *testing.T) {
	dir := t.TempDir()

	loop := NewIterationLoop(LoopConfig{
		WorktreePath: dir,
	})

	content, err := loop.readAndConsumeOverride()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty string, got: %q", content)
	}
}

func TestReadAndConsumeOverride_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	setupOverrideFile(t, dir, "")

	loop := NewIterationLoop(LoopConfig{
		WorktreePath: dir,
	})

	content, err := loop.readAndConsumeOverride()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty string, got: %q", content)
	}

	// File should NOT be renamed (still exists as override.md)
	overridePath := filepath.Join(dir, steeringDir, overrideFilename)
	if _, err := os.Stat(overridePath); os.IsNotExist(err) {
		t.Error("Expected empty override file to still exist (not consumed)")
	}
}

func TestReadAndConsumeOverride_WhitespaceOnly(t *testing.T) {
	dir := t.TempDir()
	setupOverrideFile(t, dir, "  \n\t\n  ")

	loop := NewIterationLoop(LoopConfig{
		WorktreePath: dir,
	})

	content, err := loop.readAndConsumeOverride()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty string for whitespace-only file, got: %q", content)
	}
}

func TestReadAndConsumeOverride_ValidFile(t *testing.T) {
	dir := t.TempDir()
	overrideContent := "Do NOT use the existing API client.\nCreate a new standalone HTTP client."
	setupOverrideFile(t, dir, overrideContent)

	loop := NewIterationLoop(LoopConfig{
		WorktreePath: dir,
	})

	content, err := loop.readAndConsumeOverride()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if content != overrideContent {
		t.Errorf("Expected override content %q, got: %q", overrideContent, content)
	}

	// Original file should no longer exist
	overridePath := filepath.Join(dir, steeringDir, overrideFilename)
	if _, err := os.Stat(overridePath); !os.IsNotExist(err) {
		t.Error("Expected original override.md to be removed after consumption")
	}

	// A .consumed file should exist in the steering directory
	steeringPath := filepath.Join(dir, steeringDir)
	entries, err := os.ReadDir(steeringPath)
	if err != nil {
		t.Fatalf("Failed to read steering directory: %v", err)
	}
	found := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "override.md.consumed.") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected a file matching override.md.consumed.* in steering directory")
	}
}

func TestReadAndConsumeOverride_WithHeaders(t *testing.T) {
	dir := t.TempDir()
	overrideContent := "# Override Instructions (from Foreman)\n# Severity: REDIRECT\n\nActual instructions here"
	setupOverrideFile(t, dir, overrideContent)

	loop := NewIterationLoop(LoopConfig{
		WorktreePath: dir,
	})

	content, err := loop.readAndConsumeOverride()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if content != overrideContent {
		t.Errorf("Expected full content including headers, got: %q", content)
	}
}

func TestBuildPrompt_WithoutOverride(t *testing.T) {
	tempDir := t.TempDir()

	ctx := NewContext(42, "feat/test", "main", 10)
	builder := prompt.NewBuilder(config.Defaults(), "", "")

	loop := NewIterationLoop(LoopConfig{
		PlanID:        42,
		Context:       ctx,
		Config:        config.Defaults(),
		PromptBuilder: builder,
		WorktreePath:  tempDir,
	})

	contextText := "Project: TestProject\nPlan: #42 Test Plan\nStats: 3 total | 1 done"

	result, err := loop.buildPrompt(contextText)
	if err != nil {
		t.Fatalf("buildPrompt failed: %v", err)
	}

	// Board context should be injected
	if !strings.Contains(result, "Project: TestProject") {
		t.Error("Expected Board context to be present in prompt")
	}

	// No OVERRIDE INSTRUCTIONS section should appear
	if strings.Contains(result, "OVERRIDE INSTRUCTIONS") {
		t.Error("Expected no OVERRIDE INSTRUCTIONS section when no override file exists")
	}

	// Standard prompt sections should be present
	if !strings.Contains(result, "## Workflow") {
		t.Error("Expected ## Workflow section in prompt")
	}
	if !strings.Contains(result, "## Plan State") {
		t.Error("Expected ## Plan State section in prompt")
	}
}

func TestBuildPrompt_WithOverride(t *testing.T) {
	tempDir := t.TempDir()

	ctx := NewContext(42, "feat/test", "main", 10)
	builder := prompt.NewBuilder(config.Defaults(), "", "")

	loop := NewIterationLoop(LoopConfig{
		PlanID:        42,
		Context:       ctx,
		Config:        config.Defaults(),
		PromptBuilder: builder,
		WorktreePath:  tempDir,
	})

	// Create override file
	overrideText := "Do NOT use the existing API client.\nCreate a new standalone HTTP client."
	setupOverrideFile(t, tempDir, overrideText)

	contextText := "Project: TestProject\nPlan: #42 Test Plan"

	result, err := loop.buildPrompt(contextText)
	if err != nil {
		t.Fatalf("buildPrompt failed: %v", err)
	}

	// Override content should appear in the rendered prompt
	if !strings.Contains(result, "## OVERRIDE INSTRUCTIONS (HIGH PRIORITY)") {
		t.Error("Expected OVERRIDE INSTRUCTIONS header in prompt")
	}
	if !strings.Contains(result, overrideText) {
		t.Error("Expected override content to appear in prompt")
	}
	if !strings.Contains(result, "These instructions from the Foreman orchestrator") {
		t.Error("Expected priority notice in prompt")
	}

	// Override file should have been consumed (renamed)
	overridePath := filepath.Join(tempDir, steeringDir, overrideFilename)
	if _, err := os.Stat(overridePath); !os.IsNotExist(err) {
		t.Error("Expected override file to be consumed (renamed) after buildPrompt")
	}

	// A .consumed file should exist
	steeringPath := filepath.Join(tempDir, steeringDir)
	entries, err := os.ReadDir(steeringPath)
	if err != nil {
		t.Fatalf("Failed to read steering directory: %v", err)
	}
	found := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "override.md.consumed.") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected consumed override file in steering directory")
	}
}

func TestBuildPrompt_WithOverrideAndContext(t *testing.T) {
	tempDir := t.TempDir()

	ctx := NewContext(42, "feat/test", "main", 10)
	builder := prompt.NewBuilder(config.Defaults(), "", "")

	loop := NewIterationLoop(LoopConfig{
		PlanID:        42,
		Context:       ctx,
		Config:        config.Defaults(),
		PromptBuilder: builder,
		WorktreePath:  tempDir,
	})

	// Create override file
	overrideText := "Focus on error handling first."
	setupOverrideFile(t, tempDir, overrideText)

	contextText := "Project: TestProject\nPlan: #42 Test Plan\nStats: 5 total | 2 done"

	result, err := loop.buildPrompt(contextText)
	if err != nil {
		t.Fatalf("buildPrompt failed: %v", err)
	}

	// Both Board context and override should be present
	if !strings.Contains(result, "Project: TestProject") {
		t.Error("Expected Board context in prompt")
	}
	if !strings.Contains(result, "OVERRIDE INSTRUCTIONS") {
		t.Error("Expected OVERRIDE INSTRUCTIONS in prompt")
	}
	if !strings.Contains(result, overrideText) {
		t.Error("Expected override content in prompt")
	}

	// Verify order: Board context should appear before override instructions
	boardIdx := strings.Index(result, "Project: TestProject")
	overrideIdx := strings.Index(result, "OVERRIDE INSTRUCTIONS")
	workflowIdx := strings.Index(result, "## Workflow")

	if boardIdx >= overrideIdx {
		t.Errorf("Expected Board context (idx %d) before override (idx %d)", boardIdx, overrideIdx)
	}
	if overrideIdx >= workflowIdx {
		t.Errorf("Expected override (idx %d) before Workflow section (idx %d)", overrideIdx, workflowIdx)
	}
}

func TestBuildPrompt_SetsLastOverrideApplied(t *testing.T) {
	tempDir := t.TempDir()

	ctx := NewContext(42, "feat/test", "main", 10)
	builder := prompt.NewBuilder(config.Defaults(), "", "")

	loop := NewIterationLoop(LoopConfig{
		PlanID:        42,
		Context:       ctx,
		Config:        config.Defaults(),
		PromptBuilder: builder,
		WorktreePath:  tempDir,
	})

	// Without override file, lastOverrideApplied should be false
	_, err := loop.buildPrompt("some context")
	if err != nil {
		t.Fatalf("buildPrompt failed: %v", err)
	}
	if loop.lastOverrideApplied {
		t.Error("Expected lastOverrideApplied to be false when no override file exists")
	}

	// With override file, lastOverrideApplied should be true
	setupOverrideFile(t, tempDir, "Override instructions here")
	_, err = loop.buildPrompt("some context")
	if err != nil {
		t.Fatalf("buildPrompt failed: %v", err)
	}
	if !loop.lastOverrideApplied {
		t.Error("Expected lastOverrideApplied to be true when override file exists")
	}

	// Next call without override should reset to false
	_, err = loop.buildPrompt("some context")
	if err != nil {
		t.Fatalf("buildPrompt failed: %v", err)
	}
	if loop.lastOverrideApplied {
		t.Error("Expected lastOverrideApplied to be reset to false on next call without override")
	}
}

func TestBuildProgressBody_WithOverride(t *testing.T) {
	ctx := NewContext(42, "feat/test", "main", 10)

	loop := NewIterationLoop(LoopConfig{
		Context: ctx,
	})

	result := &Result{
		TextContent: "Worked on task 1",
		Duration:    5 * time.Second,
	}

	// Without override
	loop.lastOverrideApplied = false
	body := loop.buildProgressBody(result)
	if strings.Contains(body, "[Override:") {
		t.Error("Expected no override marker when lastOverrideApplied is false")
	}

	// With override
	loop.lastOverrideApplied = true
	body = loop.buildProgressBody(result)
	if !strings.Contains(body, "[Override: steering instructions applied this iteration]") {
		t.Errorf("Expected override marker in progress body, got: %s", body)
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
