//go:build integration

// Package integration provides end-to-end tests for the Ralph CLI.
// These tests run the actual ralph binary against test plans using real Claude CLI
// and a fake atm-cli binary for task management.
//
// Run with: go test -tags=integration -v ./test/integration/...
//
// Requirements:
// - Claude CLI available in PATH
// - ralph binary built (run `make build` first)
package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/atm"
)

// testTimeout is the maximum time for a single test
const testTimeout = 10 * time.Minute

// maxIterations limits iterations during tests
const maxIterations = 5

// ralphBinary is the path to the ralph binary (set in TestMain)
var ralphBinary string

// fakeATMBinary is the path to the fake atm-cli binary (set in TestMain)
var fakeATMBinary string

func TestMain(m *testing.M) {
	// Find ralph binary (relative to test directory)
	candidates := []string{
		"./ralph",
		"../../ralph",
		filepath.Join(os.Getenv("GOPATH"), "bin", "ralph"),
	}

	for _, candidate := range candidates {
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absPath); err == nil {
			ralphBinary = absPath
			break
		}
	}

	if ralphBinary == "" {
		fmt.Fprintln(os.Stderr, "ERROR: ralph binary not found. Run 'make build' first.")
		os.Exit(1)
	}

	// Verify claude CLI is available
	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR: claude CLI not found in PATH")
		os.Exit(1)
	}

	// Build the fake atm-cli binary
	tmpDir, err := os.MkdirTemp("", "ralph-fakeatm-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to create temp dir for fakeatm: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	fakeATMBinary = filepath.Join(tmpDir, "fakeatm")

	// Find repo root for building
	repoRoot := ""
	cwd, _ := os.Getwd()
	// Try cwd first (may be repo root if running from root)
	if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
		repoRoot = cwd
	} else if _, err := os.Stat(filepath.Join(cwd, "..", "..", "go.mod")); err == nil {
		// Running from test/integration/
		repoRoot, _ = filepath.Abs(filepath.Join(cwd, "..", ".."))
	}

	if repoRoot == "" {
		fmt.Fprintln(os.Stderr, "ERROR: cannot find repo root (go.mod)")
		os.Exit(1)
	}

	buildCmd := exec.Command("go", "build", "-o", fakeATMBinary, "./test/integration/fakeatm/")
	buildCmd.Dir = repoRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to build fake atm-cli: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// =============================================================================
// ATM STATE TYPES (mirror of fakeatm/state.go, for test assertions)
// =============================================================================

// atmState is the JSON state file format used by the fake atm-cli.
type atmState struct {
	Projects []atm.Project  `json:"projects"`
	Plans    []atm.Plan     `json:"plans"`
	Tasks    []atm.Task     `json:"tasks"`
	Criteria []atm.Criterion `json:"criteria"`
	Progress []atm.Progress `json:"progress"`
	Feedback []atm.Feedback `json:"feedback"`

	NextProjectID   int `json:"next_project_id"`
	NextPlanID      int `json:"next_plan_id"`
	NextTaskID      int `json:"next_task_id"`
	NextCriterionID int `json:"next_criterion_id"`
	NextProgressID  int `json:"next_progress_id"`
	NextFeedbackID  int `json:"next_feedback_id"`
}

func loadATMState(path string) (*atmState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &atmState{
				NextProjectID:   1,
				NextPlanID:      1,
				NextTaskID:      1,
				NextCriterionID: 1,
				NextProgressID:  1,
				NextFeedbackID:  1,
			}, nil
		}
		return nil, err
	}
	var s atmState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveATMState(path string, s *atmState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func seedProject(s *atmState, name, slug string) atm.Project {
	p := atm.Project{
		ID:        s.NextProjectID,
		Name:      name,
		Slug:      slug,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.NextProjectID++
	s.Projects = append(s.Projects, p)
	return p
}

func seedPlan(s *atmState, projectID int, title, status, branch string) atm.Plan {
	p := atm.Plan{
		ID:            s.NextPlanID,
		ProjectID:     projectID,
		Title:         title,
		Status:        status,
		FeatureBranch: branch,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	s.NextPlanID++
	s.Plans = append(s.Plans, p)
	return p
}

func seedTask(s *atmState, planID int, title, description string, deps []int) atm.Task {
	// Count existing tasks for this plan to determine position.
	position := 1
	for _, t := range s.Tasks {
		if t.PlanID == planID {
			position++
		}
	}

	t := atm.Task{
		ID:          s.NextTaskID,
		PlanID:      planID,
		Title:       title,
		Description: description,
		Status:      atm.TaskStatusTodo,
		Position:    position,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	for _, depID := range deps {
		t.BlockedBy = append(t.BlockedBy, atm.Task{ID: depID})
	}
	s.NextTaskID++
	s.Tasks = append(s.Tasks, t)
	return t
}

func seedCriterion(s *atmState, taskID int, description string) atm.Criterion {
	// Count existing criteria for this task to determine position.
	position := 1
	for _, c := range s.Criteria {
		if c.TaskID == taskID {
			position++
		}
	}

	c := atm.Criterion{
		ID:          s.NextCriterionID,
		TaskID:      taskID,
		Description: description,
		Position:    position,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	s.NextCriterionID++
	s.Criteria = append(s.Criteria, c)
	return c
}

// =============================================================================
// TASK DEFINITION HELPERS
// =============================================================================

// TaskDef defines a task for seeding into the fake ATM state.
type TaskDef struct {
	Title    string
	Requires []string // task titles that this task depends on
	Criteria []string // acceptance criteria descriptions
}

// =============================================================================
// TEST: Single Task (ralph run --plan <id>)
// =============================================================================

func TestSingleTask(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	planID := ws.SeedATMPlan("single-task-test", "active", []TaskDef{
		{
			Title:    "Create marker file",
			Criteria: []string{`File output/marker.txt exists with content "ralph-test-complete"`},
		},
	})

	ws.RunRalph(t, "run", "--plan", fmt.Sprintf("%d", planID), "--max", fmt.Sprintf("%d", maxIterations), "-v")

	// Verify results
	ws.AssertFileExists(t, "output/marker.txt", "Marker file should be created")
	ws.AssertFileContains(t, "output/marker.txt", "ralph-test-complete", "Marker should have correct content")

	// Verify ATM state
	state := ws.ReadATMState(t)
	for _, task := range state.Tasks {
		if task.PlanID == planID {
			if task.Status != atm.TaskStatusDone {
				t.Errorf("Task %q should be done in ATM, got status %q", task.Title, task.Status)
			}
		}
	}
}

// =============================================================================
// TEST: Task Dependencies (ralph run --plan <id>)
// =============================================================================

func TestDependencies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	planID := ws.SeedATMPlan("dependencies-test", "active", []TaskDef{
		{
			Title: "Create first file",
			Criteria: []string{
				`File output/first.txt exists with content "step-1-done"`,
			},
		},
		{
			Title:    "Create second file",
			Requires: []string{"Create first file"},
			Criteria: []string{
				`File output/second.txt exists with content "step-2-done"`,
				`File output/first.txt still exists`,
			},
		},
	})

	ws.RunRalph(t, "run", "--plan", fmt.Sprintf("%d", planID), "--max", fmt.Sprintf("%d", maxIterations), "-v")

	// Both files should exist
	ws.AssertFileExists(t, "output/first.txt", "First file should exist")
	ws.AssertFileContains(t, "output/first.txt", "step-1-done", "First file correct content")
	ws.AssertFileExists(t, "output/second.txt", "Second file should exist")
	ws.AssertFileContains(t, "output/second.txt", "step-2-done", "Second file correct content")

	// Verify ATM state: both tasks done
	state := ws.ReadATMState(t)
	for _, task := range state.Tasks {
		if task.PlanID == planID {
			if task.Status != atm.TaskStatusDone {
				t.Errorf("Task %q should be done in ATM, got status %q", task.Title, task.Status)
			}
		}
	}
}

// =============================================================================
// TEST: Progress Tracking (ralph run --plan <id>)
// =============================================================================

func TestProgressTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	planID := ws.SeedATMPlan("progress-test", "active", []TaskDef{
		{
			Title: "Create encoded file",
			Criteria: []string{
				`File output/encoded.txt exists with base64-encoded content`,
			},
		},
	})

	ws.RunRalph(t, "run", "--plan", fmt.Sprintf("%d", planID), "--max", fmt.Sprintf("%d", maxIterations), "-v")

	ws.AssertFileExists(t, "output/encoded.txt", "Encoded file should exist")

	// Verify ATM progress entries exist
	state := ws.ReadATMState(t)
	progressEntries := 0
	for _, p := range state.Progress {
		if p.PlanID == planID {
			progressEntries++
		}
	}
	if progressEntries == 0 {
		t.Error("Expected at least one ATM progress entry for the plan")
	} else {
		t.Logf("Found %d ATM progress entries", progressEntries)
	}
}

// =============================================================================
// TEST: One Task Per Iteration (agent must stop after each task)
// =============================================================================

func TestOneTaskPerIteration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.KeepOnFailure()
	defer ws.Cleanup()

	planID := ws.SeedATMPlan("one-task-per-iteration-test", "active", []TaskDef{
		{
			Title:    "Create marker A",
			Criteria: []string{`File output/a.txt exists with content "task-a-done"`},
		},
		{
			Title:    "Create marker B",
			Criteria: []string{`File output/b.txt exists with content "task-b-done"`},
		},
		{
			Title:    "Create marker C",
			Criteria: []string{`File output/c.txt exists with content "task-c-done"`},
		},
	})

	ws.RunRalph(t, "run", "--plan", fmt.Sprintf("%d", planID), "--max", "10", "-v")

	// All 3 files should exist
	ws.AssertFileExists(t, "output/a.txt", "T1 should be completed")
	ws.AssertFileContains(t, "output/a.txt", "task-a-done", "T1 correct content")
	ws.AssertFileExists(t, "output/b.txt", "T2 should be completed")
	ws.AssertFileContains(t, "output/b.txt", "task-b-done", "T2 correct content")
	ws.AssertFileExists(t, "output/c.txt", "T3 should be completed")
	ws.AssertFileContains(t, "output/c.txt", "task-c-done", "T3 correct content")

	// Verify ATM state: all tasks done
	state := ws.ReadATMState(t)
	for _, task := range state.Tasks {
		if task.PlanID == planID && task.Status != atm.TaskStatusDone {
			t.Errorf("Task %q should be done in ATM, got status %q", task.Title, task.Status)
		}
	}

	// KEY ASSERTION: There should be at least 3 task commits (one per task).
	out, err := ws.GitOutput(t, "log", "--oneline")
	if err != nil {
		t.Fatalf("Failed to get git log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	t.Logf("Git log (%d commits):", len(lines))
	for _, line := range lines {
		t.Logf("  %s", line)
	}

	// Count commits that are NOT the initial workspace setup.
	agentCommits := 0
	for _, line := range lines {
		if !strings.Contains(line, "Initial test workspace") {
			agentCommits++
		}
	}

	if agentCommits < 3 {
		t.Errorf("ONE-TASK-PER-ITERATION VIOLATED: expected at least 3 agent commits (one per task), got %d. Agent likely batched multiple tasks into a single iteration.", agentCommits)
	} else {
		t.Logf("SUCCESS: %d agent commits for 3 tasks -- agent respected one-task-per-iteration", agentCommits)
	}

	// Additional check: verify each marker file was introduced in a SEPARATE commit.
	filesInCommits := make(map[string]string) // filename -> commit hash
	for _, marker := range []string{"output/a.txt", "output/b.txt", "output/c.txt"} {
		commitOut, err := ws.GitOutput(t, "log", "--oneline", "--diff-filter=A", "--", marker)
		if err != nil {
			t.Logf("Warning: could not find introducing commit for %s: %v", marker, err)
			continue
		}
		commitLine := strings.TrimSpace(commitOut)
		if commitLine != "" {
			hash := strings.Fields(commitLine)[0]
			filesInCommits[marker] = hash
			t.Logf("  %s introduced in commit %s", marker, hash)
		}
	}

	uniqueCommits := make(map[string]bool)
	for _, hash := range filesInCommits {
		uniqueCommits[hash] = true
	}
	if len(filesInCommits) == 3 && len(uniqueCommits) < 3 {
		t.Errorf("ONE-TASK-PER-ITERATION VIOLATED: marker files were introduced in only %d unique commits (expected 3 separate commits)", len(uniqueCommits))
	} else if len(filesInCommits) == 3 && len(uniqueCommits) == 3 {
		t.Logf("SUCCESS: all 3 marker files introduced in separate commits")
	}
}

// =============================================================================
// TEST: Worker Queue (ralph worker --once --merge)
// =============================================================================

func TestWorkerQueue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	planID := ws.SeedATMPlan("worker-queue-test", "ready", []TaskDef{
		{
			Title:    "Create worker marker",
			Criteria: []string{`File output/worker-marker.txt exists with content "worker-test-complete"`},
		},
	})

	ws.RunRalph(t, "worker", "--once", "--merge", "--max", fmt.Sprintf("%d", maxIterations), "-v")

	// Main worktree should stay on main
	ws.AssertOnBranch(t, "main", "Main worktree should stay on main")

	// Work should be merged to main
	ws.AssertFileExists(t, "output/worker-marker.txt", "Marker file should exist after merge")
	ws.AssertFileContains(t, "output/worker-marker.txt", "worker-test-complete", "Marker should have correct content")

	// ATM plan should be complete
	state := ws.ReadATMState(t)
	for _, plan := range state.Plans {
		if plan.ID == planID {
			if plan.Status != atm.PlanStatusComplete {
				t.Errorf("Plan should be 'complete' in ATM, got %q", plan.Status)
			}
		}
	}

	// Worktree should be cleaned up
	ws.AssertWorktreeNotExists(t, "worker-queue-test", "Worktree should be cleaned up after completion")
}

// =============================================================================
// TEST: Dirty State Handling (worktree isolation)
// =============================================================================

func TestDirtyState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	// Create uncommitted changes in main worktree
	dirtyFile := filepath.Join(ws.Path, "dirty-file.txt")
	if err := os.WriteFile(dirtyFile, []byte("uncommitted change"), 0644); err != nil {
		t.Fatalf("Failed to create dirty file: %v", err)
	}
	t.Log("Created dirty state in main worktree")

	planID := ws.SeedATMPlan("dirty-state-test", "ready", []TaskDef{
		{
			Title:    "Create marker in dirty workspace",
			Criteria: []string{`File output/dirty-test.txt exists`},
		},
	})

	// Worker should succeed despite dirty main worktree
	ws.RunRalph(t, "worker", "--once", "--merge", "--max", fmt.Sprintf("%d", maxIterations), "-v")

	// Dirty file should still exist (not lost)
	ws.AssertFileExists(t, "dirty-file.txt", "Dirty file should be preserved")

	// Work should complete
	ws.AssertFileExists(t, "output/dirty-test.txt", "Work should complete despite dirty state")

	// ATM plan should be complete
	state := ws.ReadATMState(t)
	for _, plan := range state.Plans {
		if plan.ID == planID {
			if plan.Status != atm.PlanStatusComplete {
				t.Errorf("Plan should be 'complete' in ATM, got %q", plan.Status)
			}
		}
	}
}

// =============================================================================
// TEST: Worktree Cleanup (ralph cleanup)
// =============================================================================

func TestWorktreeCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	// Create an orphaned worktree manually (simulates interrupted execution)
	worktreeDir := filepath.Join(ws.Path, ".ralph", "worktrees", "feat-orphan-plan")
	if err := os.MkdirAll(filepath.Dir(worktreeDir), 0755); err != nil {
		t.Fatalf("Failed to create worktrees dir: %v", err)
	}

	// Create branch and worktree
	ws.Git(t, "branch", "feat/orphan-plan", "main")
	ws.Git(t, "worktree", "add", worktreeDir, "feat/orphan-plan")

	// Verify orphan exists
	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		t.Fatal("Failed to create orphan worktree")
	}
	t.Log("Created orphaned worktree")

	// Run cleanup
	ws.RunRalph(t, "cleanup")

	// Orphan should be cleaned up
	ws.AssertWorktreeNotExists(t, "orphan-plan", "Orphaned worktree should be cleaned up")
}

// =============================================================================
// TEST: Core Principles (comprehensive)
// =============================================================================

func TestCorePrinciples(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	planID := ws.SeedATMPlan("core-principles-test", "ready", []TaskDef{
		{
			Title: "Create first marker",
			Criteria: []string{
				`File output/step1.txt exists with content "step1-complete"`,
			},
		},
		{
			Title:    "Create second marker (depends on T1)",
			Requires: []string{"Create first marker"},
			Criteria: []string{
				`File output/step2.txt exists with content "step2-complete"`,
				`File output/step1.txt still exists`,
			},
		},
		{
			Title:    "Create final marker (depends on T2)",
			Requires: []string{"Create second marker (depends on T1)"},
			Criteria: []string{
				`File output/final.txt exists with content "all-done"`,
				`Both step1.txt and step2.txt still exist`,
			},
		},
	})

	// Run worker
	ws.RunRalph(t, "worker", "--once", "--merge", "--max", "10", "-v")

	// PRINCIPLE 1 & 2: Tasks completed in dependency order
	ws.AssertFileExists(t, "output/step1.txt", "P1: T1 completed")
	ws.AssertFileContains(t, "output/step1.txt", "step1-complete", "P1: T1 correct content")
	ws.AssertFileExists(t, "output/step2.txt", "P1: T2 completed")
	ws.AssertFileContains(t, "output/step2.txt", "step2-complete", "P1: T2 correct content")
	ws.AssertFileExists(t, "output/final.txt", "P1: T3 completed")
	ws.AssertFileContains(t, "output/final.txt", "all-done", "P1: T3 correct content")

	// PRINCIPLE 5: Commits made
	out, _ := ws.GitOutput(t, "log", "--oneline", "main")
	commitCount := len(strings.Split(strings.TrimSpace(out), "\n"))
	if commitCount < 3 {
		t.Errorf("P5: Expected at least 3 commits, got %d", commitCount)
	} else {
		t.Logf("P5: %d commits on main", commitCount)
	}

	// ATM state: all tasks done, plan complete
	state := ws.ReadATMState(t)
	for _, task := range state.Tasks {
		if task.PlanID == planID && task.Status != atm.TaskStatusDone {
			t.Errorf("Task %q should be done in ATM, got status %q", task.Title, task.Status)
		}
	}
	for _, plan := range state.Plans {
		if plan.ID == planID && plan.Status != atm.PlanStatusComplete {
			t.Errorf("Plan should be 'complete' in ATM, got %q", plan.Status)
		}
	}

	// Worktree should be cleaned up
	ws.AssertWorktreeNotExists(t, "core-principles-test", "Worktree cleaned up after completion")
}

// =============================================================================
// TEST: Slack Notifications
// =============================================================================

// slackRequest represents a captured Slack API request
type slackRequest struct {
	Endpoint  string
	Channel   string
	Text      string
	Blocks    []json.RawMessage
	ThreadTS  string
	Timestamp string // for updates
}

// mockSlackServer captures Slack API requests for verification
type mockSlackServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []slackRequest
}

func newMockSlackServer() *mockSlackServer {
	m := &mockSlackServer{
		requests: make([]slackRequest, 0),
	}

	mux := http.NewServeMux()

	// Handle chat.postMessage
	mux.HandleFunc("/chat.postMessage", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		req := slackRequest{
			Endpoint: "chat.postMessage",
			Channel:  r.FormValue("channel"),
			Text:     r.FormValue("text"),
			ThreadTS: r.FormValue("thread_ts"),
		}

		if blocksStr := r.FormValue("blocks"); blocksStr != "" {
			var blocks []json.RawMessage
			if err := json.Unmarshal([]byte(blocksStr), &blocks); err == nil {
				req.Blocks = blocks
			}
		}

		m.requests = append(m.requests, req)

		// Return success with timestamp
		resp := map[string]interface{}{
			"ok":      true,
			"ts":      fmt.Sprintf("1234567890.%06d", len(m.requests)),
			"channel": req.Channel,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// Handle chat.update
	mux.HandleFunc("/chat.update", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		req := slackRequest{
			Endpoint:  "chat.update",
			Channel:   r.FormValue("channel"),
			Timestamp: r.FormValue("ts"),
		}

		if blocksStr := r.FormValue("blocks"); blocksStr != "" {
			var blocks []json.RawMessage
			if err := json.Unmarshal([]byte(blocksStr), &blocks); err == nil {
				req.Blocks = blocks
			}
		}

		m.requests = append(m.requests, req)

		resp := map[string]interface{}{
			"ok":      true,
			"ts":      req.Timestamp,
			"channel": req.Channel,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	m.Server = httptest.NewServer(mux)
	return m
}

func (m *mockSlackServer) getRequests() []slackRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]slackRequest, len(m.requests))
	copy(result, m.requests)
	return result
}

func (m *mockSlackServer) getRequestsByEndpoint(endpoint string) []slackRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []slackRequest
	for _, req := range m.requests {
		if req.Endpoint == endpoint {
			result = append(result, req)
		}
	}
	return result
}

func TestSlackNotifications(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Start mock Slack server
	slackServer := newMockSlackServer()
	defer slackServer.Close()

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	// Update config to include Slack settings with mock server
	configContent := fmt.Sprintf(`project:
  name: "Test Project"
  description: "Integration test with Slack notifications"

git:
  base_branch: "main"

commands:
  test: "echo 'no tests'"
  lint: "echo 'no lint'"

atm:
  project_slug: "test-project"
  bin_path: "%s"
  api_url: "http://localhost:9999"
  api_token: "test-token"

slack:
  bot_token: "xoxb-test-token"
  channel: "C12345TEST"
  notify_start: true
  notify_complete: true
  notify_iteration: false
  notify_error: true
  notify_blocker: true
`, fakeATMBinary)

	if err := os.WriteFile(filepath.Join(ws.Path, ".ralph/config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// Commit the config change
	ws.Git(t, "add", "-A")
	ws.Git(t, "commit", "-q", "-m", "Add Slack config")

	// Seed a plan via fake ATM
	ws.SeedATMPlan("slack-test", "ready", []TaskDef{
		{
			Title:    "Create marker file for slack test",
			Criteria: []string{`File output/done.txt exists with content "slack-test-complete"`},
		},
	})

	// Run ralph worker with mock Slack server URL via environment
	cmd := exec.Command(ralphBinary, "worker", "--once", "--max", fmt.Sprintf("%d", maxIterations), "-v")
	cmd.Dir = ws.Path
	cmd.Env = append(os.Environ(),
		"RALPH_TEST=1",
		"FAKEATM_STATE_PATH="+ws.atmStatePath,
		"SLACK_API_URL="+slackServer.URL+"/",
	)

	t.Logf("Running: ralph worker --once (with mock Slack at %s)", slackServer.URL)
	out, err := cmd.CombinedOutput()
	t.Logf("Output:\n%s", out)

	if err != nil {
		t.Logf("Worker exited with error (expected in test): %v", err)
	}

	// Check Slack requests
	requests := slackServer.getRequests()
	t.Logf("Captured %d Slack requests", len(requests))
	for i, req := range requests {
		t.Logf("  [%d] %s to channel %s (thread: %s, ts: %s)", i, req.Endpoint, req.Channel, req.ThreadTS, req.Timestamp)
	}

	// Verify we got at least a start notification (chat.postMessage)
	postMessages := slackServer.getRequestsByEndpoint("chat.postMessage")
	if len(postMessages) == 0 {
		t.Error("Expected at least one chat.postMessage request (start notification)")
	} else {
		t.Logf("Got %d chat.postMessage requests", len(postMessages))
	}

	// Check that the first message was to the correct channel
	if len(postMessages) > 0 && postMessages[0].Channel != "C12345TEST" {
		t.Errorf("Expected channel C12345TEST, got %s", postMessages[0].Channel)
	}

	// Verify we got progress updates (chat.update calls)
	updates := slackServer.getRequestsByEndpoint("chat.update")
	t.Logf("Got %d progress updates (chat.update)", len(updates))

	if len(updates) == 0 {
		t.Error("Expected at least one chat.update request (progress update)")
	}

	// Verify updates target the correct message timestamp
	for i, update := range updates {
		if update.Timestamp == "" {
			t.Errorf("Update %d has empty timestamp", i)
		}
		if update.Channel != "C12345TEST" {
			t.Errorf("Update %d to wrong channel: got %s, want C12345TEST", i, update.Channel)
		}
	}

	t.Logf("SUCCESS: Slack notifications working - %d posts, %d updates", len(postMessages), len(updates))
}

// =============================================================================
// TEST: ATM Context Failure (bad bin_path)
// =============================================================================

func TestATMContextFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	// Override config with an invalid atm bin_path
	configContent := `project:
  name: "Test Project"
  description: "Integration test with bad ATM config"

git:
  base_branch: "main"

commands:
  test: "echo 'no tests'"
  lint: "echo 'no lint'"

atm:
  project_slug: "test-project"
  bin_path: "/nonexistent/atm-cli"
  api_url: "http://localhost:9999"
  api_token: "test-token"
`

	if err := os.WriteFile(filepath.Join(ws.Path, ".ralph/config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// Commit the config change
	ws.Git(t, "add", "-A")
	ws.Git(t, "commit", "-q", "-m", "Add bad ATM config")

	// Run ralph run with a plan ID (it will try to call ATM and fail)
	cmd := exec.Command(ralphBinary, "run", "--plan", "1", "--max", "1")
	cmd.Dir = ws.Path
	cmd.Env = append(os.Environ(), "RALPH_TEST=1")

	out, err := cmd.CombinedOutput()
	output := string(out)

	if err == nil {
		t.Error("Expected ralph to fail with invalid ATM bin_path, but it succeeded")
	} else {
		t.Logf("ralph failed as expected: %v", err)
		t.Logf("Output:\n%s", output)
	}
}

// =============================================================================
// TEST: False Completion Circuit Breaker
// =============================================================================

// TestFalseCompletionCircuitBreaker is covered by unit tests in
// internal/runner/loop_test.go (TestIterationLoop_FalseCompletionCircuitBreaker).
// Integration testing this is impractical because it requires the Claude agent
// to repeatedly claim COMPLETE while ATM tasks remain incomplete, which is
// non-deterministic behavior that cannot be reliably triggered.

// =============================================================================
// WORKSPACE HELPERS
// =============================================================================

// Workspace represents an isolated test workspace
type Workspace struct {
	Path         string
	atmStatePath string
	t            *testing.T
	cleanup      bool
	projectID    int // ATM project ID (seeded during setup)
}

// setupWorkspace creates an isolated test workspace with git repo and ralph structure
func setupWorkspace(t *testing.T) *Workspace {
	t.Helper()

	// Create temp directory
	dir, err := os.MkdirTemp("", "ralph-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	ws := &Workspace{
		Path:         dir,
		atmStatePath: filepath.Join(dir, ".ralph", "atm-state.json"),
		t:            t,
		cleanup:      true,
	}

	// Initialize git repo
	ws.Git(t, "init", "-q", "-b", "main")
	ws.Git(t, "config", "user.email", "test@test.com")
	ws.Git(t, "config", "user.name", "Test")

	// Create ralph directory structure
	if err := os.MkdirAll(filepath.Join(dir, ".ralph"), 0755); err != nil {
		t.Fatalf("Failed to create .ralph: %v", err)
	}

	// Create config with ATM settings
	configContent := fmt.Sprintf(`project:
  name: "Test Project"
  description: "Integration test workspace"

git:
  base_branch: "main"

commands:
  test: "echo 'no tests'"
  lint: "echo 'no lint'"

atm:
  project_slug: "test-project"
  bin_path: "%s"
  api_url: "http://localhost:9999"
  api_token: "test-token"
`, fakeATMBinary)

	// Check for global Slack credentials and include them
	slackConfig := getGlobalSlackConfig()
	if slackConfig != "" {
		configContent += slackConfig
		t.Log("Using global Slack credentials for notifications")
	}

	if err := os.WriteFile(filepath.Join(dir, ".ralph/config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Create context files
	if err := os.WriteFile(filepath.Join(dir, ".ralph/principles.md"), []byte("# Principles\n- Keep changes minimal\n"), 0644); err != nil {
		t.Fatalf("Failed to create principles: %v", err)
	}

	// Seed default ATM project in state file
	state := &atmState{
		NextProjectID:   1,
		NextPlanID:      1,
		NextTaskID:      1,
		NextCriterionID: 1,
		NextProgressID:  1,
		NextFeedbackID:  1,
	}
	proj := seedProject(state, "Test Project", "test-project")
	ws.projectID = proj.ID

	if err := saveATMState(ws.atmStatePath, state); err != nil {
		t.Fatalf("Failed to save initial ATM state: %v", err)
	}

	// Initial commit
	ws.Git(t, "add", "-A")
	ws.Git(t, "commit", "-q", "-m", "Initial test workspace")

	t.Logf("Created workspace: %s", dir)
	return ws
}

// SeedATMPlan seeds a plan with tasks and criteria into the fake ATM state.
// Returns the plan ID.
func (ws *Workspace) SeedATMPlan(name, status string, tasks []TaskDef) int {
	ws.t.Helper()

	state, err := loadATMState(ws.atmStatePath)
	if err != nil {
		ws.t.Fatalf("Failed to load ATM state: %v", err)
	}

	// Create plan with feature branch
	branch := "feat/" + name
	plan := seedPlan(state, ws.projectID, name, status, branch)

	// Build a map of task title -> task ID for dependency resolution
	titleToID := make(map[string]int)

	for _, td := range tasks {
		// Resolve dependencies
		var deps []int
		for _, req := range td.Requires {
			depID, ok := titleToID[req]
			if !ok {
				ws.t.Fatalf("SeedATMPlan: task %q requires %q, but that task hasn't been defined yet (must appear earlier in the list)", td.Title, req)
			}
			deps = append(deps, depID)
		}

		task := seedTask(state, plan.ID, td.Title, "", deps)
		titleToID[td.Title] = task.ID

		// Seed criteria for this task
		for _, criterion := range td.Criteria {
			seedCriterion(state, task.ID, criterion)
		}
	}

	if err := saveATMState(ws.atmStatePath, state); err != nil {
		ws.t.Fatalf("Failed to save ATM state: %v", err)
	}

	ws.t.Logf("Seeded ATM plan #%d (%s) with %d tasks", plan.ID, name, len(tasks))
	return plan.ID
}

// ReadATMState loads and returns the current ATM state for assertions.
func (ws *Workspace) ReadATMState(t *testing.T) *atmState {
	t.Helper()

	state, err := loadATMState(ws.atmStatePath)
	if err != nil {
		t.Fatalf("Failed to load ATM state: %v", err)
	}
	return state
}

// Cleanup removes the workspace
func (ws *Workspace) Cleanup() {
	if ws.cleanup {
		os.RemoveAll(ws.Path)
	} else {
		ws.t.Logf("Workspace kept at: %s", ws.Path)
	}
}

// KeepOnFailure prevents cleanup if the test failed
func (ws *Workspace) KeepOnFailure() {
	if ws.t.Failed() {
		ws.cleanup = false
		ws.t.Logf("Keeping workspace for debugging: %s", ws.Path)
	}
}

// RunRalph executes the ralph binary with the given arguments
func (ws *Workspace) RunRalph(t *testing.T, args ...string) string {
	t.Helper()

	cmd := exec.Command(ralphBinary, args...)
	cmd.Dir = ws.Path
	cmd.Env = append(os.Environ(),
		"RALPH_TEST=1",
		"FAKEATM_STATE_PATH="+ws.atmStatePath,
	)

	t.Logf("Running: ralph %s", strings.Join(args, " "))
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Don't fail immediately - some tests expect non-zero exit
		t.Logf("ralph exited with error: %v\nOutput:\n%s", err, out)
	}
	return string(out)
}

// Git runs a git command in the workspace
func (ws *Workspace) Git(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = ws.Path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// GitOutput runs a git command and returns its output
func (ws *Workspace) GitOutput(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = ws.Path
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// =============================================================================
// ASSERTION HELPERS
// =============================================================================

func (ws *Workspace) AssertFileExists(t *testing.T, path, msg string) {
	t.Helper()
	fullPath := filepath.Join(ws.Path, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("%s: file does not exist: %s", msg, path)
	}
}

func (ws *Workspace) AssertFileContains(t *testing.T, path, pattern, msg string) {
	t.Helper()
	fullPath := filepath.Join(ws.Path, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Errorf("%s: cannot read file: %v", msg, err)
		return
	}
	if !strings.Contains(string(content), pattern) {
		t.Errorf("%s: file does not contain '%s'", msg, pattern)
	}
}

func (ws *Workspace) AssertDirExists(t *testing.T, path, msg string) {
	t.Helper()
	fullPath := filepath.Join(ws.Path, path)
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		t.Errorf("%s: directory does not exist: %s", msg, path)
		return
	}
	if !info.IsDir() {
		t.Errorf("%s: path is not a directory: %s", msg, path)
	}
}

func (ws *Workspace) AssertDirNotExists(t *testing.T, path, msg string) {
	t.Helper()
	fullPath := filepath.Join(ws.Path, path)
	if _, err := os.Stat(fullPath); err == nil {
		t.Errorf("%s: directory exists but shouldn't: %s", msg, path)
	}
}

func (ws *Workspace) AssertDirNotEmpty(t *testing.T, path, msg string) {
	t.Helper()
	fullPath := filepath.Join(ws.Path, path)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		t.Errorf("%s: cannot read directory: %v", msg, err)
		return
	}
	if len(entries) == 0 {
		t.Errorf("%s: directory is empty: %s", msg, path)
	}
}

func (ws *Workspace) AssertBranchExists(t *testing.T, branch, msg string) {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = ws.Path
	if err := cmd.Run(); err != nil {
		t.Errorf("%s: branch does not exist: %s", msg, branch)
	}
}

func (ws *Workspace) AssertOnBranch(t *testing.T, expected, msg string) {
	t.Helper()
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = ws.Path
	out, err := cmd.Output()
	if err != nil {
		t.Errorf("%s: cannot get current branch: %v", msg, err)
		return
	}
	current := strings.TrimSpace(string(out))
	if current != expected {
		t.Errorf("%s: expected branch '%s', got '%s'", msg, expected, current)
	}
}

func (ws *Workspace) AssertWorktreeNotExists(t *testing.T, planName, msg string) {
	t.Helper()
	worktreePath := filepath.Join(ws.Path, ".ralph/worktrees", "feat-"+planName)
	if _, err := os.Stat(worktreePath); err == nil {
		t.Errorf("%s: worktree still exists: %s", msg, worktreePath)
	}
}

// =============================================================================
// SLACK HELPERS
// =============================================================================

// getGlobalSlackConfig checks for global Slack credentials and returns
// a YAML config snippet if found.
func getGlobalSlackConfig() string {
	// Check environment variables first
	botToken := os.Getenv("SLACK_BOT_TOKEN")
	channel := os.Getenv("SLACK_CHANNEL")

	// If not in env, try loading from ~/.ralph/slack.env
	if botToken == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			envPath := filepath.Join(homeDir, ".ralph", "slack.env")
			if data, err := os.ReadFile(envPath); err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "SLACK_BOT_TOKEN=") {
						botToken = strings.TrimPrefix(line, "SLACK_BOT_TOKEN=")
						botToken = strings.Trim(botToken, "\"'")
					}
					if strings.HasPrefix(line, "SLACK_CHANNEL=") {
						channel = strings.TrimPrefix(line, "SLACK_CHANNEL=")
						channel = strings.Trim(channel, "\"'")
					}
				}
			}
		}
	}

	// Also check the global config file
	if botToken == "" || channel == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			configPath := filepath.Join(homeDir, ".ralph", "config.yaml")
			if data, err := os.ReadFile(configPath); err == nil {
				lines := strings.Split(string(data), "\n")
				inSlack := false
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if trimmed == "slack:" {
						inSlack = true
						continue
					}
					if inSlack && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
						inSlack = false
					}
					if inSlack {
						if strings.HasPrefix(trimmed, "bot_token:") && botToken == "" {
							botToken = strings.TrimSpace(strings.TrimPrefix(trimmed, "bot_token:"))
							botToken = strings.Trim(botToken, "\"'")
						}
						if strings.HasPrefix(trimmed, "channel:") && channel == "" {
							channel = strings.TrimSpace(strings.TrimPrefix(trimmed, "channel:"))
							channel = strings.Trim(channel, "\"'")
						}
					}
				}
			}
		}
	}

	if botToken == "" || channel == "" {
		return ""
	}

	return fmt.Sprintf(`
slack:
  bot_token: "%s"
  channel: "%s"
  notify_start: true
  notify_complete: true
  notify_iteration: false
  notify_error: true
  notify_blocker: true
`, botToken, channel)
}
