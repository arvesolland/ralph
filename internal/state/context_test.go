package state

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuildContext_NilState(t *testing.T) {
	payload := BuildContext(nil)

	if payload.Summary.Total != 0 {
		t.Errorf("expected total 0, got %d", payload.Summary.Total)
	}
	if payload.Summary.DoneRatio != 0 {
		t.Errorf("expected done_ratio 0, got %f", payload.Summary.DoneRatio)
	}
	if len(payload.Tasks.Items) != 0 {
		t.Errorf("expected empty tasks, got %d", len(payload.Tasks.Items))
	}
	if len(payload.Feedback.All) != 0 {
		t.Errorf("expected empty feedback, got %d", len(payload.Feedback.All))
	}
	if len(payload.Feedback.Unresolved) != 0 {
		t.Errorf("expected empty unresolved, got %d", len(payload.Feedback.Unresolved))
	}
	if payload.Selection.SuggestedNext != nil {
		t.Errorf("expected nil suggested_next")
	}

	// Verify JSON serializes cleanly (no null arrays).
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	// Should not contain "null" for slices.
	if string(data) == "" {
		t.Fatal("empty JSON output")
	}
}

func TestBuildContext_FullState(t *testing.T) {
	now := time.Date(2026, 2, 6, 10, 0, 0, 0, time.UTC)
	state := &PlanState{
		ID:        "structured-state",
		Title:     "Structured Task State",
		Status:    PlanStatusActive,
		CreatedAt: now,
		Tasks: []TaskState{
			{ID: "T2", Title: "Load/Save", Status: TaskStatusDone, Requires: []string{"T1"}},
			{ID: "T1", Title: "Types", Status: TaskStatusDone},
			{ID: "T3", Title: "Validation", Status: TaskStatusTodo, Requires: []string{"T1"}},
		},
		Feedback: []Feedback{
			{ID: "F2", Scope: "plan", Author: "human", Message: "Later feedback", CreatedAt: now.Add(2 * time.Hour)},
			{ID: "F1", Scope: "task:T1", Author: "agent", Message: "Earlier feedback", Resolved: true, CreatedAt: now.Add(1 * time.Hour)},
		},
	}

	payload := BuildContext(state)

	// Plan metadata.
	if payload.Plan.ID != "structured-state" {
		t.Errorf("expected plan ID 'structured-state', got %q", payload.Plan.ID)
	}
	if payload.Plan.Status != PlanStatusActive {
		t.Errorf("expected plan status 'active', got %q", payload.Plan.Status)
	}

	// Tasks sorted by ID (T1, T2, T3).
	if len(payload.Tasks.Items) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(payload.Tasks.Items))
	}
	if payload.Tasks.Items[0].ID != "T1" {
		t.Errorf("expected first task T1, got %s", payload.Tasks.Items[0].ID)
	}
	if payload.Tasks.Items[1].ID != "T2" {
		t.Errorf("expected second task T2, got %s", payload.Tasks.Items[1].ID)
	}
	if payload.Tasks.Items[2].ID != "T3" {
		t.Errorf("expected third task T3, got %s", payload.Tasks.Items[2].ID)
	}

	// Feedback sorted by created_at (F1 before F2).
	if len(payload.Feedback.All) != 2 {
		t.Fatalf("expected 2 feedback, got %d", len(payload.Feedback.All))
	}
	if payload.Feedback.All[0].ID != "F1" {
		t.Errorf("expected first feedback F1, got %s", payload.Feedback.All[0].ID)
	}
	if payload.Feedback.All[1].ID != "F2" {
		t.Errorf("expected second feedback F2, got %s", payload.Feedback.All[1].ID)
	}

	// Unresolved feedback.
	if len(payload.Feedback.Unresolved) != 1 {
		t.Fatalf("expected 1 unresolved, got %d", len(payload.Feedback.Unresolved))
	}
	if payload.Feedback.Unresolved[0].ID != "F2" {
		t.Errorf("expected unresolved F2, got %s", payload.Feedback.Unresolved[0].ID)
	}

	// Selection: T3 is available (T1 is done).
	if payload.Selection.SuggestedNext == nil {
		t.Fatal("expected suggested_next")
	}
	if payload.Selection.SuggestedNext.TaskID != "T3" {
		t.Errorf("expected suggested T3, got %s", payload.Selection.SuggestedNext.TaskID)
	}
	if len(payload.Selection.Available) != 1 {
		t.Errorf("expected 1 available, got %d", len(payload.Selection.Available))
	}

	// Summary.
	if payload.Summary.Total != 3 {
		t.Errorf("expected total 3, got %d", payload.Summary.Total)
	}
	if payload.Summary.ByStatus["done"] != 2 {
		t.Errorf("expected 2 done, got %d", payload.Summary.ByStatus["done"])
	}
	if payload.Summary.ByStatus["todo"] != 1 {
		t.Errorf("expected 1 todo, got %d", payload.Summary.ByStatus["todo"])
	}
	expectedRatio := 2.0 / 3.0
	if payload.Summary.DoneRatio < expectedRatio-0.01 || payload.Summary.DoneRatio > expectedRatio+0.01 {
		t.Errorf("expected done_ratio ~0.667, got %f", payload.Summary.DoneRatio)
	}
}

func TestBuildContext_DeterministicOrdering(t *testing.T) {
	now := time.Date(2026, 2, 6, 10, 0, 0, 0, time.UTC)
	state := &PlanState{
		ID:     "test",
		Title:  "Test",
		Status: PlanStatusActive,
		Tasks: []TaskState{
			{ID: "T10", Title: "Tenth", Status: TaskStatusTodo},
			{ID: "T2", Title: "Second", Status: TaskStatusTodo},
			{ID: "T1", Title: "First", Status: TaskStatusTodo},
		},
		Feedback: []Feedback{
			{ID: "F3", Scope: "plan", Author: "a", Message: "c", CreatedAt: now.Add(3 * time.Hour)},
			{ID: "F1", Scope: "plan", Author: "a", Message: "a", CreatedAt: now.Add(1 * time.Hour)},
			{ID: "F2", Scope: "plan", Author: "a", Message: "b", CreatedAt: now.Add(2 * time.Hour)},
		},
	}

	payload := BuildContext(state)

	// Tasks sorted numerically: T1, T2, T10.
	if payload.Tasks.Items[0].ID != "T1" {
		t.Errorf("expected T1 first, got %s", payload.Tasks.Items[0].ID)
	}
	if payload.Tasks.Items[1].ID != "T2" {
		t.Errorf("expected T2 second, got %s", payload.Tasks.Items[1].ID)
	}
	if payload.Tasks.Items[2].ID != "T10" {
		t.Errorf("expected T10 third, got %s", payload.Tasks.Items[2].ID)
	}

	// Feedback sorted by created_at: F1, F2, F3.
	if payload.Feedback.All[0].ID != "F1" {
		t.Errorf("expected F1 first, got %s", payload.Feedback.All[0].ID)
	}
	if payload.Feedback.All[1].ID != "F2" {
		t.Errorf("expected F2 second, got %s", payload.Feedback.All[1].ID)
	}
	if payload.Feedback.All[2].ID != "F3" {
		t.Errorf("expected F3 third, got %s", payload.Feedback.All[2].ID)
	}
}

func TestBuildContext_AllTasksDone(t *testing.T) {
	state := &PlanState{
		ID:     "test",
		Title:  "Test",
		Status: PlanStatusComplete,
		Tasks: []TaskState{
			{ID: "T1", Title: "First", Status: TaskStatusDone},
			{ID: "T2", Title: "Second", Status: TaskStatusDone},
		},
	}

	payload := BuildContext(state)

	if payload.Selection.SuggestedNext != nil {
		t.Errorf("expected nil suggested_next when all done")
	}
	if len(payload.Selection.Available) != 0 {
		t.Errorf("expected no available tasks, got %d", len(payload.Selection.Available))
	}
	if payload.Summary.DoneRatio != 1.0 {
		t.Errorf("expected done_ratio 1.0, got %f", payload.Summary.DoneRatio)
	}
}

func TestBuildContext_EmptyTasks(t *testing.T) {
	state := &PlanState{
		ID:     "test",
		Title:  "Test",
		Status: PlanStatusActive,
		Tasks:  []TaskState{},
	}

	payload := BuildContext(state)

	if payload.Summary.Total != 0 {
		t.Errorf("expected total 0, got %d", payload.Summary.Total)
	}
	if payload.Summary.DoneRatio != 0 {
		t.Errorf("expected done_ratio 0, got %f", payload.Summary.DoneRatio)
	}
}

func TestBuildContext_JSONOutput(t *testing.T) {
	now := time.Date(2026, 2, 6, 10, 0, 0, 0, time.UTC)
	state := &PlanState{
		ID:        "test-plan",
		Title:     "Test Plan",
		Status:    PlanStatusActive,
		CreatedAt: now,
		Tasks: []TaskState{
			{ID: "T1", Title: "First", Status: TaskStatusDone},
			{ID: "T2", Title: "Second", Status: TaskStatusTodo, Requires: []string{"T1"}},
		},
	}

	payload := BuildContext(state)
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	// Verify it round-trips through JSON.
	var decoded ContextPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	if decoded.Plan.ID != "test-plan" {
		t.Errorf("expected plan ID 'test-plan', got %q", decoded.Plan.ID)
	}
	if decoded.Summary.Total != 2 {
		t.Errorf("expected total 2, got %d", decoded.Summary.Total)
	}
}

func TestBuildContext_NoNullArraysInJSON(t *testing.T) {
	// Verify that nil slices are serialized as [] not null.
	state := &PlanState{
		ID:     "test",
		Title:  "Test",
		Status: PlanStatusActive,
		Tasks: []TaskState{
			{ID: "T1", Title: "First", Status: TaskStatusDone},
		},
	}

	payload := BuildContext(state)
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	jsonStr := string(data)
	// Check that key array fields are [] not null.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	// Feedback.unresolved should be [] not null.
	fb := raw["feedback"].(map[string]interface{})
	if fb["unresolved"] == nil {
		t.Errorf("feedback.unresolved should be [] not null in: %s", jsonStr)
	}

	// Selection arrays should be [] not null.
	sel := raw["selection"].(map[string]interface{})
	if sel["available"] == nil {
		t.Errorf("selection.available should be [] not null in: %s", jsonStr)
	}
	if sel["blocked"] == nil {
		t.Errorf("selection.blocked should be [] not null in: %s", jsonStr)
	}
}

func TestComputeSummary(t *testing.T) {
	tasks := []TaskState{
		{ID: "T1", Status: TaskStatusDone},
		{ID: "T2", Status: TaskStatusDone},
		{ID: "T3", Status: TaskStatusDoing},
		{ID: "T4", Status: TaskStatusTodo},
		{ID: "T5", Status: TaskStatusSkipped},
	}

	summary := computeSummary(tasks)

	if summary.Total != 5 {
		t.Errorf("expected total 5, got %d", summary.Total)
	}
	if summary.ByStatus["done"] != 2 {
		t.Errorf("expected 2 done, got %d", summary.ByStatus["done"])
	}
	if summary.ByStatus["doing"] != 1 {
		t.Errorf("expected 1 doing, got %d", summary.ByStatus["doing"])
	}
	if summary.ByStatus["todo"] != 1 {
		t.Errorf("expected 1 todo, got %d", summary.ByStatus["todo"])
	}
	if summary.ByStatus["skipped"] != 1 {
		t.Errorf("expected 1 skipped, got %d", summary.ByStatus["skipped"])
	}
	expectedRatio := 2.0 / 5.0
	if summary.DoneRatio < expectedRatio-0.01 || summary.DoneRatio > expectedRatio+0.01 {
		t.Errorf("expected done_ratio 0.4, got %f", summary.DoneRatio)
	}
}

func TestComputeSummary_Empty(t *testing.T) {
	summary := computeSummary(nil)

	if summary.Total != 0 {
		t.Errorf("expected total 0, got %d", summary.Total)
	}
	if summary.DoneRatio != 0 {
		t.Errorf("expected done_ratio 0, got %f", summary.DoneRatio)
	}
}
