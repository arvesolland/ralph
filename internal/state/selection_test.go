package state

import (
	"testing"
)

func TestComputeSelectionHappyPath(t *testing.T) {
	// T1 done, T2 depends on T1 → T2 should be available.
	s := &PlanState{
		Tasks: []TaskState{
			{ID: "T1", Status: TaskStatusDone},
			{ID: "T2", Status: TaskStatusTodo, Requires: []string{"T1"}},
		},
	}
	sel := ComputeSelection(s)

	if sel.SuggestedNext == nil {
		t.Fatal("expected suggested_next, got nil")
	}
	if sel.SuggestedNext.TaskID != "T2" {
		t.Errorf("suggested_next = %s, want T2", sel.SuggestedNext.TaskID)
	}
	if len(sel.Available) != 1 || sel.Available[0].TaskID != "T2" {
		t.Errorf("available = %v, want [T2]", sel.Available)
	}
	if len(sel.Blocked) != 0 {
		t.Errorf("blocked = %v, want empty", sel.Blocked)
	}
}

func TestComputeSelectionAllDone(t *testing.T) {
	s := &PlanState{
		Tasks: []TaskState{
			{ID: "T1", Status: TaskStatusDone},
			{ID: "T2", Status: TaskStatusDone},
		},
	}
	sel := ComputeSelection(s)

	if sel.SuggestedNext != nil {
		t.Errorf("expected nil suggested_next, got %v", sel.SuggestedNext)
	}
	if len(sel.Available) != 0 {
		t.Errorf("available = %v, want empty", sel.Available)
	}
	if len(sel.Blocked) != 0 {
		t.Errorf("blocked = %v, want empty", sel.Blocked)
	}
}

func TestComputeSelectionAllBlocked(t *testing.T) {
	// Circular dep: both depend on each other, neither done.
	s := &PlanState{
		Tasks: []TaskState{
			{ID: "T1", Status: TaskStatusTodo, Requires: []string{"T2"}},
			{ID: "T2", Status: TaskStatusTodo, Requires: []string{"T1"}},
		},
	}
	sel := ComputeSelection(s)

	if sel.SuggestedNext != nil {
		t.Errorf("expected nil suggested_next, got %v", sel.SuggestedNext)
	}
	if len(sel.Available) != 0 {
		t.Errorf("available = %v, want empty", sel.Available)
	}
	if len(sel.Blocked) != 2 {
		t.Errorf("blocked count = %d, want 2", len(sel.Blocked))
	}
}

func TestComputeSelectionDepChain(t *testing.T) {
	// T1 done, T2 depends on T1, T3 depends on T2.
	// Only T2 should be available; T3 is blocked.
	s := &PlanState{
		Tasks: []TaskState{
			{ID: "T1", Status: TaskStatusDone},
			{ID: "T2", Status: TaskStatusTodo, Requires: []string{"T1"}},
			{ID: "T3", Status: TaskStatusTodo, Requires: []string{"T2"}},
		},
	}
	sel := ComputeSelection(s)

	if sel.SuggestedNext == nil || sel.SuggestedNext.TaskID != "T2" {
		t.Errorf("suggested_next = %v, want T2", sel.SuggestedNext)
	}
	if len(sel.Available) != 1 || sel.Available[0].TaskID != "T2" {
		t.Errorf("available = %v, want [T2]", sel.Available)
	}
	if len(sel.Blocked) != 1 || sel.Blocked[0].TaskID != "T3" {
		t.Errorf("blocked = %v, want [T3]", sel.Blocked)
	}
}

func TestComputeSelectionParallelTasks(t *testing.T) {
	// T1 done, T2 and T3 both depend only on T1 → both available, T2 suggested first.
	s := &PlanState{
		Tasks: []TaskState{
			{ID: "T1", Status: TaskStatusDone},
			{ID: "T2", Status: TaskStatusTodo, Requires: []string{"T1"}},
			{ID: "T3", Status: TaskStatusTodo, Requires: []string{"T1"}},
		},
	}
	sel := ComputeSelection(s)

	if sel.SuggestedNext == nil || sel.SuggestedNext.TaskID != "T2" {
		t.Errorf("suggested_next = %v, want T2", sel.SuggestedNext)
	}
	if len(sel.Available) != 2 {
		t.Fatalf("available count = %d, want 2", len(sel.Available))
	}
	if sel.Available[0].TaskID != "T2" || sel.Available[1].TaskID != "T3" {
		t.Errorf("available = [%s, %s], want [T2, T3]", sel.Available[0].TaskID, sel.Available[1].TaskID)
	}
}

func TestComputeSelectionSkippedDepsCount(t *testing.T) {
	// T1 skipped, T2 depends on T1 → T2 should be available (skipped counts as resolved).
	s := &PlanState{
		Tasks: []TaskState{
			{ID: "T1", Status: TaskStatusSkipped},
			{ID: "T2", Status: TaskStatusTodo, Requires: []string{"T1"}},
		},
	}
	sel := ComputeSelection(s)

	if sel.SuggestedNext == nil || sel.SuggestedNext.TaskID != "T2" {
		t.Errorf("suggested_next = %v, want T2", sel.SuggestedNext)
	}
	if len(sel.Available) != 1 {
		t.Errorf("available count = %d, want 1", len(sel.Available))
	}
}

func TestComputeSelectionExplicitlyBlocked(t *testing.T) {
	// T1 has status=blocked (manually set), even with no deps.
	s := &PlanState{
		Tasks: []TaskState{
			{ID: "T1", Status: TaskStatusBlocked},
		},
	}
	sel := ComputeSelection(s)

	if sel.SuggestedNext != nil {
		t.Errorf("expected nil suggested_next, got %v", sel.SuggestedNext)
	}
	if len(sel.Available) != 0 {
		t.Errorf("available = %v, want empty", sel.Available)
	}
	if len(sel.Blocked) != 1 || sel.Blocked[0].TaskID != "T1" {
		t.Errorf("blocked = %v, want [T1 with blocked reason]", sel.Blocked)
	}
	if sel.Blocked[0].Reason != "task status is blocked" {
		t.Errorf("blocked reason = %q, want %q", sel.Blocked[0].Reason, "task status is blocked")
	}
}

func TestComputeSelectionInProgressSkipped(t *testing.T) {
	// Tasks that are doing or claimed should not appear in available or blocked.
	s := &PlanState{
		Tasks: []TaskState{
			{ID: "T1", Status: TaskStatusDoing},
			{ID: "T2", Status: TaskStatusClaimed},
			{ID: "T3", Status: TaskStatusTodo},
		},
	}
	sel := ComputeSelection(s)

	if sel.SuggestedNext == nil || sel.SuggestedNext.TaskID != "T3" {
		t.Errorf("suggested_next = %v, want T3", sel.SuggestedNext)
	}
	if len(sel.Available) != 1 || sel.Available[0].TaskID != "T3" {
		t.Errorf("available = %v, want [T3]", sel.Available)
	}
	if len(sel.Blocked) != 0 {
		t.Errorf("blocked = %v, want empty", sel.Blocked)
	}
}

func TestComputeSelectionNilState(t *testing.T) {
	sel := ComputeSelection(nil)

	if sel.SuggestedNext != nil {
		t.Errorf("expected nil suggested_next, got %v", sel.SuggestedNext)
	}
	if len(sel.Available) != 0 {
		t.Errorf("available = %v, want empty", sel.Available)
	}
	if len(sel.Blocked) != 0 {
		t.Errorf("blocked = %v, want empty", sel.Blocked)
	}
}

func TestComputeSelectionEmptyTasks(t *testing.T) {
	s := &PlanState{Tasks: nil}
	sel := ComputeSelection(s)

	if sel.SuggestedNext != nil {
		t.Errorf("expected nil suggested_next, got %v", sel.SuggestedNext)
	}
}

func TestComputeSelectionNumericSorting(t *testing.T) {
	// T10 should come after T2 in numeric order, not lexicographic.
	s := &PlanState{
		Tasks: []TaskState{
			{ID: "T10", Status: TaskStatusTodo},
			{ID: "T2", Status: TaskStatusTodo},
			{ID: "T1", Status: TaskStatusTodo},
		},
	}
	sel := ComputeSelection(s)

	if len(sel.Available) != 3 {
		t.Fatalf("available count = %d, want 3", len(sel.Available))
	}
	if sel.Available[0].TaskID != "T1" {
		t.Errorf("available[0] = %s, want T1", sel.Available[0].TaskID)
	}
	if sel.Available[1].TaskID != "T2" {
		t.Errorf("available[1] = %s, want T2", sel.Available[1].TaskID)
	}
	if sel.Available[2].TaskID != "T10" {
		t.Errorf("available[2] = %s, want T10", sel.Available[2].TaskID)
	}
	if sel.SuggestedNext.TaskID != "T1" {
		t.Errorf("suggested_next = %s, want T1", sel.SuggestedNext.TaskID)
	}
}

func TestComputeSelectionBlockedReason(t *testing.T) {
	// T2 depends on T1 which is not done — blocked reason should list unmet deps.
	s := &PlanState{
		Tasks: []TaskState{
			{ID: "T1", Status: TaskStatusTodo},
			{ID: "T2", Status: TaskStatusTodo, Requires: []string{"T1"}},
		},
	}
	sel := ComputeSelection(s)

	// T1 is available (no deps), T2 is blocked (T1 not done).
	if len(sel.Available) != 1 || sel.Available[0].TaskID != "T1" {
		t.Errorf("available = %v, want [T1]", sel.Available)
	}
	if len(sel.Blocked) != 1 || sel.Blocked[0].TaskID != "T2" {
		t.Fatalf("blocked = %v, want [T2]", sel.Blocked)
	}
	if sel.Blocked[0].Reason != "waiting on: T1" {
		t.Errorf("blocked reason = %q, want %q", sel.Blocked[0].Reason, "waiting on: T1")
	}
}

func TestParseTaskNum(t *testing.T) {
	tests := []struct {
		id   string
		want int
	}{
		{"T1", 1},
		{"T12", 12},
		{"T100", 100},
		{"T0", 0},
		{"", -1},
		{"X1", -1},
		{"T", -1},
		{"Tabc", -1},
	}
	for _, tt := range tests {
		got := parseTaskNum(tt.id)
		if got != tt.want {
			t.Errorf("parseTaskNum(%q) = %d, want %d", tt.id, got, tt.want)
		}
	}
}
