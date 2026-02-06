//go:build integration

// Package integration provides end-to-end tests for the Ralph CLI.
// These tests run the actual ralph binary against test plans using real Claude CLI.
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
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// testTimeout is the maximum time for a single test
const testTimeout = 10 * time.Minute

// maxIterations limits iterations during tests
const maxIterations = 5

// ralphBinary is the path to the ralph binary (set in TestMain)
var ralphBinary string

func TestMain(m *testing.M) {
	// Find ralph binary (relative to test directory)
	// Assumes tests run from repo root or test/integration/
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

	os.Exit(m.Run())
}

// =============================================================================
// TEST: Single Task (ralph run)
// =============================================================================

func TestSingleTask(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	// Create plan bundle
	ws.CreatePlanBundle("test-plan", `# Plan: Single Task Test

## Context
Integration test: verify Ralph can complete a single-task plan.

## Tasks

### T1: Create marker file
> Proves the loop executed successfully

**Requires:** —
**Status:** open

**Done when:**
- [ ] File `+"`output/marker.txt`"+` exists with content "ralph-test-complete"

**Subtasks:**
1. [ ] Create directory `+"`output/`"+` if needed
2. [ ] Create file `+"`output/marker.txt`"+` containing exactly "ralph-test-complete"
`)

	// Run ralph with verbose mode for debugging
	// Note: `ralph run` runs directly on the current branch without creating feature branches
	// Use `ralph worker` to test full queue workflow with branch creation
	ws.RunRalph(t, "run", "plans/pending/test-plan", "--max", fmt.Sprintf("%d", maxIterations), "-v")

	// Verify results
	ws.AssertFileExists(t, "output/marker.txt", "Marker file should be created")
	ws.AssertFileContains(t, "output/marker.txt", "ralph-test-complete", "Marker should have correct content")
	// Note: `ralph run` doesn't create feature branches - that's the worker's job
}

// =============================================================================
// TEST: Task Dependencies (ralph run)
// =============================================================================

func TestDependencies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	// Create plan with dependencies
	ws.CreatePlanBundle("test-plan", `# Plan: Dependencies Test

## Context
Test that Ralph respects task dependencies (T2 requires T1).

## Tasks

### T1: Create first file
**Requires:** —
**Status:** open

**Done when:**
- [ ] File `+"`output/first.txt`"+` exists with content "step-1-done"

**Subtasks:**
1. [ ] Create `+"`output/first.txt`"+` with content "step-1-done"

---

### T2: Create second file
**Requires:** T1
**Status:** open

**Done when:**
- [ ] File `+"`output/second.txt`"+` exists with content "step-2-done"
- [ ] File `+"`output/first.txt`"+` still exists

**Subtasks:**
1. [ ] Verify `+"`output/first.txt`"+` exists
2. [ ] Create `+"`output/second.txt`"+` with content "step-2-done"
`)

	ws.RunRalph(t, "run", "plans/pending/test-plan", "--max", fmt.Sprintf("%d", maxIterations), "-v")

	// Both files should exist
	ws.AssertFileExists(t, "output/first.txt", "First file should exist")
	ws.AssertFileContains(t, "output/first.txt", "step-1-done", "First file correct content")
	ws.AssertFileExists(t, "output/second.txt", "Second file should exist")
	ws.AssertFileContains(t, "output/second.txt", "step-2-done", "Second file correct content")
}

// =============================================================================
// TEST: Progress Tracking (ralph run)
// =============================================================================

func TestProgressTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	ws.CreatePlanBundle("test-plan", `# Plan: Progress Test

## Context
Test that progress.md is updated during execution.

## Tasks

### T1: Create encoded file
**Requires:** —
**Status:** open

**Done when:**
- [ ] File `+"`output/encoded.txt`"+` exists with base64-encoded content

**Subtasks:**
1. [ ] Create `+"`output/`"+` directory
2. [ ] Create `+"`output/encoded.txt`"+` with any base64-encoded string
`)

	ws.RunRalph(t, "run", "plans/pending/test-plan", "--max", fmt.Sprintf("%d", maxIterations), "-v")

	ws.AssertFileExists(t, "output/encoded.txt", "Encoded file should exist")

	// Progress file should be updated (in bundle directory)
	progressPath := filepath.Join(ws.Path, "plans/pending/test-plan/progress.md")
	if _, err := os.Stat(progressPath); err == nil {
		content, _ := os.ReadFile(progressPath)
		if strings.Contains(string(content), "Iteration") {
			t.Log("Progress file updated with iteration entries")
		}
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

	// Create plan in pending (worker will activate it)
	ws.CreatePlanBundle("test-plan", `# Plan: Worker Queue Test

## Context
Test worker picks up plan from pending, processes in worktree, and completes.

## Tasks

### T1: Create marker
**Requires:** —
**Status:** open

**Done when:**
- [ ] File `+"`output/worker-marker.txt`"+` exists with content "worker-test-complete"

**Subtasks:**
1. [ ] Create `+"`output/worker-marker.txt`"+` with content "worker-test-complete"
`)

	// Run worker with --once --merge
	ws.RunRalph(t, "worker", "--once", "--merge", "--max", fmt.Sprintf("%d", maxIterations), "-v")

	// Main worktree should stay on main
	ws.AssertOnBranch(t, "main", "Main worktree should stay on main")

	// Work should be merged to main
	ws.AssertFileExists(t, "output/worker-marker.txt", "Marker file should exist after merge")
	ws.AssertFileContains(t, "output/worker-marker.txt", "worker-test-complete", "Marker should have correct content")

	// Plan should be in complete/ (with date suffix)
	ws.AssertDirNotEmpty(t, "plans/complete", "Plan should be archived to complete/")

	// Worktree should be cleaned up
	ws.AssertWorktreeNotExists(t, "test-plan", "Worktree should be cleaned up after completion")
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

	ws.CreatePlanBundle("test-plan", `# Plan: Dirty State Test

## Tasks

### T1: Create marker
**Requires:** —
**Status:** open

**Done when:**
- [ ] File `+"`output/dirty-test.txt`"+` exists

**Subtasks:**
1. [ ] Create `+"`output/dirty-test.txt`"+` with any content
`)

	// Worker should succeed despite dirty main worktree
	ws.RunRalph(t, "worker", "--once", "--merge", "--max", fmt.Sprintf("%d", maxIterations), "-v")

	// Dirty file should still exist (not lost)
	ws.AssertFileExists(t, "dirty-file.txt", "Dirty file should be preserved")

	// Work should complete
	ws.AssertFileExists(t, "output/dirty-test.txt", "Work should complete despite dirty state")
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

	// Create multi-task plan to verify all principles
	ws.CreatePlanBundle("test-plan", `# Plan: Core Principles Test

## Context
Verifies all Ralph core principles:
1. One task at a time
2. Respects dependencies
3. Updates plan checkboxes
4. Updates progress log
5. Commits changes

## Tasks

### T1: Create first marker
**Requires:** —
**Status:** open

**Done when:**
- [ ] File `+"`output/step1.txt`"+` exists with content "step1-complete"

**Subtasks:**
- [ ] Create `+"`output/`"+` directory
- [ ] Create `+"`output/step1.txt`"+` with content "step1-complete"

---

### T2: Create second marker (depends on T1)
**Requires:** T1
**Status:** open

**Done when:**
- [ ] File `+"`output/step2.txt`"+` exists with content "step2-complete"
- [ ] File `+"`output/step1.txt`"+` still exists

**Subtasks:**
- [ ] Verify `+"`output/step1.txt`"+` exists
- [ ] Create `+"`output/step2.txt`"+` with content "step2-complete"

---

### T3: Create final marker (depends on T2)
**Requires:** T2
**Status:** open

**Done when:**
- [ ] File `+"`output/final.txt`"+` exists with content "all-done"
- [ ] Both step1.txt and step2.txt still exist

**Subtasks:**
- [ ] Verify both previous files exist
- [ ] Create `+"`output/final.txt`"+` with content "all-done"
`)

	// Run worker
	ws.RunRalph(t, "worker", "--once", "--merge", "--max", "10", "-v")

	// PRINCIPLE 1 & 2: Tasks completed in dependency order
	ws.AssertFileExists(t, "output/step1.txt", "P1: T1 completed")
	ws.AssertFileContains(t, "output/step1.txt", "step1-complete", "P1: T1 correct content")
	ws.AssertFileExists(t, "output/step2.txt", "P1: T2 completed")
	ws.AssertFileContains(t, "output/step2.txt", "step2-complete", "P1: T2 correct content")
	ws.AssertFileExists(t, "output/final.txt", "P1: T3 completed")
	ws.AssertFileContains(t, "output/final.txt", "all-done", "P1: T3 correct content")

	// PRINCIPLE 3 & 4: Plan updated with checkboxes
	// Find completed plan
	completeDirs, _ := filepath.Glob(filepath.Join(ws.Path, "plans/complete/*/plan.md"))
	if len(completeDirs) == 0 {
		t.Error("P3: Plan not archived to complete/")
	} else {
		planContent, _ := os.ReadFile(completeDirs[0])
		checkedCount := strings.Count(string(planContent), "[x]")
		if checkedCount < 3 {
			t.Errorf("P3: Expected at least 3 checked boxes, got %d", checkedCount)
		} else {
			t.Logf("P3: Plan updated with %d checked boxes", checkedCount)
		}
	}

	// PRINCIPLE 5: Commits made
	out, _ := ws.GitOutput(t, "log", "--oneline", "main")
	commitCount := len(strings.Split(strings.TrimSpace(out), "\n"))
	if commitCount < 3 {
		t.Errorf("P5: Expected at least 3 commits, got %d", commitCount)
	} else {
		t.Logf("P5: %d commits on main", commitCount)
	}

	// Worktree should be cleaned up
	ws.AssertWorktreeNotExists(t, "test-plan", "Worktree cleaned up after completion")
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

	// 3 independent tasks (no dependencies) — if the agent batches them,
	// they'd all land in one commit. If it obeys "one task then stop",
	// each task gets its own iteration and its own commit.
	ws.CreatePlanBundle("test-plan", `# Plan: One Task Per Iteration Test

## Context
Tests that the agent completes exactly ONE task per iteration, then stops.
Each task creates a distinct marker file. They are independent (no dependencies)
so the agent has no excuse to batch them.

## Tasks

### T1: Create marker A
**Requires:** —
**Status:** open

**Done when:**
- [ ] File `+"`output/a.txt`"+` exists with content "task-a-done"

**Subtasks:**
1. [ ] Create `+"`output/a.txt`"+` with content "task-a-done"

---

### T2: Create marker B
**Requires:** —
**Status:** open

**Done when:**
- [ ] File `+"`output/b.txt`"+` exists with content "task-b-done"

**Subtasks:**
1. [ ] Create `+"`output/b.txt`"+` with content "task-b-done"

---

### T3: Create marker C
**Requires:** —
**Status:** open

**Done when:**
- [ ] File `+"`output/c.txt`"+` exists with content "task-c-done"

**Subtasks:**
1. [ ] Create `+"`output/c.txt`"+` with content "task-c-done"
`)

	// Run with ralph run (not worker, to keep commits on same branch for easy inspection)
	ws.RunRalph(t, "run", "plans/pending/test-plan", "--max", "10", "-v")

	// All 3 files should exist
	ws.AssertFileExists(t, "output/a.txt", "T1 should be completed")
	ws.AssertFileContains(t, "output/a.txt", "task-a-done", "T1 correct content")
	ws.AssertFileExists(t, "output/b.txt", "T2 should be completed")
	ws.AssertFileContains(t, "output/b.txt", "task-b-done", "T2 correct content")
	ws.AssertFileExists(t, "output/c.txt", "T3 should be completed")
	ws.AssertFileContains(t, "output/c.txt", "task-c-done", "T3 correct content")

	// KEY ASSERTION: There should be at least 3 task commits (one per task).
	// If the agent batched all 3 into one iteration, there'd be only 1-2 commits.
	// We filter out the initial workspace commit to count only agent commits.
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
	// The initial commit message is "Initial test workspace".
	agentCommits := 0
	for _, line := range lines {
		if !strings.Contains(line, "Initial test workspace") {
			agentCommits++
		}
	}

	// We expect at least 3 agent commits (one per task).
	// The agent may also make additional commits (progress file, state, etc.)
	// but the minimum is 3 if it's doing one task per iteration.
	if agentCommits < 3 {
		t.Errorf("ONE-TASK-PER-ITERATION VIOLATED: expected at least 3 agent commits (one per task), got %d. Agent likely batched multiple tasks into a single iteration.", agentCommits)
	} else {
		t.Logf("SUCCESS: %d agent commits for 3 tasks — agent respected one-task-per-iteration", agentCommits)
	}

	// Additional check: verify each marker file was introduced in a SEPARATE commit.
	// Use git log to find which commit introduced each file.
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

	// If all 3 files were introduced in different commits, the agent obeyed the rule.
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
// TEST: Plan Bundles (ralph plan create)
// =============================================================================

func TestPlanBundleCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	// Create plan bundle via CLI
	ws.RunRalph(t, "plan", "create", "my-feature")

	// Verify bundle structure
	bundlePath := filepath.Join(ws.Path, "plans/pending/my-feature")
	ws.AssertDirExists(t, "plans/pending/my-feature", "Bundle directory should exist")
	ws.AssertFileExists(t, "plans/pending/my-feature/plan.md", "plan.md should exist")
	ws.AssertFileExists(t, "plans/pending/my-feature/progress.md", "progress.md should exist")
	ws.AssertFileExists(t, "plans/pending/my-feature/feedback.md", "feedback.md should exist")

	// Verify plan.md has template content
	planContent, err := os.ReadFile(filepath.Join(bundlePath, "plan.md"))
	if err != nil {
		t.Fatalf("Failed to read plan.md: %v", err)
	}
	if !strings.Contains(string(planContent), "# Plan:") {
		t.Error("plan.md should contain template header")
	}
	if !strings.Contains(string(planContent), "## Tasks") {
		t.Error("plan.md should contain Tasks section")
	}
}

// =============================================================================
// TEST: Reset Command (ralph reset)
// =============================================================================

func TestReset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.Cleanup()

	// Create plan directly in current/
	currentPlan := filepath.Join(ws.Path, "plans/current/test-plan")
	if err := os.MkdirAll(currentPlan, 0755); err != nil {
		t.Fatalf("Failed to create plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentPlan, "plan.md"), []byte("# Test Plan\n"), 0644); err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Run reset (--force to skip confirmation prompt)
	ws.RunRalph(t, "reset", "--force")

	// Plan should be back in pending/
	ws.AssertDirNotExists(t, "plans/current/test-plan", "Plan should be removed from current/")
	ws.AssertDirExists(t, "plans/pending/test-plan", "Plan should be moved to pending/")
}

// =============================================================================
// WORKSPACE HELPERS
// =============================================================================

// Workspace represents an isolated test workspace
type Workspace struct {
	Path    string
	t       *testing.T
	cleanup bool
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
		Path:    dir,
		t:       t,
		cleanup: true,
	}

	// Initialize git repo
	ws.Git(t, "init", "-q", "-b", "main")
	ws.Git(t, "config", "user.email", "test@test.com")
	ws.Git(t, "config", "user.name", "Test")

	// Create ralph directory structure
	dirs := []string{
		".ralph",
		"plans/pending",
		"plans/current",
		"plans/complete",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", d, err)
		}
	}

	// Create config - include Slack settings if credentials are available globally
	configContent := `project:
  name: "Test Project"
  description: "Integration test workspace"

git:
  base_branch: "main"

commands:
  test: "echo 'no tests'"
  lint: "echo 'no lint'"
`
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

	// Initial commit
	ws.Git(t, "add", "-A")
	ws.Git(t, "commit", "-q", "-m", "Initial test workspace")

	t.Logf("Created workspace: %s", dir)
	return ws
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

// CreatePlanBundle creates a plan bundle in pending/
func (ws *Workspace) CreatePlanBundle(name, content string) {
	ws.t.Helper()
	bundleDir := filepath.Join(ws.Path, "plans/pending", name)
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		ws.t.Fatalf("Failed to create bundle dir: %v", err)
	}

	// Create plan.md
	if err := os.WriteFile(filepath.Join(bundleDir, "plan.md"), []byte(content), 0644); err != nil {
		ws.t.Fatalf("Failed to create plan.md: %v", err)
	}

	// Create empty progress.md and feedback.md (scaffolded)
	if err := os.WriteFile(filepath.Join(bundleDir, "progress.md"), []byte("# Progress\n"), 0644); err != nil {
		ws.t.Fatalf("Failed to create progress.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "feedback.md"), []byte("# Feedback\n\n## Pending\n\n## Processed\n"), 0644); err != nil {
		ws.t.Fatalf("Failed to create feedback.md: %v", err)
	}
}

// RunRalph executes the ralph binary with the given arguments
func (ws *Workspace) RunRalph(t *testing.T, args ...string) string {
	t.Helper()

	cmd := exec.Command(ralphBinary, args...)
	cmd.Dir = ws.Path
	cmd.Env = append(os.Environ(), "RALPH_TEST=1")

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

func (ws *Workspace) AssertFileMatches(t *testing.T, path, pattern, msg string) {
	t.Helper()
	fullPath := filepath.Join(ws.Path, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Errorf("%s: cannot read file: %v", msg, err)
		return
	}
	matched, _ := regexp.MatchString(pattern, string(content))
	if !matched {
		t.Errorf("%s: file does not match pattern '%s'", msg, pattern)
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
// a YAML config snippet if found. This allows integration tests to send
// real Slack notifications when credentials are available.
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
				// Simple extraction - look for bot_token and channel under slack:
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

	// Return empty if no credentials found
	if botToken == "" || channel == "" {
		return ""
	}

	// Return YAML snippet for Slack config
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

slack:
  bot_token: "xoxb-test-token"
  channel: "C12345TEST"
  notify_start: true
  notify_complete: true
  notify_iteration: false
  notify_error: true
  notify_blocker: true
`)

	if err := os.WriteFile(filepath.Join(ws.Path, ".ralph/config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// Commit the config change
	ws.Git(t, "add", "-A")
	ws.Git(t, "commit", "-q", "-m", "Add Slack config")

	// Create a simple plan
	ws.CreatePlanBundle("slack-test", `# Plan: Slack Notification Test

## Context
Integration test: verify Slack notifications are sent.

## Tasks

### T1: Create marker file
**Requires:** —
**Status:** open

**Done when:**
- [ ] File `+"`output/done.txt`"+` exists with content "slack-test-complete"

**Subtasks:**
1. [ ] Create directory `+"`output/`"+` if needed
2. [ ] Create file `+"`output/done.txt`"+` containing "slack-test-complete"
`)

	// Commit the plan
	ws.Git(t, "add", "-A")
	ws.Git(t, "commit", "-q", "-m", "Add slack test plan")

	// Run ralph worker with mock Slack server URL via environment
	// The slack-go library uses SLACK_API_URL if set
	cmd := exec.Command(ralphBinary, "worker", "--once", "--max", fmt.Sprintf("%d", maxIterations), "-v")
	cmd.Dir = ws.Path
	cmd.Env = append(os.Environ(),
		"RALPH_TEST=1",
		"SLACK_API_URL="+slackServer.URL+"/",
	)

	t.Logf("Running: ralph worker --once (with mock Slack at %s)", slackServer.URL)
	out, err := cmd.CombinedOutput()
	t.Logf("Output:\n%s", out)

	if err != nil {
		// Worker may exit with error (max iterations, no remote for PR, etc.)
		// This is expected - we're testing notifications, not task completion
		t.Logf("Worker exited with error (expected in test): %v", err)
	}

	// Check Slack requests - this is the main thing we're testing
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

	// We should have at least one update for iteration progress
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

	// Log success
	t.Logf("SUCCESS: Slack notifications working - %d posts, %d updates", len(postMessages), len(updates))
}

// =============================================================================
// TEST: State Review — Standard Format (20 tasks, exact verification)
// =============================================================================

func TestStateReview_StandardFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.KeepOnFailure()
	defer ws.Cleanup()

	// Standard plan format with 20 tasks: ### T1: Title, **Requires:**, **Done when:** checkboxes.
	// The regex parser handles this format, and the LLM review should confirm ALIGNED.
	ws.CreatePlanBundle("standard-plan", standardFormatPlan)

	ws.Git(t, "add", "-A")
	ws.Git(t, "commit", "-q", "-m", "Add standard format plan")

	output := ws.RunRalph(t, "plan", "review", "plans/pending/standard-plan", "-v")
	t.Logf("Review output:\n%s", output)

	stateYAML := ws.readStateYAML(t, "plans/pending/standard-plan")

	// ── Exact task count ──
	if len(stateYAML.Tasks) != 20 {
		t.Errorf("Expected exactly 20 tasks, got %d", len(stateYAML.Tasks))
	}

	// ── Build lookup ──
	taskByID := make(map[string]*stateTask)
	for i := range stateYAML.Tasks {
		taskByID[stateYAML.Tasks[i].ID] = &stateYAML.Tasks[i]
	}

	// ── Verify every task ID exists ──
	for i := 1; i <= 20; i++ {
		id := fmt.Sprintf("T%d", i)
		if _, ok := taskByID[id]; !ok {
			t.Errorf("Missing task %s in state.yaml", id)
		}
	}

	// ── Expected titles (exact match) ──
	expectedTitles := map[string]string{
		"T1":  "Add multiselect checkbox to MetadataTypeModal",
		"T2":  "Handle dataType toggle logic",
		"T3":  "Persist multiselect flag to database",
		"T4":  "Add form validation for multiselect configuration",
		"T5":  "Create MultiSelectToggle component",
		"T6":  "Implement multiselect case in ImageMetadataPanel",
		"T7":  "Parse stored JSON array values",
		"T8":  "Handle toggle selection and deselection logic",
		"T9":  "Store multiselect values as JSON strings",
		"T10": "Add multiselect styling and visual states",
		"T11": "Update single image metadata PATCH API",
		"T12": "Update single image metadata GET API",
		"T13": "Add API input validation for multiselect values",
		"T14": "Update BatchOperationsSidebar field rendering",
		"T15": "Implement batch metadata PATCH API for arrays",
		"T16": "Handle batch merge operations",
		"T17": "Add database migration for dataType column",
		"T18": "Write unit tests for MultiSelectToggle",
		"T19": "Write API integration tests for multiselect CRUD",
		"T20": "Write E2E test for multiselect workflow",
	}
	for id, expectedTitle := range expectedTitles {
		if task, ok := taskByID[id]; ok {
			if task.Title != expectedTitle {
				t.Errorf("Task %s: expected title %q, got %q", id, expectedTitle, task.Title)
			}
		}
	}

	// ── Expected dependencies (exact match) ──
	expectedDeps := map[string][]string{
		"T1":  {},
		"T2":  {"T1"},
		"T3":  {"T2"},
		"T4":  {"T2"},
		"T5":  {},
		"T6":  {"T5", "T3"},
		"T7":  {"T6"},
		"T8":  {"T7"},
		"T9":  {"T8"},
		"T10": {"T5"},
		"T11": {"T9"},
		"T12": {"T11"},
		"T13": {"T11"},
		"T14": {"T6"},
		"T15": {"T14", "T11"},
		"T16": {"T15"},
		"T17": {"T3"},
		"T18": {"T5", "T10"},
		"T19": {"T11", "T12", "T13"},
		"T20": {"T18", "T19", "T16"},
	}
	for id, expectedReqs := range expectedDeps {
		task, ok := taskByID[id]
		if !ok {
			continue
		}
		if len(expectedReqs) == 0 {
			if len(task.Requires) != 0 {
				t.Errorf("Task %s: expected no deps, got %v", id, task.Requires)
			}
			continue
		}
		for _, dep := range expectedReqs {
			if !containsString(task.Requires, dep) {
				t.Errorf("Task %s: expected dep %s, got %v", id, dep, task.Requires)
			}
		}
		if len(task.Requires) != len(expectedReqs) {
			t.Errorf("Task %s: expected %d deps, got %d (%v)", id, len(expectedReqs), len(task.Requires), task.Requires)
		}
	}

	// ── Expected minimum criteria counts ──
	expectedMinCriteria := map[string]int{
		"T1": 2, "T2": 2, "T3": 2, "T4": 2, "T5": 3,
		"T6": 2, "T7": 3, "T8": 3, "T9": 2, "T10": 2,
		"T11": 2, "T12": 2, "T13": 2, "T14": 2, "T15": 2,
		"T16": 3, "T17": 2, "T18": 3, "T19": 3, "T20": 3,
	}
	for id, minCount := range expectedMinCriteria {
		task, ok := taskByID[id]
		if !ok {
			continue
		}
		if len(task.Criteria) < minCount {
			t.Errorf("Task %s: expected at least %d criteria, got %d", id, minCount, len(task.Criteria))
		}
	}

	// ── All tasks should be todo ──
	for _, task := range stateYAML.Tasks {
		if task.Status != "todo" {
			t.Errorf("Task %s should be 'todo', got '%s'", task.ID, task.Status)
		}
	}

	t.Logf("SUCCESS: Standard format plan correctly populated state.yaml with %d tasks", len(stateYAML.Tasks))
	for _, task := range stateYAML.Tasks {
		t.Logf("  %s: %s (requires: %v, criteria: %d)",
			task.ID, task.Title, task.Requires, len(task.Criteria))
	}
}

// =============================================================================
// TEST: State Review — Non-Standard Format (20 tasks, YAML frontmatter)
// =============================================================================

func TestStateReview_NonStandardFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ws := setupWorkspace(t)
	defer ws.KeepOnFailure()
	defer ws.Cleanup()

	// Non-standard plan format based on real-world example:
	// - YAML frontmatter with todos list, statuses, dependencies
	// - Mermaid diagram
	// - Numbered implementation steps (not ### T1: headings)
	// - Mixed prose criteria, no **Done when:** checkboxes
	// - File path references, code blocks, data format examples
	// The regex parser cannot parse any of this.
	nonStandardPlan, err := os.ReadFile("testdata/nonstandard_plan.md")
	if err != nil {
		t.Fatalf("Failed to read testdata/nonstandard_plan.md: %v", err)
	}
	ws.CreatePlanBundle("freeform-plan", string(nonStandardPlan))

	ws.Git(t, "add", "-A")
	ws.Git(t, "commit", "-q", "-m", "Add freeform plan")

	output := ws.RunRalph(t, "plan", "review", "plans/pending/freeform-plan", "-v")
	t.Logf("Review output:\n%s", output)

	stateYAML := ws.readStateYAML(t, "plans/pending/freeform-plan")

	// ── The plan has 20 logical tasks. LLM must find them all. ──
	if len(stateYAML.Tasks) < 20 {
		t.Errorf("Expected at least 20 tasks from freeform plan, got %d", len(stateYAML.Tasks))
	}

	// ── Every task must have a non-empty title ──
	for _, task := range stateYAML.Tasks {
		if task.Title == "" {
			t.Errorf("Task %s has empty title", task.ID)
		}
	}

	// ── Most tasks should have criteria (plan specifies them for each) ──
	tasksWithCriteria := 0
	for _, task := range stateYAML.Tasks {
		if len(task.Criteria) > 0 {
			tasksWithCriteria++
		}
	}
	if tasksWithCriteria < 15 {
		t.Errorf("Expected at least 15 tasks with criteria, got %d", tasksWithCriteria)
	}

	// ── Many tasks have explicit dependencies ──
	tasksWithDeps := 0
	for _, task := range stateYAML.Tasks {
		if len(task.Requires) > 0 {
			tasksWithDeps++
		}
	}
	if tasksWithDeps < 14 {
		t.Errorf("Expected at least 14 tasks with dependencies, got %d", tasksWithDeps)
	}

	// ── All tasks should be todo status ──
	for _, task := range stateYAML.Tasks {
		if task.Status != "todo" {
			t.Errorf("Task %s should be 'todo', got '%s'", task.ID, task.Status)
		}
	}

	// ── Verify key themes are covered by checking title keywords ──
	// The plan covers: modal config, component creation, panel integration,
	// JSON parsing, API updates, batch operations, migration, and tests.
	themes := map[string]bool{
		"modal":     false,
		"component": false,
		"panel":     false,
		"api":       false,
		"batch":     false,
		"migrat":    false,
		"test":      false,
		"valid":     false,
	}
	for _, task := range stateYAML.Tasks {
		titleLower := strings.ToLower(task.Title)
		for theme := range themes {
			if strings.Contains(titleLower, theme) {
				themes[theme] = true
			}
		}
	}
	for theme, found := range themes {
		if !found {
			t.Errorf("Expected at least one task with '%s' in title", theme)
		}
	}

	// ── Log everything for debugging ──
	t.Logf("Freeform plan state.yaml (%d tasks):", len(stateYAML.Tasks))
	for _, task := range stateYAML.Tasks {
		t.Logf("  %s: %s (requires: %v, criteria: %d)",
			task.ID, task.Title, task.Requires, len(task.Criteria))
	}

	t.Logf("SUCCESS: Non-standard format plan correctly populated state.yaml with %d tasks", len(stateYAML.Tasks))
}

// =============================================================================
// PLAN FIXTURES (kept as constants to avoid cluttering test functions)
// =============================================================================

// standardFormatPlan is a 20-task plan using the standard ### T1: format.
// Based on the multi-select dropdown feature but expanded to cover all
// implementation steps including components, APIs, batch ops, migration, tests.
const standardFormatPlan = `# Plan: Multi-Select Dropdown Implementation

## Context
Add support for multi-select dropdown fields in the annotation tool.
Introduces a checkbox toggle in the MetadataTypeModal, toggle-button
style multi-selection in the ImageMetadataPanel, array value storage,
batch operations support, database migration, and full test coverage.

## Tasks

### T1: Add multiselect checkbox to MetadataTypeModal
**Requires:** —
**Status:** open

**Done when:**
- [ ] "Allow Multiple Selections" checkbox renders when dataType is 'select'
- [ ] Checkbox is hidden for non-select types (text, number, date)

---

### T2: Handle dataType toggle logic
**Requires:** T1
**Status:** open

**Done when:**
- [ ] Checking the box sets dataType to 'multiselect'
- [ ] Unchecking sets dataType back to 'select'
- [ ] Options list is preserved when toggling

---

### T3: Persist multiselect flag to database
**Requires:** T2
**Status:** open

**Done when:**
- [ ] Creating a metadata type with multiselect saves dataType='multiselect'
- [ ] Editing an existing type toggles dataType correctly

---

### T4: Add form validation for multiselect configuration
**Requires:** T2
**Status:** open

**Done when:**
- [ ] Multiselect types require at least 2 options
- [ ] Error message shown when validation fails

---

### T5: Create MultiSelectToggle component
**Requires:** —
**Status:** open

**Done when:**
- [ ] Component renders list of option buttons
- [ ] Supports controlled selectedValues prop (string array)
- [ ] Fires onChange callback with updated selection array

---

### T6: Implement multiselect case in ImageMetadataPanel
**Requires:** T5, T3
**Status:** open

**Done when:**
- [ ] renderFieldContent switch has 'multiselect' case
- [ ] Case renders MultiSelectToggle with correct props

---

### T7: Parse stored JSON array values
**Requires:** T6
**Status:** open

**Done when:**
- [ ] JSON.parse(value) correctly extracts array from stored string
- [ ] Handles null/undefined/empty string gracefully (returns [])
- [ ] Handles malformed JSON gracefully (returns [])

---

### T8: Handle toggle selection and deselection logic
**Requires:** T7
**Status:** open

**Done when:**
- [ ] Clicking unselected option adds it to the array
- [ ] Clicking selected option removes it from the array
- [ ] Selection order is preserved

---

### T9: Store multiselect values as JSON strings
**Requires:** T8
**Status:** open

**Done when:**
- [ ] Selections are stored as JSON.stringify(selectedValues)
- [ ] Empty selection stores empty array "[]"

---

### T10: Add multiselect styling and visual states
**Requires:** T5
**Status:** open

**Done when:**
- [ ] Selected buttons have distinct visual style (filled/highlighted)
- [ ] Unselected buttons have outline/muted style

---

### T11: Update single image metadata PATCH API
**Requires:** T9
**Status:** open

**Done when:**
- [ ] PATCH /api/images/[imageId] accepts array values for multiselect fields
- [ ] Array values are stored as JSON string in the database

---

### T12: Update single image metadata GET API
**Requires:** T11
**Status:** open

**Done when:**
- [ ] GET response includes multiselect values as stored JSON strings
- [ ] Frontend can parse the returned value correctly

---

### T13: Add API input validation for multiselect values
**Requires:** T11
**Status:** open

**Done when:**
- [ ] API rejects non-array values for multiselect fields
- [ ] API rejects values not in the allowed options list

---

### T14: Update BatchOperationsSidebar field rendering
**Requires:** T6
**Status:** open

**Done when:**
- [ ] Multiselect fields render MultiSelectToggle in batch panel
- [ ] Batch selection is independent per image

---

### T15: Implement batch metadata PATCH API for arrays
**Requires:** T14, T11
**Status:** open

**Done when:**
- [ ] Batch PATCH endpoint accepts array values for multiselect fields
- [ ] All selected images are updated with the array value

---

### T16: Handle batch merge operations
**Requires:** T15
**Status:** open

**Done when:**
- [ ] "Replace" mode overwrites existing multiselect values
- [ ] "Add" mode unions new values with existing values
- [ ] "Remove" mode subtracts values from existing arrays

---

### T17: Add database migration for dataType column
**Requires:** T3
**Status:** open

**Done when:**
- [ ] Migration adds 'multiselect' as valid dataType enum value
- [ ] Rollback migration removes the value cleanly

---

### T18: Write unit tests for MultiSelectToggle
**Requires:** T5, T10
**Status:** open

**Done when:**
- [ ] Test: renders all options as buttons
- [ ] Test: clicking option fires onChange with toggled array
- [ ] Test: selected options have correct visual class

---

### T19: Write API integration tests for multiselect CRUD
**Requires:** T11, T12, T13
**Status:** open

**Done when:**
- [ ] Test: creating multiselect metadata type via API
- [ ] Test: saving and reading multiselect values
- [ ] Test: validation rejects invalid values

---

### T20: Write E2E test for multiselect workflow
**Requires:** T18, T19, T16
**Status:** open

**Done when:**
- [ ] Test: create multiselect field in modal
- [ ] Test: select multiple values in annotation panel
- [ ] Test: batch update multiselect values across images
`

// nonStandardFormatPlan is loaded from testdata/nonstandard_plan.md at test time.
// It uses YAML frontmatter, numbered steps, mermaid diagrams, prose criteria,
// and "Depends on:" phrasing — all of which the regex parser cannot handle.

// =============================================================================
// STATE YAML HELPERS (for review integration tests)
// =============================================================================

// stateYAMLFile represents the parsed state.yaml for assertions.
type stateYAMLFile struct {
	ID     string      `yaml:"id"`
	Title  string      `yaml:"title"`
	Status string      `yaml:"status"`
	Tasks  []stateTask `yaml:"tasks"`
}

type stateTask struct {
	ID       string          `yaml:"id"`
	Title    string          `yaml:"title"`
	Status   string          `yaml:"status"`
	Requires []string        `yaml:"requires"`
	Criteria []stateCriterion `yaml:"criteria"`
}

type stateCriterion struct {
	Text string `yaml:"text"`
	Done bool   `yaml:"done"`
}

// readStateYAML loads and parses state.yaml from a plan bundle path (relative to workspace).
func (ws *Workspace) readStateYAML(t *testing.T, bundlePath string) stateYAMLFile {
	t.Helper()

	statePath := filepath.Join(ws.Path, bundlePath, "state.yaml")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("Failed to read state.yaml at %s: %v", statePath, err)
	}

	t.Logf("state.yaml content:\n%s", string(data))

	var state stateYAMLFile
	if err := yaml.Unmarshal(data, &state); err != nil {
		t.Fatalf("Failed to parse state.yaml: %v", err)
	}

	return state
}

// containsString checks if a slice contains a specific string.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

