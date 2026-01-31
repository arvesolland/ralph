// Package plan handles plan parsing and queue management.
package plan

import (
	"regexp"
	"strings"
)

// Task represents a checkbox task in a plan.
type Task struct {
	// Line is the 1-indexed line number where this task appears.
	Line int

	// Text is the task text (without the checkbox marker).
	Text string

	// Complete is true if the checkbox is checked ([x]).
	Complete bool

	// Requires contains task identifiers this task depends on (e.g., ["T1", "T2"]).
	Requires []string

	// Subtasks are indented tasks that belong to this task.
	Subtasks []Task

	// Indent is the number of spaces/tabs before the checkbox.
	// Used internally for nesting logic.
	Indent int
}

// checkboxRegex matches markdown checkboxes: - [ ] or - [x]
// Group 1: indentation (spaces/tabs before -)
// Group 2: checkbox state (space or x)
// Group 3: task text (everything after the checkbox)
var checkboxRegex = regexp.MustCompile(`^(\s*)-\s*\[([ xX])\]\s*(.*)$`)

// requiresRegex matches "requires: T1, T2" or "Requires: T1, T2" patterns.
// Case-insensitive match at word boundary.
var requiresRegex = regexp.MustCompile(`(?i)\brequires?:\s*([^\n]+)`)

// taskIDRegex matches task identifiers like T1, T2, T10, etc.
var taskIDRegex = regexp.MustCompile(`T\d+`)

// ExtractTasks parses markdown content and extracts checkbox tasks.
// It handles:
//   - Simple tasks: - [ ] Task text
//   - Completed tasks: - [x] Task text
//   - Nested subtasks via indentation
//   - Dependencies via "requires: T1, T2" in task text
//
// Returns a slice of top-level tasks (non-indented or first-level indented).
// Subtasks are nested within their parent tasks.
func ExtractTasks(content string) []Task {
	lines := strings.Split(content, "\n")
	var allTasks []Task

	for lineNum, line := range lines {
		match := checkboxRegex.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		indent := len(match[1])
		isComplete := strings.ToLower(match[2]) == "x"
		text := strings.TrimSpace(match[3])

		// Extract dependencies from the task text
		requires := extractRequires(text)

		task := Task{
			Line:     lineNum + 1, // 1-indexed
			Text:     text,
			Complete: isComplete,
			Requires: requires,
			Subtasks: nil,
			Indent:   indent,
		}

		allTasks = append(allTasks, task)
	}

	// Build nested structure based on indentation
	return buildTaskTree(allTasks)
}

// extractRequires finds "requires: T1, T2" patterns in task text
// and returns the list of task identifiers.
func extractRequires(text string) []string {
	match := requiresRegex.FindStringSubmatch(text)
	if match == nil {
		return nil
	}

	// Extract all task IDs from the requires clause
	ids := taskIDRegex.FindAllString(match[1], -1)
	return ids
}

// buildTaskTree converts a flat list of tasks into a nested tree
// based on indentation levels.
// Tasks with greater indentation become subtasks of the previous
// task with less indentation.
func buildTaskTree(flat []Task) []Task {
	if len(flat) == 0 {
		return nil
	}

	var result []Task
	var stack []*Task

	for i := range flat {
		task := flat[i]

		// Pop tasks from stack that have >= indentation
		// (they can't be parents of this task)
		for len(stack) > 0 {
			parent := stack[len(stack)-1]
			if parent.Indent < task.Indent {
				break
			}
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			// No parent - this is a top-level task
			result = append(result, task)
			stack = append(stack, &result[len(result)-1])
		} else {
			// Add as subtask of the last item on the stack
			parent := stack[len(stack)-1]
			parent.Subtasks = append(parent.Subtasks, task)
			// Push pointer to the newly added subtask
			stack = append(stack, &parent.Subtasks[len(parent.Subtasks)-1])
		}
	}

	return result
}

// CountComplete returns the number of completed tasks (recursively including subtasks).
func CountComplete(tasks []Task) int {
	count := 0
	for _, t := range tasks {
		if t.Complete {
			count++
		}
		count += CountComplete(t.Subtasks)
	}
	return count
}

// CountTotal returns the total number of tasks (recursively including subtasks).
func CountTotal(tasks []Task) int {
	count := len(tasks)
	for _, t := range tasks {
		count += CountTotal(t.Subtasks)
	}
	return count
}

// FindNextIncomplete returns the first incomplete task where all dependencies
// are met. Returns nil if no such task exists.
// This is a simple implementation that checks dependencies by task ID pattern.
func FindNextIncomplete(tasks []Task, completedIDs map[string]bool) *Task {
	for i := range tasks {
		task := &tasks[i]
		if task.Complete {
			continue
		}

		// Check if all dependencies are met
		allMet := true
		for _, req := range task.Requires {
			if !completedIDs[req] {
				allMet = false
				break
			}
		}

		if allMet {
			return task
		}
	}
	return nil
}
