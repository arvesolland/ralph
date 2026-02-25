// Package knowledge extracts lessons from Board feedback for persistence across iterations.
package knowledge

import (
	"github.com/arvesolland/ralph/internal/board"
)

// systemAuthors are authors whose feedback is system-generated and should be excluded.
var systemAuthors = map[string]bool{
	"ralph":              true,
	"ralph-verification": true,
}

// Lesson represents a single learning extracted from Board feedback.
type Lesson struct {
	Date     string // CreatedAt date from the feedback entry.
	PlanID   int    // Source plan ID.
	Author   string // Who authored the feedback.
	Text     string // The feedback body text.
}

// ExtractLessons fetches feedback from Board for the given plan and returns
// lessons from human-authored feedback entries. System-generated feedback
// (authored by "ralph" or "ralph-verification") is excluded.
func ExtractLessons(b board.Board, planID int) ([]Lesson, error) {
	ctx, err := b.PlanContext(planID)
	if err != nil {
		return nil, err
	}

	var lessons []Lesson
	for _, fb := range ctx.RecentFeedback {
		if systemAuthors[fb.Author] {
			continue
		}
		lessons = append(lessons, Lesson{
			Date:   fb.CreatedAt,
			PlanID: fb.PlanID,
			Author: fb.Author,
			Text:   fb.Body,
		})
	}
	return lessons, nil
}
