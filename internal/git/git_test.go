package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupTestRepo creates a temporary git repository for testing.
// Returns the repo path and a cleanup function.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(dir)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		cleanup()
		t.Fatalf("git init: %v", err)
	}

	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		cleanup()
		t.Fatalf("git config email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		cleanup()
		t.Fatalf("git config name: %v", err)
	}

	return dir, cleanup
}

// createFile creates a file with the given content in the repo.
func createFile(t *testing.T, repoDir, name, content string) {
	t.Helper()
	path := filepath.Join(repoDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating parent dirs: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
}

func TestNewGit(t *testing.T) {
	g := NewGit("/some/path")
	if g.WorkDir() != "/some/path" {
		t.Errorf("WorkDir() = %q, want %q", g.WorkDir(), "/some/path")
	}
}

func TestStatus_CleanRepo(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create initial commit
	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	status, err := g.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}

	if status.Branch != "main" {
		t.Errorf("Branch = %q, want %q", status.Branch, "main")
	}
	if !status.IsClean() {
		t.Error("expected clean status")
	}
	if len(status.Staged) != 0 {
		t.Errorf("Staged = %v, want empty", status.Staged)
	}
	if len(status.Unstaged) != 0 {
		t.Errorf("Unstaged = %v, want empty", status.Unstaged)
	}
	if len(status.Untracked) != 0 {
		t.Errorf("Untracked = %v, want empty", status.Untracked)
	}
}

func TestStatus_WithChanges(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create initial commit
	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Create staged file
	createFile(t, repoDir, "staged.txt", "staged content")
	if err := g.Add("staged.txt"); err != nil {
		t.Fatalf("Add staged.txt: %v", err)
	}

	// Create unstaged modification
	createFile(t, repoDir, "README.md", "# Modified\n")

	// Create untracked file
	createFile(t, repoDir, "untracked.txt", "untracked")

	status, err := g.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}

	if status.IsClean() {
		t.Error("expected dirty status")
	}

	if len(status.Staged) != 1 || status.Staged[0] != "staged.txt" {
		t.Errorf("Staged = %v, want [staged.txt]", status.Staged)
	}

	if len(status.Unstaged) != 1 || status.Unstaged[0] != "README.md" {
		t.Errorf("Unstaged = %v, want [README.md]", status.Unstaged)
	}

	if len(status.Untracked) != 1 || status.Untracked[0] != "untracked.txt" {
		t.Errorf("Untracked = %v, want [untracked.txt]", status.Untracked)
	}
}

func TestAdd(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "file1.txt", "content1")
	createFile(t, repoDir, "file2.txt", "content2")

	g := NewGit(repoDir)

	// Add single file
	if err := g.Add("file1.txt"); err != nil {
		t.Fatalf("Add file1.txt: %v", err)
	}

	status, _ := g.Status()
	if len(status.Staged) != 1 || status.Staged[0] != "file1.txt" {
		t.Errorf("after Add file1: Staged = %v", status.Staged)
	}

	// Add multiple files
	if err := g.Add("file2.txt"); err != nil {
		t.Fatalf("Add file2.txt: %v", err)
	}

	status, _ = g.Status()
	if len(status.Staged) != 2 {
		t.Errorf("after Add file2: Staged = %v", status.Staged)
	}
}

func TestAdd_EmptyFiles(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	g := NewGit(repoDir)

	// Adding empty file list should not error
	if err := g.Add(); err != nil {
		t.Errorf("Add() with no files: %v", err)
	}
}

func TestCommit(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "file.txt", "content")

	g := NewGit(repoDir)

	// Commit with files
	if err := g.Commit("Test commit", "file.txt"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	status, _ := g.Status()
	if !status.IsClean() {
		t.Error("expected clean after commit")
	}

	// Verify commit exists
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = repoDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !contains(string(output), "Test commit") {
		t.Errorf("commit message not found in log: %s", output)
	}
}

func TestCommit_NothingToCommit(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create initial commit
	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Commit with nothing staged should not error
	if err := g.Commit("Empty commit"); err != nil {
		t.Errorf("Commit with nothing to commit: %v", err)
	}
}

func TestCurrentBranch(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Need at least one commit for branch to exist
	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	branch, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}

	if branch != "main" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "main")
	}
}

func TestCreateBranch(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Need at least one commit
	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Create branch
	if err := g.CreateBranch("feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Verify branch exists
	exists, err := g.BranchExists("feature")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if !exists {
		t.Error("branch should exist")
	}
}

func TestCreateBranch_AlreadyExists(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	if err := g.CreateBranch("feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Creating same branch should error
	err := g.CreateBranch("feature")
	if err != ErrBranchExists {
		t.Errorf("CreateBranch duplicate: got %v, want ErrBranchExists", err)
	}
}

func TestDeleteBranch(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	if err := g.CreateBranch("feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Delete branch (not force, since it's merged)
	if err := g.DeleteBranch("feature", false); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}

	exists, _ := g.BranchExists("feature")
	if exists {
		t.Error("branch should not exist after delete")
	}
}

func TestBranchExists(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// main should exist
	exists, err := g.BranchExists("main")
	if err != nil {
		t.Fatalf("BranchExists main: %v", err)
	}
	if !exists {
		t.Error("main should exist")
	}

	// nonexistent should not exist
	exists, err = g.BranchExists("nonexistent")
	if err != nil {
		t.Fatalf("BranchExists nonexistent: %v", err)
	}
	if exists {
		t.Error("nonexistent should not exist")
	}
}

func TestCheckout(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	if err := g.CreateBranch("feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Checkout feature
	if err := g.Checkout("feature"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	branch, _ := g.CurrentBranch()
	if branch != "feature" {
		t.Errorf("CurrentBranch after checkout = %q, want %q", branch, "feature")
	}
}

func TestCheckout_BranchNotFound(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	err := g.Checkout("nonexistent")
	if err != ErrBranchNotFound {
		t.Errorf("Checkout nonexistent: got %v, want ErrBranchNotFound", err)
	}
}

func TestRepoRoot(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create subdirectory
	subDir := filepath.Join(repoDir, "sub", "dir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("creating subdir: %v", err)
	}

	// NewGit from subdirectory should still find root
	g := NewGit(subDir)
	root, err := g.RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}

	// Resolve symlinks for comparison (handles macOS /private/var symlink)
	expectedRoot, _ := filepath.EvalSymlinks(filepath.Clean(repoDir))
	actualRoot, _ := filepath.EvalSymlinks(root)
	if actualRoot != expectedRoot {
		t.Errorf("RepoRoot() = %q, want %q", root, expectedRoot)
	}
}

func TestRepoRoot_NotGitRepo(t *testing.T) {
	dir, err := os.MkdirTemp("", "not-git-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	g := NewGit(dir)
	_, err = g.RepoRoot()
	if err != ErrNotGitRepo {
		t.Errorf("RepoRoot in non-git dir: got %v, want ErrNotGitRepo", err)
	}
}

func TestIsClean(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Should be clean
	clean, err := g.IsClean()
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	if !clean {
		t.Error("expected clean")
	}

	// Make dirty
	createFile(t, repoDir, "README.md", "# Modified\n")
	clean, err = g.IsClean()
	if err != nil {
		t.Fatalf("IsClean after modify: %v", err)
	}
	if clean {
		t.Error("expected dirty")
	}
}

func TestMerge(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Create feature branch and add commit
	if err := g.CreateBranch("feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := g.Checkout("feature"); err != nil {
		t.Fatalf("Checkout feature: %v", err)
	}
	createFile(t, repoDir, "feature.txt", "feature content")
	if err := g.Commit("Feature commit", "feature.txt"); err != nil {
		t.Fatalf("feature commit: %v", err)
	}

	// Switch to main and merge
	if err := g.Checkout("main"); err != nil {
		t.Fatalf("Checkout main: %v", err)
	}
	if err := g.Merge("feature", true); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Verify feature.txt exists on main
	_, err := os.Stat(filepath.Join(repoDir, "feature.txt"))
	if err != nil {
		t.Error("feature.txt should exist after merge")
	}
}

func TestStatus_IsCleanMethod(t *testing.T) {
	status := &Status{
		Branch:    "main",
		Staged:    []string{},
		Unstaged:  []string{},
		Untracked: []string{"untracked.txt"},
	}

	// Clean ignores untracked files
	if !status.IsClean() {
		t.Error("status with only untracked should be clean")
	}

	// Staged makes it dirty
	status.Staged = []string{"staged.txt"}
	if status.IsClean() {
		t.Error("status with staged should not be clean")
	}

	// Unstaged makes it dirty
	status.Staged = nil
	status.Unstaged = []string{"modified.txt"}
	if status.IsClean() {
		t.Error("status with unstaged should not be clean")
	}
}

// contains checks if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ==================== Worktree Tests ====================

func TestCreateWorktree_NewBranch(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Need at least one commit
	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Create worktree with new branch
	worktreePath := filepath.Join(repoDir, ".worktrees", "feature")
	if err := g.CreateWorktree(worktreePath, "feature"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Verify worktree directory exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Error("worktree directory should exist")
	}

	// Verify branch was created
	exists, err := g.BranchExists("feature")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if !exists {
		t.Error("feature branch should exist")
	}

	// Verify branch is checked out in worktree
	worktreeGit := NewGit(worktreePath)
	branch, err := worktreeGit.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch in worktree: %v", err)
	}
	if branch != "feature" {
		t.Errorf("worktree branch = %q, want %q", branch, "feature")
	}
}

func TestCreateWorktree_ExistingBranch(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Create branch first
	if err := g.CreateBranch("existing-branch"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Create worktree with existing branch
	worktreePath := filepath.Join(repoDir, ".worktrees", "existing")
	if err := g.CreateWorktree(worktreePath, "existing-branch"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Verify worktree has correct branch
	worktreeGit := NewGit(worktreePath)
	branch, err := worktreeGit.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch in worktree: %v", err)
	}
	if branch != "existing-branch" {
		t.Errorf("worktree branch = %q, want %q", branch, "existing-branch")
	}
}

func TestCreateWorktree_BranchAlreadyCheckedOut(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// main is already checked out in the main worktree
	worktreePath := filepath.Join(repoDir, ".worktrees", "main-copy")
	err := g.CreateWorktree(worktreePath, "main")
	if err != ErrBranchAlreadyCheckedOut {
		t.Errorf("CreateWorktree with checked out branch: got %v, want ErrBranchAlreadyCheckedOut", err)
	}
}

func TestCreateWorktree_BranchCheckedOutInOtherWorktree(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Create first worktree with feature branch
	worktree1 := filepath.Join(repoDir, ".worktrees", "wt1")
	if err := g.CreateWorktree(worktree1, "feature"); err != nil {
		t.Fatalf("CreateWorktree wt1: %v", err)
	}

	// Try to create second worktree with same branch
	worktree2 := filepath.Join(repoDir, ".worktrees", "wt2")
	err := g.CreateWorktree(worktree2, "feature")
	if err != ErrBranchAlreadyCheckedOut {
		t.Errorf("CreateWorktree with same branch: got %v, want ErrBranchAlreadyCheckedOut", err)
	}
}

func TestRemoveWorktree(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Create worktree
	worktreePath := filepath.Join(repoDir, ".worktrees", "feature")
	if err := g.CreateWorktree(worktreePath, "feature"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Remove worktree
	if err := g.RemoveWorktree(worktreePath); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}

	// Verify worktree directory is gone
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("worktree directory should not exist after removal")
	}
}

func TestRemoveWorktree_NotFound(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	err := g.RemoveWorktree("/nonexistent/path")
	if err != ErrWorktreeNotFound {
		t.Errorf("RemoveWorktree nonexistent: got %v, want ErrWorktreeNotFound", err)
	}
}

func TestRemoveWorktree_WithChanges(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Create worktree
	worktreePath := filepath.Join(repoDir, ".worktrees", "feature")
	if err := g.CreateWorktree(worktreePath, "feature"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Create uncommitted changes in worktree
	createFile(t, worktreePath, "untracked.txt", "untracked content")

	// RemoveWorktree should force-remove even with changes
	if err := g.RemoveWorktree(worktreePath); err != nil {
		t.Fatalf("RemoveWorktree with changes: %v", err)
	}

	// Verify worktree is gone
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("worktree directory should not exist after removal")
	}
}

func TestListWorktrees(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// List worktrees (should have just main worktree)
	worktrees, err := g.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(worktrees))
	}

	// Resolve symlinks for path comparison (handles macOS /private/var)
	expectedPath, _ := filepath.EvalSymlinks(repoDir)
	actualPath, _ := filepath.EvalSymlinks(worktrees[0].Path)
	if actualPath != expectedPath {
		t.Errorf("worktree path = %q, want %q", worktrees[0].Path, expectedPath)
	}
	if worktrees[0].Branch != "main" {
		t.Errorf("worktree branch = %q, want %q", worktrees[0].Branch, "main")
	}
	if worktrees[0].Commit == "" {
		t.Error("worktree commit should not be empty")
	}
}

func TestListWorktrees_Multiple(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	createFile(t, repoDir, "README.md", "# Test\n")
	g := NewGit(repoDir)
	if err := g.Commit("Initial commit", "README.md"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Create additional worktrees
	wt1 := filepath.Join(repoDir, ".worktrees", "feature1")
	if err := g.CreateWorktree(wt1, "feature1"); err != nil {
		t.Fatalf("CreateWorktree feature1: %v", err)
	}

	wt2 := filepath.Join(repoDir, ".worktrees", "feature2")
	if err := g.CreateWorktree(wt2, "feature2"); err != nil {
		t.Fatalf("CreateWorktree feature2: %v", err)
	}

	// List worktrees
	worktrees, err := g.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}

	if len(worktrees) != 3 {
		t.Fatalf("expected 3 worktrees, got %d", len(worktrees))
	}

	// Verify branches are present
	branches := make(map[string]bool)
	for _, wt := range worktrees {
		branches[wt.Branch] = true
	}

	if !branches["main"] {
		t.Error("main branch not found in worktrees")
	}
	if !branches["feature1"] {
		t.Error("feature1 branch not found in worktrees")
	}
	if !branches["feature2"] {
		t.Error("feature2 branch not found in worktrees")
	}
}

func TestWorktreeInfo(t *testing.T) {
	// Test WorktreeInfo struct directly
	info := WorktreeInfo{
		Path:   "/path/to/worktree",
		Branch: "feature",
		Commit: "abc123",
		Bare:   false,
	}

	if info.Path != "/path/to/worktree" {
		t.Errorf("Path = %q, want %q", info.Path, "/path/to/worktree")
	}
	if info.Branch != "feature" {
		t.Errorf("Branch = %q, want %q", info.Branch, "feature")
	}
	if info.Commit != "abc123" {
		t.Errorf("Commit = %q, want %q", info.Commit, "abc123")
	}
	if info.Bare {
		t.Error("Bare should be false")
	}
}
