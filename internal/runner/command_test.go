package runner

import (
	"strings"
	"testing"
)

func TestBuildCommand_DefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	cmd := BuildCommand("test prompt", opts)

	if cmd.Path == "" {
		t.Error("expected command path to be set")
	}

	// Default should include stream-json output format with --print and --verbose
	args := strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "--output-format stream-json") {
		t.Errorf("expected --output-format stream-json in args, got: %s", args)
	}
	if !strings.Contains(args, "--print") {
		t.Errorf("expected --print in args (required for output-format), got: %s", args)
	}
	if !strings.Contains(args, "--verbose") {
		t.Errorf("expected --verbose in args (required for stream-json), got: %s", args)
	}
}

func TestBuildCommand_WithModel(t *testing.T) {
	opts := Options{
		Model: "claude-3-5-haiku-20241022",
	}
	cmd := BuildCommand("test", opts)

	args := strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "--model claude-3-5-haiku-20241022") {
		t.Errorf("expected --model flag, got: %s", args)
	}
}

func TestBuildCommand_WithMaxTokens(t *testing.T) {
	opts := Options{
		MaxTokens: 4096,
	}
	cmd := BuildCommand("test", opts)

	args := strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "--max-tokens 4096") {
		t.Errorf("expected --max-tokens flag, got: %s", args)
	}
}

func TestBuildCommand_WithAllowedTools(t *testing.T) {
	tests := []struct {
		name     string
		tools    []string
		expected string
	}{
		{
			name:     "single tool",
			tools:    []string{"Read"},
			expected: "--allowedTools Read",
		},
		{
			name:     "multiple tools",
			tools:    []string{"Read", "Write", "Bash"},
			expected: "--allowedTools Read,Write,Bash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{
				AllowedTools: tt.tools,
			}
			cmd := BuildCommand("test", opts)

			args := strings.Join(cmd.Args, " ")
			if !strings.Contains(args, tt.expected) {
				t.Errorf("expected %q in args, got: %s", tt.expected, args)
			}
		})
	}
}

func TestBuildCommand_WithWorkDir(t *testing.T) {
	opts := Options{
		WorkDir: "/tmp/test-workspace",
	}
	cmd := BuildCommand("test", opts)

	if cmd.Dir != "/tmp/test-workspace" {
		t.Errorf("expected WorkDir to be set, got: %s", cmd.Dir)
	}
}

func TestBuildCommand_WithPrint(t *testing.T) {
	opts := Options{
		Print: true,
	}
	cmd := BuildCommand("test", opts)

	args := strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "--print") {
		t.Errorf("expected --print flag, got: %s", args)
	}
}

func TestBuildCommand_WithSystemPrompt(t *testing.T) {
	opts := Options{
		SystemPrompt: "You are a helpful assistant",
	}
	cmd := BuildCommand("test", opts)

	args := strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "--system-prompt") {
		t.Errorf("expected --system-prompt flag, got: %s", args)
	}
}

func TestBuildCommand_WithNoPermissions(t *testing.T) {
	opts := Options{
		NoPermissions: true,
	}
	cmd := BuildCommand("test", opts)

	args := strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "--dangerously-skip-permissions") {
		t.Errorf("expected --dangerously-skip-permissions flag, got: %s", args)
	}
}

func TestBuildCommand_AllFlags(t *testing.T) {
	opts := Options{
		Model:         "claude-sonnet-4-20250514",
		MaxTokens:     8192,
		AllowedTools:  []string{"Read", "Write", "Bash", "Glob"},
		WorkDir:       "/workspace",
		Print:         false,
		OutputFormat:  "json",
		SystemPrompt:  "Be helpful",
		NoPermissions: true,
	}
	cmd := BuildCommand("complex prompt", opts)

	args := strings.Join(cmd.Args, " ")

	checks := []string{
		"--print", // output-format requires print mode
		"--model claude-sonnet-4-20250514",
		"--max-tokens 8192",
		"--allowedTools Read,Write,Bash,Glob",
		"--output-format json",
		"--system-prompt",
		"--dangerously-skip-permissions",
	}

	for _, check := range checks {
		if !strings.Contains(args, check) {
			t.Errorf("expected %q in args, got: %s", check, args)
		}
	}

	// json format should NOT have --verbose (only stream-json needs it)
	if strings.Contains(args, "--verbose") {
		t.Errorf("did not expect --verbose for json format, got: %s", args)
	}

	if cmd.Dir != "/workspace" {
		t.Errorf("expected Dir to be /workspace, got: %s", cmd.Dir)
	}
}

func TestBuildCommand_EmptyOptions(t *testing.T) {
	opts := Options{}
	cmd := BuildCommand("test", opts)

	// Should just have "claude" with no extra args
	if len(cmd.Args) != 1 {
		t.Errorf("expected 1 arg (just command name), got: %v", cmd.Args)
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.OutputFormat != "stream-json" {
		t.Errorf("expected default OutputFormat to be stream-json, got: %s", opts.OutputFormat)
	}
}

func TestCommandString(t *testing.T) {
	opts := Options{
		Model:        "claude-3-5-haiku-20241022",
		OutputFormat: "stream-json",
	}
	cmd := BuildCommand("test", opts)

	cmdStr := CommandString(cmd)
	if !strings.HasPrefix(cmdStr, "claude ") {
		t.Errorf("expected command string to start with 'claude ', got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "--output-format stream-json") {
		t.Errorf("expected command string to contain output format, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "--print") {
		t.Errorf("expected command string to contain --print, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "--verbose") {
		t.Errorf("expected command string to contain --verbose for stream-json, got: %s", cmdStr)
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{100, "100"},
		{4096, "4096"},
		{-1, "-1"},
		{-42, "-42"},
		{1234567890, "1234567890"},
	}

	for _, tt := range tests {
		result := itoa(tt.input)
		if result != tt.expected {
			t.Errorf("itoa(%d) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

func TestBuildCommand_ZeroMaxTokens(t *testing.T) {
	// MaxTokens of 0 should not add the flag
	opts := Options{
		MaxTokens: 0,
	}
	cmd := BuildCommand("test", opts)

	args := strings.Join(cmd.Args, " ")
	if strings.Contains(args, "--max-tokens") {
		t.Errorf("did not expect --max-tokens flag when value is 0, got: %s", args)
	}
}

func TestBuildCommand_EmptyAllowedTools(t *testing.T) {
	// Empty slice should not add the flag
	opts := Options{
		AllowedTools: []string{},
	}
	cmd := BuildCommand("test", opts)

	args := strings.Join(cmd.Args, " ")
	if strings.Contains(args, "--allowedTools") {
		t.Errorf("did not expect --allowedTools flag when empty, got: %s", args)
	}
}
