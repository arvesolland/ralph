package state

import "testing"

func feedbackTestState() *PlanState {
	return &PlanState{
		ID:     "test-plan",
		Title:  "Test Plan",
		Status: PlanStatusActive,
		Tasks: []TaskState{
			{ID: "T1", Title: "First task", Status: TaskStatusDone},
			{ID: "T2", Title: "Second task", Status: TaskStatusDoing},
		},
	}
}

// --- AddFeedback ---

func TestAddFeedbackPlanScope(t *testing.T) {
	s := feedbackTestState()
	fb, err := AddFeedback(s, "plan", "human", "looks good overall")
	if err != nil {
		t.Fatalf("AddFeedback error: %v", err)
	}
	if fb.ID != "F1" {
		t.Errorf("ID = %q, want F1", fb.ID)
	}
	if fb.Scope != "plan" {
		t.Errorf("Scope = %q, want plan", fb.Scope)
	}
	if fb.Author != "human" {
		t.Errorf("Author = %q, want human", fb.Author)
	}
	if fb.Message != "looks good overall" {
		t.Errorf("Message = %q, want %q", fb.Message, "looks good overall")
	}
	if fb.Resolved {
		t.Error("new feedback should not be resolved")
	}
	if fb.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if len(s.Feedback) != 1 {
		t.Errorf("feedback count = %d, want 1", len(s.Feedback))
	}
}

func TestAddFeedbackTaskScope(t *testing.T) {
	s := feedbackTestState()
	fb, err := AddFeedback(s, "task:T2", "agent", "needs clarification")
	if err != nil {
		t.Fatalf("AddFeedback error: %v", err)
	}
	if fb.ID != "F1" {
		t.Errorf("ID = %q, want F1", fb.ID)
	}
	if fb.Scope != "task:T2" {
		t.Errorf("Scope = %q, want task:T2", fb.Scope)
	}
}

func TestAddFeedbackAutoIncrementID(t *testing.T) {
	s := feedbackTestState()
	_, _ = AddFeedback(s, "plan", "human", "first")
	fb2, err := AddFeedback(s, "plan", "human", "second")
	if err != nil {
		t.Fatalf("AddFeedback error: %v", err)
	}
	if fb2.ID != "F2" {
		t.Errorf("second feedback ID = %q, want F2", fb2.ID)
	}
}

func TestAddFeedbackInvalidScope(t *testing.T) {
	s := feedbackTestState()
	_, err := AddFeedback(s, "task:T99", "human", "bad scope")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestAddFeedbackEmptyMessage(t *testing.T) {
	s := feedbackTestState()
	_, err := AddFeedback(s, "plan", "human", "")
	if err == nil {
		t.Fatal("expected error for empty message")
	}
}

func TestAddFeedbackEmptyScope(t *testing.T) {
	s := feedbackTestState()
	_, err := AddFeedback(s, "", "human", "some message")
	if err == nil {
		t.Fatal("expected error for empty scope")
	}
}

// --- ResolveFeedback ---

func TestResolveFeedback(t *testing.T) {
	s := feedbackTestState()
	_, _ = AddFeedback(s, "plan", "human", "fix this")

	err := ResolveFeedback(s, "F1")
	if err != nil {
		t.Fatalf("ResolveFeedback error: %v", err)
	}
	if !s.Feedback[0].Resolved {
		t.Error("feedback should be resolved")
	}
	if s.Feedback[0].ResolvedAt == nil {
		t.Error("ResolvedAt should be set")
	}
}

func TestResolveFeedbackNotFound(t *testing.T) {
	s := feedbackTestState()
	err := ResolveFeedback(s, "F99")
	if err == nil {
		t.Fatal("expected error for missing feedback")
	}
}

func TestResolveFeedbackAlreadyResolved(t *testing.T) {
	s := feedbackTestState()
	_, _ = AddFeedback(s, "plan", "human", "fix this")
	_ = ResolveFeedback(s, "F1")

	err := ResolveFeedback(s, "F1")
	if err == nil {
		t.Fatal("expected error for already resolved feedback")
	}
}

// --- UnresolvedFeedback ---

func TestUnresolvedFeedback(t *testing.T) {
	s := feedbackTestState()
	_, _ = AddFeedback(s, "plan", "human", "first")
	_, _ = AddFeedback(s, "plan", "human", "second")
	_ = ResolveFeedback(s, "F1")

	unresolved := UnresolvedFeedback(s)
	if len(unresolved) != 1 {
		t.Fatalf("unresolved count = %d, want 1", len(unresolved))
	}
	if unresolved[0].ID != "F2" {
		t.Errorf("unresolved[0].ID = %q, want F2", unresolved[0].ID)
	}
}

func TestUnresolvedFeedbackEmpty(t *testing.T) {
	s := feedbackTestState()
	unresolved := UnresolvedFeedback(s)
	if len(unresolved) != 0 {
		t.Errorf("unresolved count = %d, want 0", len(unresolved))
	}
}

func TestUnresolvedFeedbackAllResolved(t *testing.T) {
	s := feedbackTestState()
	_, _ = AddFeedback(s, "plan", "human", "one")
	_ = ResolveFeedback(s, "F1")

	unresolved := UnresolvedFeedback(s)
	if len(unresolved) != 0 {
		t.Errorf("unresolved count = %d, want 0", len(unresolved))
	}
}

// --- FeedbackForTask ---

func TestFeedbackForTask(t *testing.T) {
	s := feedbackTestState()
	_, _ = AddFeedback(s, "plan", "human", "plan-level")
	_, _ = AddFeedback(s, "task:T2", "human", "task-specific")
	_, _ = AddFeedback(s, "task:T1", "human", "other task")
	_, _ = AddFeedback(s, "task:T2", "agent", "another for T2")

	got := FeedbackForTask(s, "T2")
	if len(got) != 2 {
		t.Fatalf("FeedbackForTask(T2) count = %d, want 2", len(got))
	}
	if got[0].ID != "F2" {
		t.Errorf("got[0].ID = %q, want F2", got[0].ID)
	}
	if got[1].ID != "F4" {
		t.Errorf("got[1].ID = %q, want F4", got[1].ID)
	}
}

func TestFeedbackForTaskEmpty(t *testing.T) {
	s := feedbackTestState()
	_, _ = AddFeedback(s, "plan", "human", "plan-level only")

	got := FeedbackForTask(s, "T1")
	if len(got) != 0 {
		t.Errorf("FeedbackForTask(T1) count = %d, want 0", len(got))
	}
}

func TestFeedbackForTaskNoFeedback(t *testing.T) {
	s := feedbackTestState()
	got := FeedbackForTask(s, "T1")
	if len(got) != 0 {
		t.Errorf("FeedbackForTask count = %d, want 0", len(got))
	}
}
