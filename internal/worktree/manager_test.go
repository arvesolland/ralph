package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/plan"
)

// mockGit implements git.Git interface for testing.
type mockGit struct {
	workDir    string
	repoRoot   string
	worktrees  []git.WorktreeInfo
	branches   map[string]bool
	createErr  error
	removeErr  error
	deleteBranchErr error
}

func newMockGit(workDir string) *mockGit {
	return &mockGit{
		workDir:  workDir,
		repoRoot: workDir,
		branches: make(map[string]bool),
	}
}

func (m *mockGit) Status() (*git.Status, error)                        { return &git.Status{}, nil }
func (m *mockGit) Add(files ...string) error                           { return nil }
func (m *mockGit) Commit(message string, files ...string) error        { return nil }
func (m *mockGit) Push() error                                         { return nil }
func (m *mockGit) PushWithUpstream(remote, branch string) error        { return nil }
func (m *mockGit) Pull() error                                         { return nil }
func (m *mockGit) CurrentBranch() (string, error)                      { return "main", nil }
func (m *mockGit) CreateBranch(name string) error                      { m.branches[name] = true; return nil }
func (m *mockGit) DeleteBranch(name string, force bool) error          {
	if m.deleteBranchErr != nil {
		return m.deleteBranchErr
	}
	if !m.branches[name] {
		return git.ErrBranchNotFound
	}
	delete(m.branches, name)
	return nil
}
func (m *mockGit) DeleteRemoteBranch(remote, branch string) error      { return nil }
func (m *mockGit) BranchExists(name string) (bool, error)              { return m.branches[name], nil }
func (m *mockGit) Checkout(branch string) error                        { return nil }
func (m *mockGit) Merge(branch string, noFastForward bool) error       { return nil }
func (m *mockGit) RepoRoot() (string, error)                           { return m.repoRoot, nil }
func (m *mockGit) IsClean() (bool, error)                              { return true, nil }
func (m *mockGit) WorkDir() string                                     { return m.workDir }

func (m *mockGit) CreateWorktree(path, branch string) error {
	if m.createErr != nil {
		return m.createErr
	}
	// Simulate worktree creation
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	m.worktrees = append(m.worktrees, git.WorktreeInfo{
		Path:   path,
		Branch: branch,
	})
	m.branches[branch] = true
	return nil
}

func (m *mockGit) RemoveWorktree(path string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	// Find and remove worktree from list
	for i, wt := range m.worktrees {
		if wt.Path == path {
			m.worktrees = append(m.worktrees[:i], m.worktrees[i+1:]...)
			os.RemoveAll(path)
			return nil
		}
	}
	return git.ErrWorktreeNotFound
}

func (m *mockGit) ListWorktrees() ([]git.WorktreeInfo, error) {
	return m.worktrees, nil
}

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)

	m, err := NewManager(g, ".ralph/worktrees")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if m.repoRoot != tmpDir {
		t.Errorf("repoRoot = %q, want %q", m.repoRoot, tmpDir)
	}

	expectedBase := filepath.Join(tmpDir, ".ralph/worktrees")
	if m.baseDir != expectedBase {
		t.Errorf("baseDir = %q, want %q", m.baseDir, expectedBase)
	}
}

func TestNewManager_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)

	absPath := filepath.Join(tmpDir, "custom-worktrees")
	m, err := NewManager(g, absPath)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if m.baseDir != absPath {
		t.Errorf("baseDir = %q, want %q", m.baseDir, absPath)
	}
}

func TestManager_Path(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	tests := []struct {
		planName string
		branch   string
		want     string
	}{
		{"go-rewrite", "feat/go-rewrite", filepath.Join(tmpDir, ".ralph/worktrees/go-rewrite")},
		{"my-plan", "feat/my-plan", filepath.Join(tmpDir, ".ralph/worktrees/my-plan")},
		{"complex-name-v2", "feat/complex-name-v2", filepath.Join(tmpDir, ".ralph/worktrees/complex-name-v2")},
	}

	for _, tt := range tests {
		t.Run(tt.planName, func(t *testing.T) {
			p := &plan.Plan{Name: tt.planName, Branch: tt.branch}
			got := m.Path(p)
			if got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestManager_Exists_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	if m.Exists(p) {
		t.Error("Exists() = true, want false for non-existent worktree")
	}
}

func TestManager_Exists_AfterCreate(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	_, err := m.Create(p)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if !m.Exists(p) {
		t.Error("Exists() = false, want true after Create")
	}
}

func TestManager_Create(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	wt, err := m.Create(p)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, ".ralph/worktrees/test-plan")
	if wt.Path != expectedPath {
		t.Errorf("Worktree.Path = %q, want %q", wt.Path, expectedPath)
	}

	if wt.Branch != "feat/test-plan" {
		t.Errorf("Worktree.Branch = %q, want %q", wt.Branch, "feat/test-plan")
	}

	if wt.PlanName != "test-plan" {
		t.Errorf("Worktree.PlanName = %q, want %q", wt.PlanName, "test-plan")
	}

	// Verify directory was created
	if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
		t.Error("Worktree directory was not created")
	}
}

func TestManager_Create_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	// First create should succeed
	_, err := m.Create(p)
	if err != nil {
		t.Fatalf("First Create failed: %v", err)
	}

	// Second create should fail
	_, err = m.Create(p)
	if !errors.Is(err, ErrWorktreeExists) {
		t.Errorf("Second Create error = %v, want ErrWorktreeExists", err)
	}
}

func TestManager_Create_BranchCheckedOut(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	g.createErr = git.ErrBranchAlreadyCheckedOut
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	_, err := m.Create(p)
	if err == nil {
		t.Error("Create should have failed with branch checked out error")
	}
}

func TestManager_Get_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	wt, err := m.Get(p)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if wt != nil {
		t.Error("Get() should return nil for non-existent worktree")
	}
}

func TestManager_Get_AfterCreate(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	created, _ := m.Create(p)

	got, err := m.Get(p)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got == nil {
		t.Fatal("Get() returned nil, want worktree")
	}

	if got.Path != created.Path {
		t.Errorf("Get().Path = %q, want %q", got.Path, created.Path)
	}

	if got.Branch != created.Branch {
		t.Errorf("Get().Branch = %q, want %q", got.Branch, created.Branch)
	}
}

func TestManager_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	// Create first
	_, err := m.Create(p)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Remove without deleting branch
	err = m.Remove(p, false)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify worktree no longer exists
	if m.Exists(p) {
		t.Error("Worktree should not exist after Remove")
	}

	// Branch should still exist
	if !g.branches[p.Branch] {
		t.Error("Branch should still exist when deleteBranch=false")
	}
}

func TestManager_Remove_WithDeleteBranch(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	// Create first
	_, err := m.Create(p)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Remove with branch deletion
	err = m.Remove(p, true)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Branch should be deleted
	if g.branches[p.Branch] {
		t.Error("Branch should be deleted when deleteBranch=true")
	}
}

func TestManager_Remove_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	err := m.Remove(p, false)
	if !errors.Is(err, ErrWorktreeNotFound) {
		t.Errorf("Remove error = %v, want ErrWorktreeNotFound", err)
	}
}

func TestManager_BaseDir(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	expected := filepath.Join(tmpDir, ".ralph/worktrees")
	if m.BaseDir() != expected {
		t.Errorf("BaseDir() = %q, want %q", m.BaseDir(), expected)
	}
}

func TestManager_RepoRoot(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	if m.RepoRoot() != tmpDir {
		t.Errorf("RepoRoot() = %q, want %q", m.RepoRoot(), tmpDir)
	}
}

func TestManager_FullLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	g := newMockGit(tmpDir)
	m, _ := NewManager(g, ".ralph/worktrees")

	p := &plan.Plan{Name: "lifecycle-plan", Branch: "feat/lifecycle-plan"}

	// Initially should not exist
	if m.Exists(p) {
		t.Error("Worktree should not exist initially")
	}

	// Create
	wt, err := m.Create(p)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Should exist now
	if !m.Exists(p) {
		t.Error("Worktree should exist after Create")
	}

	// Get should return same info
	got, err := m.Get(p)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Path != wt.Path {
		t.Error("Get returned different path than Create")
	}

	// Remove
	err = m.Remove(p, true)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Should not exist after remove
	if m.Exists(p) {
		t.Error("Worktree should not exist after Remove")
	}

	// Get should return nil
	got, err = m.Get(p)
	if err != nil {
		t.Fatalf("Get after Remove failed: %v", err)
	}
	if got != nil {
		t.Error("Get should return nil after Remove")
	}
}
