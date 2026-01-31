package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	// Create temp file with valid YAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
project:
  name: "Test Project"
  description: "A test project"

git:
  base_branch: "develop"

commands:
  test: "npm test"
  lint: "npm run lint"
  build: "npm run build"

slack:
  webhook_url: "https://hooks.slack.com/test"
  notify_start: true
  notify_complete: false

completion:
  mode: "merge"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify loaded values
	if cfg.Project.Name != "Test Project" {
		t.Errorf("Project.Name = %q, want %q", cfg.Project.Name, "Test Project")
	}
	if cfg.Project.Description != "A test project" {
		t.Errorf("Project.Description = %q, want %q", cfg.Project.Description, "A test project")
	}
	if cfg.Git.BaseBranch != "develop" {
		t.Errorf("Git.BaseBranch = %q, want %q", cfg.Git.BaseBranch, "develop")
	}
	if cfg.Commands.Test != "npm test" {
		t.Errorf("Commands.Test = %q, want %q", cfg.Commands.Test, "npm test")
	}
	if cfg.Slack.WebhookURL != "https://hooks.slack.com/test" {
		t.Errorf("Slack.WebhookURL = %q, want %q", cfg.Slack.WebhookURL, "https://hooks.slack.com/test")
	}
	if cfg.Completion.Mode != "merge" {
		t.Errorf("Completion.Mode = %q, want %q", cfg.Completion.Mode, "merge")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load() expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Invalid YAML (tabs in wrong places, unquoted special chars)
	content := `
project:
  name: [invalid yaml
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected error for invalid YAML, got nil")
	}
}

func TestLoad_InlineComments(t *testing.T) {
	// YAML inline comments are a key test - the bash version had bugs here
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
project:
  name: "My Project" # this is a comment
  description: "Test" # another comment

git:
  base_branch: main # default branch
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Value should be just "My Project", not "My Project # this is a comment"
	if cfg.Project.Name != "My Project" {
		t.Errorf("Project.Name = %q, want %q (inline comment not stripped)", cfg.Project.Name, "My Project")
	}
	if cfg.Project.Description != "Test" {
		t.Errorf("Project.Description = %q, want %q", cfg.Project.Description, "Test")
	}
	if cfg.Git.BaseBranch != "main" {
		t.Errorf("Git.BaseBranch = %q, want %q (inline comment not stripped)", cfg.Git.BaseBranch, "main")
	}
}

func TestLoad_NestedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
project:
  name: "Nested Test"
slack:
  webhook_url: "https://hooks.slack.com/nested"
  notify_start: true
  notify_complete: true
  notify_iteration: true
  notify_error: false
  notify_blocker: true
worktree:
  copy_env_files: ".env, .env.local"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Project.Name != "Nested Test" {
		t.Errorf("Project.Name = %q, want %q", cfg.Project.Name, "Nested Test")
	}
	if cfg.Slack.WebhookURL != "https://hooks.slack.com/nested" {
		t.Errorf("Slack.WebhookURL = %q, want %q", cfg.Slack.WebhookURL, "https://hooks.slack.com/nested")
	}
	if !cfg.Slack.NotifyIteration {
		t.Error("Slack.NotifyIteration = false, want true")
	}
	if cfg.Slack.NotifyError {
		t.Error("Slack.NotifyError = true, want false")
	}
	if cfg.Worktree.CopyEnvFiles != ".env, .env.local" {
		t.Errorf("Worktree.CopyEnvFiles = %q, want %q", cfg.Worktree.CopyEnvFiles, ".env, .env.local")
	}
}

func TestLoadWithDefaults_MissingFile(t *testing.T) {
	cfg, err := LoadWithDefaults("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("LoadWithDefaults() error = %v, want nil for missing file", err)
	}

	// Should return defaults
	defaults := Defaults()
	if cfg.Git.BaseBranch != defaults.Git.BaseBranch {
		t.Errorf("Git.BaseBranch = %q, want default %q", cfg.Git.BaseBranch, defaults.Git.BaseBranch)
	}
	if cfg.Completion.Mode != defaults.Completion.Mode {
		t.Errorf("Completion.Mode = %q, want default %q", cfg.Completion.Mode, defaults.Completion.Mode)
	}
	if cfg.Worktree.CopyEnvFiles != defaults.Worktree.CopyEnvFiles {
		t.Errorf("Worktree.CopyEnvFiles = %q, want default %q", cfg.Worktree.CopyEnvFiles, defaults.Worktree.CopyEnvFiles)
	}
}

func TestLoadWithDefaults_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Create empty file
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadWithDefaults(path)
	if err != nil {
		t.Fatalf("LoadWithDefaults() error = %v, want nil for empty file", err)
	}

	// Should return defaults
	defaults := Defaults()
	if cfg.Git.BaseBranch != defaults.Git.BaseBranch {
		t.Errorf("Git.BaseBranch = %q, want default %q", cfg.Git.BaseBranch, defaults.Git.BaseBranch)
	}
}

func TestLoadWithDefaults_PartialConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Only set some values, rest should get defaults
	content := `
project:
  name: "Partial Config"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadWithDefaults(path)
	if err != nil {
		t.Fatalf("LoadWithDefaults() error = %v", err)
	}

	// Specified value
	if cfg.Project.Name != "Partial Config" {
		t.Errorf("Project.Name = %q, want %q", cfg.Project.Name, "Partial Config")
	}

	// Defaults for unspecified values
	defaults := Defaults()
	if cfg.Git.BaseBranch != defaults.Git.BaseBranch {
		t.Errorf("Git.BaseBranch = %q, want default %q", cfg.Git.BaseBranch, defaults.Git.BaseBranch)
	}
	if cfg.Completion.Mode != defaults.Completion.Mode {
		t.Errorf("Completion.Mode = %q, want default %q", cfg.Completion.Mode, defaults.Completion.Mode)
	}
	if !cfg.Slack.NotifyStart {
		t.Error("Slack.NotifyStart should default to true")
	}
	if !cfg.Slack.NotifyComplete {
		t.Error("Slack.NotifyComplete should default to true")
	}
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	// Check critical defaults
	if cfg.Git.BaseBranch != "main" {
		t.Errorf("Git.BaseBranch = %q, want %q", cfg.Git.BaseBranch, "main")
	}
	if cfg.Completion.Mode != "pr" {
		t.Errorf("Completion.Mode = %q, want %q", cfg.Completion.Mode, "pr")
	}
	if cfg.Worktree.CopyEnvFiles != ".env" {
		t.Errorf("Worktree.CopyEnvFiles = %q, want %q", cfg.Worktree.CopyEnvFiles, ".env")
	}

	// Slack notification defaults
	if !cfg.Slack.NotifyStart {
		t.Error("Slack.NotifyStart should default to true")
	}
	if !cfg.Slack.NotifyComplete {
		t.Error("Slack.NotifyComplete should default to true")
	}
	if cfg.Slack.NotifyIteration {
		t.Error("Slack.NotifyIteration should default to false")
	}
	if !cfg.Slack.NotifyError {
		t.Error("Slack.NotifyError should default to true")
	}
	if !cfg.Slack.NotifyBlocker {
		t.Error("Slack.NotifyBlocker should default to true")
	}
}

func TestLoad_AllFieldTypes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Test all field types: string, bool
	content := `
project:
  name: "Full Config"
  description: "Testing all fields"

git:
  base_branch: "master"

commands:
  test: "go test ./..."
  lint: "golangci-lint run"
  build: "go build ./..."
  dev: "go run ./cmd/ralph"

slack:
  webhook_url: "https://hooks.slack.com/full"
  channel: "C12345678"
  bot_token: "xoxb-token"
  app_token: "xapp-token"
  global_bot: true
  notify_start: false
  notify_complete: false
  notify_iteration: true
  notify_error: false
  notify_blocker: false

worktree:
  copy_env_files: ".env, .env.local, .env.test"
  init_commands: "npm ci && npm run setup"

completion:
  mode: "merge"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify all fields
	if cfg.Project.Name != "Full Config" {
		t.Errorf("Project.Name mismatch")
	}
	if cfg.Project.Description != "Testing all fields" {
		t.Errorf("Project.Description mismatch")
	}
	if cfg.Git.BaseBranch != "master" {
		t.Errorf("Git.BaseBranch mismatch")
	}
	if cfg.Commands.Test != "go test ./..." {
		t.Errorf("Commands.Test mismatch")
	}
	if cfg.Commands.Lint != "golangci-lint run" {
		t.Errorf("Commands.Lint mismatch")
	}
	if cfg.Commands.Build != "go build ./..." {
		t.Errorf("Commands.Build mismatch")
	}
	if cfg.Commands.Dev != "go run ./cmd/ralph" {
		t.Errorf("Commands.Dev mismatch")
	}
	if cfg.Slack.WebhookURL != "https://hooks.slack.com/full" {
		t.Errorf("Slack.WebhookURL mismatch")
	}
	if cfg.Slack.Channel != "C12345678" {
		t.Errorf("Slack.Channel mismatch")
	}
	if cfg.Slack.BotToken != "xoxb-token" {
		t.Errorf("Slack.BotToken mismatch")
	}
	if cfg.Slack.AppToken != "xapp-token" {
		t.Errorf("Slack.AppToken mismatch")
	}
	if !cfg.Slack.GlobalBot {
		t.Errorf("Slack.GlobalBot should be true")
	}
	if cfg.Slack.NotifyStart {
		t.Errorf("Slack.NotifyStart should be false")
	}
	if cfg.Slack.NotifyComplete {
		t.Errorf("Slack.NotifyComplete should be false")
	}
	if !cfg.Slack.NotifyIteration {
		t.Errorf("Slack.NotifyIteration should be true")
	}
	if cfg.Slack.NotifyError {
		t.Errorf("Slack.NotifyError should be false")
	}
	if cfg.Slack.NotifyBlocker {
		t.Errorf("Slack.NotifyBlocker should be false")
	}
	if cfg.Worktree.CopyEnvFiles != ".env, .env.local, .env.test" {
		t.Errorf("Worktree.CopyEnvFiles mismatch")
	}
	if cfg.Worktree.InitCommands != "npm ci && npm run setup" {
		t.Errorf("Worktree.InitCommands mismatch")
	}
	if cfg.Completion.Mode != "merge" {
		t.Errorf("Completion.Mode mismatch")
	}
}
