package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureReference_AddsToExistingCLAUDEMD(t *testing.T) {
	dir := t.TempDir()
	claudeMD := filepath.Join(dir, "CLAUDE.md")

	original := "# My Project\n\nSome instructions.\n"
	if err := os.WriteFile(claudeMD, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureReference(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal(err)
	}

	s := string(content)

	if !strings.Contains(s, "## Operational Learnings") {
		t.Error("expected Operational Learnings section")
	}
	if !strings.Contains(s, "`.claude/learnings.md`") {
		t.Error("expected reference to .claude/learnings.md")
	}

	// Original content should still be present.
	if !strings.Contains(s, "# My Project") {
		t.Error("original content should be preserved")
	}
}

func TestEnsureReference_Idempotent(t *testing.T) {
	dir := t.TempDir()
	claudeMD := filepath.Join(dir, "CLAUDE.md")

	original := "# My Project\n\nSome instructions.\n"
	if err := os.WriteFile(claudeMD, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// First call adds reference.
	if err := EnsureReference(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}

	first, _ := os.ReadFile(claudeMD)

	// Second call should not modify the file.
	if err := EnsureReference(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}

	second, _ := os.ReadFile(claudeMD)

	if string(first) != string(second) {
		t.Error("file was modified on second call — not idempotent")
	}

	// Reference section should appear exactly once.
	count := strings.Count(string(second), "## Operational Learnings")
	if count != 1 {
		t.Errorf("expected 1 Operational Learnings section, got %d", count)
	}
}

func TestEnsureReference_MissingCLAUDEMD(t *testing.T) {
	dir := t.TempDir()

	// Should not error — just logs a warning.
	if err := EnsureReference(dir); err != nil {
		t.Fatalf("expected nil error for missing CLAUDE.md, got: %v", err)
	}

	// Should not create a CLAUDE.md.
	_, err := os.Stat(filepath.Join(dir, "CLAUDE.md"))
	if !os.IsNotExist(err) {
		t.Error("should not create CLAUDE.md when it doesn't exist")
	}
}

func TestEnsureReference_AlreadyHasReference(t *testing.T) {
	dir := t.TempDir()
	claudeMD := filepath.Join(dir, "CLAUDE.md")

	// File that already contains the marker.
	existing := "# My Project\n\n## Operational Learnings\n\nSome existing learnings pointer.\n"
	if err := os.WriteFile(claudeMD, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureReference(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(claudeMD)
	if string(content) != existing {
		t.Error("file should not be modified when reference already exists")
	}
}

func TestEnsureReference_FileWithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	claudeMD := filepath.Join(dir, "CLAUDE.md")

	// No trailing newline.
	original := "# My Project\n\nSome content"
	if err := os.WriteFile(claudeMD, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureReference(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(claudeMD)
	s := string(content)

	if !strings.Contains(s, "## Operational Learnings") {
		t.Error("expected Operational Learnings section")
	}

	// Should not have double newlines from missing trailing newline.
	if strings.Contains(s, "content\n\n\n") {
		t.Error("should not have excessive newlines")
	}
}
