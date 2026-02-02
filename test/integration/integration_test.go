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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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

	// Create minimal config
	configContent := `project:
  name: "Test Project"
  description: "Integration test workspace"

git:
  base_branch: "main"

commands:
  test: "echo 'no tests'"
  lint: "echo 'no lint'"
`
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
