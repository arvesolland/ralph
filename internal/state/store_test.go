package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestStatePath(t *testing.T) {
	got := StatePath("/tmp/plans/current/my-plan")
	want := filepath.Join("/tmp/plans/current/my-plan", "state.yaml")
	if got != want {
		t.Errorf("StatePath() = %q, want %q", got, want)
	}
}

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	original := sampleState()

	if err := SaveState(original, dir); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	// Verify file was created
	path := StatePath(dir)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state.yaml not created: %v", err)
	}

	// Load and compare
	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState() error: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadState() returned nil")
	}

	if !reflect.DeepEqual(original, loaded) {
		t.Errorf("loaded state doesn't match original.\nOriginal: %+v\nLoaded:   %+v", original, loaded)
	}
}

func TestLoadStateMissingFile(t *testing.T) {
	dir := t.TempDir()

	state, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState() should not error for missing file, got: %v", err)
	}
	if state != nil {
		t.Errorf("LoadState() should return nil for missing file, got: %+v", state)
	}
}

func TestLoadStateInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := StatePath(dir)

	// Write invalid YAML
	if err := os.WriteFile(path, []byte("{{invalid yaml: [}"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	state, err := LoadState(dir)
	if err == nil {
		t.Fatal("LoadState() should error for invalid YAML")
	}
	if state != nil {
		t.Errorf("LoadState() should return nil on error, got: %+v", state)
	}
}

func TestSaveStateCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")

	state := &PlanState{
		ID:        "test",
		Title:     "Test Plan",
		Status:    PlanStatusDraft,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Tasks: []TaskState{
			{ID: "T1", Title: "Task", Status: TaskStatusTodo},
		},
	}

	if err := SaveState(state, nested); err != nil {
		t.Fatalf("SaveState() should create nested dirs, got error: %v", err)
	}

	if _, err := os.Stat(StatePath(nested)); err != nil {
		t.Errorf("state.yaml not found after SaveState to nested dir: %v", err)
	}
}

func TestSaveStateAtomicWrite(t *testing.T) {
	dir := t.TempDir()

	state := &PlanState{
		ID:        "atomic-test",
		Title:     "Atomic Write Test",
		Status:    PlanStatusActive,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Tasks: []TaskState{
			{ID: "T1", Title: "Task 1", Status: TaskStatusTodo},
		},
	}

	// Save initial state
	if err := SaveState(state, dir); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	// Save updated state (overwrites atomically)
	state.Tasks[0].Status = TaskStatusDone
	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	state.Tasks[0].DoneAt = &now

	if err := SaveState(state, dir); err != nil {
		t.Fatalf("SaveState() overwrite error: %v", err)
	}

	// Verify the update was saved
	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState() error: %v", err)
	}
	if loaded.Tasks[0].Status != TaskStatusDone {
		t.Errorf("expected task status %q, got %q", TaskStatusDone, loaded.Tasks[0].Status)
	}

	// Verify no temp file left behind
	tmpPath := StatePath(dir) + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temp file should not exist after successful save: %s", tmpPath)
	}
}

func TestLoadStateUnreadableDir(t *testing.T) {
	// LoadState on a non-existent directory (not just missing file)
	state, err := LoadState("/nonexistent/dir/that/does/not/exist")
	if err != nil {
		t.Fatalf("LoadState() should not error for non-existent dir, got: %v", err)
	}
	if state != nil {
		t.Errorf("LoadState() should return nil for non-existent dir, got: %+v", state)
	}
}
