// Package worktree manages git worktrees for plan execution.
package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/arvesolland/ralph/internal/log"
)

// Lockfile defines a lockfile and its associated install command.
type Lockfile struct {
	// Name is the lockfile filename.
	Name string

	// Command is the install command to run.
	Command string

	// Args are the arguments to pass to the command.
	Args []string

	// Description is a human-readable description for logging.
	Description string
}

// lockfileOrder defines the order in which lockfiles are checked.
// The first matching lockfile's command is executed.
// Order matters: more specific lockfiles (e.g., pnpm-lock.yaml) come before
// less specific ones (e.g., package-lock.json).
var lockfileOrder = []Lockfile{
	// Node.js package managers - ordered by specificity
	{Name: "pnpm-lock.yaml", Command: "pnpm", Args: []string{"install", "--frozen-lockfile"}, Description: "pnpm"},
	{Name: "bun.lockb", Command: "bun", Args: []string{"install", "--frozen-lockfile"}, Description: "Bun"},
	{Name: "yarn.lock", Command: "yarn", Args: []string{"install", "--frozen-lockfile"}, Description: "Yarn"},
	{Name: "package-lock.json", Command: "npm", Args: []string{"ci"}, Description: "npm"},

	// PHP
	{Name: "composer.lock", Command: "composer", Args: []string{"install"}, Description: "Composer"},

	// Python
	{Name: "poetry.lock", Command: "poetry", Args: []string{"install"}, Description: "Poetry"},
	{Name: "requirements.txt", Command: "pip", Args: []string{"install", "-r", "requirements.txt"}, Description: "pip"},

	// Ruby
	{Name: "Gemfile.lock", Command: "bundle", Args: []string{"install"}, Description: "Bundler"},

	// Go
	{Name: "go.sum", Command: "go", Args: []string{"mod", "download"}, Description: "Go modules"},

	// Rust
	{Name: "Cargo.lock", Command: "cargo", Args: []string{"fetch"}, Description: "Cargo"},
}

// ErrCommandNotFound is returned when the install command is not found in PATH.
var ErrCommandNotFound = errors.New("command not found")

// InstallResult contains the result of a dependency installation.
type InstallResult struct {
	// Lockfile is the detected lockfile.
	Lockfile string

	// Command is the command that was run.
	Command string

	// Output is the combined stdout/stderr output.
	Output string
}

// DetectAndInstall detects the project type from lockfiles in the given directory
// and runs the appropriate dependency installation command.
//
// Returns nil if no lockfile is found (not an error - some projects have no dependencies).
// Returns the InstallResult if a lockfile was found and the command was run.
// Returns an error if the command fails or is not found.
func DetectAndInstall(worktreePath string) (*InstallResult, error) {
	// Check each lockfile in order
	for _, lf := range lockfileOrder {
		lockfilePath := filepath.Join(worktreePath, lf.Name)
		if _, err := os.Stat(lockfilePath); err == nil {
			// Lockfile found - run the install command
			log.Debug("Detected %s lockfile: %s", lf.Description, lf.Name)
			return runInstallCommand(worktreePath, lf)
		}
	}

	// No lockfile found - this is normal for some projects
	log.Debug("No lockfile found, skipping dependency installation")
	return nil, nil
}

// runInstallCommand executes the install command for the given lockfile.
func runInstallCommand(workDir string, lf Lockfile) (*InstallResult, error) {
	// Check if command exists in PATH
	cmdPath, err := exec.LookPath(lf.Command)
	if err != nil {
		return nil, fmt.Errorf("%w: %s (required for %s)", ErrCommandNotFound, lf.Command, lf.Description)
	}

	log.Info("Installing dependencies with %s...", lf.Description)

	// Build and execute the command
	cmd := exec.Command(cmdPath, lf.Args...)
	cmd.Dir = workDir

	// Capture combined output
	output, err := cmd.CombinedOutput()

	result := &InstallResult{
		Lockfile: lf.Name,
		Command:  fmt.Sprintf("%s %v", lf.Command, lf.Args),
		Output:   string(output),
	}

	if err != nil {
		// Command failed - include output in error for debugging
		return result, fmt.Errorf("running %s: %w\nOutput:\n%s", lf.Command, err, output)
	}

	log.Success("Dependencies installed successfully")
	return result, nil
}

// DetectLockfile returns the first matching lockfile in the directory without running any commands.
// Returns empty string if no lockfile is found.
func DetectLockfile(worktreePath string) string {
	for _, lf := range lockfileOrder {
		lockfilePath := filepath.Join(worktreePath, lf.Name)
		if _, err := os.Stat(lockfilePath); err == nil {
			return lf.Name
		}
	}
	return ""
}

// GetLockfileInfo returns the Lockfile info for a given lockfile name.
// Returns nil if the lockfile is not recognized.
func GetLockfileInfo(lockfileName string) *Lockfile {
	for _, lf := range lockfileOrder {
		if lf.Name == lockfileName {
			return &lf
		}
	}
	return nil
}
