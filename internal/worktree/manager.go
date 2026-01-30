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

// CleanupResult contains information about a cleaned up worktree.
type CleanupResult struct {
	// Path is the absolute path to the removed worktree.
	Path string

	// PlanName is the derived plan name from the worktree directory name.
	PlanName string

	// Skipped is true if the worktree was not removed (e.g., has uncommitted changes).
	Skipped bool

	// SkipReason explains why the worktree was skipped.
	SkipReason string
}

// Cleanup removes orphaned worktrees that no longer have associated plans.
// A worktree is orphaned if it exists in .ralph/worktrees/ but has no matching
// plan in pending/ or current/.
// Worktrees with uncommitted changes are NOT removed (safety check).
// Returns the list of cleanup results (removed and skipped worktrees).
func (m *WorktreeManager) Cleanup(queue *plan.Queue) ([]CleanupResult, error) {
	var results []CleanupResult

	// List all directories in baseDir
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No worktrees directory = nothing to clean
			return results, nil
		}
		return nil, fmt.Errorf("reading worktrees directory: %w", err)
	}

	// Get active plan names (from pending and current)
	activePlans := make(map[string]bool)

	pending, err := queue.Pending()
	if err != nil {
		return nil, fmt.Errorf("listing pending plans: %w", err)
	}
	for _, p := range pending {
		// Map plan name to directory name (matches Path() logic)
		dirName := strings.TrimPrefix(p.Branch, "feat/")
		activePlans[dirName] = true
	}

	current, err := queue.Current()
	if err != nil {
		return nil, fmt.Errorf("getting current plan: %w", err)
	}
	if current != nil {
		dirName := strings.TrimPrefix(current.Branch, "feat/")
		activePlans[dirName] = true
	}

	// Check each directory in baseDir
	for _, entry := range entries {
		if !entry.IsDir() {
			continue // Skip non-directories
		}

		dirName := entry.Name()
		worktreePath := filepath.Join(m.baseDir, dirName)

		// Check if this worktree has an associated active plan
		if activePlans[dirName] {
			continue // Not orphaned - skip
		}

		// This worktree appears orphaned - check for uncommitted changes
		// Create a Git instance for this worktree to check its status
		wtGit := git.NewGit(worktreePath)
		isClean, err := wtGit.IsClean()
		if err != nil {
			// If we can't check status (e.g., not a valid git worktree),
			// skip it to be safe and log the reason
			results = append(results, CleanupResult{
				Path:       worktreePath,
				PlanName:   dirName,
				Skipped:    true,
				SkipReason: fmt.Sprintf("could not check status: %v", err),
			})
			continue
		}

		if !isClean {
			// Has uncommitted changes - skip for safety
			results = append(results, CleanupResult{
				Path:       worktreePath,
				PlanName:   dirName,
				Skipped:    true,
				SkipReason: "has uncommitted changes",
			})
			continue
		}

		// Safe to remove - use git worktree remove
		if err := m.git.RemoveWorktree(worktreePath); err != nil {
			// If git remove fails, try to clean up the directory directly
			// This can happen if the worktree metadata is corrupted
			if errors.Is(err, git.ErrWorktreeNotFound) {
				// Not a valid worktree, just remove the directory
				if removeErr := os.RemoveAll(worktreePath); removeErr != nil {
					results = append(results, CleanupResult{
						Path:       worktreePath,
						PlanName:   dirName,
						Skipped:    true,
						SkipReason: fmt.Sprintf("failed to remove directory: %v", removeErr),
					})
					continue
				}
			} else {
				results = append(results, CleanupResult{
					Path:       worktreePath,
					PlanName:   dirName,
					Skipped:    true,
					SkipReason: fmt.Sprintf("git worktree remove failed: %v", err),
				})
				continue
			}
		}

		// Successfully removed
		results = append(results, CleanupResult{
			Path:     worktreePath,
			PlanName: dirName,
			Skipped:  false,
		})
	}

	return results, nil
}
