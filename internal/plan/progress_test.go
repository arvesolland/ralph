package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProgressPath(t *testing.T) {
	t.Run("flat file paths", func(t *testing.T) {
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
	})

	t.Run("bundle paths", func(t *testing.T) {
		plan := &Plan{
			Path:      "/plans/current/my-plan/plan.md",
			Name:      "my-plan",
			BundleDir: "/plans/current/my-plan",
		}
		got := ProgressPath(plan)
		expected := "/plans/current/my-plan/progress.md"
		if got != expected {
			t.Errorf("ProgressPath() = %q, want %q", got, expected)
		}
	})
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

	// Progress is 0/0 (0%) when plan has no tasks
	expected := "\n## Iteration 1 (2026-01-31 14:30) - 0/0 (0%)\nDid the thing.\n\n"
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
	if !strings.Contains(content, "## Iteration 1 (2026-01-31 15:00) - 0/0 (0%)") {
		t.Errorf("Content should have iteration header with progress: got %q", content)
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

	// Check all iterations are present (progress is 0/0 for empty task list)
	if !strings.Contains(content, "## Iteration 1 (2026-01-31 10:00) - 0/0 (0%)") {
		t.Errorf("Missing iteration 1: %q", content)
	}
	if !strings.Contains(content, "## Iteration 2 (2026-01-31 11:00) - 0/0 (0%)") {
		t.Errorf("Missing iteration 2: %q", content)
	}
	if !strings.Contains(content, "## Iteration 3 (2026-01-31 12:00) - 0/0 (0%)") {
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

func TestAppendProgress_WithTasks(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a plan with tasks (2 complete, 3 total = 66%)
	plan := &Plan{
		Path: planPath,
		Name: "test",
		Tasks: []Task{
			{Text: "Task 1", Complete: true},
			{Text: "Task 2", Complete: true},
			{Text: "Task 3", Complete: false},
		},
	}
	timestamp := time.Date(2026, 1, 31, 14, 30, 0, 0, time.UTC)

	err := AppendProgressWithTime(plan, 5, "Completed tasks 1 and 2.", timestamp)
	if err != nil {
		t.Fatalf("AppendProgressWithTime() error: %v", err)
	}

	content, err := ReadProgress(plan)
	if err != nil {
		t.Fatalf("ReadProgress() error: %v", err)
	}

	// Should include progress: 2/3 (66%)
	if !strings.Contains(content, "## Iteration 5 (2026-01-31 14:30) - 2/3 (66%)") {
		t.Errorf("Expected progress in header, got %q", content)
	}
}

func TestProgressPath_Bundle(t *testing.T) {
	tmpDir := t.TempDir()

	// Create bundle directory structure
	bundleDir := filepath.Join(tmpDir, "plans", "current", "my-bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatal(err)
	}

	planPath := filepath.Join(bundleDir, "plan.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{
		Path:      planPath,
		Name:      "my-bundle",
		BundleDir: bundleDir,
	}

	progressPath := ProgressPath(plan)
	expected := filepath.Join(bundleDir, "progress.md")
	if progressPath != expected {
		t.Errorf("ProgressPath() = %q, want %q", progressPath, expected)
	}
}

func TestReadProgress_Bundle(t *testing.T) {
	tmpDir := t.TempDir()

	// Create bundle directory structure
	bundleDir := filepath.Join(tmpDir, "my-bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatal(err)
	}

	planPath := filepath.Join(bundleDir, "plan.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create progress file in bundle
	progressContent := "# Progress: my-bundle\n\nIteration log.\n"
	progressPath := filepath.Join(bundleDir, "progress.md")
	if err := os.WriteFile(progressPath, []byte(progressContent), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{
		Path:      planPath,
		Name:      "my-bundle",
		BundleDir: bundleDir,
	}

	content, err := ReadProgress(plan)
	if err != nil {
		t.Fatalf("ReadProgress() error: %v", err)
	}

	if content != progressContent {
		t.Errorf("ReadProgress() = %q, want %q", content, progressContent)
	}
}

func TestAppendProgress_Bundle(t *testing.T) {
	tmpDir := t.TempDir()

	// Create bundle directory structure
	bundleDir := filepath.Join(tmpDir, "my-bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatal(err)
	}

	planPath := filepath.Join(bundleDir, "plan.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{
		Path:      planPath,
		Name:      "my-bundle",
		BundleDir: bundleDir,
	}
	timestamp := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)

	err := AppendProgressWithTime(plan, 1, "First iteration.", timestamp)
	if err != nil {
		t.Fatalf("AppendProgressWithTime() error: %v", err)
	}

	// Verify progress file is in bundle directory
	progressPath := filepath.Join(bundleDir, "progress.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		t.Fatalf("Failed to read progress file: %v", err)
	}

	if !strings.Contains(string(content), "## Iteration 1 (2026-02-01 10:00) - 0/0 (0%)") {
		t.Errorf("Expected iteration header in bundle progress file, got %q", string(content))
	}
}

func TestStripTemplateComments(t *testing.T) {
	t.Run("strips template comment block", func(t *testing.T) {
		input := `# Progress: test

Iteration log - what was done, gotchas, and next steps.

<!--
FORMAT FOR EACH ITERATION:
---
### Iteration N: Task identifier
**Completed:** What you actually did - be specific about files changed
**Gotcha:** Optional - surprises, edge cases, things that didn't work
**Next:** What the next iteration should tackle
-->
`
		expected := `# Progress: test

Iteration log - what was done, gotchas, and next steps.
`
		got := stripTemplateComments(input)
		if got != expected {
			t.Errorf("stripTemplateComments() = %q, want %q", got, expected)
		}
	})

	t.Run("preserves content without template", func(t *testing.T) {
		input := `# Progress: test

Iteration log - what was done, gotchas, and next steps.
`
		got := stripTemplateComments(input)
		if got != input {
			t.Errorf("stripTemplateComments() = %q, want %q", got, input)
		}
	})

	t.Run("preserves other HTML comments", func(t *testing.T) {
		input := `# Progress: test

<!-- Some other comment -->
`
		got := stripTemplateComments(input)
		if got != input {
			t.Errorf("stripTemplateComments() = %q, want %q", got, input)
		}
	})
}

func TestAppendProgress_StripsTemplateOnFirstIteration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create bundle directory structure
	bundleDir := filepath.Join(tmpDir, "my-bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatal(err)
	}

	planPath := filepath.Join(bundleDir, "plan.md")
	if err := os.WriteFile(planPath, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create scaffolded progress file with template comment
	progressContent := `# Progress: my-bundle

Iteration log - what was done, gotchas, and next steps.

<!--
FORMAT FOR EACH ITERATION:
---
### Iteration N: Task identifier
**Completed:** What you actually did - be specific about files changed
**Gotcha:** Optional - surprises, edge cases, things that didn't work
**Next:** What the next iteration should tackle
-->
`
	progressPath := filepath.Join(bundleDir, "progress.md")
	if err := os.WriteFile(progressPath, []byte(progressContent), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{
		Path:      planPath,
		Name:      "my-bundle",
		BundleDir: bundleDir,
	}
	timestamp := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)

	// Append iteration 1 - should strip template comments
	err := AppendProgressWithTime(plan, 1, "First iteration.", timestamp)
	if err != nil {
		t.Fatalf("AppendProgressWithTime() error: %v", err)
	}

	content, err := os.ReadFile(progressPath)
	if err != nil {
		t.Fatalf("Failed to read progress file: %v", err)
	}

	contentStr := string(content)

	// Template comment should be removed
	if strings.Contains(contentStr, "FORMAT FOR EACH ITERATION") {
		t.Errorf("Template comment should be stripped, got %q", contentStr)
	}

	// Header should be preserved
	if !strings.Contains(contentStr, "# Progress: my-bundle") {
		t.Errorf("Header should be preserved, got %q", contentStr)
	}

	// New iteration should be added
	if !strings.Contains(contentStr, "## Iteration 1 (2026-02-01 10:00)") {
		t.Errorf("Iteration should be added, got %q", contentStr)
	}
}
