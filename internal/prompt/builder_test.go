package prompt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arvesolland/ralph/internal/config"
)

func TestBuilder_Build_WithConfig(t *testing.T) {
	// Create temp directories
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".ralph")
	promptsDir := filepath.Join(tempDir, "prompts")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test template
	templateContent := `Project: {{PROJECT_NAME}}
Description: {{PROJECT_DESCRIPTION}}
Test: {{TEST_COMMAND}}
Lint: {{LINT_COMMAND}}
Build: {{BUILD_COMMAND}}`

	templatePath := filepath.Join(promptsDir, "test.md")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create config
	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:        "TestProject",
			Description: "A test project",
		},
		Commands: config.CommandsConfig{
			Test:  "go test ./...",
			Lint:  "golangci-lint run",
			Build: "go build ./...",
		},
	}

	// Build prompt
	builder := NewBuilder(cfg, configDir, promptsDir)
	result, err := builder.Build("test.md", nil)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	expected := `Project: TestProject
Description: A test project
Test: go test ./...
Lint: golangci-lint run
Build: go build ./...`

	if result != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, result)
	}
}

func TestBuilder_Build_UnknownPlaceholders(t *testing.T) {
	// Create temp directories
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".ralph")
	promptsDir := filepath.Join(tempDir, "prompts")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test template with unknown placeholder
	templateContent := `Known: {{PROJECT_NAME}}
Unknown: {{FUTURE_PLACEHOLDER}}`

	templatePath := filepath.Join(promptsDir, "test.md")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name: "TestProject",
		},
	}

	// Build prompt - unknown placeholder should be left as-is
	builder := NewBuilder(cfg, configDir, promptsDir)
	result, err := builder.Build("test.md", nil)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	expected := `Known: TestProject
Unknown: {{FUTURE_PLACEHOLDER}}`

	if result != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, result)
	}
}

func TestBuilder_Build_WithExplicitOverrides(t *testing.T) {
	// Create temp directories
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".ralph")
	promptsDir := filepath.Join(tempDir, "prompts")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test template
	templateContent := `Project: {{PROJECT_NAME}}
Custom: {{CUSTOM_VALUE}}`

	templatePath := filepath.Join(promptsDir, "test.md")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name: "ConfigProject",
		},
	}

	// Build prompt with overrides
	overrides := map[string]string{
		"PROJECT_NAME": "OverriddenProject",
		"CUSTOM_VALUE": "CustomData",
	}

	builder := NewBuilder(cfg, configDir, promptsDir)
	result, err := builder.Build("test.md", overrides)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	expected := `Project: OverriddenProject
Custom: CustomData`

	if result != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, result)
	}
}

func TestBuilder_Build_EmbeddedPrompt(t *testing.T) {
	// Use embedded prompt (no external files)
	builder := NewBuilder(&config.Config{
		Project: config.ProjectConfig{
			Name: "TestProject",
		},
	}, "", "")

	// Build using embedded prompt.md
	result, err := builder.Build("prompt.md", nil)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should contain the project name substitution
	if !contains(result, "TestProject") {
		t.Errorf("Expected result to contain 'TestProject', got:\n%s", result)
	}

	// Should have processed the template (contains expected text from prompt.md)
	if !contains(result, "You are Ralph") {
		t.Errorf("Expected result to contain 'You are Ralph', got:\n%s", result)
	}
}

func TestBuilder_Build_AbsolutePath(t *testing.T) {
	// Create temp directories
	tempDir := t.TempDir()

	// Create a test template at an absolute path
	templateContent := `Hello {{PROJECT_NAME}}`
	templatePath := filepath.Join(tempDir, "absolute-test.md")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name: "AbsoluteProject",
		},
	}

	// Build with absolute path
	builder := NewBuilder(cfg, "", "")
	result, err := builder.Build(templatePath, nil)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	expected := "Hello AbsoluteProject"
	if result != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, result)
	}
}

func TestListEmbeddedPrompts(t *testing.T) {
	prompts, err := ListEmbeddedPrompts()
	if err != nil {
		t.Fatalf("ListEmbeddedPrompts failed: %v", err)
	}

	// Should have at least prompt.md
	found := false
	for _, p := range prompts {
		if p == "prompt.md" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected to find 'prompt.md' in embedded prompts, got: %v", prompts)
	}
}

func TestPlaceholderRegex(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"{{PROJECT_NAME}}", []string{"{{PROJECT_NAME}}"}},
		{"{{A}} and {{B}}", []string{"{{A}}", "{{B}}"}},
		{"no placeholders", nil},
		{"{{lowercase}}", nil}, // lowercase should not match
		{"{{ SPACED }}", nil},  // spaces should not match
		{"{{123}}", nil},       // numbers only should not match
		{"{{A_B_C}}", []string{"{{A_B_C}}"}},
	}

	for _, tt := range tests {
		matches := placeholderRegex.FindAllString(tt.input, -1)
		if len(matches) != len(tt.expected) {
			t.Errorf("For input %q, expected %d matches, got %d: %v", tt.input, len(tt.expected), len(matches), matches)
			continue
		}
		for i, m := range matches {
			if m != tt.expected[i] {
				t.Errorf("For input %q, match %d: expected %q, got %q", tt.input, i, tt.expected[i], m)
			}
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
