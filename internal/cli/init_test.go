package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withStdinInput replaces os.Stdin with a pipe that provides the given input.
func withStdinInput(t *testing.T, input string) func() {
	t.Helper()
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	_, _ = w.WriteString(input)
	w.Close()
	return func() {
		os.Stdin = oldStdin
		r.Close()
	}
}

// withStdinNewlines replaces os.Stdin with a pipe that provides newlines
// for the Board config prompts (3 prompts: slug, URL, token).
func withStdinNewlines(t *testing.T) func() {
	return withStdinInput(t, "\n\n\n")
}

func TestRunInit_CreatesDirectoryStructure(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "ralph-init-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Reset detect flag
	detectFlag = false

	// Provide stdin for Board prompts
	cleanup := withStdinNewlines(t)
	defer cleanup()

	// Run init
	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	// Verify directories exist
	expectedDirs := []string{
		".ralph",
		".ralph/worktrees",
		"specs",
	}

	for _, dir := range expectedDirs {
		path := filepath.Join(tmpDir, dir)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Directory %s does not exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}

	// Verify files exist
	expectedFiles := []string{
		".ralph/config.yaml",
		".ralph/worktrees/.gitignore",
		"specs/INDEX.md",
	}

	for _, file := range expectedFiles {
		path := filepath.Join(tmpDir, file)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("File %s does not exist: %v", file, err)
		}
	}
}

func TestRunInit_WithDetection(t *testing.T) {
	// Create temp directory with package.json
	tmpDir, err := os.MkdirTemp("", "ralph-init-detect-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create package.json
	pkgJSON := `{"name":"test","scripts":{"test":"jest","lint":"eslint ."}}`
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Set detect flag
	detectFlag = true
	defer func() { detectFlag = false }()

	// Provide stdin for Board prompts
	cleanup := withStdinNewlines(t)
	defer cleanup()

	// Run init
	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	// Read config file
	configData, err := os.ReadFile(filepath.Join(tmpDir, ".ralph/config.yaml"))
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	configStr := string(configData)

	// Verify detected commands are in config
	if !strings.Contains(configStr, "npm test") {
		t.Error("Config should contain 'npm test' command")
	}
	if !strings.Contains(configStr, "npm run lint") {
		t.Error("Config should contain 'npm run lint' command")
	}
}

func TestRunInit_PreservesExistingSpecs(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "ralph-init-preserve-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing specs/INDEX.md
	specsDir := filepath.Join(tmpDir, "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}

	existingIndex := "# Existing Index\nThis should be preserved."
	indexPath := filepath.Join(specsDir, "INDEX.md")
	if err := os.WriteFile(indexPath, []byte(existingIndex), 0644); err != nil {
		t.Fatalf("Failed to create existing INDEX.md: %v", err)
	}

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Reset detect flag
	detectFlag = false

	// Provide stdin for Board prompts
	cleanup := withStdinNewlines(t)
	defer cleanup()

	// Run init
	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	// Verify existing INDEX.md was preserved
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("Failed to read INDEX.md: %v", err)
	}

	if string(indexData) != existingIndex {
		t.Error("Existing INDEX.md should be preserved, not overwritten")
	}
}

func TestRunInit_PreservesExistingConfig(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "ralph-init-reinit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing .ralph/config.yaml with custom values
	ralphDir := filepath.Join(tmpDir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatalf("Failed to create .ralph dir: %v", err)
	}

	existingConfig := `project:
    name: my-project
    description: My custom description
commands:
    test: make test
    lint: golangci-lint run ./...
    build: make build
slack:
    channel: C12345
    notify_start: true
board:
    project_slug: my-slug
`
	configPath := filepath.Join(ralphDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(existingConfig), 0644); err != nil {
		t.Fatalf("Failed to create existing config: %v", err)
	}

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Reset detect flag
	detectFlag = false

	// No stdin needed — all Board fields are already set or skipped
	cleanup := withStdinInput(t, "")
	defer cleanup()

	// Run init (re-init)
	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	// Read the updated config
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}
	configStr := string(configData)

	// Verify existing values were preserved
	if !strings.Contains(configStr, "my-project") {
		t.Error("Config should preserve project name 'my-project'")
	}
	if !strings.Contains(configStr, "My custom description") {
		t.Error("Config should preserve project description")
	}
	if !strings.Contains(configStr, "make test") {
		t.Error("Config should preserve test command 'make test'")
	}
	if !strings.Contains(configStr, "golangci-lint run") {
		t.Error("Config should preserve lint command")
	}
	if !strings.Contains(configStr, "C12345") {
		t.Error("Config should preserve Slack channel")
	}
	if !strings.Contains(configStr, "my-slug") {
		t.Error("Config should preserve Board project slug")
	}
}

func TestRunInit_ReinitDetectDoesNotOverwriteCommands(t *testing.T) {
	// Create temp directory with package.json AND existing config
	tmpDir, err := os.MkdirTemp("", "ralph-init-reinit-detect-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create package.json (would detect npm test)
	pkgJSON := `{"name":"test","scripts":{"test":"jest","lint":"eslint ."}}`
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create existing config with custom test command
	ralphDir := filepath.Join(tmpDir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatalf("Failed to create .ralph dir: %v", err)
	}

	existingConfig := `project:
    name: test-project
commands:
    test: "make test-custom"
    lint: ""
`
	if err := os.WriteFile(filepath.Join(ralphDir, "config.yaml"), []byte(existingConfig), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Enable detection
	detectFlag = true
	defer func() { detectFlag = false }()

	// No stdin needed
	cleanup := withStdinInput(t, "\n")
	defer cleanup()

	// Run init (re-init with detect)
	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	// Read config
	configData, err := os.ReadFile(filepath.Join(ralphDir, "config.yaml"))
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}
	configStr := string(configData)

	// Test command should be preserved (not overwritten by detection)
	if !strings.Contains(configStr, "make test-custom") {
		t.Errorf("Config should preserve existing test command 'make test-custom', got: %s", configStr)
	}

	// Lint command was empty, so detection should fill it in
	if !strings.Contains(configStr, "npm run lint") {
		t.Errorf("Config should fill in empty lint command with detected 'npm run lint', got: %s", configStr)
	}
}

func TestSpecsIndexContent(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "ralph-index-test-*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Create index
	if err := createSpecsIndex(tmpFile.Name()); err != nil {
		t.Fatalf("createSpecsIndex failed: %v", err)
	}

	// Read content
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read index: %v", err)
	}

	contentStr := string(content)

	// Verify essential sections
	expectedSections := []string{
		"# Specifications Index",
		"## Format",
		"## Specifications",
		"## Creating a New Specification",
		"## Specification Template",
	}

	for _, section := range expectedSections {
		if !strings.Contains(contentStr, section) {
			t.Errorf("INDEX.md should contain section: %s", section)
		}
	}
}
