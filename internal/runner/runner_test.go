package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewCLIRunner(t *testing.T) {
	runner := NewCLIRunner()

	if runner == nil {
		t.Fatal("NewCLIRunner returned nil")
	}
	if runner.retrier == nil {
		t.Error("retrier should not be nil")
	}
	if runner.terminationGracePeriod != 5*time.Second {
		t.Errorf("terminationGracePeriod = %v, want %v", runner.terminationGracePeriod, 5*time.Second)
	}
}

func TestNewCLIRunnerWithRetrier(t *testing.T) {
	config := RetryConfig{
		MaxRetries:   3,
		InitialDelay: time.Second,
		MaxDelay:     10 * time.Second,
	}
	retrier := NewRetrier(config)
	runner := NewCLIRunnerWithRetrier(retrier)

	if runner == nil {
		t.Fatal("NewCLIRunnerWithRetrier returned nil")
	}
	if runner.retrier != retrier {
		t.Error("retrier should be the provided retrier")
	}
}

func TestContainsCompletionMarker(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "exact marker",
			output: "<promise>COMPLETE</promise>",
			want:   true,
		},
		{
			name:   "marker in middle of text",
			output: "Some text before <promise>COMPLETE</promise> and after",
			want:   true,
		},
		{
			name:   "marker at end",
			output: "Task completed successfully.\n<promise>COMPLETE</promise>",
			want:   true,
		},
		{
			name:   "no marker",
			output: "Just regular output",
			want:   false,
		},
		{
			name:   "partial marker - start only",
			output: "<promise>COMPLETE",
			want:   false,
		},
		{
			name:   "partial marker - end only",
			output: "COMPLETE</promise>",
			want:   false,
		},
		{
			name:   "lowercase marker - should not match",
			output: "<promise>complete</promise>",
			want:   false,
		},
		{
			name:   "mentions marker in text - should not match",
			output: "Use <promise>COMPLETE</promise> when you're done",
			want:   true, // This actually does contain the marker
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name:   "marker with whitespace inside - should not match",
			output: "<promise> COMPLETE </promise>",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsCompletionMarker(tt.output)
			if got != tt.want {
				t.Errorf("containsCompletionMarker(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestIsRetryableExitError(t *testing.T) {
	tests := []struct {
		name   string
		code   int
		stderr string
		want   bool
	}{
		{
			name:   "rate limit error",
			code:   1,
			stderr: "Error: rate limit exceeded",
			want:   true,
		},
		{
			name:   "too many requests",
			code:   1,
			stderr: "too many requests, please retry later",
			want:   true,
		},
		{
			name:   "network error",
			code:   1,
			stderr: "network error: connection refused",
			want:   true,
		},
		{
			name:   "connection timeout",
			code:   1,
			stderr: "connection timeout",
			want:   true,
		},
		{
			name:   "500 server error",
			code:   1,
			stderr: "API returned 500",
			want:   true,
		},
		{
			name:   "502 bad gateway",
			code:   1,
			stderr: "502 Bad Gateway",
			want:   true,
		},
		{
			name:   "503 service unavailable",
			code:   1,
			stderr: "503 Service Unavailable",
			want:   true,
		},
		{
			name:   "504 gateway timeout",
			code:   1,
			stderr: "504 Gateway Timeout",
			want:   true,
		},
		{
			name:   "exit code 1 with empty stderr",
			code:   1,
			stderr: "",
			want:   true,
		},
		{
			name:   "exit code 2 with empty stderr - not retryable",
			code:   2,
			stderr: "",
			want:   false,
		},
		{
			name:   "invalid argument",
			code:   1,
			stderr: "invalid argument: model not found",
			want:   false,
		},
		{
			name:   "auth error",
			code:   1,
			stderr: "authentication failed",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableExitError(tt.code, tt.stderr)
			if got != tt.want {
				t.Errorf("isRetryableExitError(%d, %q) = %v, want %v", tt.code, tt.stderr, got, tt.want)
			}
		})
	}
}

func TestResult_Fields(t *testing.T) {
	result := &Result{
		Output:      "raw output",
		TextContent: "text content",
		Duration:    5 * time.Second,
		Attempts:    3,
		IsComplete:  true,
		Blocker:     &Blocker{Content: "blocker content"},
	}

	if result.Output != "raw output" {
		t.Errorf("Output = %q, want %q", result.Output, "raw output")
	}
	if result.TextContent != "text content" {
		t.Errorf("TextContent = %q, want %q", result.TextContent, "text content")
	}
	if result.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want %v", result.Duration, 5*time.Second)
	}
	if result.Attempts != 3 {
		t.Errorf("Attempts = %d, want %d", result.Attempts, 3)
	}
	if !result.IsComplete {
		t.Error("IsComplete = false, want true")
	}
	if result.Blocker == nil || result.Blocker.Content != "blocker content" {
		t.Error("Blocker not set correctly")
	}
}

func TestBlocker_Fields(t *testing.T) {
	blocker := &Blocker{
		Content:     "full content",
		Description: "something is blocked",
		Action:      "do this action",
		Resume:      "then continue",
		Hash:        "abc12345",
	}

	if blocker.Content != "full content" {
		t.Errorf("Content = %q, want %q", blocker.Content, "full content")
	}
	if blocker.Description != "something is blocked" {
		t.Errorf("Description = %q, want %q", blocker.Description, "something is blocked")
	}
	if blocker.Action != "do this action" {
		t.Errorf("Action = %q, want %q", blocker.Action, "do this action")
	}
	if blocker.Resume != "then continue" {
		t.Errorf("Resume = %q, want %q", blocker.Resume, "then continue")
	}
	if blocker.Hash != "abc12345" {
		t.Errorf("Hash = %q, want %q", blocker.Hash, "abc12345")
	}
}

// Integration tests using mock scripts
// These tests require the mock scripts to be present in testdata/

func TestCLIRunner_RunWithMockScript_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Get absolute path to mock script
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	mockScript := filepath.Join(wd, "testdata", "mock-claude-success.sh")

	// Skip if mock script doesn't exist
	if _, err := os.Stat(mockScript); os.IsNotExist(err) {
		t.Skip("mock script not found")
	}

	// Create a runner that uses our mock script
	runner := NewCLIRunner()

	// We need to create a custom BuildCommand that uses our mock
	// For this test, we'll directly test the components
	t.Run("mock script produces expected output", func(t *testing.T) {
		cmd := exec.Command(mockScript)
		cmd.Stdin = strings.NewReader("test prompt")

		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("mock script failed: %v", err)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "<promise>COMPLETE</promise>") {
			t.Errorf("output should contain completion marker, got: %s", outputStr)
		}
	})

	// Test that our parser works with the mock output
	t.Run("parser extracts completion marker", func(t *testing.T) {
		cmd := exec.Command(mockScript)
		cmd.Stdin = strings.NewReader("test prompt")

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}

		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}

		parser := NewStreamParser()
		if err := parser.ParseReader(stdout); err != nil {
			t.Fatalf("ParseReader failed: %v", err)
		}

		cmd.Wait()

		if !containsCompletionMarker(parser.TextContent()) {
			t.Errorf("parser should detect completion marker in: %s", parser.TextContent())
		}
	})

	_ = runner // Verify runner type
}

func TestCLIRunner_RunWithMockScript_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Get absolute path to mock script
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	mockScript := filepath.Join(wd, "testdata", "mock-claude-timeout.sh")

	// Skip if mock script doesn't exist
	if _, err := os.Stat(mockScript); os.IsNotExist(err) {
		t.Skip("mock script not found")
	}

	t.Run("process is terminated on context timeout", func(t *testing.T) {
		// Create a context with a short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		cmd := exec.CommandContext(ctx, mockScript)
		cmd.Stdin = strings.NewReader("test prompt")

		err := cmd.Run()
		if err == nil {
			t.Error("expected error due to timeout")
		}
		if ctx.Err() != context.DeadlineExceeded {
			t.Errorf("expected deadline exceeded, got: %v", ctx.Err())
		}
	})
}

func TestCLIRunner_RunWithMockScript_Error(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Get absolute path to mock script
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	mockScript := filepath.Join(wd, "testdata", "mock-claude-error.sh")

	// Skip if mock script doesn't exist
	if _, err := os.Stat(mockScript); os.IsNotExist(err) {
		t.Skip("mock script not found")
	}

	t.Run("error script returns expected error", func(t *testing.T) {
		cmd := exec.Command(mockScript)
		cmd.Stdin = strings.NewReader("test prompt")

		_, err := cmd.Output()
		if err == nil {
			t.Error("expected error from mock script")
		}

		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("expected ExitError, got %T: %v", err, err)
		}

		stderr := string(exitErr.Stderr)
		if !isRetryableExitError(exitErr.ExitCode(), stderr) {
			t.Error("rate limit error should be retryable")
		}
	})
}

func TestCLIRunner_TerminateProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runner := &CLIRunner{
		terminationGracePeriod: 100 * time.Millisecond,
	}

	t.Run("terminate nil command", func(t *testing.T) {
		err := runner.terminateProcess(nil)
		if err != nil {
			t.Errorf("terminateProcess(nil) = %v, want nil", err)
		}
	})

	t.Run("terminate command with nil process", func(t *testing.T) {
		cmd := &exec.Cmd{}
		err := runner.terminateProcess(cmd)
		if err != nil {
			t.Errorf("terminateProcess(cmd with nil process) = %v, want nil", err)
		}
	})

	t.Run("terminate sleeping process", func(t *testing.T) {
		// Start a sleep command
		cmd := exec.Command("sleep", "60")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}

		// Terminate it
		start := time.Now()
		err := runner.terminateProcess(cmd)
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("terminateProcess failed: %v", err)
		}

		// Should terminate quickly (within grace period)
		if elapsed > 2*time.Second {
			t.Errorf("termination took too long: %v", elapsed)
		}

		// Wait for process to fully exit
		cmd.Wait()
	})
}

// Test that Runner interface is satisfied
func TestRunnerInterface(t *testing.T) {
	var _ Runner = (*CLIRunner)(nil)
}

// Test concurrent safety
func TestCLIRunner_ConcurrentAccess(t *testing.T) {
	runner := NewCLIRunner()

	// Test that currentCmd field is protected
	done := make(chan bool)

	go func() {
		runner.mu.Lock()
		runner.currentCmd = nil
		runner.mu.Unlock()
		done <- true
	}()

	go func() {
		runner.mu.Lock()
		_ = runner.currentCmd
		runner.mu.Unlock()
		done <- true
	}()

	<-done
	<-done
}
