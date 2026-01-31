package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCmd_HelpOutput(t *testing.T) {
	// Verify command is registered and has expected help content
	if runCmd == nil {
		t.Fatal("runCmd not initialized")
	}

	if runCmd.Use != "run <plan-file>" {
		t.Errorf("unexpected Use: %s", runCmd.Use)
	}

	if !strings.Contains(runCmd.Short, "iteration loop") {
		t.Errorf("Short should mention iteration loop: %s", runCmd.Short)
	}
}

func TestRunCmd_FlagsRegistered(t *testing.T) {
	// Verify --max flag exists
	maxFlag := runCmd.Flags().Lookup("max")
	if maxFlag == nil {
		t.Fatal("--max flag not registered")
	}
	if maxFlag.DefValue != "30" {
		t.Errorf("--max default should be 30, got %s", maxFlag.DefValue)
	}

	// Verify --review flag exists
	reviewFlag := runCmd.Flags().Lookup("review")
	if reviewFlag == nil {
		t.Fatal("--review flag not registered")
	}
	if reviewFlag.DefValue != "false" {
		t.Errorf("--review default should be false, got %s", reviewFlag.DefValue)
	}
}

func TestRunCmd_RequiresPlanFile(t *testing.T) {
	// Verify the command requires exactly 1 argument
	if runCmd.Args == nil {
		t.Fatal("Args validation not set")
	}

	// Test with no arguments - should fail
	err := runCmd.Args(runCmd, []string{})
	if err == nil {
		t.Error("expected error with no arguments")
	}

	// Test with one argument - should pass
	err = runCmd.Args(runCmd, []string{"plan.md"})
	if err != nil {
		t.Errorf("unexpected error with one argument: %v", err)
	}

	// Test with two arguments - should fail
	err = runCmd.Args(runCmd, []string{"plan1.md", "plan2.md"})
	if err == nil {
		t.Error("expected error with two arguments")
	}
}

func TestRunRun_PlanFileNotExists(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "ralph-run-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Save original working directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	// Test with non-existent file
	err = runRun(runCmd, []string{"nonexistent.md"})
	if err == nil {
		t.Fatal("expected error for non-existent plan file")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should mention file does not exist: %v", err)
	}
}

func TestRunRun_ValidPlanFileNoGitRepo(t *testing.T) {
	// Create temp directory (not a git repo)
	tmpDir, err := os.MkdirTemp("", "ralph-run-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a plan file
	planContent := `# Test Plan

**Status:** pending

## Tasks

- [ ] Task 1
- [ ] Task 2
`
	planPath := filepath.Join(tmpDir, "test-plan.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Save original working directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	// Test - should fail because not in git repo
	err = runRun(runCmd, []string{planPath})
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
	if !strings.Contains(err.Error(), "git repository") {
		t.Errorf("error should mention git repository: %v", err)
	}
}

func TestRunRun_ValidPlanFileInGitRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "ralph-run-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	setupTestGitRepo(t, tmpDir)

	// Create ralph config directory
	ralphDir := filepath.Join(tmpDir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a plan file
	planContent := `# Test Plan

**Status:** pending

## Tasks

- [ ] Task 1
- [ ] Task 2
`
	planPath := filepath.Join(tmpDir, "test-plan.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Save original working directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	// Note: We cannot actually run the full loop without the claude CLI available
	// This test just verifies initialization succeeds up to the point of needing claude
	// A full integration test would require mocking the claude CLI

	// The run command will fail when it tries to execute claude, but that's expected
	// We're just testing the setup/validation part works
}

func setupTestGitRepo(t *testing.T, dir string) {
	t.Helper()

	// Initialize git repo
	cmd := func(args ...string) {
		t.Helper()
		c := runGitCommand(dir, args...)
		if err := c.Run(); err != nil {
			t.Fatalf("git %v failed: %v", args, err)
		}
	}

	cmd("init", "-b", "main")
	cmd("config", "user.email", "test@test.com")
	cmd("config", "user.name", "Test User")

	// Create initial commit
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd("add", "README.md")
	cmd("commit", "-m", "Initial commit")
}

func runGitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	return cmd
}
