package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResetCmd_HelpOutput(t *testing.T) {
	// Verify the command is properly registered
	cmd := resetCmd

	if cmd.Use != "reset" {
		t.Errorf("expected Use = 'reset', got %q", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("expected Short description to be set")
	}

	if cmd.RunE == nil {
		t.Error("expected RunE to be set")
	}
}

func TestResetCmd_FlagsRegistered(t *testing.T) {
	cmd := resetCmd

	// Check --force flag
	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("expected --force flag to be registered")
	} else {
		if forceFlag.Shorthand != "f" {
			t.Errorf("expected --force shorthand to be 'f', got %q", forceFlag.Shorthand)
		}
		if forceFlag.DefValue != "false" {
			t.Errorf("expected --force default to be false, got %q", forceFlag.DefValue)
		}
	}

	// Check --keep-worktree flag
	keepFlag := cmd.Flags().Lookup("keep-worktree")
	if keepFlag == nil {
		t.Error("expected --keep-worktree flag to be registered")
	} else {
		if keepFlag.DefValue != "false" {
			t.Errorf("expected --keep-worktree default to be false, got %q", keepFlag.DefValue)
		}
	}
}

func TestResetCmd_RequiresGitRepo(t *testing.T) {
	// Create temp directory (not a git repo)
	tmpDir, err := os.MkdirTemp("", "reset-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Run reset
	err = runReset(resetCmd, []string{})

	if err == nil {
		t.Error("expected error when not in git repo")
	}
	if !strings.Contains(err.Error(), "not in a git repository") {
		t.Errorf("expected 'not in a git repository' error, got: %v", err)
	}
}

func TestResetCmd_NoCurrent(t *testing.T) {
	// Create temp directory with git repo
	tmpDir, err := os.MkdirTemp("", "reset-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}

	// Create plans directory structure (empty)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "pending"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "current"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "complete"), 0755)

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Run reset (with --force to skip prompt)
	resetForce = true
	defer func() { resetForce = false }()

	err = runReset(resetCmd, []string{})

	if err == nil {
		t.Error("expected error when no current plan")
	}
	if !strings.Contains(err.Error(), "no current plan") {
		t.Errorf("expected 'no current plan' error, got: %v", err)
	}
}

func TestResetCmd_ResetsPlan(t *testing.T) {
	// Create temp directory with git repo
	tmpDir, err := os.MkdirTemp("", "reset-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}

	// Create plans directory structure
	pendingDir := filepath.Join(tmpDir, "plans", "pending")
	currentDir := filepath.Join(tmpDir, "plans", "current")
	completeDir := filepath.Join(tmpDir, "plans", "complete")
	os.MkdirAll(pendingDir, 0755)
	os.MkdirAll(currentDir, 0755)
	os.MkdirAll(completeDir, 0755)

	// Create a plan in current
	planContent := `# Plan: Test
**Status:** open

## Tasks
- [ ] Task 1
`
	planPath := filepath.Join(currentDir, "test-plan.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Run reset (with --force to skip prompt)
	resetForce = true
	defer func() { resetForce = false }()

	err = runReset(resetCmd, []string{})

	if err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	// Verify plan moved to pending
	if _, err := os.Stat(filepath.Join(pendingDir, "test-plan.md")); os.IsNotExist(err) {
		t.Error("expected plan to be in pending/")
	}

	// Verify plan not in current
	if _, err := os.Stat(filepath.Join(currentDir, "test-plan.md")); !os.IsNotExist(err) {
		t.Error("expected plan to be removed from current/")
	}
}

func TestResetCmd_RemovesWorktree(t *testing.T) {
	// Create temp directory with git repo
	tmpDir, err := os.MkdirTemp("", "reset-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo with initial commit
	gitInit := exec.Command("git", "init", "-b", "main")
	gitInit.Dir = tmpDir
	if err := gitInit.Run(); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}

	// Create initial commit so we can create worktrees
	readmeFile := filepath.Join(tmpDir, "README.md")
	os.WriteFile(readmeFile, []byte("# Test"), 0644)

	gitAdd := exec.Command("git", "add", ".")
	gitAdd.Dir = tmpDir
	gitAdd.Run()

	gitCommit := exec.Command("git", "-c", "user.email=test@test.com", "-c", "user.name=Test", "commit", "-m", "initial")
	gitCommit.Dir = tmpDir
	if err := gitCommit.Run(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create plans directory structure
	pendingDir := filepath.Join(tmpDir, "plans", "pending")
	currentDir := filepath.Join(tmpDir, "plans", "current")
	completeDir := filepath.Join(tmpDir, "plans", "complete")
	worktreesDir := filepath.Join(tmpDir, ".ralph", "worktrees")
	os.MkdirAll(pendingDir, 0755)
	os.MkdirAll(currentDir, 0755)
	os.MkdirAll(completeDir, 0755)
	os.MkdirAll(worktreesDir, 0755)

	// Create a plan in current
	planContent := `# Plan: Test
**Status:** open

## Tasks
- [ ] Task 1
`
	planPath := filepath.Join(currentDir, "test-plan.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}

	// Create a worktree for this plan
	worktreePath := filepath.Join(worktreesDir, "test-plan")
	gitWorktree := exec.Command("git", "worktree", "add", "-b", "feat/test-plan", worktreePath)
	gitWorktree.Dir = tmpDir
	if err := gitWorktree.Run(); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Verify worktree exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatal("worktree should exist before reset")
	}

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Run reset (with --force to skip prompt)
	resetForce = true
	resetKeepWorktree = false
	defer func() { resetForce = false; resetKeepWorktree = false }()

	err = runReset(resetCmd, []string{})

	if err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	// Verify worktree removed
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("expected worktree to be removed")
	}

	// Verify plan moved to pending
	if _, err := os.Stat(filepath.Join(pendingDir, "test-plan.md")); os.IsNotExist(err) {
		t.Error("expected plan to be in pending/")
	}
}

func TestResetCmd_KeepWorktree(t *testing.T) {
	// Create temp directory with git repo
	tmpDir, err := os.MkdirTemp("", "reset-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo with initial commit
	gitInit := exec.Command("git", "init", "-b", "main")
	gitInit.Dir = tmpDir
	if err := gitInit.Run(); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}

	// Create initial commit
	readmeFile := filepath.Join(tmpDir, "README.md")
	os.WriteFile(readmeFile, []byte("# Test"), 0644)

	gitAdd := exec.Command("git", "add", ".")
	gitAdd.Dir = tmpDir
	gitAdd.Run()

	gitCommit := exec.Command("git", "-c", "user.email=test@test.com", "-c", "user.name=Test", "commit", "-m", "initial")
	gitCommit.Dir = tmpDir
	if err := gitCommit.Run(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create plans directory structure
	pendingDir := filepath.Join(tmpDir, "plans", "pending")
	currentDir := filepath.Join(tmpDir, "plans", "current")
	completeDir := filepath.Join(tmpDir, "plans", "complete")
	worktreesDir := filepath.Join(tmpDir, ".ralph", "worktrees")
	os.MkdirAll(pendingDir, 0755)
	os.MkdirAll(currentDir, 0755)
	os.MkdirAll(completeDir, 0755)
	os.MkdirAll(worktreesDir, 0755)

	// Create a plan in current
	planContent := `# Plan: Test
**Status:** open

## Tasks
- [ ] Task 1
`
	planPath := filepath.Join(currentDir, "test-plan.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}

	// Create a worktree for this plan
	worktreePath := filepath.Join(worktreesDir, "test-plan")
	gitWorktree := exec.Command("git", "worktree", "add", "-b", "feat/test-plan", worktreePath)
	gitWorktree.Dir = tmpDir
	if err := gitWorktree.Run(); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Run reset (with --force and --keep-worktree)
	resetForce = true
	resetKeepWorktree = true
	defer func() { resetForce = false; resetKeepWorktree = false }()

	err = runReset(resetCmd, []string{})

	if err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	// Verify worktree still exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Error("expected worktree to be kept")
	}

	// Verify plan moved to pending
	if _, err := os.Stat(filepath.Join(pendingDir, "test-plan.md")); os.IsNotExist(err) {
		t.Error("expected plan to be in pending/")
	}
}
