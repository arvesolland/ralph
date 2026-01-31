package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFeedbackPath(t *testing.T) {
	tests := []struct {
		name     string
		planPath string
		want     string
	}{
		{
			name:     "simple plan",
			planPath: "plans/current/my-plan.md",
			want:     "plans/current/my-plan.feedback.md",
		},
		{
			name:     "nested path",
			planPath: "/home/user/project/plans/pending/feature.md",
			want:     "/home/user/project/plans/pending/feature.feedback.md",
		},
		{
			name:     "plan with dots in name",
			planPath: "plans/v1.2.3-release.md",
			want:     "plans/v1.2.3-release.feedback.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &Plan{Path: tt.planPath}
			got := FeedbackPath(plan)
			if got != tt.want {
				t.Errorf("FeedbackPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadFeedback_NonExistent(t *testing.T) {
	plan := &Plan{
		Path: "/nonexistent/path/plan.md",
		Name: "plan",
	}

	content, err := ReadFeedback(plan)
	if err != nil {
		t.Errorf("ReadFeedback() error = %v, want nil", err)
	}
	if content != "" {
		t.Errorf("ReadFeedback() = %q, want empty string", content)
	}
}

func TestReadFeedback_Existing(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "my-plan.md")
	feedbackPath := filepath.Join(dir, "my-plan.feedback.md")

	// Create feedback file
	feedbackContent := `# Feedback: my-plan

## Pending
- [2024-01-30 14:32] Package is now public
- [2024-01-30 15:00] Use OAuth instead of API keys

## Processed
- [2024-01-30 10:00] Already handled item
`
	if err := os.WriteFile(feedbackPath, []byte(feedbackContent), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "my-plan"}
	content, err := ReadFeedback(plan)
	if err != nil {
		t.Errorf("ReadFeedback() error = %v", err)
	}

	// Should only return pending section content
	if !strings.Contains(content, "Package is now public") {
		t.Errorf("ReadFeedback() should contain pending items, got %q", content)
	}
	if !strings.Contains(content, "Use OAuth") {
		t.Errorf("ReadFeedback() should contain all pending items, got %q", content)
	}
	if strings.Contains(content, "Already handled") {
		t.Errorf("ReadFeedback() should NOT contain processed items, got %q", content)
	}
}

func TestReadFeedback_EmptyPendingSection(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "my-plan.md")
	feedbackPath := filepath.Join(dir, "my-plan.feedback.md")

	// Create feedback file with empty pending section
	feedbackContent := `# Feedback: my-plan

## Pending

## Processed
- [2024-01-30 10:00] Already handled item
`
	if err := os.WriteFile(feedbackPath, []byte(feedbackContent), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "my-plan"}
	content, err := ReadFeedback(plan)
	if err != nil {
		t.Errorf("ReadFeedback() error = %v", err)
	}

	if content != "" {
		t.Errorf("ReadFeedback() = %q, want empty string", content)
	}
}

func TestAppendFeedback_NewFile(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "my-plan.md")
	feedbackPath := filepath.Join(dir, "my-plan.feedback.md")

	plan := &Plan{Path: planPath, Name: "my-plan"}
	timestamp := time.Date(2024, 1, 30, 14, 32, 0, 0, time.UTC)

	err := AppendFeedbackWithTime(plan, "slack", "Task completed successfully", timestamp)
	if err != nil {
		t.Fatalf("AppendFeedback() error = %v", err)
	}

	// Check file was created
	content, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatalf("Reading feedback file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "# Feedback: my-plan") {
		t.Errorf("File should have header, got %q", contentStr)
	}
	if !strings.Contains(contentStr, "## Pending") {
		t.Errorf("File should have Pending section, got %q", contentStr)
	}
	if !strings.Contains(contentStr, "[2024-01-30 14:32] slack: Task completed successfully") {
		t.Errorf("File should have entry, got %q", contentStr)
	}
	if !strings.Contains(contentStr, "## Processed") {
		t.Errorf("File should have Processed section, got %q", contentStr)
	}
}

func TestAppendFeedback_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "my-plan.md")
	feedbackPath := filepath.Join(dir, "my-plan.feedback.md")

	// Create existing feedback file
	existingContent := `# Feedback: my-plan

## Pending
- [2024-01-30 10:00] First entry

## Processed
- [2024-01-29 09:00] Old processed entry
`
	if err := os.WriteFile(feedbackPath, []byte(existingContent), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "my-plan"}
	timestamp := time.Date(2024, 1, 30, 14, 32, 0, 0, time.UTC)

	err := AppendFeedbackWithTime(plan, "", "New feedback item", timestamp)
	if err != nil {
		t.Fatalf("AppendFeedback() error = %v", err)
	}

	content, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatal(err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "First entry") {
		t.Errorf("Should preserve existing entries, got %q", contentStr)
	}
	if !strings.Contains(contentStr, "[2024-01-30 14:32] New feedback item") {
		t.Errorf("Should add new entry, got %q", contentStr)
	}
	if !strings.Contains(contentStr, "Old processed entry") {
		t.Errorf("Should preserve processed section, got %q", contentStr)
	}
}

func TestAppendFeedback_NoSource(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "my-plan.md")
	feedbackPath := filepath.Join(dir, "my-plan.feedback.md")

	plan := &Plan{Path: planPath, Name: "my-plan"}
	timestamp := time.Date(2024, 1, 30, 14, 32, 0, 0, time.UTC)

	err := AppendFeedbackWithTime(plan, "", "Feedback without source", timestamp)
	if err != nil {
		t.Fatalf("AppendFeedback() error = %v", err)
	}

	content, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatal(err)
	}

	contentStr := string(content)
	// Should NOT have ":" after timestamp when no source
	if strings.Contains(contentStr, "14:32] :") {
		t.Errorf("Should not have colon when no source, got %q", contentStr)
	}
	if !strings.Contains(contentStr, "[2024-01-30 14:32] Feedback without source") {
		t.Errorf("Should have entry without source prefix, got %q", contentStr)
	}
}

func TestMarkProcessed_Success(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "my-plan.md")
	feedbackPath := filepath.Join(dir, "my-plan.feedback.md")

	// Create feedback file with pending entry
	existingContent := `# Feedback: my-plan

## Pending
- [2024-01-30 14:32] Entry to process
- [2024-01-30 15:00] Another pending entry

## Processed
- [2024-01-29 09:00] Previously processed
`
	if err := os.WriteFile(feedbackPath, []byte(existingContent), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "my-plan"}

	err := MarkProcessed(plan, "- [2024-01-30 14:32] Entry to process")
	if err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}

	content, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatal(err)
	}

	contentStr := string(content)

	// Entry should be moved to processed
	pendingSection := extractPendingSection(contentStr)
	if strings.Contains(pendingSection, "Entry to process") {
		t.Errorf("Entry should be removed from Pending, got pending section: %q", pendingSection)
	}

	// Other pending entry should remain
	if !strings.Contains(pendingSection, "Another pending entry") {
		t.Errorf("Other pending entries should remain, got pending section: %q", pendingSection)
	}

	// Entry should appear in processed section
	if !strings.Contains(contentStr, "## Processed") {
		t.Errorf("Should have Processed section, got %q", contentStr)
	}

	// Check it's in processed section (comes after "## Processed")
	processedIdx := strings.Index(contentStr, "## Processed")
	entryIdx := strings.LastIndex(contentStr, "Entry to process")
	if entryIdx < processedIdx {
		t.Errorf("Entry should be in Processed section, but appears before it")
	}
}

func TestMarkProcessed_EntryNotFound(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "my-plan.md")
	feedbackPath := filepath.Join(dir, "my-plan.feedback.md")

	// Create feedback file
	existingContent := `# Feedback: my-plan

## Pending
- [2024-01-30 14:32] Some entry

## Processed
`
	if err := os.WriteFile(feedbackPath, []byte(existingContent), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "my-plan"}

	err := MarkProcessed(plan, "- [2024-01-30 99:99] Nonexistent entry")
	if err == nil {
		t.Error("MarkProcessed() should return error for nonexistent entry")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error should mention entry not found, got: %v", err)
	}
}

func TestMarkProcessed_FileNotExists(t *testing.T) {
	plan := &Plan{
		Path: "/nonexistent/path/plan.md",
		Name: "plan",
	}

	err := MarkProcessed(plan, "- [2024-01-30 14:32] Some entry")
	if err == nil {
		t.Error("MarkProcessed() should return error for nonexistent file")
	}
}

func TestCreateFeedbackFile_NewFile(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "my-plan.md")
	feedbackPath := filepath.Join(dir, "my-plan.feedback.md")

	plan := &Plan{Path: planPath, Name: "my-plan"}

	err := CreateFeedbackFile(plan)
	if err != nil {
		t.Fatalf("CreateFeedbackFile() error = %v", err)
	}

	content, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatal(err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "# Feedback: my-plan") {
		t.Errorf("Should have header, got %q", contentStr)
	}
	if !strings.Contains(contentStr, "## Pending") {
		t.Errorf("Should have Pending section, got %q", contentStr)
	}
	if !strings.Contains(contentStr, "## Processed") {
		t.Errorf("Should have Processed section, got %q", contentStr)
	}
}

func TestCreateFeedbackFile_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "my-plan.md")
	feedbackPath := filepath.Join(dir, "my-plan.feedback.md")

	// Create existing file with custom content
	existingContent := "custom content that should not be overwritten"
	if err := os.WriteFile(feedbackPath, []byte(existingContent), 0644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Path: planPath, Name: "my-plan"}

	err := CreateFeedbackFile(plan)
	if err != nil {
		t.Fatalf("CreateFeedbackFile() error = %v", err)
	}

	// File should not be modified
	content, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != existingContent {
		t.Errorf("CreateFeedbackFile() should not modify existing file, got %q", string(content))
	}
}

func TestExtractPendingSection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "normal case",
			content: `# Feedback

## Pending
- [2024-01-30 14:32] First item
- [2024-01-30 15:00] Second item

## Processed
- [2024-01-29 09:00] Old item
`,
			want: "- [2024-01-30 14:32] First item\n- [2024-01-30 15:00] Second item",
		},
		{
			name: "empty pending",
			content: `# Feedback

## Pending

## Processed
- Old item
`,
			want: "",
		},
		{
			name: "no processed section",
			content: `# Feedback

## Pending
- [2024-01-30 14:32] Only item
`,
			want: "- [2024-01-30 14:32] Only item",
		},
		{
			name:    "no pending section",
			content: "# Feedback\n\nSome other content",
			want:    "",
		},
		{
			name: "with comments",
			content: `## Pending
<!-- This is a comment -->
- [2024-01-30 14:32] Real item

## Processed
`,
			want: "- [2024-01-30 14:32] Real item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPendingSection(tt.content)
			if got != tt.want {
				t.Errorf("extractPendingSection() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFeedbackPath_PreservesDirectory(t *testing.T) {
	plan := &Plan{
		Path: "/absolute/path/to/plans/current/test-plan.md",
	}

	got := FeedbackPath(plan)
	wantDir := "/absolute/path/to/plans/current"

	if filepath.Dir(got) != wantDir {
		t.Errorf("FeedbackPath() directory = %q, want %q", filepath.Dir(got), wantDir)
	}
}
