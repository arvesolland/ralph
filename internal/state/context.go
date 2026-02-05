package state

import (
	"sort"
	"time"
)

// ContextPayload is the top-level structure returned by `ralph context --json`.
// It contains everything an agent needs to understand the current plan state.
type ContextPayload struct {
	Plan      PayloadPlan      `json:"plan"`
	Tasks     PayloadTasks     `json:"tasks"`
	Feedback  PayloadFeedback  `json:"feedback"`
	Selection PayloadSelection `json:"selection"`
	Summary   PayloadSummary   `json:"summary"`
}

// PayloadPlan contains plan-level metadata.
type PayloadPlan struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Status    PlanStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
}

// PayloadTasks wraps the full task list.
type PayloadTasks struct {
	Items []TaskState `json:"items"`
}

// PayloadFeedback contains unresolved and all feedback entries.
type PayloadFeedback struct {
	Unresolved []Feedback `json:"unresolved"`
	All        []Feedback `json:"all"`
}

// PayloadSelection mirrors Selection with JSON output.
type PayloadSelection struct {
	SuggestedNext *TaskPick  `json:"suggested_next"`
	Available     []TaskPick `json:"available"`
	Blocked       []TaskPick `json:"blocked"`
}

// PayloadSummary provides aggregate statistics about plan progress.
type PayloadSummary struct {
	Total    int                `json:"total"`
	ByStatus map[string]int     `json:"by_status"`
	DoneRatio float64           `json:"done_ratio"`
}

// BuildContext assembles a full ContextPayload from plan state.
// Tasks are sorted by ID. Feedback is sorted by created_at.
// If state is nil, returns a zero-value payload.
func BuildContext(state *PlanState) *ContextPayload {
	if state == nil {
		return &ContextPayload{
			Tasks:    PayloadTasks{Items: []TaskState{}},
			Feedback: PayloadFeedback{Unresolved: []Feedback{}, All: []Feedback{}},
			Selection: PayloadSelection{Available: []TaskPick{}, Blocked: []TaskPick{}},
			Summary:  PayloadSummary{ByStatus: map[string]int{}},
		}
	}

	// Sort tasks by numeric ID for deterministic output.
	sortedTasks := make([]TaskState, len(state.Tasks))
	copy(sortedTasks, state.Tasks)
	sort.Slice(sortedTasks, func(i, j int) bool {
		ni, nj := parseTaskNum(sortedTasks[i].ID), parseTaskNum(sortedTasks[j].ID)
		if ni != nj {
			return ni < nj
		}
		return sortedTasks[i].ID < sortedTasks[j].ID
	})

	// Sort all feedback by created_at for deterministic output.
	sortedFeedback := make([]Feedback, len(state.Feedback))
	copy(sortedFeedback, state.Feedback)
	sort.Slice(sortedFeedback, func(i, j int) bool {
		return sortedFeedback[i].CreatedAt.Before(sortedFeedback[j].CreatedAt)
	})

	// Compute unresolved feedback (also sorted by created_at since source is sorted).
	var unresolved []Feedback
	for _, f := range sortedFeedback {
		if !f.Resolved {
			unresolved = append(unresolved, f)
		}
	}
	if unresolved == nil {
		unresolved = []Feedback{}
	}

	// Compute selection.
	sel := ComputeSelection(state)

	// Compute summary.
	summary := computeSummary(state.Tasks)

	return &ContextPayload{
		Plan: PayloadPlan{
			ID:        state.ID,
			Title:     state.Title,
			Status:    state.Status,
			CreatedAt: state.CreatedAt,
		},
		Tasks: PayloadTasks{
			Items: sortedTasks,
		},
		Feedback: PayloadFeedback{
			Unresolved: unresolved,
			All:        sortedFeedback,
		},
		Selection: PayloadSelection{
			SuggestedNext: sel.SuggestedNext,
			Available:     nonNilPicks(sel.Available),
			Blocked:       nonNilPicks(sel.Blocked),
		},
		Summary: summary,
	}
}

// computeSummary calculates aggregate task statistics.
func computeSummary(tasks []TaskState) PayloadSummary {
	byStatus := make(map[string]int)
	for _, t := range tasks {
		byStatus[string(t.Status)]++
	}

	total := len(tasks)
	done := byStatus[string(TaskStatusDone)]

	var doneRatio float64
	if total > 0 {
		doneRatio = float64(done) / float64(total)
	}

	return PayloadSummary{
		Total:     total,
		ByStatus:  byStatus,
		DoneRatio: doneRatio,
	}
}

// nonNilPicks ensures a nil slice is returned as an empty slice for clean JSON output.
func nonNilPicks(picks []TaskPick) []TaskPick {
	if picks == nil {
		return []TaskPick{}
	}
	return picks
}
