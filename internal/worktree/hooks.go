// Package worktree manages git worktrees for plan execution.
package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/log"
)

// hookFileName is the name of the custom worktree initialization hook.
const hookFileName = "worktree-init"

// HookResult contains the result of running init hooks.
type HookResult struct {
	// Method describes how initialization was performed.
	// One of: "hook", "init_commands", "auto_detect", "none"
	Method string

	// Command is the command that was run (if any).
	Command string

	// Output is the combined stdout/stderr output (if any).
	Output string
}

// RunInitHooks initializes a worktree after creation by running the appropriate
// initialization method in this order of priority:
//
//  1. Custom hook: .ralph/hooks/worktree-init (if executable)
//  2. Init commands: config.worktree.init_commands (if set)
//  3. Auto-detection: DetectAndInstall (if no hook or init_commands)
//
// The mainWorktreePath is set as MAIN_WORKTREE environment variable for hooks.
func RunInitHooks(worktreePath string, cfg *config.Config, mainWorktreePath string) (*HookResult, error) {
	log.Debug("Running worktree init hooks for: %s", worktreePath)

	// 1. Check for custom hook file
	hookPath := filepath.Join(mainWorktreePath, ".ralph", "hooks", hookFileName)
	if isExecutable(hookPath) {
		log.Info("Running custom worktree-init hook...")
		output, err := runHook(hookPath, worktreePath, mainWorktreePath)
		if err != nil {
			return &HookResult{Method: "hook", Command: hookPath, Output: output}, err
		}
		log.Success("Custom hook completed successfully")
		return &HookResult{Method: "hook", Command: hookPath, Output: output}, nil
	}
	log.Debug("No executable hook found at: %s", hookPath)

	// 2. Check for init_commands in config
	if cfg != nil && cfg.Worktree.InitCommands != "" {
		log.Info("Running init commands from config...")
		output, err := runInitCommands(cfg.Worktree.InitCommands, worktreePath, mainWorktreePath)
		if err != nil {
			return &HookResult{Method: "init_commands", Command: cfg.Worktree.InitCommands, Output: output}, err
		}
		log.Success("Init commands completed successfully")
		return &HookResult{Method: "init_commands", Command: cfg.Worktree.InitCommands, Output: output}, nil
	}
	log.Debug("No init_commands configured")

	// 3. Fall back to auto-detection
	log.Debug("Falling back to dependency auto-detection...")
	result, err := DetectAndInstall(worktreePath)
	if err != nil {
		return &HookResult{Method: "auto_detect", Command: result.Command, Output: result.Output}, err
	}

	if result == nil {
		log.Debug("No lockfile found, skipping dependency installation")
		return &HookResult{Method: "none"}, nil
	}

	return &HookResult{Method: "auto_detect", Command: result.Command, Output: result.Output}, nil
}

// isExecutable checks if a file exists and is executable.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	// On Windows, check if it's a file (Windows doesn't have executable bit)
	if runtime.GOOS == "windows" {
		return !info.IsDir()
	}

	// On Unix-like systems, check the executable bit
	mode := info.Mode()
	return !info.IsDir() && (mode&0111) != 0
}

// runHook executes the custom hook script with proper environment.
func runHook(hookPath, worktreePath, mainWorktreePath string) (string, error) {
	log.Debug("Executing hook: %s", hookPath)
	log.Debug("  Working directory: %s", worktreePath)
	log.Debug("  MAIN_WORKTREE: %s", mainWorktreePath)

	cmd := exec.Command(hookPath)
	cmd.Dir = worktreePath
	cmd.Env = append(os.Environ(), "MAIN_WORKTREE="+mainWorktreePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("hook failed: %w\nOutput:\n%s", err, output)
	}

	return string(output), nil
}

// runInitCommands executes the init_commands string in a shell.
func runInitCommands(commands, worktreePath, mainWorktreePath string) (string, error) {
	log.Debug("Executing init commands: %s", commands)
	log.Debug("  Working directory: %s", worktreePath)
	log.Debug("  MAIN_WORKTREE: %s", mainWorktreePath)

	// Run commands in a shell
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", commands)
	} else {
		cmd = exec.Command("sh", "-c", commands)
	}

	cmd.Dir = worktreePath
	cmd.Env = append(os.Environ(), "MAIN_WORKTREE="+mainWorktreePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("init commands failed: %w\nOutput:\n%s", err, output)
	}

	return string(output), nil
}

// HookExists checks if the custom worktree-init hook exists and is executable.
func HookExists(mainWorktreePath string) bool {
	hookPath := filepath.Join(mainWorktreePath, ".ralph", "hooks", hookFileName)
	return isExecutable(hookPath)
}
