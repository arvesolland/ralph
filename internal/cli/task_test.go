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

func createTaskTestBundle(t *testing.T) (string, *state.PlanState) {
	t.Helper()
	dir := t.TempDir()
	bundleDir := filepath.Join(dir, "test-plan")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("creating bundle dir: %v", err)
	}
	os.WriteFile(filepath.Join(bundleDir, "plan.md"), []byte("# Plan: Test\n"), 0644)

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
	return bundleDir, st
}

func TestRunTaskAdd(t *testing.T) {
	bundleDir, _ := createTaskTestBundle(t)

	taskAddTitle = "New task"
	taskAddRequires = "T1"
	taskAddCriteria = "Tests pass;Lint clean"
	taskJSON = false
	defer func() {
		taskAddTitle = ""
		taskAddRequires = ""
		taskAddCriteria = ""
	}()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runTaskAdd(nil, []string{bundleDir})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "T3") {
		t.Errorf("expected T3 in output, got: %s", output)
	}
	if !strings.Contains(output, "New task") {
		t.Errorf("expected 'New task' in output, got: %s", output)
	}

	// Verify state was saved
	st, err := state.LoadState(bundleDir)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	if len(st.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(st.Tasks))
	}
	if st.Tasks[2].ID != "T3" {
		t.Errorf("expected T3, got %s", st.Tasks[2].ID)
	}
	if len(st.Tasks[2].Criteria) != 2 {
		t.Errorf("expected 2 criteria, got %d", len(st.Tasks[2].Criteria))
	}
}

func TestRunTaskAdd_JSON(t *testing.T) {
	bundleDir, _ := createTaskTestBundle(t)

	taskAddTitle = "JSON task"
	taskAddRequires = ""
	taskAddCriteria = ""
	taskJSON = true
	defer func() {
		taskAddTitle = ""
		taskJSON = false
	}()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runTaskAdd(nil, []string{bundleDir})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result state.TaskState
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, buf.String())
	}
	if result.ID != "T3" {
		t.Errorf("expected T3, got %s", result.ID)
	}
	if result.Title != "JSON task" {
		t.Errorf("expected 'JSON task', got %s", result.Title)
	}
}

func TestRunTaskClaim(t *testing.T) {
	bundleDir, _ := createTaskTestBundle(t)

	taskJSON = false

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runTaskClaim(nil, []string{bundleDir, "T2"})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "Claimed task T2") {
		t.Errorf("expected claim message, got: %s", buf.String())
	}

	// Verify state
	st, _ := state.LoadState(bundleDir)
	for _, task := range st.Tasks {
		if task.ID == "T2" {
			if task.Status != state.TaskStatusDoing {
				t.Errorf("expected doing, got %s", task.Status)
			}
			return
		}
	}
	t.Fatal("T2 not found in state")
}

func TestRunTaskClaim_JSON(t *testing.T) {
	bundleDir, _ := createTaskTestBundle(t)

	taskJSON = true
	defer func() { taskJSON = false }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runTaskClaim(nil, []string{bundleDir, "T2"})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["task_id"] != "T2" {
		t.Errorf("expected T2, got %s", result["task_id"])
	}
	if result["status"] != "doing" {
		t.Errorf("expected doing, got %s", result["status"])
	}
}

func TestRunTaskComplete(t *testing.T) {
	bundleDir, _ := createTaskTestBundle(t)

	// First claim T2 and check all criteria
	state.ClaimTask(must(state.LoadState(bundleDir)), "T2")
	st, _ := state.LoadState(bundleDir)
	state.ClaimTask(st, "T2")
	state.CheckCriterion(st, "T2", 1)
	state.CheckCriterion(st, "T2", 2)
	state.SaveState(st, bundleDir)

	taskCompleteCommits = "abc123,def456"
	taskJSON = false
	defer func() { taskCompleteCommits = "" }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runTaskComplete(nil, []string{bundleDir, "T2"})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Completed task T2") {
		t.Errorf("expected complete message, got: %s", buf.String())
	}

	// Verify state
	st, _ = state.LoadState(bundleDir)
	for _, task := range st.Tasks {
		if task.ID == "T2" {
			if task.Status != state.TaskStatusDone {
				t.Errorf("expected done, got %s", task.Status)
			}
			if len(task.Artifacts.Commits) != 2 {
				t.Errorf("expected 2 commits, got %d", len(task.Artifacts.Commits))
			}
			return
		}
	}
	t.Fatal("T2 not found")
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestRunTaskSkip(t *testing.T) {
	bundleDir, _ := createTaskTestBundle(t)

	taskSkipReason = "Not needed"
	taskJSON = false
	defer func() { taskSkipReason = "" }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runTaskSkip(nil, []string{bundleDir, "T2"})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Skipped task T2") {
		t.Errorf("expected skip message, got: %s", buf.String())
	}

	st, _ := state.LoadState(bundleDir)
	for _, task := range st.Tasks {
		if task.ID == "T2" {
			if task.Status != state.TaskStatusSkipped {
				t.Errorf("expected skipped, got %s", task.Status)
			}
			return
		}
	}
}

func TestRunTaskCriterionCheck(t *testing.T) {
	bundleDir, _ := createTaskTestBundle(t)

	taskJSON = false

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runTaskCriterionCheck(nil, []string{bundleDir, "T2", "1"})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Checked criterion 1") {
		t.Errorf("expected check message, got: %s", buf.String())
	}

	st, _ := state.LoadState(bundleDir)
	for _, task := range st.Tasks {
		if task.ID == "T2" {
			if !task.Criteria[0].Done {
				t.Error("expected criterion 1 to be done")
			}
			return
		}
	}
}

func TestRunTaskCriterionUncheck(t *testing.T) {
	bundleDir, _ := createTaskTestBundle(t)

	// First check the criterion
	st, _ := state.LoadState(bundleDir)
	state.CheckCriterion(st, "T2", 1)
	state.SaveState(st, bundleDir)

	taskJSON = false

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runTaskCriterionUncheck(nil, []string{bundleDir, "T2", "1"})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Unchecked criterion 1") {
		t.Errorf("expected uncheck message, got: %s", buf.String())
	}

	st, _ = state.LoadState(bundleDir)
	for _, task := range st.Tasks {
		if task.ID == "T2" {
			if task.Criteria[0].Done {
				t.Error("expected criterion 1 to be not done")
			}
			return
		}
	}
}

func TestRunTaskCriterionCheck_JSON(t *testing.T) {
	bundleDir, _ := createTaskTestBundle(t)

	taskJSON = true
	defer func() { taskJSON = false }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runTaskCriterionCheck(nil, []string{bundleDir, "T2", "2"})

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if result["task_id"] != "T2" {
		t.Errorf("expected T2, got %v", result["task_id"])
	}
	if result["criterion"] != float64(2) {
		t.Errorf("expected criterion 2, got %v", result["criterion"])
	}
	if result["done"] != true {
		t.Errorf("expected done=true, got %v", result["done"])
	}
}

func TestRunTaskCriterionCheck_InvalidIndex(t *testing.T) {
	bundleDir, _ := createTaskTestBundle(t)
	taskJSON = false

	err := runTaskCriterionCheck(nil, []string{bundleDir, "T2", "abc"})
	if err == nil {
		t.Fatal("expected error for invalid index")
	}
	if !strings.Contains(err.Error(), "invalid criterion index") {
		t.Errorf("expected 'invalid criterion index' error, got: %v", err)
	}
}

func TestRunTaskAdd_NoStateYaml(t *testing.T) {
	dir := t.TempDir()
	bundleDir := filepath.Join(dir, "empty-plan")
	os.MkdirAll(bundleDir, 0755)
	os.WriteFile(filepath.Join(bundleDir, "plan.md"), []byte("# Plan\n"), 0644)

	taskAddTitle = "New task"
	taskJSON = false
	defer func() { taskAddTitle = "" }()

	err := runTaskAdd(nil, []string{bundleDir})
	if err == nil {
		t.Fatal("expected error for missing state.yaml")
	}
	if !strings.Contains(err.Error(), "no state.yaml") {
		t.Errorf("expected 'no state.yaml' error, got: %v", err)
	}
}

func TestRunTaskClaim_DepsNotMet(t *testing.T) {
	dir := t.TempDir()
	bundleDir := filepath.Join(dir, "deps-plan")
	os.MkdirAll(bundleDir, 0755)
	os.WriteFile(filepath.Join(bundleDir, "plan.md"), []byte("# Plan\n"), 0644)

	st := &state.PlanState{
		ID:     "deps-plan",
		Title:  "Deps Plan",
		Status: state.PlanStatusActive,
		Tasks: []state.TaskState{
			{ID: "T1", Title: "First", Status: state.TaskStatusTodo},
			{ID: "T2", Title: "Second", Status: state.TaskStatusTodo, Requires: []string{"T1"}},
		},
	}
	state.SaveState(st, bundleDir)

	taskJSON = false
	err := runTaskClaim(nil, []string{bundleDir, "T2"})
	if err == nil {
		t.Fatal("expected error for unmet deps")
	}
	if !strings.Contains(err.Error(), "dependency") {
		t.Errorf("expected dependency error, got: %v", err)
	}
}

func TestParseCommaSep(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"T1", []string{"T1"}},
		{"T1,T2,T3", []string{"T1", "T2", "T3"}},
		{" T1 , T2 ", []string{"T1", "T2"}},
		{"T1,,T2", []string{"T1", "T2"}},
	}
	for _, tt := range tests {
		result := parseCommaSep(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseCommaSep(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("parseCommaSep(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestParseSemicolonSep(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"Tests pass", []string{"Tests pass"}},
		{"Tests pass;Lint clean", []string{"Tests pass", "Lint clean"}},
		{" a ; b ", []string{"a", "b"}},
	}
	for _, tt := range tests {
		result := parseSemicolonSep(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseSemicolonSep(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("parseSemicolonSep(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
			}
		}
	}
}
