package state

import (
	"strings"
	"testing"
)

func TestValidatePlanTransition_ValidTransitions(t *testing.T) {
	valid := []struct {
		from, to PlanStatus
	}{
		{PlanStatusDraft, PlanStatusReady},
		{PlanStatusReady, PlanStatusActive},
		{PlanStatusActive, PlanStatusBlocked},
		{PlanStatusActive, PlanStatusComplete},
		{PlanStatusBlocked, PlanStatusActive},
	}
	for _, tc := range valid {
		t.Run(string(tc.from)+"→"+string(tc.to), func(t *testing.T) {
			if err := ValidatePlanTransition(tc.from, tc.to); err != nil {
				t.Errorf("expected valid transition %s→%s, got error: %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestValidatePlanTransition_InvalidTransitions(t *testing.T) {
	invalid := []struct {
		from, to PlanStatus
	}{
		{PlanStatusDraft, PlanStatusActive},     // must go through ready
		{PlanStatusDraft, PlanStatusComplete},    // must go through ready→active
		{PlanStatusReady, PlanStatusBlocked},     // can't block before active
		{PlanStatusComplete, PlanStatusActive},   // terminal state
		{PlanStatusComplete, PlanStatusDraft},    // terminal state
		{PlanStatusBlocked, PlanStatusComplete},  // must unblock first
		{PlanStatusActive, PlanStatusDraft},      // can't go backward
		{PlanStatusActive, PlanStatusReady},      // can't go backward
		{PlanStatusDraft, PlanStatusDraft},       // no self-transition
	}
	for _, tc := range invalid {
		t.Run(string(tc.from)+"→"+string(tc.to), func(t *testing.T) {
			if err := ValidatePlanTransition(tc.from, tc.to); err == nil {
				t.Errorf("expected error for invalid transition %s→%s", tc.from, tc.to)
			}
		})
	}
}

func TestValidatePlanTransition_InvalidStatuses(t *testing.T) {
	if err := ValidatePlanTransition("bogus", PlanStatusActive); err == nil {
		t.Error("expected error for invalid source status")
	} else if !strings.Contains(err.Error(), "invalid source") {
		t.Errorf("expected 'invalid source' in error, got: %v", err)
	}

	if err := ValidatePlanTransition(PlanStatusDraft, "bogus"); err == nil {
		t.Error("expected error for invalid target status")
	} else if !strings.Contains(err.Error(), "invalid target") {
		t.Errorf("expected 'invalid target' in error, got: %v", err)
	}
}

func TestValidateTaskTransition_ValidTransitions(t *testing.T) {
	valid := []struct {
		from, to TaskStatus
	}{
		{TaskStatusTodo, TaskStatusClaimed},
		{TaskStatusTodo, TaskStatusSkipped},
		{TaskStatusClaimed, TaskStatusDoing},
		{TaskStatusClaimed, TaskStatusSkipped},
		{TaskStatusDoing, TaskStatusBlocked},
		{TaskStatusDoing, TaskStatusDone},
		{TaskStatusDoing, TaskStatusSkipped},
		{TaskStatusBlocked, TaskStatusDoing},
		{TaskStatusBlocked, TaskStatusSkipped},
	}
	for _, tc := range valid {
		t.Run(string(tc.from)+"→"+string(tc.to), func(t *testing.T) {
			if err := ValidateTaskTransition(tc.from, tc.to); err != nil {
				t.Errorf("expected valid transition %s→%s, got error: %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestValidateTaskTransition_InvalidTransitions(t *testing.T) {
	invalid := []struct {
		from, to TaskStatus
	}{
		{TaskStatusTodo, TaskStatusDoing},        // must claim first
		{TaskStatusTodo, TaskStatusDone},          // must claim→doing first
		{TaskStatusTodo, TaskStatusBlocked},       // must be doing to block
		{TaskStatusClaimed, TaskStatusDone},       // must do first
		{TaskStatusClaimed, TaskStatusBlocked},    // must be doing to block
		{TaskStatusDone, TaskStatusTodo},          // terminal
		{TaskStatusDone, TaskStatusDoing},         // terminal
		{TaskStatusSkipped, TaskStatusTodo},       // terminal
		{TaskStatusSkipped, TaskStatusDoing},      // terminal
		{TaskStatusBlocked, TaskStatusDone},       // must unblock and do
		{TaskStatusBlocked, TaskStatusClaimed},    // must go to doing
		{TaskStatusDoing, TaskStatusTodo},         // can't go backward
		{TaskStatusDoing, TaskStatusClaimed},      // can't go backward
	}
	for _, tc := range invalid {
		t.Run(string(tc.from)+"→"+string(tc.to), func(t *testing.T) {
			if err := ValidateTaskTransition(tc.from, tc.to); err == nil {
				t.Errorf("expected error for invalid transition %s→%s", tc.from, tc.to)
			}
		})
	}
}

func TestValidateTaskTransition_InvalidStatuses(t *testing.T) {
	if err := ValidateTaskTransition("bogus", TaskStatusDoing); err == nil {
		t.Error("expected error for invalid source status")
	}
	if err := ValidateTaskTransition(TaskStatusTodo, "bogus"); err == nil {
		t.Error("expected error for invalid target status")
	}
}

func TestValidateDependencies_Valid(t *testing.T) {
	tasks := []TaskState{
		{ID: "T1"},
		{ID: "T2", Requires: []string{"T1"}},
		{ID: "T3", Requires: []string{"T1", "T2"}},
	}
	if err := ValidateDependencies(tasks); err != nil {
		t.Errorf("expected no error for valid deps, got: %v", err)
	}
}

func TestValidateDependencies_NoDeps(t *testing.T) {
	tasks := []TaskState{
		{ID: "T1"},
		{ID: "T2"},
	}
	if err := ValidateDependencies(tasks); err != nil {
		t.Errorf("expected no error for tasks without deps, got: %v", err)
	}
}

func TestValidateDependencies_MissingID(t *testing.T) {
	tasks := []TaskState{
		{ID: "T1"},
		{ID: "T2", Requires: []string{"T99"}},
	}
	err := ValidateDependencies(tasks)
	if err == nil {
		t.Fatal("expected error for missing dependency ID")
	}
	if !strings.Contains(err.Error(), "T99") {
		t.Errorf("expected error to mention T99, got: %v", err)
	}
}

func TestValidateDependencies_DirectCycle(t *testing.T) {
	tasks := []TaskState{
		{ID: "T1", Requires: []string{"T2"}},
		{ID: "T2", Requires: []string{"T1"}},
	}
	err := ValidateDependencies(tasks)
	if err == nil {
		t.Fatal("expected error for direct cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected 'cycle' in error, got: %v", err)
	}
}

func TestValidateDependencies_IndirectCycle(t *testing.T) {
	tasks := []TaskState{
		{ID: "T1", Requires: []string{"T3"}},
		{ID: "T2", Requires: []string{"T1"}},
		{ID: "T3", Requires: []string{"T2"}},
	}
	err := ValidateDependencies(tasks)
	if err == nil {
		t.Fatal("expected error for indirect cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected 'cycle' in error, got: %v", err)
	}
}

func TestValidateDependencies_SelfCycle(t *testing.T) {
	tasks := []TaskState{
		{ID: "T1", Requires: []string{"T1"}},
	}
	err := ValidateDependencies(tasks)
	if err == nil {
		t.Fatal("expected error for self-referencing dependency")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected 'cycle' in error, got: %v", err)
	}
}

func TestValidateCompletion_AllCriteriaMet(t *testing.T) {
	task := &TaskState{
		ID: "T1",
		Criteria: []Criterion{
			{Text: "tests pass", Done: true},
			{Text: "docs updated", Done: true},
		},
	}
	if err := ValidateCompletion(task); err != nil {
		t.Errorf("expected no error when all criteria met, got: %v", err)
	}
}

func TestValidateCompletion_CriterionNotMet(t *testing.T) {
	task := &TaskState{
		ID: "T1",
		Criteria: []Criterion{
			{Text: "tests pass", Done: true},
			{Text: "docs updated", Done: false},
		},
	}
	err := ValidateCompletion(task)
	if err == nil {
		t.Fatal("expected error when criterion not met")
	}
	if !strings.Contains(err.Error(), "criterion 2") {
		t.Errorf("expected 'criterion 2' in error (1-indexed), got: %v", err)
	}
	if !strings.Contains(err.Error(), "docs updated") {
		t.Errorf("expected criterion text in error, got: %v", err)
	}
}

func TestValidateCompletion_NoCriteria(t *testing.T) {
	task := &TaskState{ID: "T1"}
	if err := ValidateCompletion(task); err != nil {
		t.Errorf("expected no error for task without criteria, got: %v", err)
	}
}

func TestValidateFeedbackScope_Plan(t *testing.T) {
	tasks := []TaskState{{ID: "T1"}}
	if err := ValidateFeedbackScope("plan", tasks); err != nil {
		t.Errorf("expected 'plan' scope to be valid, got: %v", err)
	}
}

func TestValidateFeedbackScope_ValidTask(t *testing.T) {
	tasks := []TaskState{{ID: "T1"}, {ID: "T2"}}
	if err := ValidateFeedbackScope("task:T2", tasks); err != nil {
		t.Errorf("expected 'task:T2' scope to be valid, got: %v", err)
	}
}

func TestValidateFeedbackScope_UnknownTask(t *testing.T) {
	tasks := []TaskState{{ID: "T1"}}
	err := ValidateFeedbackScope("task:T99", tasks)
	if err == nil {
		t.Fatal("expected error for unknown task scope")
	}
	if !strings.Contains(err.Error(), "T99") {
		t.Errorf("expected T99 in error, got: %v", err)
	}
}

func TestValidateFeedbackScope_InvalidFormat(t *testing.T) {
	tasks := []TaskState{{ID: "T1"}}
	invalid := []string{"", "global", "task:", "PLAN", "task"}
	for _, scope := range invalid {
		t.Run(scope, func(t *testing.T) {
			if err := ValidateFeedbackScope(scope, tasks); err == nil {
				t.Errorf("expected error for invalid scope %q", scope)
			}
		})
	}
}
