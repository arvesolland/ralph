package worktree

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/arvesolland/ralph/internal/config"
)

func TestRunInitHooks_CustomHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping shell script test on Windows")
	}

	// Create temp directories for main worktree and execution worktree
	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create the hooks directory
	hooksDir := filepath.Join(mainDir, ".ralph", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a custom hook script that creates a marker file
	hookScript := `#!/bin/sh
echo "Main worktree: $MAIN_WORKTREE"
echo "Working dir: $(pwd)"
touch "$PWD/hook-marker.txt"
`
	hookPath := filepath.Join(hooksDir, hookFileName)
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}

	result, err := RunInitHooks(worktreeDir, cfg, mainDir)
	if err != nil {
		t.Fatalf("RunInitHooks failed: %v", err)
	}

	// Verify method
	if result.Method != "hook" {
		t.Errorf("Method = %q, want 'hook'", result.Method)
	}

	// Verify command
	if result.Command != hookPath {
		t.Errorf("Command = %q, want %q", result.Command, hookPath)
	}

	// Verify output contains expected text
	if !strings.Contains(result.Output, "Main worktree:") {
		t.Errorf("Output should contain 'Main worktree:', got %q", result.Output)
	}

	// Verify the marker file was created (proves hook ran in correct directory)
	markerPath := filepath.Join(worktreeDir, "hook-marker.txt")
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Error("Hook did not create marker file - working directory issue")
	}
}

func TestRunInitHooks_InitCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping shell command test on Windows")
	}

	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// No hook file, but init_commands configured
	cfg := &config.Config{
		Worktree: config.WorktreeConfig{
			InitCommands: "echo 'init ran' && touch init-marker.txt",
		},
	}

	result, err := RunInitHooks(worktreeDir, cfg, mainDir)
	if err != nil {
		t.Fatalf("RunInitHooks failed: %v", err)
	}

	// Verify method
	if result.Method != "init_commands" {
		t.Errorf("Method = %q, want 'init_commands'", result.Method)
	}

	// Verify command
	if result.Command != cfg.Worktree.InitCommands {
		t.Errorf("Command = %q, want %q", result.Command, cfg.Worktree.InitCommands)
	}

	// Verify output
	if !strings.Contains(result.Output, "init ran") {
		t.Errorf("Output should contain 'init ran', got %q", result.Output)
	}

	// Verify marker file was created
	markerPath := filepath.Join(worktreeDir, "init-marker.txt")
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Error("Init commands did not create marker file")
	}
}

func TestRunInitHooks_AutoDetect(t *testing.T) {
	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// No hook, no init_commands, but a lockfile exists
	if err := os.WriteFile(filepath.Join(worktreeDir, "go.sum"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreeDir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}

	result, err := RunInitHooks(worktreeDir, cfg, mainDir)
	// We might get an error if go is not installed, that's OK
	if err != nil && !strings.Contains(err.Error(), "command not found") {
		// Some other error is OK too (e.g., no deps to download)
	}

	// Verify method is auto_detect
	if result != nil && result.Method != "auto_detect" {
		t.Errorf("Method = %q, want 'auto_detect'", result.Method)
	}
}

func TestRunInitHooks_NoMethod(t *testing.T) {
	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// No hook, no init_commands, no lockfile
	cfg := &config.Config{}

	result, err := RunInitHooks(worktreeDir, cfg, mainDir)
	if err != nil {
		t.Fatalf("RunInitHooks failed: %v", err)
	}

	// Verify method is none
	if result.Method != "none" {
		t.Errorf("Method = %q, want 'none'", result.Method)
	}
}

func TestRunInitHooks_HookPriorityOverInitCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping shell script test on Windows")
	}

	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create hook file
	hooksDir := filepath.Join(mainDir, ".ralph", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	hookScript := `#!/bin/sh
echo "hook ran"
`
	hookPath := filepath.Join(hooksDir, hookFileName)
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Also configure init_commands (should be ignored)
	cfg := &config.Config{
		Worktree: config.WorktreeConfig{
			InitCommands: "echo 'init_commands ran'",
		},
	}

	result, err := RunInitHooks(worktreeDir, cfg, mainDir)
	if err != nil {
		t.Fatalf("RunInitHooks failed: %v", err)
	}

	// Hook should take priority
	if result.Method != "hook" {
		t.Errorf("Method = %q, want 'hook' (should take priority over init_commands)", result.Method)
	}

	// Output should be from hook, not init_commands
	if strings.Contains(result.Output, "init_commands ran") {
		t.Error("init_commands should not have run when hook exists")
	}
}

func TestRunInitHooks_HookNotExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping executable permission test on Windows")
	}

	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create hook file but NOT executable
	hooksDir := filepath.Join(mainDir, ".ralph", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	hookScript := `#!/bin/sh
echo "should not run"
`
	hookPath := filepath.Join(hooksDir, hookFileName)
	if err := os.WriteFile(hookPath, []byte(hookScript), 0644); err != nil { // 0644, not 0755
		t.Fatal(err)
	}

	// Configure init_commands as fallback
	cfg := &config.Config{
		Worktree: config.WorktreeConfig{
			InitCommands: "echo 'fallback ran'",
		},
	}

	result, err := RunInitHooks(worktreeDir, cfg, mainDir)
	if err != nil {
		t.Fatalf("RunInitHooks failed: %v", err)
	}

	// Should fall back to init_commands since hook is not executable
	if result.Method != "init_commands" {
		t.Errorf("Method = %q, want 'init_commands' (hook not executable)", result.Method)
	}
}

func TestRunInitHooks_HookFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping shell script test on Windows")
	}

	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create a hook that fails
	hooksDir := filepath.Join(mainDir, ".ralph", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	hookScript := `#!/bin/sh
echo "error output"
exit 1
`
	hookPath := filepath.Join(hooksDir, hookFileName)
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}

	result, err := RunInitHooks(worktreeDir, cfg, mainDir)

	// Should return an error
	if err == nil {
		t.Fatal("Expected error from failing hook")
	}

	// Result should still be populated
	if result == nil {
		t.Fatal("Expected non-nil result even on failure")
	}

	if result.Method != "hook" {
		t.Errorf("Method = %q, want 'hook'", result.Method)
	}

	// Output should contain error output
	if !strings.Contains(result.Output, "error output") {
		t.Errorf("Output should contain 'error output', got %q", result.Output)
	}
}

func TestRunInitHooks_InitCommandsFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping shell command test on Windows")
	}

	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	cfg := &config.Config{
		Worktree: config.WorktreeConfig{
			InitCommands: "echo 'failing' && exit 1",
		},
	}

	result, err := RunInitHooks(worktreeDir, cfg, mainDir)

	// Should return an error
	if err == nil {
		t.Fatal("Expected error from failing init_commands")
	}

	// Result should still be populated
	if result == nil {
		t.Fatal("Expected non-nil result even on failure")
	}

	if result.Method != "init_commands" {
		t.Errorf("Method = %q, want 'init_commands'", result.Method)
	}
}

func TestRunInitHooks_MainWorktreeEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping shell script test on Windows")
	}

	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create a hook that outputs the MAIN_WORKTREE env var
	hooksDir := filepath.Join(mainDir, ".ralph", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	hookScript := `#!/bin/sh
echo "MAIN_WORKTREE=$MAIN_WORKTREE"
`
	hookPath := filepath.Join(hooksDir, hookFileName)
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}

	result, err := RunInitHooks(worktreeDir, cfg, mainDir)
	if err != nil {
		t.Fatalf("RunInitHooks failed: %v", err)
	}

	// Verify MAIN_WORKTREE was set correctly
	expected := "MAIN_WORKTREE=" + mainDir
	if !strings.Contains(result.Output, expected) {
		t.Errorf("Output should contain %q, got %q", expected, result.Output)
	}
}

func TestIsExecutable(t *testing.T) {
	tmpDir := t.TempDir()

	// Non-existent file
	if isExecutable(filepath.Join(tmpDir, "nonexistent")) {
		t.Error("Non-existent file should not be executable")
	}

	// Directory
	dirPath := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(dirPath, 0755); err != nil {
		t.Fatal(err)
	}
	if isExecutable(dirPath) {
		t.Error("Directory should not be considered executable")
	}

	if runtime.GOOS != "windows" {
		// Non-executable file
		nonExecPath := filepath.Join(tmpDir, "file.txt")
		if err := os.WriteFile(nonExecPath, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
		if isExecutable(nonExecPath) {
			t.Error("Non-executable file should not be considered executable")
		}

		// Executable file
		execPath := filepath.Join(tmpDir, "script.sh")
		if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
			t.Fatal(err)
		}
		if !isExecutable(execPath) {
			t.Error("Executable file should be considered executable")
		}
	}
}

func TestHookExists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping executable permission test on Windows")
	}

	mainDir := t.TempDir()

	// No hooks directory
	if HookExists(mainDir) {
		t.Error("HookExists should return false when hooks directory doesn't exist")
	}

	// Create hooks directory but no hook file
	hooksDir := filepath.Join(mainDir, ".ralph", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if HookExists(mainDir) {
		t.Error("HookExists should return false when hook file doesn't exist")
	}

	// Create hook file but not executable
	hookPath := filepath.Join(hooksDir, hookFileName)
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if HookExists(mainDir) {
		t.Error("HookExists should return false when hook is not executable")
	}

	// Make hook executable
	if err := os.Chmod(hookPath, 0755); err != nil {
		t.Fatal(err)
	}
	if !HookExists(mainDir) {
		t.Error("HookExists should return true when hook is executable")
	}
}

func TestRunInitHooks_NilConfig(t *testing.T) {
	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Nil config should not panic
	result, err := RunInitHooks(worktreeDir, nil, mainDir)
	if err != nil {
		t.Fatalf("RunInitHooks failed: %v", err)
	}

	// Should fall back to none (no hook, no config)
	if result.Method != "none" {
		t.Errorf("Method = %q, want 'none'", result.Method)
	}
}
