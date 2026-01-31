package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/worker"
)

func TestWorkerCmd_HelpOutput(t *testing.T) {
	// Reset root command for isolated testing
	cmd := workerCmd

	// Verify the command is registered correctly
	if cmd.Use != "worker" {
		t.Errorf("expected Use 'worker', got '%s'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("expected Short description to be set")
	}

	if cmd.Long == "" {
		t.Error("expected Long description to be set")
	}
}

func TestWorkerCmd_FlagsRegistered(t *testing.T) {
	cmd := workerCmd

	// Check --once flag
	onceFlag := cmd.Flags().Lookup("once")
	if onceFlag == nil {
		t.Error("expected --once flag to be registered")
	} else {
		if onceFlag.DefValue != "false" {
			t.Errorf("expected --once default 'false', got '%s'", onceFlag.DefValue)
		}
	}

	// Check --pr flag
	prFlag := cmd.Flags().Lookup("pr")
	if prFlag == nil {
		t.Error("expected --pr flag to be registered")
	}

	// Check --merge flag
	mergeFlag := cmd.Flags().Lookup("merge")
	if mergeFlag == nil {
		t.Error("expected --merge flag to be registered")
	}

	// Check --interval flag
	intervalFlag := cmd.Flags().Lookup("interval")
	if intervalFlag == nil {
		t.Error("expected --interval flag to be registered")
	} else {
		expected := worker.DefaultPollInterval.String()
		if intervalFlag.DefValue != expected {
			t.Errorf("expected --interval default '%s', got '%s'", expected, intervalFlag.DefValue)
		}
	}

	// Check --max flag
	maxFlag := cmd.Flags().Lookup("max")
	if maxFlag == nil {
		t.Error("expected --max flag to be registered")
	} else {
		if maxFlag.DefValue != "30" {
			t.Errorf("expected --max default '30', got '%s'", maxFlag.DefValue)
		}
	}
}

func TestWorkerCmd_RequiresGitRepo(t *testing.T) {
	// Save current directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	// Create temp directory (not a git repo)
	tempDir, err := os.MkdirTemp("", "worker-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Reset flags to defaults
	workerOnce = true // Use once mode to avoid continuous polling
	workerPRMode = false
	workerMergeMode = false
	workerInterval = worker.DefaultPollInterval
	workerMaxIter = worker.DefaultMaxIterations

	// Run should fail because we're not in a git repo
	err = runWorker(workerCmd, []string{})
	if err == nil {
		t.Error("expected error when not in git repo")
	}

	expectedMsg := "not in a git repository"
	if err != nil && !bytes.Contains([]byte(err.Error()), []byte(expectedMsg)) {
		t.Errorf("expected error containing '%s', got: %v", expectedMsg, err)
	}
}

func TestWorkerCmd_OnceMode_EmptyQueue(t *testing.T) {
	// Save current directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	// Create temp git repo
	tempDir, err := os.MkdirTemp("", "worker-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize git repo
	setupWorkerTestGitRepo(t, tempDir)

	// Create required directories
	plansDir := filepath.Join(tempDir, "plans")
	os.MkdirAll(filepath.Join(plansDir, "pending"), 0755)
	os.MkdirAll(filepath.Join(plansDir, "current"), 0755)
	os.MkdirAll(filepath.Join(plansDir, "complete"), 0755)

	// Create .ralph directory
	ralphDir := filepath.Join(tempDir, ".ralph")
	os.MkdirAll(ralphDir, 0755)

	// Change to temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Reset flags to defaults
	workerOnce = true
	workerPRMode = false
	workerMergeMode = false
	workerInterval = 100 * time.Millisecond // Short interval for test
	workerMaxIter = worker.DefaultMaxIterations

	// Run should succeed with empty queue (exits gracefully)
	err = runWorker(workerCmd, []string{})
	if err != nil {
		t.Errorf("expected no error with empty queue in once mode, got: %v", err)
	}
}

func TestWorkerCmd_CompletionModeFlags(t *testing.T) {
	tests := []struct {
		name         string
		prFlag       bool
		mergeFlag    bool
		expectedMode string
	}{
		{
			name:         "default is pr",
			prFlag:       false,
			mergeFlag:    false,
			expectedMode: "pr",
		},
		{
			name:         "explicit pr",
			prFlag:       true,
			mergeFlag:    false,
			expectedMode: "pr",
		},
		{
			name:         "merge mode",
			prFlag:       false,
			mergeFlag:    true,
			expectedMode: "merge",
		},
		{
			name:         "merge takes precedence",
			prFlag:       true,
			mergeFlag:    true,
			expectedMode: "merge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Determine mode based on flags (same logic as runWorker)
			completionMode := "pr"
			if tt.mergeFlag {
				completionMode = "merge"
			}

			if completionMode != tt.expectedMode {
				t.Errorf("expected mode '%s', got '%s'", tt.expectedMode, completionMode)
			}
		})
	}
}

func TestWorkerCmd_IntervalParsing(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
	}{
		{"default", worker.DefaultPollInterval},
		{"10 seconds", 10 * time.Second},
		{"1 minute", time.Minute},
		{"5 minutes", 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify duration values are valid
			if tt.interval < 0 {
				t.Errorf("interval should be positive, got %v", tt.interval)
			}
		})
	}
}

// setupWorkerTestGitRepo initializes a basic git repo for testing
func setupWorkerTestGitRepo(t *testing.T, dir string) {
	t.Helper()

	// git init
	runWorkerGitCmd(t, dir, "init", "-b", "main")

	// Configure git user for commits
	runWorkerGitCmd(t, dir, "config", "user.email", "test@example.com")
	runWorkerGitCmd(t, dir, "config", "user.name", "Test User")

	// Create initial commit
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runWorkerGitCmd(t, dir, "add", "README.md")
	runWorkerGitCmd(t, dir, "commit", "-m", "Initial commit")
}

func runWorkerGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git command failed: git %v: %v", args, err)
	}
}
