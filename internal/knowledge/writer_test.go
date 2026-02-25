package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendLessons_NewFile(t *testing.T) {
	dir := t.TempDir()

	lessons := []Lesson{
		{Date: "2026-02-20", PlanID: 42, Author: "arve", Text: "Use the existing board mock for tests"},
	}

	if err := AppendLessons(dir, lessons); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(LearningsPath(dir))
	if err != nil {
		t.Fatalf("failed to read learnings file: %v", err)
	}

	s := string(content)

	// Should start with header.
	if !strings.HasPrefix(s, "# Operational Learnings") {
		t.Error("missing header")
	}

	// Should contain the lesson entry.
	expected := "- [2026-02-20, plan #42, arve] Use the existing board mock for tests"
	if !strings.Contains(s, expected) {
		t.Errorf("missing lesson entry, got:\n%s", s)
	}

	// Should have created .claude/ directory.
	info, err := os.Stat(filepath.Join(dir, ".claude"))
	if err != nil {
		t.Fatalf("expected .claude directory: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected .claude to be a directory")
	}
}

func TestAppendLessons_AppendToExisting(t *testing.T) {
	dir := t.TempDir()

	// Write initial lessons.
	first := []Lesson{
		{Date: "2026-02-20", PlanID: 42, Author: "arve", Text: "First lesson"},
	}
	if err := AppendLessons(dir, first); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Append more lessons.
	second := []Lesson{
		{Date: "2026-02-21", PlanID: 42, Author: "bob", Text: "Second lesson"},
	}
	if err := AppendLessons(dir, second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(LearningsPath(dir))
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	s := string(content)

	if !strings.Contains(s, "First lesson") {
		t.Error("missing first lesson")
	}
	if !strings.Contains(s, "Second lesson") {
		t.Error("missing second lesson")
	}

	// Header should appear only once.
	if strings.Count(s, "# Operational Learnings") != 1 {
		t.Error("header appears more than once")
	}
}

func TestAppendLessons_Deduplication(t *testing.T) {
	dir := t.TempDir()

	lessons := []Lesson{
		{Date: "2026-02-20", PlanID: 42, Author: "arve", Text: "Don't modify CLAUDE.md directly"},
	}

	// Write once.
	if err := AppendLessons(dir, lessons); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Write same lesson again (possibly from different plan/date).
	duplicate := []Lesson{
		{Date: "2026-02-25", PlanID: 99, Author: "bob", Text: "Don't modify CLAUDE.md directly"},
	}
	if err := AppendLessons(dir, duplicate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(LearningsPath(dir))
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	s := string(content)
	count := strings.Count(s, "Don't modify CLAUDE.md directly")
	if count != 1 {
		t.Errorf("expected 1 occurrence, got %d", count)
	}
}

func TestAppendLessons_DeduplicationWithinBatch(t *testing.T) {
	dir := t.TempDir()

	// Same text in a single batch should only appear once.
	lessons := []Lesson{
		{Date: "2026-02-20", PlanID: 42, Author: "arve", Text: "Same lesson"},
		{Date: "2026-02-21", PlanID: 43, Author: "bob", Text: "Same lesson"},
	}

	if err := AppendLessons(dir, lessons); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(LearningsPath(dir))
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	count := strings.Count(string(content), "Same lesson")
	if count != 1 {
		t.Errorf("expected 1 occurrence, got %d", count)
	}
}

func TestAppendLessons_EmptyLessons(t *testing.T) {
	dir := t.TempDir()

	if err := AppendLessons(dir, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should not be created.
	_, err := os.Stat(LearningsPath(dir))
	if !os.IsNotExist(err) {
		t.Error("expected no file to be created for empty lessons")
	}
}

func TestAppendLessons_EmptySlice(t *testing.T) {
	dir := t.TempDir()

	if err := AppendLessons(dir, []Lesson{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := os.Stat(LearningsPath(dir))
	if !os.IsNotExist(err) {
		t.Error("expected no file to be created for empty slice")
	}
}

func TestAppendLessons_MultipleLessons(t *testing.T) {
	dir := t.TempDir()

	lessons := []Lesson{
		{Date: "2026-02-20", PlanID: 42, Author: "arve", Text: "First"},
		{Date: "2026-02-21", PlanID: 42, Author: "arve", Text: "Second"},
		{Date: "2026-02-22", PlanID: 43, Author: "bob", Text: "Third"},
	}

	if err := AppendLessons(dir, lessons); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(LearningsPath(dir))
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "- [2026-02-20, plan #42, arve] First") {
		t.Error("missing first lesson")
	}
	if !strings.Contains(s, "- [2026-02-21, plan #42, arve] Second") {
		t.Error("missing second lesson")
	}
	if !strings.Contains(s, "- [2026-02-22, plan #43, bob] Third") {
		t.Error("missing third lesson")
	}
}

func TestLearningsPath(t *testing.T) {
	got := LearningsPath("/repo")
	want := filepath.Join("/repo", ".claude", "learnings.md")
	if got != want {
		t.Errorf("LearningsPath = %q, want %q", got, want)
	}
}
