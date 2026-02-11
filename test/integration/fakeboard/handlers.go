package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/arvesolland/ralph/internal/board"
)

// handleProjectContext handles: project context <slug>
func handleProjectContext(s *State, args []string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: project context <slug>")
	}
	slug := args[0]
	proj := findProject(s, slug)
	if proj == nil {
		return nil, fmt.Errorf("project not found: %s", slug)
	}

	ctx := board.AgentContext{
		Project: *proj,
	}

	// Find active plan for this project.
	plan := activePlanForProject(s, proj.ID)
	if plan != nil {
		ctx.Plan = enrichPlan(s, *plan)
		ctx.Stats = computeStats(s, plan.ID)
		ctx.AvailableTasks = enrichTaskList(s, availableTasks(s, plan.ID))
		ctx.BlockedTasks = enrichTaskList(s, blockedTasks(s, plan.ID))
		ctx.RecentProgress = recentEntries(progressForPlan(s, plan.ID), 5)
		ctx.RecentFeedback = recentFeedback(feedbackForPlan(s, plan.ID), 5)
	}

	return ctx, nil
}

// handlePlanContext handles: plan context <id> [--format text]
func handlePlanContext(s *State, args []string, formatText bool) (any, bool, error) {
	if len(args) < 1 {
		return nil, false, fmt.Errorf("usage: plan context <id> [--format text]")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, false, fmt.Errorf("invalid plan ID: %s", args[0])
	}
	plan := findPlan(s, id)
	if plan == nil {
		return nil, false, fmt.Errorf("plan not found: %d", id)
	}

	proj := findProjectByID(s, plan.ProjectID)
	if proj == nil {
		proj = &board.Project{}
	}

	enrichedPlan := enrichPlan(s, *plan)
	stats := computeStats(s, plan.ID)
	avail := enrichTaskList(s, availableTasks(s, plan.ID))
	blocked := enrichTaskList(s, blockedTasks(s, plan.ID))
	progress := progressForPlan(s, plan.ID)
	feedback := feedbackForPlan(s, plan.ID)

	ctx := board.AgentContext{
		Project:        *proj,
		Plan:           enrichedPlan,
		Stats:          stats,
		AvailableTasks: avail,
		BlockedTasks:   blocked,
		RecentProgress: recentEntries(progress, 5),
		RecentFeedback: recentFeedback(feedback, 5),
	}

	if formatText {
		text := formatContextAsText(ctx)
		return text, true, nil
	}

	return ctx, false, nil
}

// handlePlanList handles: plan list <slug> [--status X]
func handlePlanList(s *State, args []string, status string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: plan list <slug> [--status X]")
	}
	slug := args[0]
	proj := findProject(s, slug)
	if proj == nil {
		return nil, fmt.Errorf("project not found: %s", slug)
	}

	plans := plansForProject(s, proj.ID)
	if status != "" {
		var filtered []board.Plan
		for _, p := range plans {
			if p.Status == status {
				filtered = append(filtered, p)
			}
		}
		plans = filtered
	}

	return plans, nil
}

// handlePlanShow handles: plan show <id>
func handlePlanShow(s *State, args []string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: plan show <id>")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid plan ID: %s", args[0])
	}
	plan := findPlan(s, id)
	if plan == nil {
		return nil, fmt.Errorf("plan not found: %d", id)
	}
	return enrichPlan(s, *plan), nil
}

// handlePlanStatus handles: plan status <id> --status <status>
func handlePlanStatus(s *State, args []string, newStatus string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: plan status <id> --status <status>")
	}
	if newStatus == "" {
		return nil, fmt.Errorf("--status flag is required")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid plan ID: %s", args[0])
	}
	plan := findPlan(s, id)
	if plan == nil {
		return nil, fmt.Errorf("plan not found: %d", id)
	}
	plan.Status = newStatus
	plan.UpdatedAt = now()
	return enrichPlan(s, *plan), nil
}

// handleTaskList handles: task list <planID> [--status X] [--available]
func handleTaskList(s *State, args []string, status string, available bool) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: task list <planID> [--status X] [--available]")
	}
	planID, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid plan ID: %s", args[0])
	}

	tasks := tasksForPlan(s, planID)

	if available {
		tasks = availableTasks(s, planID)
	} else if status != "" {
		var filtered []board.Task
		for _, t := range tasks {
			if t.Status == status {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	return enrichTaskList(s, tasks), nil
}

// handleTaskShow handles: task show <id>
func handleTaskShow(s *State, args []string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: task show <id>")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid task ID: %s", args[0])
	}
	task := findTask(s, id)
	if task == nil {
		return nil, fmt.Errorf("task not found: %d", id)
	}
	return enrichTask(s, *task), nil
}

// handleTaskClaim handles: task claim <id> --assignee X
func handleTaskClaim(s *State, args []string, assignee string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: task claim <id> --assignee X")
	}
	if assignee == "" {
		return nil, fmt.Errorf("--assignee flag is required")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid task ID: %s", args[0])
	}
	task := findTask(s, id)
	if task == nil {
		return nil, fmt.Errorf("task not found: %d", id)
	}
	if task.Status != board.TaskStatusTodo {
		return nil, fmt.Errorf("task %d cannot be claimed: status is %s", id, task.Status)
	}
	task.Status = board.TaskStatusClaimed
	task.Assignee = assignee
	task.UpdatedAt = now()
	return enrichTask(s, *task), nil
}

// handleTaskStart handles: task start <id>
func handleTaskStart(s *State, args []string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: task start <id>")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid task ID: %s", args[0])
	}
	task := findTask(s, id)
	if task == nil {
		return nil, fmt.Errorf("task not found: %d", id)
	}
	// Allow start from todo or claimed.
	if task.Status != board.TaskStatusTodo && task.Status != board.TaskStatusClaimed {
		return nil, fmt.Errorf("task %d cannot be started: status is %s", id, task.Status)
	}
	task.Status = board.TaskStatusDoing
	task.StartedAt = now()
	task.UpdatedAt = now()
	return enrichTask(s, *task), nil
}

// handleTaskComplete handles: task complete <id>
func handleTaskComplete(s *State, args []string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: task complete <id>")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid task ID: %s", args[0])
	}
	task := findTask(s, id)
	if task == nil {
		return nil, fmt.Errorf("task not found: %d", id)
	}
	if task.Status != board.TaskStatusDoing && task.Status != board.TaskStatusClaimed && task.Status != board.TaskStatusTodo {
		return nil, fmt.Errorf("task %d cannot be completed: status is %s", id, task.Status)
	}
	task.Status = board.TaskStatusDone
	task.CompletedAt = now()
	task.UpdatedAt = now()
	return enrichTask(s, *task), nil
}

// handleTaskBlock handles: task block <id> --reason X
func handleTaskBlock(s *State, args []string, reason string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: task block <id> --reason X")
	}
	if reason == "" {
		return nil, fmt.Errorf("--reason flag is required")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid task ID: %s", args[0])
	}
	task := findTask(s, id)
	if task == nil {
		return nil, fmt.Errorf("task not found: %d", id)
	}
	task.Status = board.TaskStatusBlocked
	task.UpdatedAt = now()
	return enrichTask(s, *task), nil
}

// handleTaskSkip handles: task skip <id> [--reason X]
func handleTaskSkip(s *State, args []string, reason string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: task skip <id> [--reason X]")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid task ID: %s", args[0])
	}
	task := findTask(s, id)
	if task == nil {
		return nil, fmt.Errorf("task not found: %d", id)
	}
	task.Status = board.TaskStatusSkipped
	task.UpdatedAt = now()
	return enrichTask(s, *task), nil
}

// handleCriteriaCheck handles: criteria check <id>
func handleCriteriaCheck(s *State, args []string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: criteria check <id>")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid criterion ID: %s", args[0])
	}
	c := findCriterion(s, id)
	if c == nil {
		return nil, fmt.Errorf("criterion not found: %d", id)
	}
	c.IsChecked = true
	c.CheckedAt = now()
	c.UpdatedAt = now()
	return *c, nil
}

// handleCriteriaUncheck handles: criteria uncheck <id>
func handleCriteriaUncheck(s *State, args []string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: criteria uncheck <id>")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid criterion ID: %s", args[0])
	}
	c := findCriterion(s, id)
	if c == nil {
		return nil, fmt.Errorf("criterion not found: %d", id)
	}
	c.IsChecked = false
	c.CheckedAt = ""
	c.UpdatedAt = now()
	return *c, nil
}

// handleProgressAdd handles: progress add <planID> --author X --body X
func handleProgressAdd(s *State, args []string, author, body string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: progress add <planID> --author X --body X")
	}
	if author == "" || body == "" {
		return nil, fmt.Errorf("--author and --body flags are required")
	}
	planID, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid plan ID: %s", args[0])
	}
	plan := findPlan(s, planID)
	if plan == nil {
		return nil, fmt.Errorf("plan not found: %d", planID)
	}
	entry := board.Progress{
		ID:        s.NextProgressID,
		PlanID:    planID,
		Author:    author,
		Body:      body,
		CreatedAt: now(),
	}
	s.NextProgressID++
	s.Progress = append(s.Progress, entry)
	return entry, nil
}

// handleFeedbackAdd handles: feedback add <planID> --author X --body X
func handleFeedbackAdd(s *State, args []string, author, body string) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: feedback add <planID> --author X --body X")
	}
	if author == "" || body == "" {
		return nil, fmt.Errorf("--author and --body flags are required")
	}
	planID, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid plan ID: %s", args[0])
	}
	plan := findPlan(s, planID)
	if plan == nil {
		return nil, fmt.Errorf("plan not found: %d", planID)
	}
	entry := board.Feedback{
		ID:        s.NextFeedbackID,
		PlanID:    planID,
		Author:    author,
		Body:      body,
		CreatedAt: now(),
	}
	s.NextFeedbackID++
	s.Feedback = append(s.Feedback, entry)
	return entry, nil
}

// Text formatting helpers.

func enrichTaskList(s *State, tasks []board.Task) []board.Task {
	result := make([]board.Task, len(tasks))
	for i, t := range tasks {
		result[i] = enrichTask(s, t)
	}
	return result
}

func recentEntries(entries []board.Progress, n int) []board.Progress {
	if len(entries) <= n {
		return entries
	}
	return entries[len(entries)-n:]
}

func recentFeedback(entries []board.Feedback, n int) []board.Feedback {
	if len(entries) <= n {
		return entries
	}
	return entries[len(entries)-n:]
}

// formatContextAsText renders the AgentContext as structured text suitable for
// the {{BOARD_CONTEXT}} prompt placeholder. This is used by `plan context <id> --format text`.
func formatContextAsText(ctx board.AgentContext) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Plan: %s\n", ctx.Plan.Title)
	fmt.Fprintf(&b, "Status: %s | Branch: %s\n", ctx.Plan.Status, ctx.Plan.FeatureBranch)
	if ctx.Plan.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", ctx.Plan.Description)
	}

	fmt.Fprintf(&b, "\n### Stats\n")
	fmt.Fprintf(&b, "Total: %d | Done: %d | Doing: %d | Claimed: %d | Blocked: %d | Available: %d | Skipped: %d\n",
		ctx.Stats.TotalTasks, ctx.Stats.Done, ctx.Stats.Doing, ctx.Stats.Claimed,
		ctx.Stats.Blocked, ctx.Stats.Available, ctx.Stats.Skipped)

	if len(ctx.AvailableTasks) > 0 {
		fmt.Fprintf(&b, "\n### Available tasks\n")
		for _, t := range ctx.AvailableTasks {
			fmt.Fprintf(&b, "- [%d] %s", t.ID, t.Title)
			if t.Description != "" {
				fmt.Fprintf(&b, ": %s", t.Description)
			}
			fmt.Fprintln(&b)
			for _, c := range t.AcceptanceCriteria {
				check := " "
				if c.IsChecked {
					check = "x"
				}
				fmt.Fprintf(&b, "  - [%s] (%d) %s\n", check, c.ID, c.Description)
			}
		}
	}

	if len(ctx.BlockedTasks) > 0 {
		fmt.Fprintf(&b, "\n### Blocked tasks\n")
		for _, t := range ctx.BlockedTasks {
			fmt.Fprintf(&b, "- [%d] %s (blocked)\n", t.ID, t.Title)
		}
	}

	// Show all tasks for full picture.
	if len(ctx.Plan.Tasks) > 0 {
		fmt.Fprintf(&b, "\n### All tasks\n")
		for _, t := range ctx.Plan.Tasks {
			status := t.Status
			fmt.Fprintf(&b, "- [%d] %s (%s)", t.ID, t.Title, status)
			if t.Assignee != "" {
				fmt.Fprintf(&b, " @%s", t.Assignee)
			}
			fmt.Fprintln(&b)
			for _, c := range t.AcceptanceCriteria {
				check := " "
				if c.IsChecked {
					check = "x"
				}
				fmt.Fprintf(&b, "  - [%s] (%d) %s\n", check, c.ID, c.Description)
			}
		}
	}

	if len(ctx.RecentProgress) > 0 {
		fmt.Fprintf(&b, "\n### Recent progress\n")
		for _, p := range ctx.RecentProgress {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", p.CreatedAt, p.Author, p.Body)
		}
	}

	if len(ctx.RecentFeedback) > 0 {
		fmt.Fprintf(&b, "\n### Recent feedback\n")
		for _, f := range ctx.RecentFeedback {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", f.CreatedAt, f.Author, f.Body)
		}
	}

	return b.String()
}
