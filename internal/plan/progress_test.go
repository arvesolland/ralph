package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProgressPath(t *testing.T) {
	tests := []struct {
		name     string
		planPath string
		expected string
	}{
		{
			name:     "simple plan",
			planPath: "/plans/current/go-rewrite.md",
			expected: "/plans/current/go-rewrite.progress.md",
		},
		{
			name:     "nested path",
			planPath: "/home/user/project/plans/pending/feature.md",
			expected: "/home/user/project/plans/pending/feature.progress.md",
		},
		{
			name:     "plan with multiple dots",
			planPath: "/plans/my.plan.md",
			expected: "/plans/my.plan.progress.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &Plan{Path: tt.planPath, Name: "test"}
			got := ProgressPath(plan)
			if got != tt.expected {
				t.Errorf("ProgressPath() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestReadProgress_NonExistent(t *testing.T) {
	// Create a plan pointing to a non-existent file
	tmpDir := t.TempDir()
	plan := &Plan{
		Path: filepath.Join(tmpDir, "nonexistent.md"),
		Name: "nonexistent",
	}

	content, err := ReadProgress(plan)
	if err != nil {
		t.Errorf("ReadProgress() unexpected error: %v", err)
	}
	if content != "" {
		t.Errorf("ReadProgress() = %q, want empty string", content)
	}
}

func TestReadProgress_Existing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create plan file
	planPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create progress file
	progressPath := filepath.Join(tmpDir, "test.progress.md")
	progressContent := "# Progress\n\n## Iteration 1\nDid stuff\n"
	if err := os.WriteFile(progressPath, []byte(progressContent), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "test"}
	content, err := ReadProgress(plan)
	if err != nil {
		t.Errorf("ReadProgress() unexpected error: %v", err)
	}
	if content != progressContent {
		t.Errorf("ReadProgress() = %q, want %q", content, progressContent)
	}
}

func TestAppendProgress_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "test"}
	timestamp := time.Date(2026, 1, 31, 14, 30, 0, 0, time.UTC)

	err := AppendProgressWithTime(plan, 1, "Did the thing.\n", timestamp)
	if err != nil {
		t.Fatalf("AppendProgressWithTime() error: %v", err)
	}

	content, err := ReadProgress(plan)
	if err != nil {
		t.Fatalf("ReadProgress() error: %v", err)
	}

	expected := "\n## Iteration 1 (2026-01-31 14:30)\nDid the thing.\n\n"
	if content != expected {
		t.Errorf("ReadProgress() = %q, want %q", content, expected)
	}
}

func TestAppendProgress_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create existing progress file
	progressPath := filepath.Join(tmpDir, "test.progress.md")
	existing := "# Progress: test\n\nIteration log.\n"
	if err := os.WriteFile(progressPath, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "test"}
	timestamp := time.Date(2026, 1, 31, 15, 0, 0, 0, time.UTC)

	err := AppendProgressWithTime(plan, 1, "First iteration work.", timestamp)
	if err != nil {
		t.Fatalf("AppendProgressWithTime() error: %v", err)
	}

	content, err := ReadProgress(plan)
	if err != nil {
		t.Fatalf("ReadProgress() error: %v", err)
	}

	if !strings.HasPrefix(content, existing) {
		t.Errorf("Content should preserve existing: got %q", content)
	}
	if !strings.Contains(content, "## Iteration 1 (2026-01-31 15:00)") {
		t.Errorf("Content should have iteration header: got %q", content)
	}
	if !strings.Contains(content, "First iteration work.") {
		t.Errorf("Content should have iteration content: got %q", content)
	}
}

func TestAppendProgress_MultipleIterations(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "test"}

	// Append multiple iterations
	ts1 := time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 1, 31, 11, 0, 0, 0, time.UTC)
	ts3 := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	if err := AppendProgressWithTime(plan, 1, "First.", ts1); err != nil {
		t.Fatal(err)
	}
	if err := AppendProgressWithTime(plan, 2, "Second.", ts2); err != nil {
		t.Fatal(err)
	}
	if err := AppendProgressWithTime(plan, 3, "Third.", ts3); err != nil {
		t.Fatal(err)
	}

	content, err := ReadProgress(plan)
	if err != nil {
		t.Fatal(err)
	}

	// Check all iterations are present
	if !strings.Contains(content, "## Iteration 1 (2026-01-31 10:00)") {
		t.Errorf("Missing iteration 1: %q", content)
	}
	if !strings.Contains(content, "## Iteration 2 (2026-01-31 11:00)") {
		t.Errorf("Missing iteration 2: %q", content)
	}
	if !strings.Contains(content, "## Iteration 3 (2026-01-31 12:00)") {
		t.Errorf("Missing iteration 3: %q", content)
	}
	if !strings.Contains(content, "First.") {
		t.Errorf("Missing first content: %q", content)
	}
	if !strings.Contains(content, "Second.") {
		t.Errorf("Missing second content: %q", content)
	}
	if !strings.Contains(content, "Third.") {
		t.Errorf("Missing third content: %q", content)
	}
}

func TestCreateProgressFile_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "myplan.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "myplan"}

	err := CreateProgressFile(plan)
	if err != nil {
		t.Fatalf("CreateProgressFile() error: %v", err)
	}

	content, err := ReadProgress(plan)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(content, "# Progress: myplan") {
		t.Errorf("Missing header: %q", content)
	}
	if !strings.Contains(content, "Iteration log") {
		t.Errorf("Missing description: %q", content)
	}
}

func TestCreateProgressFile_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create existing progress file with custom content
	progressPath := filepath.Join(tmpDir, "test.progress.md")
	existing := "Custom content that should not be overwritten"
	if err := os.WriteFile(progressPath, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "test"}

	err := CreateProgressFile(plan)
	if err != nil {
		t.Fatalf("CreateProgressFile() error: %v", err)
	}

	// Content should be unchanged
	content, err := ReadProgress(plan)
	if err != nil {
		t.Fatal(err)
	}

	if content != existing {
		t.Errorf("Content was modified: got %q, want %q", content, existing)
	}
}

func TestAppendProgress_CreatesParentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "a", "b", "c", "test.md")

	plan := &Plan{Path: nestedPath, Name: "test"}
	timestamp := time.Date(2026, 1, 31, 14, 0, 0, 0, time.UTC)

	err := AppendProgressWithTime(plan, 1, "Content", timestamp)
	if err != nil {
		t.Fatalf("AppendProgressWithTime() error: %v", err)
	}

	// Verify file was created
	progressPath := ProgressPath(plan)
	if _, err := os.Stat(progressPath); os.IsNotExist(err) {
		t.Errorf("Progress file was not created at %q", progressPath)
	}
}

func TestProgressPath_PreservesDirectory(t *testing.T) {
	// Verify that progress path is in the same directory as plan
	plan := &Plan{
		Path: "/some/path/to/plans/current/feature.md",
		Name: "feature",
	}

	progressPath := ProgressPath(plan)
	planDir := filepath.Dir(plan.Path)
	progressDir := filepath.Dir(progressPath)

	if planDir != progressDir {
		t.Errorf("Progress dir %q != plan dir %q", progressDir, planDir)
	}
}
