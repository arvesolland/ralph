package state

import (
	"os"
	"path/filepath"
	"testing"
)

const testPlanSimple = `# Plan: Simple Feature

**Created:** 2026-02-06
**Status:** pending

## Tasks

### T1: Set up project structure
> Create the initial directory layout

**Requires:** —
**Status:** pending

**Done when:**
- [ ] Directory structure created
- [ ] Configuration file added

**Subtasks:**
1. [ ] Create src directory
2. [ ] Add config.yaml
`

const testPlanMultipleTasks = `# Plan: Multi Task Plan

**Created:** 2026-02-06
**Status:** pending

## Tasks

### T1: Foundation types
> Define the core data types

**Requires:** —
**Status:** pending

**Done when:**
- [ ] Types defined
- [ ] Tests pass

---

### T2: Implement storage
> Build the persistence layer

**Requires:** T1
**Status:** pending

**Done when:**
- [ ] Load function works
- [ ] Save function works
- [ ] Round-trip test passes

---

### T3: Build API
> Create the HTTP endpoints

**Requires:** T1, T2
**Status:** pending

**Done when:**
- [ ] GET endpoint works
- [ ] POST endpoint works

---

## Discovered

<!-- Tasks found during implementation -->
`

const testPlanNoDoneWhen = `# Plan: No Done When

## Tasks

### T1: Simple task

**Requires:** —
**Status:** pending

Some description text here.

---

### T2: Another task

**Requires:** T1
**Status:** pending

Just do the thing.
`

const testPlanNoTasks = `# Plan: Empty Plan

**Created:** 2026-02-06
**Status:** pending

## Context

This plan has no task sections.

## Discovered
`

const testPlanCheckedCriteria = `# Plan: Partially Done

## Tasks

### T1: Already done task

**Requires:** —
**Status:** complete

**Done when:**
- [x] First criterion
- [x] Second criterion

---

### T2: In progress task

**Requires:** T1
**Status:** pending

**Done when:**
- [x] Already checked
- [ ] Not yet done
`

func TestInitStateFromPlan_Simple(t *testing.T) {
	st, err := InitStateFromPlan(testPlanSimple, "simple-feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.ID != "simple-feature" {
		t.Errorf("expected ID 'simple-feature', got %q", st.ID)
	}
	if st.Title != "Simple Feature" {
		t.Errorf("expected title 'Simple Feature', got %q", st.Title)
	}
	if st.Status != PlanStatusActive {
		t.Errorf("expected status 'active', got %q", st.Status)
	}
	if len(st.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(st.Tasks))
	}

	task := st.Tasks[0]
	if task.ID != "T1" {
		t.Errorf("expected task ID 'T1', got %q", task.ID)
	}
	if task.Title != "Set up project structure" {
		t.Errorf("expected title 'Set up project structure', got %q", task.Title)
	}
	if task.Status != TaskStatusTodo {
		t.Errorf("expected status 'todo', got %q", task.Status)
	}
	if len(task.Requires) != 0 {
		t.Errorf("expected no requires, got %v", task.Requires)
	}
	if len(task.Criteria) != 2 {
		t.Fatalf("expected 2 criteria, got %d", len(task.Criteria))
	}
	if task.Criteria[0].Text != "Directory structure created" {
		t.Errorf("expected first criterion text 'Directory structure created', got %q", task.Criteria[0].Text)
	}
	if task.Criteria[1].Text != "Configuration file added" {
		t.Errorf("expected second criterion text 'Configuration file added', got %q", task.Criteria[1].Text)
	}
	// Criteria should start unchecked regardless of plan.md checkbox state
	if task.Criteria[0].Done {
		t.Error("expected first criterion to be not done")
	}
}

func TestInitStateFromPlan_MultipleTasks(t *testing.T) {
	st, err := InitStateFromPlan(testPlanMultipleTasks, "multi-task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.Title != "Multi Task Plan" {
		t.Errorf("expected title 'Multi Task Plan', got %q", st.Title)
	}
	if len(st.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(st.Tasks))
	}

	// T1: no deps
	if st.Tasks[0].ID != "T1" {
		t.Errorf("expected T1, got %q", st.Tasks[0].ID)
	}
	if st.Tasks[0].Title != "Foundation types" {
		t.Errorf("expected 'Foundation types', got %q", st.Tasks[0].Title)
	}
	if len(st.Tasks[0].Requires) != 0 {
		t.Errorf("expected no requires for T1, got %v", st.Tasks[0].Requires)
	}
	if len(st.Tasks[0].Criteria) != 2 {
		t.Errorf("expected 2 criteria for T1, got %d", len(st.Tasks[0].Criteria))
	}

	// T2: depends on T1
	if st.Tasks[1].ID != "T2" {
		t.Errorf("expected T2, got %q", st.Tasks[1].ID)
	}
	if len(st.Tasks[1].Requires) != 1 || st.Tasks[1].Requires[0] != "T1" {
		t.Errorf("expected requires [T1], got %v", st.Tasks[1].Requires)
	}
	if len(st.Tasks[1].Criteria) != 3 {
		t.Errorf("expected 3 criteria for T2, got %d", len(st.Tasks[1].Criteria))
	}

	// T3: depends on T1, T2
	if st.Tasks[2].ID != "T3" {
		t.Errorf("expected T3, got %q", st.Tasks[2].ID)
	}
	if len(st.Tasks[2].Requires) != 2 {
		t.Errorf("expected 2 requires for T3, got %d", len(st.Tasks[2].Requires))
	}
	if st.Tasks[2].Requires[0] != "T1" || st.Tasks[2].Requires[1] != "T2" {
		t.Errorf("expected requires [T1, T2], got %v", st.Tasks[2].Requires)
	}
	if len(st.Tasks[2].Criteria) != 2 {
		t.Errorf("expected 2 criteria for T3, got %d", len(st.Tasks[2].Criteria))
	}
}

func TestInitStateFromPlan_NoDoneWhen(t *testing.T) {
	st, err := InitStateFromPlan(testPlanNoDoneWhen, "no-done-when")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(st.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(st.Tasks))
	}

	// Tasks without Done when should have no criteria
	if len(st.Tasks[0].Criteria) != 0 {
		t.Errorf("expected 0 criteria for T1, got %d", len(st.Tasks[0].Criteria))
	}
	if len(st.Tasks[1].Criteria) != 0 {
		t.Errorf("expected 0 criteria for T2, got %d", len(st.Tasks[1].Criteria))
	}

	// T2 should still have dependency on T1
	if len(st.Tasks[1].Requires) != 1 || st.Tasks[1].Requires[0] != "T1" {
		t.Errorf("expected requires [T1] for T2, got %v", st.Tasks[1].Requires)
	}
}

func TestInitStateFromPlan_NoTasks(t *testing.T) {
	st, err := InitStateFromPlan(testPlanNoTasks, "empty-plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(st.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(st.Tasks))
	}
	if st.Status != PlanStatusActive {
		t.Errorf("expected active status, got %q", st.Status)
	}
}

func TestInitStateFromPlan_CriteriaAlwaysUnchecked(t *testing.T) {
	st, err := InitStateFromPlan(testPlanCheckedCriteria, "partial")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(st.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(st.Tasks))
	}

	// Even though plan.md has [x] checkboxes, state.yaml criteria should start unchecked
	for i, task := range st.Tasks {
		for j, c := range task.Criteria {
			if c.Done {
				t.Errorf("task %d criterion %d should be unchecked, but was checked", i, j)
			}
		}
		// All tasks start as todo regardless of plan.md Status field
		if task.Status != TaskStatusTodo {
			t.Errorf("task %d should be todo, got %q", i, task.Status)
		}
	}
}

func TestInitStateFromPlan_TitleFallback(t *testing.T) {
	content := `# Some Random Title

## Tasks

### T1: Do something

**Requires:** —
`
	st, err := InitStateFromPlan(content, "fallback-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use the first heading as title (fallback)
	if st.Title != "Some Random Title" {
		t.Errorf("expected 'Some Random Title', got %q", st.Title)
	}
}

func TestInitStateFromPlan_EmptyContent(t *testing.T) {
	st, err := InitStateFromPlan("", "empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use plan ID as title fallback
	if st.Title != "empty" {
		t.Errorf("expected title 'empty', got %q", st.Title)
	}
	if len(st.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(st.Tasks))
	}
}

func TestInitStateFromPlan_RealPlanFixture(t *testing.T) {
	// Test with a fixture file that mimics a real plan
	fixture := filepath.Join("testdata", "init-fixture-plan.md")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("skipping fixture test: %v", err)
	}

	st, err := InitStateFromPlan(string(data), "fixture-plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(st.Tasks) == 0 {
		t.Error("expected tasks from fixture, got none")
	}

	// Verify all tasks have valid IDs
	for _, task := range st.Tasks {
		if parseTaskNum(task.ID) < 0 {
			t.Errorf("invalid task ID: %q", task.ID)
		}
	}
}

func TestInitStateFromPlan_SaveAndLoad(t *testing.T) {
	// Test that an initialized state can be saved and loaded (round-trip)
	st, err := InitStateFromPlan(testPlanMultipleTasks, "round-trip-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dir := t.TempDir()
	if err := SaveState(st, dir); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	if loaded.ID != st.ID {
		t.Errorf("ID mismatch: %q vs %q", loaded.ID, st.ID)
	}
	if loaded.Title != st.Title {
		t.Errorf("title mismatch: %q vs %q", loaded.Title, st.Title)
	}
	if len(loaded.Tasks) != len(st.Tasks) {
		t.Fatalf("task count mismatch: %d vs %d", len(loaded.Tasks), len(st.Tasks))
	}
	for i, task := range loaded.Tasks {
		if task.ID != st.Tasks[i].ID {
			t.Errorf("task %d ID mismatch: %q vs %q", i, task.ID, st.Tasks[i].ID)
		}
		if task.Title != st.Tasks[i].Title {
			t.Errorf("task %d title mismatch: %q vs %q", i, task.Title, st.Tasks[i].Title)
		}
		if len(task.Criteria) != len(st.Tasks[i].Criteria) {
			t.Errorf("task %d criteria count mismatch: %d vs %d", i, len(task.Criteria), len(st.Tasks[i].Criteria))
		}
	}
}
