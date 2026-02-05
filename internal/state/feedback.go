package state

import (
	"fmt"
	"time"
)

// AddFeedback creates a new feedback entry with an auto-assigned F{n} ID.
func AddFeedback(state *PlanState, scope, author, message string) (*Feedback, error) {
	if err := ValidateFeedbackScope(scope, state.Tasks); err != nil {
		return nil, err
	}
	if message == "" {
		return nil, fmt.Errorf("feedback message cannot be empty")
	}

	fb := Feedback{
		ID:        fmt.Sprintf("F%d", nextFeedbackNum(state)),
		Scope:     scope,
		Author:    author,
		Message:   message,
		Resolved:  false,
		CreatedAt: time.Now(),
	}

	state.Feedback = append(state.Feedback, fb)
	return &state.Feedback[len(state.Feedback)-1], nil
}

// ResolveFeedback marks a feedback entry as resolved.
func ResolveFeedback(state *PlanState, feedbackID string) error {
	for i := range state.Feedback {
		if state.Feedback[i].ID == feedbackID {
			if state.Feedback[i].Resolved {
				return fmt.Errorf("feedback %s is already resolved", feedbackID)
			}
			now := time.Now()
			state.Feedback[i].Resolved = true
			state.Feedback[i].ResolvedAt = &now
			return nil
		}
	}
	return fmt.Errorf("feedback %s not found", feedbackID)
}

// UnresolvedFeedback returns all feedback entries that are not yet resolved.
func UnresolvedFeedback(state *PlanState) []Feedback {
	var result []Feedback
	for _, f := range state.Feedback {
		if !f.Resolved {
			result = append(result, f)
		}
	}
	return result
}

// FeedbackForTask returns all feedback entries scoped to the given task ID.
func FeedbackForTask(state *PlanState, taskID string) []Feedback {
	scope := "task:" + taskID
	var result []Feedback
	for _, f := range state.Feedback {
		if f.Scope == scope {
			result = append(result, f)
		}
	}
	return result
}
