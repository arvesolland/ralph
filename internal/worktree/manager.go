// Package worktree manages git worktrees for plan execution.
package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/plan"
)

// Common errors returned by WorktreeManager operations.
var (
	ErrWorktreeExists   = errors.New("worktree already exists")
	ErrWorktreeNotFound = errors.New("worktree not found")
)

// Worktree represents an existing worktree for a plan.
type Worktree struct {
	// Path is the absolute path to the worktree directory.
	Path string

	// Branch is the git branch checked out in this worktree.
	Branch string

	// PlanName is the name of the plan associated with this worktree.
	PlanName string
}

// WorktreeManager handles high-level worktree operations for plans.
type WorktreeManager struct {
	// git is the Git interface for running git commands.
	git git.Git

	// baseDir is the directory where worktrees are created (.ralph/worktrees/).
	baseDir string

	// repoRoot is the root of the git repository.
	repoRoot string
}

// NewManager creates a new WorktreeManager.
// baseDir is typically ".ralph/worktrees/" relative to the repo root.
func NewManager(g git.Git, baseDir string) (*WorktreeManager, error) {
	repoRoot, err := g.RepoRoot()
	if err != nil {
		return nil, fmt.Errorf("getting repo root: %w", err)
	}

	// Make baseDir absolute if needed
	if !filepath.IsAbs(baseDir) {
		baseDir = filepath.Join(repoRoot, baseDir)
	}

	return &WorktreeManager{
		git:      g,
		baseDir:  baseDir,
		repoRoot: repoRoot,
	}, nil
}

// Path returns the worktree path for a plan.
// The path is: <baseDir>/<branch-name> (without feat/ prefix for cleaner directory names).
func (m *WorktreeManager) Path(p *plan.Plan) string {
	// Use branch name without the feat/ prefix for shorter directory names
	dirName := strings.TrimPrefix(p.Branch, "feat/")
	return filepath.Join(m.baseDir, dirName)
}

// Exists checks if a worktree exists for the given plan.
func (m *WorktreeManager) Exists(p *plan.Plan) bool {
	worktreePath := m.Path(p)
	info, err := os.Stat(worktreePath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Get returns the worktree for a plan if it exists, or nil if not.
func (m *WorktreeManager) Get(p *plan.Plan) (*Worktree, error) {
	if !m.Exists(p) {
		return nil, nil
	}

	worktreePath := m.Path(p)

	// Verify it's actually a git worktree by listing worktrees
	worktrees, err := m.git.ListWorktrees()
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}

	// Find our worktree in the list
	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path: %w", err)
	}

	for _, wt := range worktrees {
		// Resolve symlinks for comparison (macOS /tmp vs /private/var)
		wtPath, _ := filepath.EvalSymlinks(wt.Path)
		checkPath, _ := filepath.EvalSymlinks(absPath)

		if wtPath == checkPath || wt.Path == absPath {
			return &Worktree{
				Path:     wt.Path,
				Branch:   wt.Branch,
				PlanName: p.Name,
			}, nil
		}
	}

	// Directory exists but not a valid worktree - could be leftover
	return nil, nil
}

// Create creates a new worktree for the given plan.
// Returns the Worktree on success.
// Returns ErrWorktreeExists if a worktree already exists for this plan.
// Returns git.ErrBranchAlreadyCheckedOut if the branch is checked out elsewhere.
func (m *WorktreeManager) Create(p *plan.Plan) (*Worktree, error) {
	// Check if worktree already exists
	if m.Exists(p) {
		return nil, ErrWorktreeExists
	}

	// Ensure base directory exists
	if err := os.MkdirAll(m.baseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating base directory: %w", err)
	}

	worktreePath := m.Path(p)

	// Create the worktree using git
	if err := m.git.CreateWorktree(worktreePath, p.Branch); err != nil {
		return nil, fmt.Errorf("creating worktree: %w", err)
	}

	return &Worktree{
		Path:     worktreePath,
		Branch:   p.Branch,
		PlanName: p.Name,
	}, nil
}

// Remove removes the worktree for the given plan.
// If deleteBranch is true, also deletes the git branch.
// Returns ErrWorktreeNotFound if no worktree exists for this plan.
func (m *WorktreeManager) Remove(p *plan.Plan, deleteBranch bool) error {
	worktreePath := m.Path(p)

	// Check if worktree exists
	if !m.Exists(p) {
		return ErrWorktreeNotFound
	}

	// Remove the worktree using git
	if err := m.git.RemoveWorktree(worktreePath); err != nil {
		// If git says it's not found, treat as success (already removed)
		if errors.Is(err, git.ErrWorktreeNotFound) {
			// Clean up directory if it exists
			os.RemoveAll(worktreePath)
		} else {
			return fmt.Errorf("removing worktree: %w", err)
		}
	}

	// Optionally delete the branch
	if deleteBranch {
		if err := m.git.DeleteBranch(p.Branch, true); err != nil {
			// Branch not found is not an error (may have been deleted)
			if !errors.Is(err, git.ErrBranchNotFound) {
				return fmt.Errorf("deleting branch: %w", err)
			}
		}
	}

	return nil
}

// BaseDir returns the base directory where worktrees are created.
func (m *WorktreeManager) BaseDir() string {
	return m.baseDir
}

// RepoRoot returns the repository root path.
func (m *WorktreeManager) RepoRoot() string {
	return m.repoRoot
}
