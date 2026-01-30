package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateCheckbox_Complete(t *testing.T) {
	content := `# Plan

- [ ] Task 1
- [ ] Task 2
- [x] Task 3
`
	// Complete Task 1 (line 3)
	result, err := UpdateCheckbox(content, 3, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "- [x] Task 1") {
		t.Errorf("expected Task 1 to be checked, got:\n%s", result)
	}
	// Other tasks should remain unchanged
	if !strings.Contains(result, "- [ ] Task 2") {
		t.Errorf("expected Task 2 to remain unchecked")
	}
	if !strings.Contains(result, "- [x] Task 3") {
		t.Errorf("expected Task 3 to remain checked")
	}
}

func TestUpdateCheckbox_Uncomplete(t *testing.T) {
	content := `# Plan

- [x] Task 1
- [x] Task 2
`
	// Uncomplete Task 1 (line 3)
	result, err := UpdateCheckbox(content, 3, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "- [ ] Task 1") {
		t.Errorf("expected Task 1 to be unchecked, got:\n%s", result)
	}
	// Task 2 should remain checked
	if !strings.Contains(result, "- [x] Task 2") {
		t.Errorf("expected Task 2 to remain checked")
	}
}

func TestUpdateCheckbox_PreservesWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		complete bool
		expected string
	}{
		{
			name:     "no indent",
			line:     "- [ ] Task",
			complete: true,
			expected: "- [x] Task",
		},
		{
			name:     "2 space indent",
			line:     "  - [ ] Task",
			complete: true,
			expected: "  - [x] Task",
		},
		{
			name:     "4 space indent",
			line:     "    - [ ] Task",
			complete: true,
			expected: "    - [x] Task",
		},
		{
			name:     "tab indent",
			line:     "\t- [ ] Task",
			complete: true,
			expected: "\t- [x] Task",
		},
		{
			name:     "extra spaces after dash",
			line:     "-  [ ] Task",
			complete: true,
			expected: "-  [x] Task",
		},
		{
			name:     "uppercase X",
			line:     "- [X] Task",
			complete: false,
			expected: "- [ ] Task",
		},
		{
			name:     "preserves task text exactly",
			line:     "- [ ] Task with **bold** and `code`",
			complete: true,
			expected: "- [x] Task with **bold** and `code`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "header\n" + tt.line + "\nfooter"
			result, err := UpdateCheckbox(content, 2, tt.complete)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			lines := strings.Split(result, "\n")
			if lines[1] != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, lines[1])
			}
		})
	}
}

func TestUpdateCheckbox_ErrorNoCheckbox(t *testing.T) {
	content := `# Plan

This is just text
- Not a checkbox, no brackets
`
	_, err := UpdateCheckbox(content, 3, true)
	if err == nil {
		t.Fatal("expected error for line without checkbox")
	}
	if !strings.Contains(err.Error(), "does not contain a checkbox") {
		t.Errorf("expected 'does not contain a checkbox' error, got: %v", err)
	}
}

func TestUpdateCheckbox_ErrorInvalidLine(t *testing.T) {
	content := "line 1\nline 2\nline 3"

	tests := []struct {
		name    string
		lineNum int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too large", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := UpdateCheckbox(content, tt.lineNum, true)
			if err == nil {
				t.Fatal("expected error for invalid line number")
			}
			if !strings.Contains(err.Error(), "out of range") {
				t.Errorf("expected 'out of range' error, got: %v", err)
			}
		})
	}
}

func TestUpdateCheckbox_PreservesSurroundingMarkdown(t *testing.T) {
	content := `# Plan: Test

## Tasks

**Status:** open

- [ ] First task
- [ ] Second task

## Notes

Some notes here.
`
	// Check first task
	result, err := UpdateCheckbox(content, 7, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the structure is preserved
	if !strings.Contains(result, "# Plan: Test") {
		t.Error("header not preserved")
	}
	if !strings.Contains(result, "## Tasks") {
		t.Error("section header not preserved")
	}
	if !strings.Contains(result, "**Status:** open") {
		t.Error("status not preserved")
	}
	if !strings.Contains(result, "- [x] First task") {
		t.Error("first task not checked")
	}
	if !strings.Contains(result, "- [ ] Second task") {
		t.Error("second task should remain unchecked")
	}
	if !strings.Contains(result, "## Notes") {
		t.Error("notes section not preserved")
	}
	if !strings.Contains(result, "Some notes here.") {
		t.Error("notes content not preserved")
	}
}

func TestSave_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")

	plan := &Plan{
		Path:    planPath,
		Content: "# Test Plan\n\n- [ ] Task 1\n",
	}

	err := Save(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file exists with correct content
	content, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if string(content) != plan.Content {
		t.Errorf("content mismatch:\nexpected: %q\ngot: %q", plan.Content, string(content))
	}
}

func TestSave_OverwritesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")

	// Create initial file
	initialContent := "# Old Content"
	if err := os.WriteFile(planPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}

	// Save new content
	plan := &Plan{
		Path:    planPath,
		Content: "# New Content\n\n- [x] Updated task\n",
	}

	err := Save(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file has new content
	content, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if string(content) != plan.Content {
		t.Errorf("content mismatch:\nexpected: %q\ngot: %q", plan.Content, string(content))
	}
}

func TestSave_PreservesPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")

	// Create initial file with specific permissions
	initialPerm := os.FileMode(0600)
	if err := os.WriteFile(planPath, []byte("initial"), initialPerm); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}

	plan := &Plan{
		Path:    planPath,
		Content: "# Updated Content",
	}

	err := Save(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify permissions preserved
	info, err := os.Stat(planPath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Mode().Perm() != initialPerm {
		t.Errorf("permissions not preserved: expected %o, got %o", initialPerm, info.Mode().Perm())
	}
}

func TestSave_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")

	// Create initial file
	initialContent := "# Original Content"
	if err := os.WriteFile(planPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}

	plan := &Plan{
		Path:    planPath,
		Content: "# New Content",
	}

	err := Save(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check no temp files left behind
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".plan-") && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}
}

func TestSave_ErrorNilPlan(t *testing.T) {
	err := Save(nil)
	if err == nil {
		t.Fatal("expected error for nil plan")
	}
}

func TestSave_ErrorEmptyPath(t *testing.T) {
	plan := &Plan{
		Path:    "",
		Content: "content",
	}
	err := Save(plan)
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestPlan_SetCheckbox(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")

	content := `# Test Plan

- [ ] Task 1
- [ ] Task 2
`
	if err := os.WriteFile(planPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create plan file: %v", err)
	}

	plan, err := Load(planPath)
	if err != nil {
		t.Fatalf("failed to load plan: %v", err)
	}

	// Verify initial state
	if len(plan.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(plan.Tasks))
	}
	if plan.Tasks[0].Complete {
		t.Error("task 1 should start incomplete")
	}

	// Complete task 1
	err = plan.SetCheckbox(3, true)
	if err != nil {
		t.Fatalf("failed to set checkbox: %v", err)
	}

	// Verify Content and Tasks are both updated
	if !strings.Contains(plan.Content, "- [x] Task 1") {
		t.Error("Content not updated with checked checkbox")
	}
	if !plan.Tasks[0].Complete {
		t.Error("Tasks not updated - task 1 should be complete")
	}
}
