package state

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// nextTaskID scans existing tasks and returns the next T{n} ID.
func nextTaskID(state *PlanState) string {
	max := 0
	for _, t := range state.Tasks {
		if n := parseTaskNum(t.ID); n > max {
			max = n
		}
	}
	return fmt.Sprintf("T%d", max+1)
}

// findTask returns a pointer to the task with the given ID, or an error if not found.
func findTask(state *PlanState, taskID string) (*TaskState, error) {
	for i := range state.Tasks {
		if state.Tasks[i].ID == taskID {
			return &state.Tasks[i], nil
		}
	}
	return nil, fmt.Errorf("task %s not found", taskID)
}

// AddTask creates a new task with an auto-assigned ID and appends it to the plan.
func AddTask(state *PlanState, title string, requires []string, criteria []string) (*TaskState, error) {
	if title == "" {
		return nil, fmt.Errorf("task title cannot be empty")
	}

	// Validate that all required task IDs exist.
	if len(requires) > 0 {
		ids := make(map[string]bool, len(state.Tasks))
		for _, t := range state.Tasks {
			ids[t.ID] = true
		}
		for _, dep := range requires {
			if !ids[dep] {
				return nil, fmt.Errorf("required task %s not found", dep)
			}
		}
	}

	task := TaskState{
		ID:       nextTaskID(state),
		Title:    title,
		Status:   TaskStatusTodo,
		Requires: requires,
	}
	for _, c := range criteria {
		task.Criteria = append(task.Criteria, Criterion{Text: c})
	}

	state.Tasks = append(state.Tasks, task)
	return &state.Tasks[len(state.Tasks)-1], nil
}

// ClaimTask sets a task's status to doing. The task must be in todo status
// and all dependencies must be met (done or skipped).
func ClaimTask(state *PlanState, taskID string) error {
	task, err := findTask(state, taskID)
	if err != nil {
		return err
	}

	// Transition todo → claimed → doing (no leases in v1, so claim == start doing).
	if err := ValidateTaskTransition(task.Status, TaskStatusClaimed); err != nil {
		return fmt.Errorf("cannot claim task %s: %w", taskID, err)
	}

	// Check dependencies are met.
	doneIDs := make(map[string]bool, len(state.Tasks))
	for _, t := range state.Tasks {
		if t.Status == TaskStatusDone || t.Status == TaskStatusSkipped {
			doneIDs[t.ID] = true
		}
	}
	for _, dep := range task.Requires {
		if !doneIDs[dep] {
			return fmt.Errorf("cannot claim task %s: dependency %s is not done", taskID, dep)
		}
	}

	now := time.Now()
	task.Status = TaskStatusDoing
	task.StartedAt = &now
	return nil
}

// CheckCriterion marks a criterion as done (1-indexed).
func CheckCriterion(state *PlanState, taskID string, index int) error {
	task, err := findTask(state, taskID)
	if err != nil {
		return err
	}

	if index < 1 || index > len(task.Criteria) {
		return fmt.Errorf("criterion index %d out of range for task %s (has %d criteria)", index, taskID, len(task.Criteria))
	}

	now := time.Now()
	task.Criteria[index-1].Done = true
	task.Criteria[index-1].DoneAt = &now
	return nil
}

// UncheckCriterion marks a criterion as not done (1-indexed).
func UncheckCriterion(state *PlanState, taskID string, index int) error {
	task, err := findTask(state, taskID)
	if err != nil {
		return err
	}

	if index < 1 || index > len(task.Criteria) {
		return fmt.Errorf("criterion index %d out of range for task %s (has %d criteria)", index, taskID, len(task.Criteria))
	}

	task.Criteria[index-1].Done = false
	task.Criteria[index-1].DoneAt = nil
	return nil
}

// CompleteTask validates that all criteria are met, then sets the task to done.
func CompleteTask(state *PlanState, taskID string, commits []string, filesTouched []string) error {
	task, err := findTask(state, taskID)
	if err != nil {
		return err
	}

	if err := ValidateTaskTransition(task.Status, TaskStatusDone); err != nil {
		return fmt.Errorf("cannot complete task %s: %w", taskID, err)
	}

	if err := ValidateCompletion(task); err != nil {
		return err
	}

	now := time.Now()
	task.Status = TaskStatusDone
	task.DoneAt = &now
	task.Artifacts.Commits = append(task.Artifacts.Commits, commits...)
	task.Artifacts.FilesTouched = append(task.Artifacts.FilesTouched, filesTouched...)
	return nil
}

// SkipTask sets a task to skipped and records the reason in notes.
func SkipTask(state *PlanState, taskID string, reason string) error {
	task, err := findTask(state, taskID)
	if err != nil {
		return err
	}

	if err := ValidateTaskTransition(task.Status, TaskStatusSkipped); err != nil {
		return fmt.Errorf("cannot skip task %s: %w", taskID, err)
	}

	task.Status = TaskStatusSkipped
	if reason != "" {
		task.Notes = append(task.Notes, "skipped: "+reason)
	}
	return nil
}

// SetPlanStatus validates the transition and sets the plan status.
func SetPlanStatus(state *PlanState, status PlanStatus, reason string) error {
	if err := ValidatePlanTransition(state.Status, status); err != nil {
		return err
	}
	state.Status = status
	_ = reason // reason available for future event logging
	return nil
}

// depsMetForClaim checks if all dependencies of a task are satisfied (helper used in ClaimTask).
// Exported for testing convenience as part of the internal package.
func depsMetForClaim(task *TaskState, state *PlanState) []string {
	doneIDs := make(map[string]bool, len(state.Tasks))
	for _, t := range state.Tasks {
		if t.Status == TaskStatusDone || t.Status == TaskStatusSkipped {
			doneIDs[t.ID] = true
		}
	}
	var unmet []string
	for _, dep := range task.Requires {
		if !doneIDs[dep] {
			unmet = append(unmet, dep)
		}
	}
	return unmet
}

// parseRequires splits a comma-separated requires string into task IDs.
func parseRequires(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// parseCriteria splits a semicolon-separated criteria string into individual criterion texts.
func parseCriteria(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// nextFeedbackNum returns the next numeric suffix for feedback IDs (scans existing feedback).
func nextFeedbackNum(state *PlanState) int {
	max := 0
	for _, f := range state.Feedback {
		if len(f.ID) > 1 && f.ID[0] == 'F' {
			if n, err := strconv.Atoi(f.ID[1:]); err == nil && n > max {
				max = n
			}
		}
	}
	return max + 1
}
