package state

import (
	"testing"
	"time"
)

func testState() *PlanState {
	return &PlanState{
		ID:     "test-plan",
		Title:  "Test Plan",
		Status: PlanStatusActive,
		Tasks: []TaskState{
			{ID: "T1", Title: "First task", Status: TaskStatusDone, Criteria: []Criterion{
				{Text: "criterion 1", Done: true},
			}},
			{ID: "T2", Title: "Second task", Status: TaskStatusTodo, Requires: []string{"T1"}, Criteria: []Criterion{
				{Text: "criterion A", Done: false},
				{Text: "criterion B", Done: false},
			}},
			{ID: "T3", Title: "Third task", Status: TaskStatusTodo, Requires: []string{"T1", "T2"}},
		},
	}
}

// --- nextTaskID ---

func TestNextTaskID(t *testing.T) {
	s := testState()
	got := nextTaskID(s)
	if got != "T4" {
		t.Errorf("nextTaskID = %q, want T4", got)
	}
}

func TestNextTaskIDEmpty(t *testing.T) {
	s := &PlanState{}
	got := nextTaskID(s)
	if got != "T1" {
		t.Errorf("nextTaskID (empty) = %q, want T1", got)
	}
}

// --- findTask ---

func TestFindTask(t *testing.T) {
	s := testState()
	task, err := findTask(s, "T2")
	if err != nil {
		t.Fatalf("findTask returned error: %v", err)
	}
	if task.Title != "Second task" {
		t.Errorf("findTask title = %q, want %q", task.Title, "Second task")
	}
}

func TestFindTaskNotFound(t *testing.T) {
	s := testState()
	_, err := findTask(s, "T99")
	if err == nil {
		t.Fatal("expected error for missing task")
	}
}

// --- AddTask ---

func TestAddTask(t *testing.T) {
	s := testState()
	task, err := AddTask(s, "New task", []string{"T1"}, []string{"crit 1", "crit 2"})
	if err != nil {
		t.Fatalf("AddTask error: %v", err)
	}
	if task.ID != "T4" {
		t.Errorf("AddTask ID = %q, want T4", task.ID)
	}
	if task.Status != TaskStatusTodo {
		t.Errorf("AddTask status = %q, want todo", task.Status)
	}
	if len(task.Criteria) != 2 {
		t.Errorf("AddTask criteria count = %d, want 2", len(task.Criteria))
	}
	if task.Criteria[0].Text != "crit 1" {
		t.Errorf("AddTask criteria[0] = %q, want %q", task.Criteria[0].Text, "crit 1")
	}
	if len(s.Tasks) != 4 {
		t.Errorf("state tasks count = %d, want 4", len(s.Tasks))
	}
}

func TestAddTaskEmptyTitle(t *testing.T) {
	s := testState()
	_, err := AddTask(s, "", nil, nil)
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestAddTaskUnknownDep(t *testing.T) {
	s := testState()
	_, err := AddTask(s, "Bad deps", []string{"T99"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
}

func TestAddTaskNoCriteria(t *testing.T) {
	s := testState()
	task, err := AddTask(s, "No criteria", nil, nil)
	if err != nil {
		t.Fatalf("AddTask error: %v", err)
	}
	if len(task.Criteria) != 0 {
		t.Errorf("AddTask criteria count = %d, want 0", len(task.Criteria))
	}
}

// --- ClaimTask ---

func TestClaimTask(t *testing.T) {
	s := testState()
	err := ClaimTask(s, "T2")
	if err != nil {
		t.Fatalf("ClaimTask error: %v", err)
	}
	task, _ := findTask(s, "T2")
	if task.Status != TaskStatusDoing {
		t.Errorf("ClaimTask status = %q, want doing", task.Status)
	}
	if task.StartedAt == nil {
		t.Error("ClaimTask should set StartedAt")
	}
}

func TestClaimTaskAlreadyDoing(t *testing.T) {
	s := testState()
	s.Tasks[1].Status = TaskStatusDoing
	err := ClaimTask(s, "T2")
	if err == nil {
		t.Fatal("expected error claiming task already doing")
	}
}

func TestClaimTaskUnmetDeps(t *testing.T) {
	s := testState()
	s.Tasks[0].Status = TaskStatusTodo // T1 not done
	err := ClaimTask(s, "T2")
	if err == nil {
		t.Fatal("expected error for unmet dependencies")
	}
}

func TestClaimTaskNotFound(t *testing.T) {
	s := testState()
	err := ClaimTask(s, "T99")
	if err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestClaimTaskSkippedDepOK(t *testing.T) {
	s := testState()
	s.Tasks[0].Status = TaskStatusSkipped // T1 skipped counts as resolved
	err := ClaimTask(s, "T2")
	if err != nil {
		t.Fatalf("ClaimTask with skipped dep: %v", err)
	}
}

// --- CheckCriterion ---

func TestCheckCriterion(t *testing.T) {
	s := testState()
	err := CheckCriterion(s, "T2", 1)
	if err != nil {
		t.Fatalf("CheckCriterion error: %v", err)
	}
	task, _ := findTask(s, "T2")
	if !task.Criteria[0].Done {
		t.Error("criterion 1 should be done")
	}
	if task.Criteria[0].DoneAt == nil {
		t.Error("criterion 1 DoneAt should be set")
	}
}

func TestCheckCriterionOutOfRange(t *testing.T) {
	s := testState()
	err := CheckCriterion(s, "T2", 0)
	if err == nil {
		t.Fatal("expected error for index 0")
	}
	err = CheckCriterion(s, "T2", 3)
	if err == nil {
		t.Fatal("expected error for index 3 (only 2 criteria)")
	}
}

func TestCheckCriterionNotFound(t *testing.T) {
	s := testState()
	err := CheckCriterion(s, "T99", 1)
	if err == nil {
		t.Fatal("expected error for missing task")
	}
}

// --- UncheckCriterion ---

func TestUncheckCriterion(t *testing.T) {
	s := testState()
	now := time.Now()
	s.Tasks[1].Criteria[0].Done = true
	s.Tasks[1].Criteria[0].DoneAt = &now

	err := UncheckCriterion(s, "T2", 1)
	if err != nil {
		t.Fatalf("UncheckCriterion error: %v", err)
	}
	task, _ := findTask(s, "T2")
	if task.Criteria[0].Done {
		t.Error("criterion 1 should not be done")
	}
	if task.Criteria[0].DoneAt != nil {
		t.Error("criterion 1 DoneAt should be nil")
	}
}

func TestUncheckCriterionOutOfRange(t *testing.T) {
	s := testState()
	err := UncheckCriterion(s, "T2", 5)
	if err == nil {
		t.Fatal("expected error for out of range index")
	}
}

// --- CompleteTask ---

func TestCompleteTask(t *testing.T) {
	s := testState()
	s.Tasks[1].Status = TaskStatusDoing
	s.Tasks[1].Criteria[0].Done = true
	s.Tasks[1].Criteria[1].Done = true

	err := CompleteTask(s, "T2", []string{"abc123"}, []string{"file.go"})
	if err != nil {
		t.Fatalf("CompleteTask error: %v", err)
	}
	task, _ := findTask(s, "T2")
	if task.Status != TaskStatusDone {
		t.Errorf("status = %q, want done", task.Status)
	}
	if task.DoneAt == nil {
		t.Error("DoneAt should be set")
	}
	if len(task.Artifacts.Commits) != 1 || task.Artifacts.Commits[0] != "abc123" {
		t.Errorf("commits = %v, want [abc123]", task.Artifacts.Commits)
	}
	if len(task.Artifacts.FilesTouched) != 1 || task.Artifacts.FilesTouched[0] != "file.go" {
		t.Errorf("files_touched = %v, want [file.go]", task.Artifacts.FilesTouched)
	}
}

func TestCompleteTaskCriteriaNotMet(t *testing.T) {
	s := testState()
	s.Tasks[1].Status = TaskStatusDoing
	// Criteria not checked
	err := CompleteTask(s, "T2", nil, nil)
	if err == nil {
		t.Fatal("expected error for unmet criteria")
	}
}

func TestCompleteTaskWrongStatus(t *testing.T) {
	s := testState()
	// T2 is still todo, can't transition to done
	err := CompleteTask(s, "T2", nil, nil)
	if err == nil {
		t.Fatal("expected error for wrong status transition")
	}
}

func TestCompleteTaskNoCriteria(t *testing.T) {
	s := testState()
	// T3 has no criteria — should be completable if doing
	s.Tasks[2].Status = TaskStatusDoing
	err := CompleteTask(s, "T3", nil, nil)
	if err != nil {
		t.Fatalf("CompleteTask (no criteria) error: %v", err)
	}
}

// --- SkipTask ---

func TestSkipTask(t *testing.T) {
	s := testState()
	err := SkipTask(s, "T2", "not needed")
	if err != nil {
		t.Fatalf("SkipTask error: %v", err)
	}
	task, _ := findTask(s, "T2")
	if task.Status != TaskStatusSkipped {
		t.Errorf("status = %q, want skipped", task.Status)
	}
	if len(task.Notes) != 1 || task.Notes[0] != "skipped: not needed" {
		t.Errorf("notes = %v, want [skipped: not needed]", task.Notes)
	}
}

func TestSkipTaskEmptyReason(t *testing.T) {
	s := testState()
	err := SkipTask(s, "T2", "")
	if err != nil {
		t.Fatalf("SkipTask error: %v", err)
	}
	task, _ := findTask(s, "T2")
	if len(task.Notes) != 0 {
		t.Errorf("notes should be empty when reason is blank, got %v", task.Notes)
	}
}

func TestSkipTaskAlreadyDone(t *testing.T) {
	s := testState()
	err := SkipTask(s, "T1", "don't need")
	if err == nil {
		t.Fatal("expected error skipping done task")
	}
}

// --- SetPlanStatus ---

func TestSetPlanStatus(t *testing.T) {
	s := testState()
	err := SetPlanStatus(s, PlanStatusComplete, "all done")
	if err != nil {
		t.Fatalf("SetPlanStatus error: %v", err)
	}
	if s.Status != PlanStatusComplete {
		t.Errorf("status = %q, want complete", s.Status)
	}
}

func TestSetPlanStatusInvalid(t *testing.T) {
	s := testState()
	err := SetPlanStatus(s, PlanStatusDraft, "revert")
	if err == nil {
		t.Fatal("expected error for invalid transition active→draft")
	}
}

// --- parseRequires / parseCriteria ---

func TestParseRequires(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"T1", 1},
		{"T1,T2", 2},
		{"T1, T2, T3", 3},
		{" , , ", 0},
	}
	for _, tc := range tests {
		got := parseRequires(tc.input)
		if len(got) != tc.want {
			t.Errorf("parseRequires(%q) = %v (len %d), want len %d", tc.input, got, len(got), tc.want)
		}
	}
}

func TestParseCriteria(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"single", 1},
		{"a;b;c", 3},
		{"a ; b ; c", 3},
	}
	for _, tc := range tests {
		got := parseCriteria(tc.input)
		if len(got) != tc.want {
			t.Errorf("parseCriteria(%q) = %v (len %d), want len %d", tc.input, got, len(got), tc.want)
		}
	}
}

// --- nextFeedbackNum ---

func TestNextFeedbackNum(t *testing.T) {
	s := &PlanState{
		Feedback: []Feedback{
			{ID: "F1"},
			{ID: "F3"},
		},
	}
	got := nextFeedbackNum(s)
	if got != 4 {
		t.Errorf("nextFeedbackNum = %d, want 4", got)
	}
}

func TestNextFeedbackNumEmpty(t *testing.T) {
	s := &PlanState{}
	got := nextFeedbackNum(s)
	if got != 1 {
		t.Errorf("nextFeedbackNum (empty) = %d, want 1", got)
	}
}
