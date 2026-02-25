package knowledge

import (
	"fmt"
	"testing"

	"github.com/arvesolland/ralph/internal/board"
)

func TestExtractLessons_NoFeedback(t *testing.T) {
	mock := board.NewMockBoard()
	mock.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			RecentFeedback: nil,
		}, nil
	}

	lessons, err := ExtractLessons(mock, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lessons) != 0 {
		t.Fatalf("expected 0 lessons, got %d", len(lessons))
	}
}

func TestExtractLessons_SystemOnlyFeedback(t *testing.T) {
	mock := board.NewMockBoard()
	mock.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			RecentFeedback: []board.Feedback{
				{ID: 1, PlanID: 42, Author: "ralph", Body: "False completion detected", CreatedAt: "2026-02-20"},
				{ID: 2, PlanID: 42, Author: "ralph-verification", Body: "Tasks remaining: 3", CreatedAt: "2026-02-20"},
			},
		}, nil
	}

	lessons, err := ExtractLessons(mock, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lessons) != 0 {
		t.Fatalf("expected 0 lessons, got %d", len(lessons))
	}
}

func TestExtractLessons_HumanFeedback(t *testing.T) {
	mock := board.NewMockBoard()
	mock.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			RecentFeedback: []board.Feedback{
				{ID: 1, PlanID: 42, Author: "arve", Body: "Use the existing board mock for tests", CreatedAt: "2026-02-20"},
				{ID: 2, PlanID: 42, Author: "arve", Body: "Don't modify CLAUDE.md directly", CreatedAt: "2026-02-21"},
			},
		}, nil
	}

	lessons, err := ExtractLessons(mock, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lessons) != 2 {
		t.Fatalf("expected 2 lessons, got %d", len(lessons))
	}

	if lessons[0].Author != "arve" {
		t.Errorf("expected author 'arve', got %q", lessons[0].Author)
	}
	if lessons[0].PlanID != 42 {
		t.Errorf("expected plan ID 42, got %d", lessons[0].PlanID)
	}
	if lessons[0].Text != "Use the existing board mock for tests" {
		t.Errorf("unexpected lesson text: %q", lessons[0].Text)
	}
	if lessons[0].Date != "2026-02-20" {
		t.Errorf("expected date '2026-02-20', got %q", lessons[0].Date)
	}

	if lessons[1].Text != "Don't modify CLAUDE.md directly" {
		t.Errorf("unexpected lesson text: %q", lessons[1].Text)
	}
}

func TestExtractLessons_MixedFeedback(t *testing.T) {
	mock := board.NewMockBoard()
	mock.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return &board.AgentContext{
			RecentFeedback: []board.Feedback{
				{ID: 1, PlanID: 42, Author: "ralph", Body: "False completion detected", CreatedAt: "2026-02-20"},
				{ID: 2, PlanID: 42, Author: "arve", Body: "Tests must pass before committing", CreatedAt: "2026-02-20"},
				{ID: 3, PlanID: 42, Author: "ralph-verification", Body: "Tasks remaining: 2", CreatedAt: "2026-02-21"},
				{ID: 4, PlanID: 42, Author: "bob", Body: "Check edge case with empty input", CreatedAt: "2026-02-21"},
			},
		}, nil
	}

	lessons, err := ExtractLessons(mock, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lessons) != 2 {
		t.Fatalf("expected 2 lessons, got %d", len(lessons))
	}

	if lessons[0].Author != "arve" {
		t.Errorf("expected first lesson author 'arve', got %q", lessons[0].Author)
	}
	if lessons[0].Text != "Tests must pass before committing" {
		t.Errorf("unexpected first lesson text: %q", lessons[0].Text)
	}

	if lessons[1].Author != "bob" {
		t.Errorf("expected second lesson author 'bob', got %q", lessons[1].Author)
	}
	if lessons[1].Text != "Check edge case with empty input" {
		t.Errorf("unexpected second lesson text: %q", lessons[1].Text)
	}
}

func TestExtractLessons_BoardError(t *testing.T) {
	mock := board.NewMockBoard()
	mock.PlanContextFunc = func(planID int) (*board.AgentContext, error) {
		return nil, fmt.Errorf("board: connection refused")
	}

	lessons, err := ExtractLessons(mock, 42)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if lessons != nil {
		t.Fatalf("expected nil lessons on error, got %d", len(lessons))
	}
}
