package plan

import (
	"testing"
)

func TestExtractTasks_SimpleTasks(t *testing.T) {
	content := `# Test Plan

- [ ] First task
- [ ] Second task
- [x] Third task (completed)
`
	tasks := ExtractTasks(content)

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// First task
	if tasks[0].Text != "First task" {
		t.Errorf("expected 'First task', got %q", tasks[0].Text)
	}
	if tasks[0].Complete {
		t.Error("expected first task to be incomplete")
	}
	if tasks[0].Line != 3 {
		t.Errorf("expected line 3, got %d", tasks[0].Line)
	}

	// Second task
	if tasks[1].Text != "Second task" {
		t.Errorf("expected 'Second task', got %q", tasks[1].Text)
	}
	if tasks[1].Complete {
		t.Error("expected second task to be incomplete")
	}

	// Third task (completed)
	if tasks[2].Text != "Third task (completed)" {
		t.Errorf("expected 'Third task (completed)', got %q", tasks[2].Text)
	}
	if !tasks[2].Complete {
		t.Error("expected third task to be complete")
	}
}

func TestExtractTasks_NestedSubtasks(t *testing.T) {
	content := `# Test Plan

- [ ] Parent task 1
  - [ ] Subtask 1.1
  - [x] Subtask 1.2
    - [ ] Sub-subtask 1.2.1
- [ ] Parent task 2
`
	tasks := ExtractTasks(content)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 top-level tasks, got %d", len(tasks))
	}

	// Parent task 1 with subtasks
	if tasks[0].Text != "Parent task 1" {
		t.Errorf("expected 'Parent task 1', got %q", tasks[0].Text)
	}
	if len(tasks[0].Subtasks) != 2 {
		t.Fatalf("expected 2 subtasks, got %d", len(tasks[0].Subtasks))
	}

	// Subtask 1.1
	if tasks[0].Subtasks[0].Text != "Subtask 1.1" {
		t.Errorf("expected 'Subtask 1.1', got %q", tasks[0].Subtasks[0].Text)
	}
	if tasks[0].Subtasks[0].Complete {
		t.Error("expected subtask 1.1 to be incomplete")
	}

	// Subtask 1.2 (completed, with sub-subtask)
	if !tasks[0].Subtasks[1].Complete {
		t.Error("expected subtask 1.2 to be complete")
	}
	if len(tasks[0].Subtasks[1].Subtasks) != 1 {
		t.Fatalf("expected 1 sub-subtask, got %d", len(tasks[0].Subtasks[1].Subtasks))
	}

	// Sub-subtask 1.2.1
	if tasks[0].Subtasks[1].Subtasks[0].Text != "Sub-subtask 1.2.1" {
		t.Errorf("expected 'Sub-subtask 1.2.1', got %q", tasks[0].Subtasks[1].Subtasks[0].Text)
	}

	// Parent task 2 (no subtasks)
	if tasks[1].Text != "Parent task 2" {
		t.Errorf("expected 'Parent task 2', got %q", tasks[1].Text)
	}
	if len(tasks[1].Subtasks) != 0 {
		t.Errorf("expected 0 subtasks for parent 2, got %d", len(tasks[1].Subtasks))
	}
}

func TestExtractTasks_WithDependencies(t *testing.T) {
	content := `# Test Plan

- [ ] T1: First task
- [ ] T2: Second task (requires: T1)
- [ ] T3: Third task (Requires: T1, T2)
`
	tasks := ExtractTasks(content)

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// T1 has no dependencies
	if len(tasks[0].Requires) != 0 {
		t.Errorf("expected 0 dependencies for T1, got %d", len(tasks[0].Requires))
	}

	// T2 requires T1
	if len(tasks[1].Requires) != 1 {
		t.Fatalf("expected 1 dependency for T2, got %d", len(tasks[1].Requires))
	}
	if tasks[1].Requires[0] != "T1" {
		t.Errorf("expected T1 dependency, got %q", tasks[1].Requires[0])
	}

	// T3 requires T1 and T2
	if len(tasks[2].Requires) != 2 {
		t.Fatalf("expected 2 dependencies for T3, got %d", len(tasks[2].Requires))
	}
	if tasks[2].Requires[0] != "T1" || tasks[2].Requires[1] != "T2" {
		t.Errorf("expected [T1, T2] dependencies, got %v", tasks[2].Requires)
	}
}

func TestExtractTasks_MixedCompleteIncomplete(t *testing.T) {
	content := `# Test Plan

**Status:** open

## Tasks
- [x] Completed task
- [ ] Incomplete task
- [X] Also completed (uppercase X)
- [ ] Another incomplete
`
	tasks := ExtractTasks(content)

	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}

	if !tasks[0].Complete {
		t.Error("expected task 1 to be complete")
	}
	if tasks[1].Complete {
		t.Error("expected task 2 to be incomplete")
	}
	if !tasks[2].Complete {
		t.Error("expected task 3 to be complete (uppercase X)")
	}
	if tasks[3].Complete {
		t.Error("expected task 4 to be incomplete")
	}
}

func TestExtractTasks_EmptyContent(t *testing.T) {
	tasks := ExtractTasks("")
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks for empty content, got %d", len(tasks))
	}
}

func TestExtractTasks_NoCheckboxes(t *testing.T) {
	content := `# Test Plan

This is a plan with no checkboxes.

- Regular list item
- Another list item
`
	tasks := ExtractTasks(content)
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks (no checkboxes), got %d", len(tasks))
	}
}

func TestExtractTasks_LineNumbers(t *testing.T) {
	content := `Line 1
Line 2
- [ ] Task on line 3
Line 4
- [x] Task on line 5
`
	tasks := ExtractTasks(content)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	if tasks[0].Line != 3 {
		t.Errorf("expected line 3, got %d", tasks[0].Line)
	}
	if tasks[1].Line != 5 {
		t.Errorf("expected line 5, got %d", tasks[1].Line)
	}
}

func TestExtractRequires(t *testing.T) {
	tests := []struct {
		text     string
		expected []string
	}{
		{"Simple task", nil},
		{"Task (requires: T1)", []string{"T1"}},
		{"Task (Requires: T1, T2)", []string{"T1", "T2"}},
		{"Task requires: T1, T2, T3", []string{"T1", "T2", "T3"}},
		{"Task (require: T10)", []string{"T10"}},
		{"No match here", nil},
	}

	for _, tc := range tests {
		result := extractRequires(tc.text)
		if len(result) != len(tc.expected) {
			t.Errorf("extractRequires(%q): expected %v, got %v", tc.text, tc.expected, result)
			continue
		}
		for i := range tc.expected {
			if result[i] != tc.expected[i] {
				t.Errorf("extractRequires(%q)[%d]: expected %q, got %q", tc.text, i, tc.expected[i], result[i])
			}
		}
	}
}

func TestCountComplete(t *testing.T) {
	tasks := []Task{
		{Complete: true, Subtasks: []Task{
			{Complete: true},
			{Complete: false},
		}},
		{Complete: false},
	}

	count := CountComplete(tasks)
	if count != 2 {
		t.Errorf("expected 2 complete tasks, got %d", count)
	}
}

func TestCountTotal(t *testing.T) {
	tasks := []Task{
		{Complete: true, Subtasks: []Task{
			{Complete: true},
			{Complete: false},
		}},
		{Complete: false},
	}

	count := CountTotal(tasks)
	if count != 4 {
		t.Errorf("expected 4 total tasks, got %d", count)
	}
}

func TestFindNextIncomplete(t *testing.T) {
	tasks := []Task{
		{Text: "T1", Complete: true},
		{Text: "T2", Complete: false, Requires: []string{"T1"}},
		{Text: "T3", Complete: false, Requires: []string{"T2"}},
	}

	completedIDs := map[string]bool{"T1": true}

	next := FindNextIncomplete(tasks, completedIDs)
	if next == nil {
		t.Fatal("expected to find next incomplete task")
	}
	if next.Text != "T2" {
		t.Errorf("expected T2, got %q", next.Text)
	}

	// Mark T2 as complete (both in completedIDs and the task itself)
	completedIDs["T2"] = true
	tasks[1].Complete = true
	next = FindNextIncomplete(tasks, completedIDs)
	if next == nil {
		t.Fatal("expected to find T3")
	}
	if next.Text != "T3" {
		t.Errorf("expected T3, got %q", next.Text)
	}

	// All complete, no next
	tasks[2].Complete = true
	next = FindNextIncomplete(tasks, completedIDs)
	if next != nil {
		t.Errorf("expected nil when all complete, got %q", next.Text)
	}
}

func TestExtractTasks_RealWorldPlan(t *testing.T) {
	// Test with a plan format similar to the actual Go rewrite plan
	content := `# Plan: Test

**Status:** open

## Tasks

### T1: Initialize project
**Requires:** —
**Status:** complete

**Done when:**
- [x] go.mod exists
- [x] Directory structure created

**Subtasks:**
- [x] Run go mod init
- [x] Create directories

---

### T2: Implement logging
**Requires:** T1
**Status:** open

**Done when:**
- [ ] Logger interface defined
- [ ] Unit tests pass

**Subtasks:**
- [ ] Define Logger interface
- [ ] Implement ConsoleLogger
`
	tasks := ExtractTasks(content)

	// Should find 8 checkboxes total
	total := CountTotal(tasks)
	if total != 8 {
		t.Errorf("expected 8 total tasks, got %d", total)
	}

	// Should find 4 complete
	complete := CountComplete(tasks)
	if complete != 4 {
		t.Errorf("expected 4 complete tasks, got %d", complete)
	}
}
