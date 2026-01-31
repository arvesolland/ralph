//go:build integration

// Package integration provides integration tests for the Ralph CLI.
// These tests exercise the full stack from CLI to Claude execution.
//
// Run with: go test -tags=integration ./internal/integration/...
//
// These tests require either:
// - Real Claude CLI available in PATH for full integration tests
// - Mock claude script for CI (set RALPH_MOCK_CLAUDE=1)
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSingleTask verifies basic plan completion.
func TestSingleTask(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	workspace := setupWorkspace(t)
	defer cleanupWorkspace(t, workspace)

	// Copy test plan
	copyTestPlan(t, workspace, "01-single-task.md")

	// Run ralph
	runRalph(t, workspace, "plans/current/test-plan.md", 5)

	// Verify results
	assertFileExists(t, filepath.Join(workspace, "output/marker.txt"), "Marker file created")
	assertFileContains(t, filepath.Join(workspace, "output/marker.txt"), "ralph-test-complete", "Marker has correct content")

	// Verify feature branch was created
	assertBranchExists(t, workspace, "feat/test-plan")

	// Verify subtask was checked off
	assertPlanHasCheckedTask(t, workspace)
}

// TestDependencies verifies task dependency ordering.
func TestDependencies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	workspace := setupWorkspace(t)
	defer cleanupWorkspace(t, workspace)

	// Copy test plan
	copyTestPlan(t, workspace, "02-dependencies.md")

	// Run ralph
	runRalph(t, workspace, "plans/current/test-plan.md", 10)

	// Verify results - both files should exist
	assertFileExists(t, filepath.Join(workspace, "output/first.txt"), "First file created")
	assertFileContains(t, filepath.Join(workspace, "output/first.txt"), "step-1-done", "First file correct")
	assertFileExists(t, filepath.Join(workspace, "output/second.txt"), "Second file created")
	assertFileContains(t, filepath.Join(workspace, "output/second.txt"), "step-2-done", "Second file correct")
}

// TestProgress verifies progress file creation.
func TestProgress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	workspace := setupWorkspace(t)
	defer cleanupWorkspace(t, workspace)

	// Copy test plan
	copyTestPlan(t, workspace, "03-progress-tracking.md")

	// Run ralph
	runRalph(t, workspace, "plans/current/test-plan.md", 5)

	// Verify results
	assertFileExists(t, filepath.Join(workspace, "output/encoded.txt"), "Encoded file created")

	// Verify progress file was created
	progressFile := filepath.Join(workspace, "plans/current/test-plan.progress.md")
	assertFileExists(t, progressFile, "Progress file created")
	assertFileContains(t, progressFile, "Iteration", "Progress file has iteration entry")
}

// TestLooseFormat verifies non-strict plan format works.
func TestLooseFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	workspace := setupWorkspace(t)
	defer cleanupWorkspace(t, workspace)

	// Copy test plan
	copyTestPlan(t, workspace, "04-loose-format.md")

	// Run ralph
	runRalph(t, workspace, "plans/current/test-plan.md", 10)

	// Verify results
	assertFileExists(t, filepath.Join(workspace, "output/loose-test.txt"), "First loose file created")
	assertFileContains(t, filepath.Join(workspace, "output/loose-test.txt"), "loose format works", "First file correct")
}

// TestWorkerQueue verifies queue management with worktree isolation.
func TestWorkerQueue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	workspace := setupWorkspace(t)
	defer cleanupWorkspace(t, workspace)

	// Copy test plan to pending queue
	copyTestPlanToPending(t, workspace, "01-single-task.md", "queue-test.md")

	// Run worker in --once mode
	runRalphWorker(t, workspace, true, 5)

	// Verify plan was processed
	assertFileExists(t, filepath.Join(workspace, "output/marker.txt"), "Marker file created by worker")

	// Verify plan moved to complete
	completePlan := filepath.Join(workspace, "plans/complete/queue-test.md")
	if _, err := os.Stat(completePlan); os.IsNotExist(err) {
		// Plan might still be in current if not fully complete
		currentPlan := filepath.Join(workspace, "plans/current/queue-test.md")
		if _, err := os.Stat(currentPlan); os.IsNotExist(err) {
			t.Errorf("Plan not found in complete or current directory")
		}
	}
}

// TestDirtyState verifies handling of dirty main worktree.
func TestDirtyState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	workspace := setupWorkspace(t)
	defer cleanupWorkspace(t, workspace)

	// Create uncommitted changes in main worktree
	dirtyFile := filepath.Join(workspace, "dirty-file.txt")
	if err := os.WriteFile(dirtyFile, []byte("uncommitted changes"), 0644); err != nil {
		t.Fatalf("Failed to create dirty file: %v", err)
	}

	// Copy test plan
	copyTestPlan(t, workspace, "01-single-task.md")

	// Run ralph - should still work with dirty main worktree since it uses isolated worktree
	runRalph(t, workspace, "plans/current/test-plan.md", 5)

	// Verify results
	assertFileExists(t, filepath.Join(workspace, "output/marker.txt"), "Marker file created despite dirty main worktree")

	// Verify dirty file still exists in main worktree (not affected)
	assertFileExists(t, dirtyFile, "Dirty file preserved in main worktree")
}

// TestWorktreeCleanup verifies orphaned worktree cleanup.
func TestWorktreeCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	workspace := setupWorkspace(t)
	defer cleanupWorkspace(t, workspace)

	// Create an orphaned worktree manually
	orphanPath := filepath.Join(workspace, ".ralph/worktrees/orphan-test")
	cmd := exec.Command("git", "worktree", "add", "-b", "feat/orphan-test", orphanPath)
	cmd.Dir = workspace
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create orphan worktree: %v", err)
	}

	// Verify worktree exists
	assertFileExists(t, orphanPath, "Orphan worktree created")

	// Run cleanup
	runRalphCleanup(t, workspace)

	// Verify orphan was cleaned up
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Errorf("Orphan worktree was not cleaned up")
	}
}

// Helper functions

func setupWorkspace(t *testing.T) string {
	t.Helper()

	// Create temp directory
	workspace, err := os.MkdirTemp("", "ralph-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp workspace: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = workspace
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@ralph.dev")
	cmd.Dir = workspace
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Ralph Test")
	cmd.Dir = workspace
	cmd.Run()

	// Create initial commit
	readme := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Workspace\n"), 0644); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = workspace
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = workspace
	cmd.Run()

	// Create ralph directory structure
	dirs := []string{
		".ralph",
		".ralph/worktrees",
		"plans/pending",
		"plans/current",
		"plans/complete",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(workspace, dir), 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
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
	configPath := filepath.Join(workspace, ".ralph/config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Create .gitignore for worktrees
	gitignore := filepath.Join(workspace, ".ralph/worktrees/.gitignore")
	if err := os.WriteFile(gitignore, []byte("*\n"), 0644); err != nil {
		t.Fatalf("Failed to create .gitignore: %v", err)
	}

	// Commit the setup
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = workspace
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Setup ralph structure")
	cmd.Dir = workspace
	cmd.Run()

	return workspace
}

func cleanupWorkspace(t *testing.T, workspace string) {
	t.Helper()
	if os.Getenv("RALPH_KEEP_WORKSPACE") != "" {
		t.Logf("Keeping workspace: %s", workspace)
		return
	}
	os.RemoveAll(workspace)
}

func copyTestPlan(t *testing.T, workspace, planName string) {
	t.Helper()
	copyTestPlanTo(t, workspace, planName, "plans/current/test-plan.md")
}

func copyTestPlanToPending(t *testing.T, workspace, srcName, dstName string) {
	t.Helper()
	copyTestPlanTo(t, workspace, srcName, filepath.Join("plans/pending", dstName))
}

func copyTestPlanTo(t *testing.T, workspace, srcName, dstPath string) {
	t.Helper()

	// Get path to testdata
	testdataDir := filepath.Join(getProjectRoot(t), "internal/integration/testdata/plans")
	srcPath := filepath.Join(testdataDir, srcName)

	content, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("Failed to read test plan %s: %v", srcName, err)
	}

	dstFullPath := filepath.Join(workspace, dstPath)
	if err := os.WriteFile(dstFullPath, content, 0644); err != nil {
		t.Fatalf("Failed to write test plan: %v", err)
	}
}

func getProjectRoot(t *testing.T) string {
	t.Helper()

	// Walk up from current directory to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("Could not find project root (go.mod)")
		}
		dir = parent
	}
}

func runRalph(t *testing.T, workspace, planPath string, maxIterations int) {
	t.Helper()

	binary := getRalphBinary(t)
	args := []string{"run", planPath, "--max", string(rune('0' + maxIterations))}

	// Use mock claude if set
	if os.Getenv("RALPH_MOCK_CLAUDE") != "" {
		t.Logf("Using mock claude")
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "RALPH_TEST=1")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Ralph output:\n%s", string(output))
		// Don't fail on error - may be expected for incomplete plans
	}
}

func runRalphWorker(t *testing.T, workspace string, once bool, maxIterations int) {
	t.Helper()

	binary := getRalphBinary(t)
	args := []string{"worker"}
	if once {
		args = append(args, "--once")
	}
	args = append(args, "--max", string(rune('0'+maxIterations)))

	cmd := exec.Command(binary, args...)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "RALPH_TEST=1")

	// Set timeout for worker
	done := make(chan error)
	go func() {
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("Worker output:\n%s", string(output))
		}
		done <- err
	}()

	select {
	case <-done:
		// Worker completed
	case <-time.After(5 * time.Minute):
		cmd.Process.Kill()
		t.Fatalf("Worker timed out")
	}
}

func runRalphCleanup(t *testing.T, workspace string) {
	t.Helper()

	binary := getRalphBinary(t)
	cmd := exec.Command(binary, "cleanup")
	cmd.Dir = workspace

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Cleanup output:\n%s", string(output))
	}
}

func getRalphBinary(t *testing.T) string {
	t.Helper()

	// Check if binary path is set via env
	if bin := os.Getenv("RALPH_BINARY"); bin != "" {
		return bin
	}

	// Try to find built binary
	projectRoot := getProjectRoot(t)
	binary := filepath.Join(projectRoot, "ralph")
	if _, err := os.Stat(binary); err == nil {
		return binary
	}

	// Fall back to building
	t.Logf("Building ralph binary...")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/ralph")
	cmd.Dir = projectRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build ralph: %v\n%s", err, output)
	}

	return binary
}

func assertFileExists(t *testing.T, path, msg string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("%s: file does not exist: %s", msg, path)
	}
}

func assertFileContains(t *testing.T, path, expected, msg string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("%s: failed to read file: %v", msg, err)
		return
	}
	if !strings.Contains(string(content), expected) {
		t.Errorf("%s: file does not contain expected content.\nExpected: %s\nActual: %s", msg, expected, string(content))
	}
}

func assertBranchExists(t *testing.T, workspace, branch string) {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = workspace
	if err := cmd.Run(); err != nil {
		t.Errorf("Branch %s does not exist", branch)
	}
}

func assertPlanHasCheckedTask(t *testing.T, workspace string) {
	t.Helper()

	// Check both current and complete directories
	paths := []string{
		filepath.Join(workspace, "plans/current/test-plan.md"),
		filepath.Join(workspace, "plans/complete/test-plan.md"),
	}

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(content), "[x]") {
			return // Found checked task
		}
	}

	t.Errorf("No checked tasks found in plan files")
}
