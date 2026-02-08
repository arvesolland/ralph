package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCmd_HelpOutput(t *testing.T) {
	if runCmd == nil {
		t.Fatal("runCmd not initialized")
	}

	if !strings.Contains(runCmd.Use, "plan") {
		t.Errorf("Use should mention plan: %s", runCmd.Use)
	}

	if !strings.Contains(runCmd.Short, "iteration loop") {
		t.Errorf("Short should mention iteration loop: %s", runCmd.Short)
	}
}

func TestRunCmd_FlagsRegistered(t *testing.T) {
	// Verify --plan flag exists
	planFlag := runCmd.Flags().Lookup("plan")
	if planFlag == nil {
		t.Fatal("--plan flag not registered")
	}

	// Verify --max flag exists
	maxFlag := runCmd.Flags().Lookup("max")
	if maxFlag == nil {
		t.Fatal("--max flag not registered")
	}
	if maxFlag.DefValue != "30" {
		t.Errorf("--max default should be 30, got %s", maxFlag.DefValue)
	}

	// Verify --completion-mode flag exists
	modeFlag := runCmd.Flags().Lookup("completion-mode")
	if modeFlag == nil {
		t.Fatal("--completion-mode flag not registered")
	}

	// Verify --push flag exists
	pushFlag := runCmd.Flags().Lookup("push")
	if pushFlag == nil {
		t.Fatal("--push flag not registered")
	}
}

func TestParsePlanID(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"42", 42, false},
		{"#42", 42, false},
		{"1", 1, false},
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parsePlanID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePlanID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parsePlanID(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func setupTestGitRepo(t *testing.T, dir string) {
	t.Helper()

	cmd := func(args ...string) {
		t.Helper()
		c := runGitCommand(dir, args...)
		if err := c.Run(); err != nil {
			t.Fatalf("git %v failed: %v", args, err)
		}
	}

	cmd("init", "-b", "main")
	cmd("config", "user.email", "test@test.com")
	cmd("config", "user.name", "Test User")

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd("add", "README.md")
	cmd("commit", "-m", "Initial commit")
}

func runGitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	return cmd
}
