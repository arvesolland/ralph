package state

import (
	"regexp"
	"strings"
	"time"
)

// taskHeadingRegex matches "### T1: Title" or "### T12: Some Title Here"
var taskHeadingRegex = regexp.MustCompile(`^###\s+(T\d+):\s+(.+)$`)

// requiresLineRegex matches "**Requires:** T1, T2" or "**Requires:** —"
var requiresLineRegex = regexp.MustCompile(`^\*\*Requires:\*\*\s*(.+)$`)

// taskIDExtractRegex matches task IDs like T1, T2, T10 within a string.
var taskIDExtractRegex = regexp.MustCompile(`T\d+`)

// doneWhenLineRegex matches "**Done when:**"
var doneWhenLineRegex = regexp.MustCompile(`^\*\*Done when:\*\*`)

// criterionRegex matches "- [ ] criterion text" or "- [x] criterion text"
var criterionRegex = regexp.MustCompile(`^-\s*\[([ xX])\]\s+(.+)$`)

// InitStateFromPlan parses a plan.md content string and creates an initial PlanState.
// It extracts:
//   - Plan title from the first "# Plan: Title" heading
//   - Task IDs and titles from "### T{n}: Title" headings
//   - Dependencies from "**Requires:** T1, T2" lines
//   - Acceptance criteria from "**Done when:**" checkbox lists
//
// Tasks start as "todo". The plan starts as "active".
// Returns an error if no tasks are found.
func InitStateFromPlan(planContent string, planID string) (*PlanState, error) {
	title := extractPlanTitle(planContent)
	if title == "" {
		title = planID
	}

	tasks := extractTasksFromPlan(planContent)

	now := time.Now()
	st := &PlanState{
		ID:        planID,
		Title:     title,
		Status:    PlanStatusActive,
		CreatedAt: now,
		Tasks:     tasks,
	}

	return st, nil
}

// extractPlanTitle finds the first "# Plan: Title" heading in the content.
func extractPlanTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# Plan:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# Plan:"))
		}
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			// First top-level heading as fallback
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

// extractTasksFromPlan parses all ### T{n}: Title sections from plan content.
func extractTasksFromPlan(content string) []TaskState {
	lines := strings.Split(content, "\n")
	var tasks []TaskState
	var current *taskParseState

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Check for task heading
		if m := taskHeadingRegex.FindStringSubmatch(line); m != nil {
			// Save previous task if any
			if current != nil {
				tasks = append(tasks, current.toTaskState())
			}
			current = &taskParseState{
				id:    m[1],
				title: m[2],
			}
			continue
		}

		if current == nil {
			continue
		}

		// Check for requires line
		if m := requiresLineRegex.FindStringSubmatch(line); m != nil {
			reqText := m[1]
			// "—" or "-" means no requirements
			if reqText != "—" && reqText != "-" && reqText != "none" && reqText != "None" {
				current.requires = taskIDExtractRegex.FindAllString(reqText, -1)
			}
			continue
		}

		// Check for "Done when:" section start
		if doneWhenLineRegex.MatchString(line) {
			current.inDoneWhen = true
			continue
		}

		// If we're in a Done When section, look for criteria checkboxes
		if current.inDoneWhen {
			if m := criterionRegex.FindStringSubmatch(line); m != nil {
				current.criteria = append(current.criteria, m[2])
				continue
			}
			// A non-checkbox, non-empty line that isn't a continuation ends the Done When section
			if line != "" && !strings.HasPrefix(line, "-") {
				current.inDoneWhen = false
			}
		}

		// Another ### heading (not a T heading) signals end of current task section
		if strings.HasPrefix(line, "### ") && !taskHeadingRegex.MatchString(line) {
			// Non-task heading - save current task and stop tracking
			tasks = append(tasks, current.toTaskState())
			current = nil
		}

		// A "---" separator or "## " heading signals end of task section
		if line == "---" || (strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "### ")) {
			if current != nil {
				tasks = append(tasks, current.toTaskState())
				current = nil
			}
		}
	}

	// Don't forget the last task
	if current != nil {
		tasks = append(tasks, current.toTaskState())
	}

	return tasks
}

// taskParseState accumulates data while parsing a single task section.
type taskParseState struct {
	id         string
	title      string
	requires   []string
	criteria   []string
	inDoneWhen bool
}

func (t *taskParseState) toTaskState() TaskState {
	ts := TaskState{
		ID:       t.id,
		Title:    t.title,
		Status:   TaskStatusTodo,
		Requires: t.requires,
	}
	for _, c := range t.criteria {
		ts.Criteria = append(ts.Criteria, Criterion{Text: c})
	}
	return ts
}
