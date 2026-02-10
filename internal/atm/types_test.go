package atm

import (
	"encoding/json"
	"testing"
)

func TestStatsJSONDeserialization(t *testing.T) {
	raw := `{"total_tasks":5,"done":3,"doing":1,"claimed":0,"blocked":0,"available":1,"skipped":0}`
	var s Stats
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.TotalTasks != 5 {
		t.Errorf("TotalTasks = %d, want 5", s.TotalTasks)
	}
	if s.Done != 3 {
		t.Errorf("Done = %d, want 3", s.Done)
	}
	if s.Doing != 1 {
		t.Errorf("Doing = %d, want 1", s.Doing)
	}
	if s.Claimed != 0 {
		t.Errorf("Claimed = %d, want 0", s.Claimed)
	}
	if s.Blocked != 0 {
		t.Errorf("Blocked = %d, want 0", s.Blocked)
	}
	if s.Available != 1 {
		t.Errorf("Available = %d, want 1", s.Available)
	}
	if s.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", s.Skipped)
	}
}

func TestStatsJSONDeserializationZeroValues(t *testing.T) {
	raw := `{"total_tasks":0,"done":0,"doing":0,"claimed":0,"blocked":0,"available":0,"skipped":0}`
	var s Stats
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.TotalTasks != 0 {
		t.Errorf("TotalTasks = %d, want 0", s.TotalTasks)
	}
	if s.Done != 0 {
		t.Errorf("Done = %d, want 0", s.Done)
	}
}

func TestAgentContextJSONDeserialization(t *testing.T) {
	raw := `{
		"project": {"id": 1, "name": "Test Project", "slug": "test"},
		"plan": {
			"id": 42,
			"title": "Test Plan",
			"status": "active",
			"feature_branch": "feat/test"
		},
		"stats": {"total_tasks": 3, "done": 1, "doing": 1, "claimed": 0, "blocked": 0, "available": 1, "skipped": 0},
		"available_tasks": [
			{
				"id": 10,
				"plan_id": 42,
				"title": "Task One",
				"status": "todo",
				"acceptance_criteria": [
					{"id": 100, "task_id": 10, "description": "It works", "is_checked": false}
				]
			}
		],
		"blocked_tasks": [],
		"recent_progress": [{"id": 1, "plan_id": 42, "author": "ralph", "body": "started"}],
		"recent_feedback": []
	}`
	var ac AgentContext
	if err := json.Unmarshal([]byte(raw), &ac); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ac.Project.ID != 1 {
		t.Errorf("Project.ID = %d, want 1", ac.Project.ID)
	}
	if ac.Project.Slug != "test" {
		t.Errorf("Project.Slug = %q, want %q", ac.Project.Slug, "test")
	}
	if ac.Plan.ID != 42 {
		t.Errorf("Plan.ID = %d, want 42", ac.Plan.ID)
	}
	if ac.Plan.Status != "active" {
		t.Errorf("Plan.Status = %q, want %q", ac.Plan.Status, "active")
	}
	if ac.Plan.FeatureBranch != "feat/test" {
		t.Errorf("Plan.FeatureBranch = %q, want %q", ac.Plan.FeatureBranch, "feat/test")
	}
	if ac.Stats.TotalTasks != 3 {
		t.Errorf("Stats.TotalTasks = %d, want 3", ac.Stats.TotalTasks)
	}
	if len(ac.AvailableTasks) != 1 {
		t.Fatalf("AvailableTasks len = %d, want 1", len(ac.AvailableTasks))
	}
	task := ac.AvailableTasks[0]
	if task.ID != 10 {
		t.Errorf("Task.ID = %d, want 10", task.ID)
	}
	if len(task.AcceptanceCriteria) != 1 {
		t.Fatalf("AcceptanceCriteria len = %d, want 1", len(task.AcceptanceCriteria))
	}
	if task.AcceptanceCriteria[0].Description != "It works" {
		t.Errorf("Criterion.Description = %q, want %q", task.AcceptanceCriteria[0].Description, "It works")
	}
	if len(ac.RecentProgress) != 1 {
		t.Errorf("RecentProgress len = %d, want 1", len(ac.RecentProgress))
	}
}

func TestBranchNameWithFeatureBranch(t *testing.T) {
	p := Plan{FeatureBranch: "my-custom-branch"}
	if got := p.BranchName(); got != "my-custom-branch" {
		t.Errorf("BranchName() = %q, want %q", got, "my-custom-branch")
	}
}

func TestBranchNameGenerated(t *testing.T) {
	p := Plan{Title: "Add user authentication"}
	if got := p.BranchName(); got != "feat/add-user-authentication" {
		t.Errorf("BranchName() = %q, want %q", got, "feat/add-user-authentication")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"AKB -> ATM Integration", "akb-atm-integration"},
		{"Simple Title", "simple-title"},
		{"", ""},
		{"---special---chars!!!---", "special-chars"},
		{"Already lowercase", "already-lowercase"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"Trailing Arrow ->", "trailing-arrow"},
		{"123 numbers 456", "123-numbers-456"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := slugify(tt.input); got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
