package state

import (
	"sort"
	"strconv"
	"strings"
)

// TaskPick represents a task that has been classified as available or blocked,
// along with a reason explaining why.
type TaskPick struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

// Selection holds the result of computing which tasks are available to work on.
type Selection struct {
	SuggestedNext *TaskPick  `json:"suggested_next"`
	Available     []TaskPick `json:"available"`
	Blocked       []TaskPick `json:"blocked"`
}

// ComputeSelection analyzes plan state and returns which tasks are available,
// which are blocked, and which one should be worked on next.
func ComputeSelection(state *PlanState) *Selection {
	if state == nil {
		return &Selection{}
	}

	// Build set of done/skipped task IDs for dependency checking.
	doneIDs := make(map[string]bool, len(state.Tasks))
	for _, t := range state.Tasks {
		if t.Status == TaskStatusDone || t.Status == TaskStatusSkipped {
			doneIDs[t.ID] = true
		}
	}

	var available, blocked []TaskPick

	for _, t := range state.Tasks {
		// Skip tasks that are already done, skipped, or in progress.
		switch t.Status {
		case TaskStatusDone, TaskStatusSkipped, TaskStatusDoing, TaskStatusClaimed:
			continue
		}

		// Check if task is explicitly blocked.
		if t.Status == TaskStatusBlocked {
			blocked = append(blocked, TaskPick{
				TaskID: t.ID,
				Reason: "task status is blocked",
			})
			continue
		}

		// Task is todo — check dependencies.
		unmet := unmetDependencies(t.Requires, doneIDs)
		if len(unmet) > 0 {
			blocked = append(blocked, TaskPick{
				TaskID: t.ID,
				Reason: "waiting on: " + strings.Join(unmet, ", "),
			})
			continue
		}

		available = append(available, TaskPick{
			TaskID: t.ID,
			Reason: "all dependencies met",
		})
	}

	// Sort available and blocked by numeric task ID for deterministic ordering.
	sortByTaskID(available)
	sortByTaskID(blocked)

	sel := &Selection{
		Available: available,
		Blocked:   blocked,
	}

	if len(available) > 0 {
		first := available[0]
		sel.SuggestedNext = &first
	}

	return sel
}

// unmetDependencies returns the list of required task IDs that are not done/skipped.
func unmetDependencies(requires []string, doneIDs map[string]bool) []string {
	var unmet []string
	for _, dep := range requires {
		if !doneIDs[dep] {
			unmet = append(unmet, dep)
		}
	}
	return unmet
}

// parseTaskNum extracts the numeric part from a task ID like "T12" → 12.
// Returns -1 if the ID doesn't match the T{n} pattern.
func parseTaskNum(id string) int {
	if len(id) < 2 || id[0] != 'T' {
		return -1
	}
	n, err := strconv.Atoi(id[1:])
	if err != nil {
		return -1
	}
	return n
}

// sortByTaskID sorts TaskPick slices by numeric task ID (T1 < T2 < T12).
func sortByTaskID(picks []TaskPick) {
	sort.Slice(picks, func(i, j int) bool {
		ni, nj := parseTaskNum(picks[i].TaskID), parseTaskNum(picks[j].TaskID)
		if ni != nj {
			return ni < nj
		}
		return picks[i].TaskID < picks[j].TaskID
	})
}
