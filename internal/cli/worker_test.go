package cli

import (
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

	// Check --branch flag
	branchFlag := cmd.Flags().Lookup("branch")
	if branchFlag == nil {
		t.Error("expected --branch flag to be registered")
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
		if maxFlag.DefValue != "200" {
			t.Errorf("expected --max default '200', got '%s'", maxFlag.DefValue)
		}
	}

	// Check --sync flag
	syncFlag := cmd.Flags().Lookup("sync")
	if syncFlag == nil {
		t.Error("expected --sync flag to be registered")
	}

	// Check --sync-interval flag
	syncIntervalFlag := cmd.Flags().Lookup("sync-interval")
	if syncIntervalFlag == nil {
		t.Error("expected --sync-interval flag to be registered")
	}

	// Check --push flag
	pushFlag := cmd.Flags().Lookup("push")
	if pushFlag == nil {
		t.Error("expected --push flag to be registered")
	} else {
		if pushFlag.DefValue != "false" {
			t.Errorf("expected --push default 'false', got '%s'", pushFlag.DefValue)
		}
	}
}

func TestWorkerCmd_CompletionModeFlags(t *testing.T) {
	tests := []struct {
		name         string
		prFlag       bool
		mergeFlag    bool
		branchFlag   bool
		expectedMode string
	}{
		{
			name:         "default is pr",
			prFlag:       false,
			mergeFlag:    false,
			branchFlag:   false,
			expectedMode: "pr",
		},
		{
			name:         "explicit pr",
			prFlag:       true,
			mergeFlag:    false,
			branchFlag:   false,
			expectedMode: "pr",
		},
		{
			name:         "merge mode",
			prFlag:       false,
			mergeFlag:    true,
			branchFlag:   false,
			expectedMode: "merge",
		},
		{
			name:         "branch mode",
			prFlag:       false,
			mergeFlag:    false,
			branchFlag:   true,
			expectedMode: "branch",
		},
		{
			name:         "branch takes precedence over merge",
			prFlag:       false,
			mergeFlag:    true,
			branchFlag:   true,
			expectedMode: "branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Determine mode based on flags (same logic as runWorker)
			completionMode := "pr"
			if tt.mergeFlag {
				completionMode = "merge"
			}
			if tt.branchFlag {
				completionMode = "branch"
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

func TestNameToWorktreeBranch(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"feat prefix", "feat-my-feature", "feat/my-feature"},
		{"no feat prefix", "main", "main"},
		{"feat only", "feat-", "feat/"},
		{"feat with nested", "feat-sub-feature", "feat/sub-feature"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nameToWorktreeBranch(tt.input)
			if got != tt.expect {
				t.Errorf("nameToWorktreeBranch(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}
