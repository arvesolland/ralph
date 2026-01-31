package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/arvesolland/ralph/internal/log"
)

// Runner defines the interface for Claude CLI execution.
type Runner interface {
	// Run executes Claude with the given prompt and options.
	// The context controls timeout and cancellation.
	Run(ctx context.Context, prompt string, opts Options) (*Result, error)
}

// Result holds the result of a Claude CLI execution.
type Result struct {
	// Output is the raw output from Claude CLI
	Output string

	// TextContent is the extracted text content from the stream
	TextContent string

	// Duration is how long the execution took
	Duration time.Duration

	// Attempts is the number of attempts (including retries)
	Attempts int

	// IsComplete is true if output contains <promise>COMPLETE</promise>
	IsComplete bool

	// Blocker holds extracted blocker information if present
	Blocker *Blocker
}

// Blocker represents extracted blocker information from Claude output.
// Used to signal that human input is required before continuing.
type Blocker struct {
	// Content is the raw content between <blocker> tags
	Content string
	// Description is the blocker description (first part or Description: field)
	Description string
	// Action is what the human should do (Action: field)
	Action string
	// Resume is what happens after the blocker is resolved (Resume: field)
	Resume string
	// Hash is the first 8 characters of MD5 of content (for deduplication)
	Hash string
}

// CLIRunner implements Runner by executing the claude CLI.
type CLIRunner struct {
	retrier *Retrier

	// terminationGracePeriod is how long to wait after SIGTERM before SIGKILL
	terminationGracePeriod time.Duration

	// mu protects currentCmd
	mu         sync.Mutex
	currentCmd *exec.Cmd
}

// NewCLIRunner creates a new CLIRunner with default settings.
func NewCLIRunner() *CLIRunner {
	return &CLIRunner{
		retrier:                NewRetrier(DefaultRetryConfig()),
		terminationGracePeriod: 5 * time.Second,
	}
}

// NewCLIRunnerWithRetrier creates a new CLIRunner with a custom retrier.
func NewCLIRunnerWithRetrier(retrier *Retrier) *CLIRunner {
	return &CLIRunner{
		retrier:                retrier,
		terminationGracePeriod: 5 * time.Second,
	}
}

// Run executes Claude with the given prompt and options.
// It handles timeout via context, streams output in real-time,
// and retries on transient failures.
func (r *CLIRunner) Run(ctx context.Context, prompt string, opts Options) (*Result, error) {
	start := time.Now()
	var lastResult *Result
	var attempts int

	err := r.retrier.DoWithContext(ctx, func() error {
		attempts++
		result, err := r.runOnce(ctx, prompt, opts)
		if result != nil {
			lastResult = result
		}
		return err
	})

	if lastResult == nil {
		lastResult = &Result{}
	}
	lastResult.Duration = time.Since(start)
	lastResult.Attempts = attempts

	return lastResult, err
}

// runOnce executes a single Claude CLI invocation.
func (r *CLIRunner) runOnce(ctx context.Context, prompt string, opts Options) (*Result, error) {
	// Build the command
	cmd := BuildCommand(prompt, opts)
	cmd.Stdin = strings.NewReader(prompt)

	// Set up pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Track the current command for termination
	r.mu.Lock()
	r.currentCmd = cmd
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.currentCmd = nil
		r.mu.Unlock()
	}()

	// Start the command
	log.Debug("Starting Claude CLI: %s", CommandString(cmd))
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start claude: %w", err)
	}

	// Set up streaming parser
	parser := NewStreamParser()
	parser.OnText = func(text string) {
		// Real-time output to user
		fmt.Print(text)
	}

	// Collect stderr in background
	var stderrBuf strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		io.Copy(&stderrBuf, stderr)
		close(stderrDone)
	}()

	// Stream and parse stdout
	streamDone := make(chan error, 1)
	go func() {
		err := parser.ParseReader(stdout)
		streamDone <- err
	}()

	// Wait for completion or context cancellation
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	var waitErr error
	select {
	case <-ctx.Done():
		// Context cancelled/timeout - terminate the process
		log.Warn("Context cancelled, terminating Claude process")
		if termErr := r.terminateProcess(cmd); termErr != nil {
			log.Error("Failed to terminate process: %v", termErr)
		}
		// Wait for the process to actually exit
		waitErr = <-waitDone
		if ctx.Err() == context.DeadlineExceeded {
			return nil, context.DeadlineExceeded
		}
		return nil, ctx.Err()

	case waitErr = <-waitDone:
		// Process finished normally
	}

	// Wait for stream parsing to complete
	<-streamDone
	<-stderrDone

	// Build result
	result := &Result{
		Output:      parser.FullOutput(),
		TextContent: parser.TextContent(),
	}

	// Check for completion marker
	result.IsComplete = containsCompletionMarker(result.TextContent)

	// Extract blocker if present
	result.Blocker = ExtractBlocker(result.TextContent)

	// Check exit status
	if waitErr != nil {
		// Check if it's just a non-zero exit (Claude CLI returns non-zero on some errors)
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			stderrStr := stderrBuf.String()
			log.Debug("Claude exited with code %d, stderr: %s", exitErr.ExitCode(), stderrStr)

			// Determine if this is a retryable error
			if isRetryableExitError(exitErr.ExitCode(), stderrStr) {
				return result, fmt.Errorf("claude exited with code %d: %s", exitErr.ExitCode(), stderrStr)
			}

			// Non-retryable exit error
			return result, WrapNonRetryable(fmt.Errorf("claude exited with code %d: %s", exitErr.ExitCode(), stderrStr))
		}
		return result, waitErr
	}

	return result, nil
}

// terminateProcess sends SIGTERM, waits for grace period, then SIGKILL if needed.
func (r *CLIRunner) terminateProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	log.Debug("Sending SIGTERM to process %d", pid)

	// Send SIGTERM first
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Process may have already exited
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for process to exit or grace period to elapse
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Debug("Process %d terminated gracefully", pid)
		return nil
	case <-time.After(r.terminationGracePeriod):
		log.Warn("Process %d did not terminate within %v, sending SIGKILL", pid, r.terminationGracePeriod)
	}

	// Send SIGKILL
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return fmt.Errorf("failed to send SIGKILL: %w", err)
	}

	return nil
}

// containsCompletionMarker checks if the output contains the completion marker.
func containsCompletionMarker(output string) bool {
	return strings.Contains(output, "<promise>COMPLETE</promise>")
}

// isRetryableExitError determines if an exit code indicates a retryable error.
func isRetryableExitError(code int, stderr string) bool {
	stderrLower := strings.ToLower(stderr)

	// Rate limiting
	if strings.Contains(stderrLower, "rate limit") ||
		strings.Contains(stderrLower, "too many requests") {
		return true
	}

	// Network/connection issues
	if strings.Contains(stderrLower, "network") ||
		strings.Contains(stderrLower, "connection") ||
		strings.Contains(stderrLower, "timeout") {
		return true
	}

	// Server errors
	if strings.Contains(stderrLower, "500") ||
		strings.Contains(stderrLower, "502") ||
		strings.Contains(stderrLower, "503") ||
		strings.Contains(stderrLower, "504") {
		return true
	}

	// Exit code 1 with empty stderr might be transient
	if code == 1 && stderr == "" {
		return true
	}

	return false
}
