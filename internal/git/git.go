// Package git provides git operations.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Common errors returned by Git operations.
var (
	ErrNotGitRepo        = errors.New("not a git repository")
	ErrUncommittedChanges = errors.New("uncommitted changes exist")
	ErrBranchNotFound    = errors.New("branch not found")
	ErrBranchExists      = errors.New("branch already exists")
	ErrMergeConflict     = errors.New("merge conflict")
)

// Status represents the state of a git working tree.
type Status struct {
	Branch    string   // Current branch name
	Staged    []string // Files staged for commit
	Unstaged  []string // Modified files not staged
	Untracked []string // Untracked files
}

// IsClean returns true if there are no uncommitted changes.
func (s *Status) IsClean() bool {
	return len(s.Staged) == 0 && len(s.Unstaged) == 0
}

// Git defines the interface for git operations.
type Git interface {
	// Status returns the current status of the working tree.
	Status() (*Status, error)

	// Add stages files for commit.
	Add(files ...string) error

	// Commit creates a commit with the given message.
	// If files are provided, they are staged before committing.
	Commit(message string, files ...string) error

	// Push pushes the current branch to remote.
	Push() error

	// PushWithUpstream pushes and sets upstream tracking.
	PushWithUpstream(remote, branch string) error

	// Pull pulls changes from remote.
	Pull() error

	// CurrentBranch returns the name of the current branch.
	CurrentBranch() (string, error)

	// CreateBranch creates a new branch at the current HEAD.
	CreateBranch(name string) error

	// DeleteBranch deletes a local branch.
	DeleteBranch(name string, force bool) error

	// DeleteRemoteBranch deletes a remote branch.
	DeleteRemoteBranch(remote, branch string) error

	// BranchExists checks if a branch exists locally.
	BranchExists(name string) (bool, error)

	// Checkout switches to a branch.
	Checkout(branch string) error

	// Merge merges a branch into the current branch.
	Merge(branch string, noFastForward bool) error

	// RepoRoot returns the root directory of the repository.
	RepoRoot() (string, error)

	// IsClean returns true if there are no uncommitted changes.
	IsClean() (bool, error)

	// WorkDir returns the working directory.
	WorkDir() string
}

// CLIGit implements Git interface using git CLI commands.
type CLIGit struct {
	workDir string
}

// NewGit creates a new Git instance for the specified directory.
func NewGit(workDir string) Git {
	return &CLIGit{workDir: workDir}
}

// WorkDir returns the working directory.
func (g *CLIGit) WorkDir() string {
	return g.workDir
}

// run executes a git command and returns stdout, stderr, and error.
// Output is trimmed of leading/trailing whitespace.
func (g *CLIGit) run(args ...string) (string, string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// runRaw executes a git command and returns stdout, stderr, and error.
// Output is NOT trimmed, preserving exact formatting.
func (g *CLIGit) runRaw(args ...string) (string, string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// runWithEnv executes a git command with custom environment variables.
func (g *CLIGit) runWithEnv(env []string, args ...string) (string, string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.workDir
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// Status returns the current status of the working tree.
func (g *CLIGit) Status() (*Status, error) {
	status := &Status{}

	// Get current branch
	// Use symbolic-ref which works even in repos with no commits
	branch, _, err := g.run("symbolic-ref", "--short", "HEAD")
	if err != nil {
		// Fallback for detached HEAD or other edge cases
		branch, _, err = g.run("rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			// In a repo with no commits, we can't determine the branch
			// but we can still get status. Default to empty string.
			branch = ""
		}
	}
	status.Branch = branch

	// Get status in porcelain format (use runRaw to preserve leading spaces)
	output, _, err := g.runRaw("status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("getting status: %w", err)
	}

	output = strings.TrimSuffix(output, "\n")
	if output == "" {
		return status, nil
	}

	// Parse porcelain output
	// Format: XY PATH
	// X = index status, Y = working tree status
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 3 {
			continue
		}

		indexStatus := line[0]
		workTreeStatus := line[1]
		file := line[3:] // Don't TrimSpace - filename starts at position 3

		// Handle renamed files (format: "R  old -> new")
		if strings.Contains(file, " -> ") {
			parts := strings.Split(file, " -> ")
			file = parts[len(parts)-1]
		}

		// Check index status (staged)
		if indexStatus != ' ' && indexStatus != '?' {
			status.Staged = append(status.Staged, file)
		}

		// Check working tree status (unstaged)
		if workTreeStatus != ' ' && workTreeStatus != '?' {
			status.Unstaged = append(status.Unstaged, file)
		}

		// Check for untracked
		if indexStatus == '?' && workTreeStatus == '?' {
			status.Untracked = append(status.Untracked, file)
		}
	}

	return status, nil
}

// Add stages files for commit.
func (g *CLIGit) Add(files ...string) error {
	if len(files) == 0 {
		return nil
	}

	args := append([]string{"add", "--"}, files...)
	_, stderr, err := g.run(args...)
	if err != nil {
		return fmt.Errorf("git add: %s: %w", stderr, err)
	}
	return nil
}

// Commit creates a commit with the given message.
func (g *CLIGit) Commit(message string, files ...string) error {
	// Stage files if provided
	if len(files) > 0 {
		if err := g.Add(files...); err != nil {
			return err
		}
	}

	// Run commit - check both stdout and stderr for "nothing to commit"
	stdout, stderr, err := g.run("commit", "-m", message)
	if err != nil {
		// "nothing to commit" can appear in stdout or stderr depending on git version
		if strings.Contains(stderr, "nothing to commit") || strings.Contains(stdout, "nothing to commit") {
			return nil // Not an error if nothing to commit
		}
		return fmt.Errorf("git commit: %s: %w", stderr, err)
	}
	return nil
}

// Push pushes the current branch to remote.
func (g *CLIGit) Push() error {
	_, stderr, err := g.run("push")
	if err != nil {
		return fmt.Errorf("git push: %s: %w", stderr, err)
	}
	return nil
}

// PushWithUpstream pushes and sets upstream tracking.
func (g *CLIGit) PushWithUpstream(remote, branch string) error {
	_, stderr, err := g.run("push", "-u", remote, branch)
	if err != nil {
		return fmt.Errorf("git push -u: %s: %w", stderr, err)
	}
	return nil
}

// Pull pulls changes from remote.
func (g *CLIGit) Pull() error {
	_, stderr, err := g.run("pull")
	if err != nil {
		return fmt.Errorf("git pull: %s: %w", stderr, err)
	}
	return nil
}

// CurrentBranch returns the name of the current branch.
func (g *CLIGit) CurrentBranch() (string, error) {
	branch, stderr, err := g.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("getting current branch: %s: %w", stderr, err)
	}
	return branch, nil
}

// CreateBranch creates a new branch at the current HEAD.
func (g *CLIGit) CreateBranch(name string) error {
	_, stderr, err := g.run("branch", name)
	if err != nil {
		if strings.Contains(stderr, "already exists") {
			return ErrBranchExists
		}
		return fmt.Errorf("git branch: %s: %w", stderr, err)
	}
	return nil
}

// DeleteBranch deletes a local branch.
func (g *CLIGit) DeleteBranch(name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, stderr, err := g.run("branch", flag, name)
	if err != nil {
		if strings.Contains(stderr, "not found") {
			return ErrBranchNotFound
		}
		return fmt.Errorf("git branch %s: %s: %w", flag, stderr, err)
	}
	return nil
}

// DeleteRemoteBranch deletes a remote branch.
func (g *CLIGit) DeleteRemoteBranch(remote, branch string) error {
	_, stderr, err := g.run("push", remote, "--delete", branch)
	if err != nil {
		return fmt.Errorf("git push --delete: %s: %w", stderr, err)
	}
	return nil
}

// BranchExists checks if a branch exists locally.
func (g *CLIGit) BranchExists(name string) (bool, error) {
	_, _, err := g.run("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err != nil {
		// Exit code 1 means branch doesn't exist
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("checking branch: %w", err)
	}
	return true, nil
}

// Checkout switches to a branch.
func (g *CLIGit) Checkout(branch string) error {
	_, stderr, err := g.run("checkout", branch)
	if err != nil {
		if strings.Contains(stderr, "did not match any") {
			return ErrBranchNotFound
		}
		return fmt.Errorf("git checkout: %s: %w", stderr, err)
	}
	return nil
}

// Merge merges a branch into the current branch.
func (g *CLIGit) Merge(branch string, noFastForward bool) error {
	args := []string{"merge"}
	if noFastForward {
		args = append(args, "--no-ff")
	}
	args = append(args, branch)

	_, stderr, err := g.run(args...)
	if err != nil {
		if strings.Contains(stderr, "CONFLICT") || strings.Contains(stderr, "Automatic merge failed") {
			return ErrMergeConflict
		}
		return fmt.Errorf("git merge: %s: %w", stderr, err)
	}
	return nil
}

// RepoRoot returns the root directory of the repository.
func (g *CLIGit) RepoRoot() (string, error) {
	root, stderr, err := g.run("rev-parse", "--show-toplevel")
	if err != nil {
		if strings.Contains(stderr, "not a git repository") {
			return "", ErrNotGitRepo
		}
		return "", fmt.Errorf("getting repo root: %s: %w", stderr, err)
	}
	// Clean up the path for consistency
	return filepath.Clean(root), nil
}

// IsClean returns true if there are no uncommitted changes.
func (g *CLIGit) IsClean() (bool, error) {
	status, err := g.Status()
	if err != nil {
		return false, err
	}
	return status.IsClean(), nil
}
