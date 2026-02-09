package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withStdinNewlines replaces os.Stdin with a pipe that provides newlines
// for the ATM config prompts (3 prompts: slug, URL, token).
func withStdinNewlines(t *testing.T) func() {
	t.Helper()
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	// Write 3 newlines for 3 ATM prompts
	_, _ = w.WriteString("\n\n\n")
	w.Close()
	return func() {
		os.Stdin = oldStdin
		r.Close()
	}
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

	// Provide stdin for ATM prompts
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
		"plans/pending",
		"plans/current",
		"plans/complete",
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

	// Provide stdin for ATM prompts
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

	// Provide stdin for ATM prompts
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
