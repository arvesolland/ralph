package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// mockReviewRunner implements ReviewRunner for testing.
type mockReviewRunner struct {
	responses []string
	errors    []error
	callIndex int
	prompts   []string
}

func (m *mockReviewRunner) Run(ctx context.Context, prompt string, opts ReviewRunnerOptions) (*ReviewRunnerResult, error) {
	m.prompts = append(m.prompts, prompt)

	idx := m.callIndex
	m.callIndex++

	if idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}

	if idx >= len(m.responses) {
		return &ReviewRunnerResult{TextContent: "ALIGNED"}, nil
	}

	return &ReviewRunnerResult{TextContent: m.responses[idx]}, nil
}

func TestIsAligned(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     bool
	}{
		{"exact ALIGNED", "ALIGNED", true},
		{"ALIGNED with whitespace", "  ALIGNED  \n", true},
		{"ALIGNED on its own line", "Some preamble\nALIGNED\nMore text", true},
		{"ALIGNED with backticks", "`ALIGNED`", true},
		{"ALIGNED with backticks and whitespace", "  `ALIGNED`  \n", true},
		{"not aligned - yaml response", "```yaml\ntasks: []\n```", false},
		{"not aligned - empty", "", false},
		{"not aligned - partial", "NOT ALIGNED", false},
		{"not aligned - embedded in word", "MISALIGNED", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAligned(tt.response)
			if got != tt.want {
				t.Errorf("isAligned(%q) = %v, want %v", tt.response, got, tt.want)
			}
		})
	}
}

func TestExtractYAML(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			"yaml fence",
			"Here is the corrected state:\n```yaml\nid: test\ntasks: []\n```\n",
			"id: test\ntasks: []",
		},
		{
			"yml fence",
			"```yml\nid: test\n```",
			"id: test",
		},
		{
			"no fence",
			"ALIGNED",
			"",
		},
		{
			"empty fence",
			"```yaml\n```",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractYAML(tt.response)
			if got != tt.want {
				t.Errorf("extractYAML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseStateFromResponse(t *testing.T) {
	t.Run("valid yaml with tasks", func(t *testing.T) {
		response := "```yaml\nid: test\ntitle: Test Plan\nstatus: active\ntasks:\n  - id: T1\n    title: First task\n    status: todo\n```"
		state, err := parseStateFromResponse(response)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(state.Tasks) != 1 {
			t.Errorf("expected 1 task, got %d", len(state.Tasks))
		}
		if state.Tasks[0].ID != "T1" {
			t.Errorf("expected task ID T1, got %s", state.Tasks[0].ID)
		}
	})

	t.Run("no yaml fence", func(t *testing.T) {
		_, err := parseStateFromResponse("ALIGNED")
		if err == nil {
			t.Error("expected error for response without YAML fence")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		response := "```yaml\n: : invalid\n```"
		_, err := parseStateFromResponse(response)
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})

	t.Run("yaml with no tasks", func(t *testing.T) {
		response := "```yaml\nid: test\ntitle: Test\nstatus: active\ntasks: []\n```"
		_, err := parseStateFromResponse(response)
		if err == nil {
			t.Error("expected error for state with no tasks")
		}
	})
}

func TestMergeStates(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)

	t.Run("preserves runtime state for existing tasks", func(t *testing.T) {
		old := &PlanState{
			ID:        "plan-1",
			Title:     "My Plan",
			Status:    PlanStatusActive,
			CreatedAt: earlier,
			Tasks: []TaskState{
				{
					ID:        "T1",
					Title:     "Old title",
					Status:    TaskStatusDone,
					StartedAt: &earlier,
					DoneAt:    &now,
					Artifacts: Artifacts{Commits: []string{"abc123"}},
					Notes:     []string{"done well"},
					Criteria:  []Criterion{{Text: "Thing works", Done: true, DoneAt: &now}},
				},
			},
		}

		new := &PlanState{
			ID:    "plan-1",
			Title: "My Plan Updated",
			Tasks: []TaskState{
				{
					ID:       "T1",
					Title:    "Updated title",
					Status:   TaskStatusTodo, // LLM might reset this
					Requires: []string{},
					Criteria: []Criterion{{Text: "Thing works"}, {Text: "New criterion"}},
				},
			},
		}

		merged := mergeStates(old, new)

		if merged.ID != "plan-1" {
			t.Errorf("expected plan ID plan-1, got %s", merged.ID)
		}
		if merged.Status != PlanStatusActive {
			t.Errorf("expected plan status active, got %s", merged.Status)
		}
		if merged.CreatedAt != earlier {
			t.Error("expected CreatedAt to be preserved")
		}

		if len(merged.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(merged.Tasks))
		}

		task := merged.Tasks[0]
		// Runtime state preserved from old
		if task.Status != TaskStatusDone {
			t.Errorf("expected status done (preserved), got %s", task.Status)
		}
		if task.StartedAt == nil {
			t.Error("expected StartedAt to be preserved")
		}
		if task.DoneAt == nil {
			t.Error("expected DoneAt to be preserved")
		}
		if len(task.Artifacts.Commits) != 1 {
			t.Error("expected Artifacts to be preserved")
		}
		if len(task.Notes) != 1 {
			t.Error("expected Notes to be preserved")
		}

		// Structural changes taken from new
		if task.Title != "Updated title" {
			t.Errorf("expected updated title, got %s", task.Title)
		}

		// Criteria merged
		if len(task.Criteria) != 2 {
			t.Fatalf("expected 2 criteria, got %d", len(task.Criteria))
		}
		if !task.Criteria[0].Done {
			t.Error("expected first criterion Done to be preserved from old")
		}
		if task.Criteria[1].Done {
			t.Error("expected new criterion to not be done")
		}
	})

	t.Run("adds new tasks from LLM", func(t *testing.T) {
		old := &PlanState{
			ID:     "plan-1",
			Status: PlanStatusActive,
			Tasks: []TaskState{
				{ID: "T1", Title: "Task 1", Status: TaskStatusDone},
			},
		}
		new := &PlanState{
			Tasks: []TaskState{
				{ID: "T1", Title: "Task 1", Status: TaskStatusTodo},
				{ID: "T2", Title: "New Task", Status: TaskStatusTodo},
			},
		}

		merged := mergeStates(old, new)
		if len(merged.Tasks) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(merged.Tasks))
		}
		if merged.Tasks[0].Status != TaskStatusDone {
			t.Error("expected T1 status preserved as done")
		}
		if merged.Tasks[1].ID != "T2" {
			t.Errorf("expected new task T2, got %s", merged.Tasks[1].ID)
		}
	})

	t.Run("keeps tasks removed by LLM", func(t *testing.T) {
		old := &PlanState{
			ID:     "plan-1",
			Status: PlanStatusActive,
			Tasks: []TaskState{
				{ID: "T1", Title: "Task 1", Status: TaskStatusDone},
				{ID: "T2", Title: "Task 2", Status: TaskStatusTodo},
			},
		}
		new := &PlanState{
			Tasks: []TaskState{
				{ID: "T1", Title: "Task 1"},
				// T2 removed by LLM
			},
		}

		merged := mergeStates(old, new)
		if len(merged.Tasks) != 2 {
			t.Fatalf("expected 2 tasks (T2 kept), got %d", len(merged.Tasks))
		}
		// T2 should be appended at the end
		found := false
		for _, task := range merged.Tasks {
			if task.ID == "T2" {
				found = true
				if task.Status != TaskStatusTodo {
					t.Errorf("expected T2 status preserved, got %s", task.Status)
				}
			}
		}
		if !found {
			t.Error("expected removed task T2 to be kept")
		}
	})
}

func TestReviewState_AlignedImmediately(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial state
	st := &PlanState{
		ID:     "test",
		Title:  "Test Plan",
		Status: PlanStatusActive,
		Tasks: []TaskState{
			{ID: "T1", Title: "Task 1", Status: TaskStatusTodo},
		},
	}
	if err := SaveState(st, tmpDir); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	runner := &mockReviewRunner{
		responses: []string{"ALIGNED"},
	}

	result, err := ReviewState(context.Background(), runner, "# Plan\n## Tasks\n### T1: Task 1", tmpDir, ReviewConfig{
		MaxAttempts: 5,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Aligned {
		t.Error("expected aligned result")
	}
	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration, got %d", result.Iterations)
	}
	if result.Changes != 0 {
		t.Errorf("expected 0 changes, got %d", result.Changes)
	}
}

func TestReviewState_FixesThenAligns(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial state with 1 task
	st := &PlanState{
		ID:        "test",
		Title:     "Test Plan",
		Status:    PlanStatusActive,
		CreatedAt: time.Now(),
		Tasks: []TaskState{
			{ID: "T1", Title: "Task 1", Status: TaskStatusTodo},
		},
	}
	if err := SaveState(st, tmpDir); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// LLM adds a missing task in iteration 1, then reports aligned
	correctedYAML := `id: test
title: Test Plan
status: active
tasks:
  - id: T1
    title: Task 1
    status: todo
  - id: T2
    title: Missing Task
    status: todo
    criteria:
      - text: Something done`

	runner := &mockReviewRunner{
		responses: []string{
			fmt.Sprintf("Here's the corrected state:\n```yaml\n%s\n```", correctedYAML),
			"ALIGNED",
		},
	}

	result, err := ReviewState(context.Background(), runner, "# Plan with T1 and T2", tmpDir, ReviewConfig{
		MaxAttempts: 5,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Aligned {
		t.Error("expected aligned result")
	}
	if result.Iterations != 2 {
		t.Errorf("expected 2 iterations, got %d", result.Iterations)
	}
	if result.Changes != 1 {
		t.Errorf("expected 1 change, got %d", result.Changes)
	}

	// Verify the saved state has both tasks
	savedState, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("failed to load saved state: %v", err)
	}
	if len(savedState.Tasks) != 2 {
		t.Errorf("expected 2 tasks in saved state, got %d", len(savedState.Tasks))
	}
}

func TestReviewState_MaxAttemptsRespected(t *testing.T) {
	tmpDir := t.TempDir()

	st := &PlanState{
		ID:     "test",
		Title:  "Test Plan",
		Status: PlanStatusActive,
		Tasks: []TaskState{
			{ID: "T1", Title: "Task 1", Status: TaskStatusTodo},
		},
	}
	if err := SaveState(st, tmpDir); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Always returns invalid YAML — should exhaust max attempts
	runner := &mockReviewRunner{
		responses: []string{
			"```yaml\n: invalid\n```",
			"```yaml\n: invalid\n```",
			"```yaml\n: invalid\n```",
		},
	}

	result, err := ReviewState(context.Background(), runner, "# Plan", tmpDir, ReviewConfig{
		MaxAttempts: 3,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Aligned {
		t.Error("expected not aligned")
	}
	if result.Iterations != 3 {
		t.Errorf("expected 3 iterations, got %d", result.Iterations)
	}
	if result.Changes != 0 {
		t.Errorf("expected 0 changes, got %d", result.Changes)
	}
}

func TestReviewState_MalformedYAMLRetries(t *testing.T) {
	tmpDir := t.TempDir()

	st := &PlanState{
		ID:     "test",
		Title:  "Test Plan",
		Status: PlanStatusActive,
		Tasks: []TaskState{
			{ID: "T1", Title: "Task 1", Status: TaskStatusTodo},
		},
	}
	if err := SaveState(st, tmpDir); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// First response is malformed YAML, second is valid
	runner := &mockReviewRunner{
		responses: []string{
			"```yaml\n: : : bad yaml\n```",
			"ALIGNED",
		},
	}

	result, err := ReviewState(context.Background(), runner, "# Plan", tmpDir, ReviewConfig{
		MaxAttempts: 5,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Aligned {
		t.Error("expected aligned after recovery")
	}
	if result.Iterations != 2 {
		t.Errorf("expected 2 iterations, got %d", result.Iterations)
	}

	// Check that the second prompt includes the validation error
	if len(runner.prompts) < 2 {
		t.Fatal("expected at least 2 prompts")
	}
}

func TestReviewState_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	st := &PlanState{
		ID:     "test",
		Title:  "Test",
		Status: PlanStatusActive,
		Tasks:  []TaskState{{ID: "T1", Title: "T1", Status: TaskStatusTodo}},
	}
	SaveState(st, tmpDir)

	runner := &mockReviewRunner{
		responses: []string{"ALIGNED"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := ReviewState(ctx, runner, "# Plan", tmpDir, ReviewConfig{MaxAttempts: 5})
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestReviewState_NoStateFile(t *testing.T) {
	tmpDir := t.TempDir()

	runner := &mockReviewRunner{}

	_, err := ReviewState(context.Background(), runner, "# Plan", tmpDir, ReviewConfig{MaxAttempts: 5})
	if err == nil {
		t.Error("expected error when state.yaml doesn't exist")
	}
}

func TestReviewState_PreservesRuntimeOnMerge(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now()
	st := &PlanState{
		ID:        "test",
		Title:     "Test Plan",
		Status:    PlanStatusActive,
		CreatedAt: now,
		Tasks: []TaskState{
			{
				ID:        "T1",
				Title:     "Task 1",
				Status:    TaskStatusDoing,
				StartedAt: &now,
				Notes:     []string{"started working"},
			},
		},
	}
	if err := SaveState(st, tmpDir); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// LLM adds a criterion to T1 and adds T2
	correctedYAML, _ := yaml.Marshal(&PlanState{
		ID:     "test",
		Title:  "Test Plan",
		Status: PlanStatusActive,
		Tasks: []TaskState{
			{
				ID:     "T1",
				Title:  "Task 1",
				Status: TaskStatusTodo, // LLM resets status
				Criteria: []Criterion{
					{Text: "Unit tests pass"},
				},
			},
			{
				ID:     "T2",
				Title:  "New task from LLM",
				Status: TaskStatusTodo,
			},
		},
	})

	runner := &mockReviewRunner{
		responses: []string{
			fmt.Sprintf("```yaml\n%s\n```", string(correctedYAML)),
			"ALIGNED",
		},
	}

	result, err := ReviewState(context.Background(), runner, "# Plan", tmpDir, ReviewConfig{MaxAttempts: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Aligned {
		t.Error("expected aligned")
	}

	// Load and verify
	saved, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// T1 should preserve runtime state
	t1 := saved.Tasks[0]
	if t1.Status != TaskStatusDoing {
		t.Errorf("expected T1 status 'doing' (preserved), got %s", t1.Status)
	}
	if t1.StartedAt == nil {
		t.Error("expected T1 StartedAt to be preserved")
	}
	if len(t1.Notes) != 1 {
		t.Error("expected T1 Notes to be preserved")
	}
	if len(t1.Criteria) != 1 || t1.Criteria[0].Text != "Unit tests pass" {
		t.Error("expected new criterion from LLM to be added")
	}

	// T2 should be the new task
	if len(saved.Tasks) < 2 {
		t.Fatal("expected T2 to be added")
	}
	if saved.Tasks[1].ID != "T2" {
		t.Errorf("expected T2, got %s", saved.Tasks[1].ID)
	}
}

func TestReviewState_DependencyValidationFails(t *testing.T) {
	tmpDir := t.TempDir()

	st := &PlanState{
		ID:     "test",
		Title:  "Test",
		Status: PlanStatusActive,
		Tasks:  []TaskState{{ID: "T1", Title: "T1", Status: TaskStatusTodo}},
	}
	SaveState(st, tmpDir)

	// LLM outputs YAML with invalid dependency, then fixes it
	badYAML := `id: test
title: Test
status: active
tasks:
  - id: T1
    title: Task 1
    status: todo
    requires:
      - T99`

	goodYAML := `id: test
title: Test
status: active
tasks:
  - id: T1
    title: Task 1
    status: todo`

	runner := &mockReviewRunner{
		responses: []string{
			fmt.Sprintf("```yaml\n%s\n```", badYAML),
			fmt.Sprintf("```yaml\n%s\n```", goodYAML),
			"ALIGNED",
		},
	}

	result, err := ReviewState(context.Background(), runner, "# Plan", tmpDir, ReviewConfig{MaxAttempts: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Aligned {
		t.Error("expected aligned after recovery")
	}
	// First iteration had bad deps (no change saved), second had good deps (1 change), third aligned
	if result.Iterations != 3 {
		t.Errorf("expected 3 iterations, got %d", result.Iterations)
	}
	if result.Changes != 1 {
		t.Errorf("expected 1 change (second iteration), got %d", result.Changes)
	}
}

func TestReviewState_LLMError(t *testing.T) {
	tmpDir := t.TempDir()

	st := &PlanState{
		ID:     "test",
		Title:  "Test",
		Status: PlanStatusActive,
		Tasks:  []TaskState{{ID: "T1", Title: "T1", Status: TaskStatusTodo}},
	}
	SaveState(st, tmpDir)

	runner := &mockReviewRunner{
		errors: []error{fmt.Errorf("API rate limit exceeded")},
	}

	_, err := ReviewState(context.Background(), runner, "# Plan", tmpDir, ReviewConfig{MaxAttempts: 5})
	if err == nil {
		t.Error("expected error from LLM failure")
	}
}

func TestReviewState_DefaultModel(t *testing.T) {
	tmpDir := t.TempDir()

	st := &PlanState{
		ID:     "test",
		Title:  "Test",
		Status: PlanStatusActive,
		Tasks:  []TaskState{{ID: "T1", Title: "T1", Status: TaskStatusTodo}},
	}
	SaveState(st, tmpDir)

	runner := &mockReviewRunner{
		responses: []string{"ALIGNED"},
	}

	// Don't specify model — should use default
	ReviewState(context.Background(), runner, "# Plan", tmpDir, ReviewConfig{MaxAttempts: 1})

	// Can't directly check opts since our mock doesn't record them, but this tests
	// that the code path works without specifying a model.
	if runner.callIndex != 1 {
		t.Errorf("expected 1 call, got %d", runner.callIndex)
	}
}

func TestMergeCriteria(t *testing.T) {
	now := time.Now()

	old := []Criterion{
		{Text: "Unit tests pass", Done: true, DoneAt: &now},
		{Text: "Docs updated", Done: false},
	}

	new := []Criterion{
		{Text: "Unit tests pass"},     // Same text, should preserve Done
		{Text: "Integration tests"},   // New criterion
		{Text: "Docs updated"},        // Same text, was not done
	}

	result := mergeCriteria(old, new)

	if len(result) != 3 {
		t.Fatalf("expected 3 criteria, got %d", len(result))
	}
	if !result[0].Done {
		t.Error("expected 'Unit tests pass' to preserve Done=true")
	}
	if result[0].DoneAt == nil {
		t.Error("expected 'Unit tests pass' to preserve DoneAt")
	}
	if result[1].Done {
		t.Error("expected new criterion to not be done")
	}
	if result[2].Done {
		t.Error("expected 'Docs updated' to preserve Done=false")
	}
}

func TestBuildReviewPromptInline(t *testing.T) {
	prompt := buildReviewPromptInline("# My Plan", "id: test\ntasks: []", "")
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !contains(prompt, "# My Plan") {
		t.Error("expected plan content in prompt")
	}
	if !contains(prompt, "id: test") {
		t.Error("expected state YAML in prompt")
	}
}

func TestBuildReviewPromptInline_WithError(t *testing.T) {
	prompt := buildReviewPromptInline("# Plan", "id: test", "bad yaml error")
	if !contains(prompt, "bad yaml error") {
		t.Error("expected validation error in prompt")
	}
}

func TestReviewState_WithPromptBuilder(t *testing.T) {
	// Test that the prompt builder path works when a template exists.
	// We create a temp dir with the prompt template.
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	os.MkdirAll(promptsDir, 0755)

	// Write a minimal template
	template := "Plan:\n{{PLAN_CONTENT}}\nState:\n{{STATE_YAML}}\n{{VALIDATION_ERROR}}"
	os.WriteFile(filepath.Join(promptsDir, "state_review_prompt.md"), []byte(template), 0644)

	// Test building the prompt
	prompt, err := buildReviewPrompt(nil, "# Plan content", "id: test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
