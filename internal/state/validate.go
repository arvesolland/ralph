package state

import "fmt"

// validPlanTransitions maps each PlanStatus to its allowed next statuses.
var validPlanTransitions = map[PlanStatus][]PlanStatus{
	PlanStatusDraft:    {PlanStatusReady},
	PlanStatusReady:    {PlanStatusActive},
	PlanStatusActive:   {PlanStatusBlocked, PlanStatusComplete},
	PlanStatusBlocked:  {PlanStatusActive},
	PlanStatusComplete: {},
}

// validTaskTransitions maps each TaskStatus to its allowed next statuses.
var validTaskTransitions = map[TaskStatus][]TaskStatus{
	TaskStatusTodo:    {TaskStatusClaimed, TaskStatusSkipped},
	TaskStatusClaimed: {TaskStatusDoing, TaskStatusSkipped},
	TaskStatusDoing:   {TaskStatusBlocked, TaskStatusDone, TaskStatusSkipped},
	TaskStatusBlocked: {TaskStatusDoing, TaskStatusSkipped},
	TaskStatusDone:    {},
	TaskStatusSkipped: {},
}

// ValidatePlanTransition checks if moving from one plan status to another is allowed.
func ValidatePlanTransition(from, to PlanStatus) error {
	if !from.IsValid() {
		return fmt.Errorf("invalid source plan status: %q", from)
	}
	if !to.IsValid() {
		return fmt.Errorf("invalid target plan status: %q", to)
	}
	for _, allowed := range validPlanTransitions[from] {
		if allowed == to {
			return nil
		}
	}
	return fmt.Errorf("invalid plan transition: %s → %s", from, to)
}

// ValidateTaskTransition checks if moving from one task status to another is allowed.
func ValidateTaskTransition(from, to TaskStatus) error {
	if !from.IsValid() {
		return fmt.Errorf("invalid source task status: %q", from)
	}
	if !to.IsValid() {
		return fmt.Errorf("invalid target task status: %q", to)
	}
	for _, allowed := range validTaskTransitions[from] {
		if allowed == to {
			return nil
		}
	}
	return fmt.Errorf("invalid task transition: %s → %s", from, to)
}

// ValidateDependencies checks that all task dependency references are valid (no missing IDs)
// and that there are no dependency cycles.
func ValidateDependencies(tasks []TaskState) error {
	ids := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		ids[t.ID] = true
	}

	// Check for missing dependency IDs
	for _, t := range tasks {
		for _, dep := range t.Requires {
			if !ids[dep] {
				return fmt.Errorf("task %s requires unknown task %s", t.ID, dep)
			}
		}
	}

	// Check for cycles using DFS with coloring
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // fully explored
	)
	color := make(map[string]int, len(tasks))
	adj := make(map[string][]string, len(tasks))
	for _, t := range tasks {
		adj[t.ID] = t.Requires
	}

	var visit func(id string) error
	visit = func(id string) error {
		color[id] = gray
		for _, dep := range adj[id] {
			switch color[dep] {
			case gray:
				return fmt.Errorf("dependency cycle detected: %s → %s", id, dep)
			case white:
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		color[id] = black
		return nil
	}

	for _, t := range tasks {
		if color[t.ID] == white {
			if err := visit(t.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

// ValidateCompletion checks that a task can be marked as done — all criteria must be checked.
func ValidateCompletion(task *TaskState) error {
	if len(task.Criteria) == 0 {
		return nil // tasks without criteria can always complete
	}
	for i, c := range task.Criteria {
		if !c.Done {
			return fmt.Errorf("task %s criterion %d not met: %s", task.ID, i+1, c.Text)
		}
	}
	return nil
}

// ValidateFeedbackScope checks that a feedback scope is valid: either "plan" or "task:Tn"
// where Tn is an existing task ID.
func ValidateFeedbackScope(scope string, tasks []TaskState) error {
	if scope == "plan" {
		return nil
	}

	const prefix = "task:"
	if len(scope) <= len(prefix) || scope[:len(prefix)] != prefix {
		return fmt.Errorf("invalid feedback scope %q: must be \"plan\" or \"task:<task-id>\"", scope)
	}

	taskID := scope[len(prefix):]
	for _, t := range tasks {
		if t.ID == taskID {
			return nil
		}
	}
	return fmt.Errorf("invalid feedback scope %q: task %s not found", scope, taskID)
}
