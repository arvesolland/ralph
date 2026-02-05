package state

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestPlanStatusIsValid(t *testing.T) {
	valid := []PlanStatus{PlanStatusDraft, PlanStatusReady, PlanStatusActive, PlanStatusBlocked, PlanStatusComplete}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalid := []PlanStatus{"", "unknown", "in_progress", "ACTIVE"}
	for _, s := range invalid {
		if s.IsValid() {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestTaskStatusIsValid(t *testing.T) {
	valid := []TaskStatus{TaskStatusTodo, TaskStatusClaimed, TaskStatusDoing, TaskStatusBlocked, TaskStatusDone, TaskStatusSkipped}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalid := []TaskStatus{"", "unknown", "complete", "TODO"}
	for _, s := range invalid {
		if s.IsValid() {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func sampleState() *PlanState {
	now := time.Date(2026, 2, 6, 12, 0, 0, 0, time.UTC)
	criterionDoneAt := time.Date(2026, 2, 6, 13, 0, 0, 0, time.UTC)
	taskStarted := time.Date(2026, 2, 6, 12, 30, 0, 0, time.UTC)

	return &PlanState{
		ID:        "structured-state",
		Title:     "Structured Task State",
		Status:    PlanStatusActive,
		CreatedAt: now,
		Tasks: []TaskState{
			{
				ID:       "T1",
				Title:    "Define state types",
				Status:   TaskStatusDoing,
				Requires: nil,
				Criteria: []Criterion{
					{Text: "PlanState struct defined", Done: true, DoneAt: &criterionDoneAt},
					{Text: "Tests pass", Done: false},
				},
				Notes:     []string{"Started with types"},
				Artifacts: Artifacts{Commits: []string{"abc123"}, FilesTouched: []string{"internal/state/types.go"}},
				StartedAt: &taskStarted,
			},
			{
				ID:       "T2",
				Title:    "Implement load/save",
				Status:   TaskStatusTodo,
				Requires: []string{"T1"},
				Criteria: []Criterion{
					{Text: "LoadState works", Done: false},
				},
			},
		},
		Feedback: []Feedback{
			{
				ID:        "F1",
				Scope:     "task:T1",
				Author:    "human",
				Message:   "Looks good so far",
				Resolved:  false,
				CreatedAt: now,
			},
		},
	}
}

func TestYAMLRoundTrip(t *testing.T) {
	original := sampleState()

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}

	var restored PlanState
	if err := yaml.Unmarshal(data, &restored); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(original, &restored) {
		t.Errorf("YAML round-trip mismatch.\nOriginal: %+v\nRestored: %+v", original, &restored)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := sampleState()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored PlanState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(original, &restored) {
		t.Errorf("JSON round-trip mismatch.\nOriginal: %+v\nRestored: %+v", original, &restored)
	}
}

func TestJSONMarshalOutput(t *testing.T) {
	original := sampleState()

	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}

	// Verify key fields appear in JSON output
	s := string(data)
	checks := []string{
		`"id": "structured-state"`,
		`"status": "active"`,
		`"id": "T1"`,
		`"status": "doing"`,
		`"id": "T2"`,
		`"status": "todo"`,
		`"id": "F1"`,
		`"scope": "task:T1"`,
	}
	for _, check := range checks {
		if !containsString(s, check) {
			t.Errorf("JSON output missing expected content %q", check)
		}
	}
}

func TestYAMLMarshalOutput(t *testing.T) {
	original := sampleState()

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}

	// Verify key fields appear in YAML output
	s := string(data)
	checks := []string{
		"id: structured-state",
		"status: active",
		"id: T1",
		"status: doing",
		"id: T2",
		"status: todo",
		"id: F1",
		"scope: task:T1",
	}
	for _, check := range checks {
		if !containsString(s, check) {
			t.Errorf("YAML output missing expected content %q\nFull output:\n%s", check, s)
		}
	}
}

func TestEmptyOptionalFields(t *testing.T) {
	// A minimal state with no optional fields should round-trip cleanly
	minimal := &PlanState{
		ID:        "minimal",
		Title:     "Minimal Plan",
		Status:    PlanStatusDraft,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Tasks: []TaskState{
			{
				ID:     "T1",
				Title:  "Only task",
				Status: TaskStatusTodo,
			},
		},
	}

	// YAML round-trip
	yamlData, err := yaml.Marshal(minimal)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}
	var yamlRestored PlanState
	if err := yaml.Unmarshal(yamlData, &yamlRestored); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(minimal, &yamlRestored) {
		t.Errorf("YAML round-trip mismatch for minimal state")
	}

	// JSON round-trip
	jsonData, err := json.Marshal(minimal)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var jsonRestored PlanState
	if err := json.Unmarshal(jsonData, &jsonRestored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(minimal, &jsonRestored) {
		t.Errorf("JSON round-trip mismatch for minimal state")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
