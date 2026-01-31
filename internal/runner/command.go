// Package runner handles Claude CLI execution.
package runner

import (
	"os/exec"
	"strings"
)

// Options configures Claude CLI execution.
type Options struct {
	// Model specifies the model to use (e.g., "claude-sonnet-4-20250514", "claude-3-5-haiku-20241022")
	Model string

	// MaxTokens limits the response length
	MaxTokens int

	// AllowedTools is a list of tools the agent can use
	AllowedTools []string

	// WorkDir is the working directory for command execution
	WorkDir string

	// Print outputs the prompt that would be sent (dry-run mode)
	Print bool

	// OutputFormat specifies the output format (e.g., "stream-json", "json", "text")
	OutputFormat string

	// SystemPrompt is an additional system prompt to prepend
	SystemPrompt string

	// NoPermissions runs without permission prompts (dangerous, use carefully)
	NoPermissions bool

	// Timeout in seconds for the command (0 = no timeout)
	Timeout int
}

// DefaultOptions returns options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		OutputFormat: "stream-json",
	}
}

// BuildCommand creates an exec.Cmd for the claude CLI with the given options.
// The prompt is passed via stdin to avoid shell escaping issues.
// The returned command has Stdin set to nil - the caller should set it to
// a reader containing the prompt.
func BuildCommand(prompt string, opts Options) *exec.Cmd {
	args := buildArgs(opts)

	cmd := exec.Command("claude", args...)

	// Set working directory if specified
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	// Note: Prompt is passed via stdin by the caller
	// This avoids shell escaping issues with complex prompts
	// The caller should do: cmd.Stdin = strings.NewReader(prompt)

	return cmd
}

// buildArgs constructs the argument list for the claude command.
func buildArgs(opts Options) []string {
	var args []string

	// Print mode - required when using --output-format
	// Also add --print explicitly if requested
	if opts.OutputFormat != "" || opts.Print {
		args = append(args, "--print")
	}

	// Output format (requires --print mode)
	if opts.OutputFormat != "" {
		args = append(args, "--output-format", opts.OutputFormat)
		// stream-json requires --verbose flag
		if opts.OutputFormat == "stream-json" {
			args = append(args, "--verbose")
		}
	}

	// Model selection
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	// Max tokens limit
	if opts.MaxTokens > 0 {
		args = append(args, "--max-tokens", itoa(opts.MaxTokens))
	}

	// Allowed tools (comma-separated)
	if len(opts.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(opts.AllowedTools, ","))
	}

	// System prompt
	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}

	// No permissions mode (skip prompts)
	if opts.NoPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	return args
}

// itoa converts an integer to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	// Max int64 has 19 digits
	var buf [20]byte
	i := len(buf)

	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}

	if negative {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}

// CommandString returns the command as a string for logging/debugging.
func CommandString(cmd *exec.Cmd) string {
	return strings.Join(cmd.Args, " ")
}
