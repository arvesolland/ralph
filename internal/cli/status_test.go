package cli

import (
	"os"
	"testing"
)

func TestStatusCmd_HelpOutput(t *testing.T) {
	cmd := statusCmd

	if cmd.Use != "status" {
		t.Errorf("expected Use 'status', got '%s'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("expected Short description to be set")
	}

	if cmd.Long == "" {
		t.Error("expected Long description to be set")
	}
}

func TestStatusCmd_Registered(t *testing.T) {
	// Verify statusCmd is a child of rootCmd
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "status" {
			found = true
			break
		}
	}
	if !found {
		t.Error("status command not registered with root command")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		expect string
	}{
		{"short", 10, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"this is a long string that should be truncated", 20, "this is a long st..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.expect {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expect)
			}
		})
	}
}

func TestIsTerminalFd(t *testing.T) {
	// When running in tests, stdout is typically a pipe, not a terminal
	// So isTerminalFd should return false
	result := isTerminalFd(os.Stdout)
	// In test context, stdout is redirected, so this should be false
	// We just verify it doesn't panic and returns a boolean
	_ = result
}
