package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/state"
)

func createTestBundle(t *testing.T, dir string) string {
	t.Helper()
	bundleDir := filepath.Join(dir, "test-plan")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("creating bundle dir: %v", err)
	}
	// Create plan.md
	os.WriteFile(filepath.Join(bundleDir, "plan.md"), []byte("# Plan: Test\n**Status:** active\n"), 0644)
	return bundleDir
}

func createTestState(t *testing.T, bundleDir string) *state.PlanState {
	t.Helper()
	now := time.Date(2026, 2, 6, 10, 0, 0, 0, time.UTC)
	st := &state.PlanState{
		ID:        "test-plan",
		Title:     "Test Plan",
		Status:    state.PlanStatusActive,
		CreatedAt: now,
		Tasks: []state.TaskState{
			{
				ID:     "T1",
				Title:  "First task",
				Status: state.TaskStatusDone,
				Criteria: []state.Criterion{
					{Text: "Tests pass", Done: true, DoneAt: &now},
				},
				DoneAt: &now,
			},
			{
				ID:       "T2",
				Title:    "Second task",
				Status:   state.TaskStatusTodo,
				Requires: []string{"T1"},
				Criteria: []state.Criterion{
					{Text: "Endpoint works", Done: false},
					{Text: "Tests pass", Done: false},
				},
			},
		},
	}
	if err := state.SaveState(st, bundleDir); err != nil {
		t.Fatalf("saving state: %v", err)
	}
	return st
}

func TestRunContext_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := createTestBundle(t, tmpDir)
	createTestState(t, bundleDir)

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	contextJSON = true
	defer func() { contextJSON = false }()

	err := runContext(nil, []string{bundleDir})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	output := buf.String()

	// Verify it's valid JSON
	var payload state.ContextPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	// Verify payload contents
	if payload.Plan.ID != "test-plan" {
		t.Errorf("expected plan ID 'test-plan', got %q", payload.Plan.ID)
	}
	if payload.Plan.Title != "Test Plan" {
		t.Errorf("expected plan title 'Test Plan', got %q", payload.Plan.Title)
	}
	if len(payload.Tasks.Items) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(payload.Tasks.Items))
	}
	if payload.Summary.Total != 2 {
		t.Errorf("expected total=2, got %d", payload.Summary.Total)
	}
	if payload.Summary.DoneRatio != 0.5 {
		t.Errorf("expected done_ratio=0.5, got %f", payload.Summary.DoneRatio)
	}
	if payload.Selection.SuggestedNext == nil {
		t.Fatal("expected suggested_next to be set")
	}
	if payload.Selection.SuggestedNext.TaskID != "T2" {
		t.Errorf("expected suggested_next T2, got %q", payload.Selection.SuggestedNext.TaskID)
	}
}

func TestRunContext_Human(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := createTestBundle(t, tmpDir)
	createTestState(t, bundleDir)

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	contextJSON = false

	err := runContext(nil, []string{bundleDir})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	output := buf.String()

	// Check key sections present
	if !strings.Contains(output, "Test Plan") {
		t.Errorf("expected plan title, got: %s", output)
	}
	if !strings.Contains(output, "test-plan") {
		t.Errorf("expected plan ID, got: %s", output)
	}
	if !strings.Contains(output, "1/2 tasks done") {
		t.Errorf("expected progress summary, got: %s", output)
	}
	if !strings.Contains(output, "T2") {
		t.Errorf("expected suggested next T2, got: %s", output)
	}
	if !strings.Contains(output, "T1") {
		t.Errorf("expected T1 in tasks, got: %s", output)
	}
}

func TestRunContext_NoStateYaml(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := createTestBundle(t, tmpDir)
	// No state.yaml created

	err := runContext(nil, []string{bundleDir})

	if err == nil {
		t.Fatal("expected error for missing state.yaml")
	}
	if !strings.Contains(err.Error(), "no state.yaml") {
		t.Errorf("expected 'no state.yaml' error, got: %v", err)
	}
}

func TestRunContext_NonexistentPath(t *testing.T) {
	err := runContext(nil, []string{"/nonexistent/path"})

	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' error, got: %v", err)
	}
}

func TestRunContext_PlanFilePath(t *testing.T) {
	// Passing plan.md file path should also work (resolves to bundle dir)
	tmpDir := t.TempDir()
	bundleDir := createTestBundle(t, tmpDir)
	createTestState(t, bundleDir)

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	contextJSON = true
	defer func() { contextJSON = false }()

	err := runContext(nil, []string{filepath.Join(bundleDir, "plan.md")})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify it's valid JSON
	var payload state.ContextPayload
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if payload.Plan.ID != "test-plan" {
		t.Errorf("expected plan ID 'test-plan', got %q", payload.Plan.ID)
	}
}

func TestResolveBundleDir_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := createTestBundle(t, tmpDir)

	result, err := resolveBundleDir(bundleDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	absExpected, _ := filepath.Abs(bundleDir)
	if result != absExpected {
		t.Errorf("expected %q, got %q", absExpected, result)
	}
}

func TestResolveBundleDir_PlanFile(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := createTestBundle(t, tmpDir)
	planFile := filepath.Join(bundleDir, "plan.md")

	result, err := resolveBundleDir(planFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	absExpected, _ := filepath.Abs(bundleDir)
	if result != absExpected {
		t.Errorf("expected %q, got %q", absExpected, result)
	}
}

func TestResolveBundleDir_FlatFile(t *testing.T) {
	tmpDir := t.TempDir()
	flatFile := filepath.Join(tmpDir, "my-plan.md")
	os.WriteFile(flatFile, []byte("# Plan: Test\n"), 0644)

	_, err := resolveBundleDir(flatFile)
	if err == nil {
		t.Fatal("expected error for flat file")
	}
	if !strings.Contains(err.Error(), "not a bundle") {
		t.Errorf("expected 'not a bundle' error, got: %v", err)
	}
}
