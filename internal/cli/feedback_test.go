package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arvesolland/ralph/internal/state"
)

func createFeedbackTestBundle(t *testing.T) (string, *state.PlanState) {
	t.Helper()
	dir := t.TempDir()
	bundleDir := filepath.Join(dir, "fb-plan")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("creating bundle dir: %v", err)
	}
	os.WriteFile(filepath.Join(bundleDir, "plan.md"), []byte("# Plan: Feedback Test\n"), 0644)

	st := &state.PlanState{
		ID:     "fb-plan",
		Title:  "Feedback Plan",
		Status: state.PlanStatusActive,
		Tasks: []state.TaskState{
			{ID: "T1", Title: "First task", Status: state.TaskStatusTodo},
		},
	}
	if err := state.SaveState(st, bundleDir); err != nil {
		t.Fatalf("saving state: %v", err)
	}
	return bundleDir, st
}

func TestRunFeedbackAdd(t *testing.T) {
	bundleDir, _ := createFeedbackTestBundle(t)

	feedbackAddScope = "plan"
	feedbackAddMessage = "Consider using JWT"
	feedbackAddAuthor = "human"
	feedbackJSON = false
	defer func() {
		feedbackAddScope = ""
		feedbackAddMessage = ""
		feedbackAddAuthor = ""
	}()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runFeedbackAdd(nil, []string{bundleDir})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "F1") {
		t.Errorf("expected F1 in output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "Consider using JWT") {
		t.Errorf("expected message in output, got: %s", buf.String())
	}

	// Verify state
	st, _ := state.LoadState(bundleDir)
	if len(st.Feedback) != 1 {
		t.Fatalf("expected 1 feedback, got %d", len(st.Feedback))
	}
	if st.Feedback[0].ID != "F1" {
		t.Errorf("expected F1, got %s", st.Feedback[0].ID)
	}
	if st.Feedback[0].Author != "human" {
		t.Errorf("expected 'human' author, got %s", st.Feedback[0].Author)
	}
}

func TestRunFeedbackAdd_JSON(t *testing.T) {
	bundleDir, _ := createFeedbackTestBundle(t)

	feedbackAddScope = "task:T1"
	feedbackAddMessage = "Needs more tests"
	feedbackAddAuthor = "agent"
	feedbackJSON = true
	defer func() {
		feedbackAddScope = ""
		feedbackAddMessage = ""
		feedbackAddAuthor = ""
		feedbackJSON = false
	}()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runFeedbackAdd(nil, []string{bundleDir})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result state.Feedback
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if result.ID != "F1" {
		t.Errorf("expected F1, got %s", result.ID)
	}
	if result.Scope != "task:T1" {
		t.Errorf("expected scope task:T1, got %s", result.Scope)
	}
}

func TestRunFeedbackAdd_InvalidScope(t *testing.T) {
	bundleDir, _ := createFeedbackTestBundle(t)

	feedbackAddScope = "task:T99"
	feedbackAddMessage = "Bad scope"
	feedbackAddAuthor = "human"
	feedbackJSON = false
	defer func() {
		feedbackAddScope = ""
		feedbackAddMessage = ""
		feedbackAddAuthor = ""
	}()

	err := runFeedbackAdd(nil, []string{bundleDir})
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestRunFeedbackResolve(t *testing.T) {
	bundleDir, _ := createFeedbackTestBundle(t)

	// Add a feedback first
	st, _ := state.LoadState(bundleDir)
	state.AddFeedback(st, "plan", "human", "Some feedback")
	state.SaveState(st, bundleDir)

	feedbackJSON = false

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runFeedbackResolve(nil, []string{bundleDir, "F1"})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Resolved feedback F1") {
		t.Errorf("expected resolve message, got: %s", buf.String())
	}

	// Verify state
	st, _ = state.LoadState(bundleDir)
	if !st.Feedback[0].Resolved {
		t.Error("expected feedback to be resolved")
	}
}

func TestRunFeedbackResolve_JSON(t *testing.T) {
	bundleDir, _ := createFeedbackTestBundle(t)

	st, _ := state.LoadState(bundleDir)
	state.AddFeedback(st, "plan", "human", "Some feedback")
	state.SaveState(st, bundleDir)

	feedbackJSON = true
	defer func() { feedbackJSON = false }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runFeedbackResolve(nil, []string{bundleDir, "F1"})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if result["feedback_id"] != "F1" {
		t.Errorf("expected F1, got %s", result["feedback_id"])
	}
	if result["resolved"] != "true" {
		t.Errorf("expected resolved=true, got %s", result["resolved"])
	}
}

func TestRunFeedbackResolve_NotFound(t *testing.T) {
	bundleDir, _ := createFeedbackTestBundle(t)

	feedbackJSON = false
	err := runFeedbackResolve(nil, []string{bundleDir, "F99"})
	if err == nil {
		t.Fatal("expected error for missing feedback")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestRunFeedbackAdd_NoStateYaml(t *testing.T) {
	dir := t.TempDir()
	bundleDir := filepath.Join(dir, "empty-plan")
	os.MkdirAll(bundleDir, 0755)
	os.WriteFile(filepath.Join(bundleDir, "plan.md"), []byte("# Plan\n"), 0644)

	feedbackAddScope = "plan"
	feedbackAddMessage = "Test"
	feedbackAddAuthor = "human"
	feedbackJSON = false
	defer func() {
		feedbackAddScope = ""
		feedbackAddMessage = ""
		feedbackAddAuthor = ""
	}()

	err := runFeedbackAdd(nil, []string{bundleDir})
	if err == nil {
		t.Fatal("expected error for missing state.yaml")
	}
	if !strings.Contains(err.Error(), "no state.yaml") {
		t.Errorf("expected 'no state.yaml' error, got: %v", err)
	}
}
