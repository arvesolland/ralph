package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	learningsDir  = ".claude"
	learningsFile = "learnings.md"
	learningsHeader = `# Operational Learnings

Automated learnings from Ralph development iterations. Read by fresh Claude Code sessions.

---
`
)

// LearningsPath returns the path to the learnings file within a repo.
func LearningsPath(repoPath string) string {
	return filepath.Join(repoPath, learningsDir, learningsFile)
}

// AppendLessons writes lessons to .claude/learnings.md in the target repo.
// Creates the directory and file with header if they don't exist.
// Deduplicates by matching lesson text body (ignoring the date/plan/author prefix).
func AppendLessons(repoPath string, lessons []Lesson) error {
	if len(lessons) == 0 {
		return nil
	}

	dir := filepath.Join(repoPath, learningsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .claude directory: %w", err)
	}

	filePath := LearningsPath(repoPath)

	existing, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read learnings file: %w", err)
	}

	content := string(existing)
	if content == "" {
		content = learningsHeader
	}

	// Extract existing lesson text bodies for deduplication.
	existingBodies := extractBodies(content)

	var newEntries []string
	for _, lesson := range lessons {
		if existingBodies[lesson.Text] {
			continue
		}
		entry := fmt.Sprintf("- [%s, plan #%d, %s] %s", lesson.Date, lesson.PlanID, lesson.Author, lesson.Text)
		newEntries = append(newEntries, entry)
		existingBodies[lesson.Text] = true
	}

	if len(newEntries) == 0 {
		return nil
	}

	// Ensure content ends with a newline before appending.
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	content += strings.Join(newEntries, "\n") + "\n"

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write learnings file: %w", err)
	}

	return nil
}

// extractBodies parses existing bullet entries and returns a set of their text bodies.
// Each entry is expected to be: `- [date, plan #ID, author] Text body`
func extractBodies(content string) map[string]bool {
	bodies := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		// Find the closing bracket of the prefix.
		idx := strings.Index(line, "] ")
		if idx == -1 {
			continue
		}
		body := line[idx+2:]
		if body != "" {
			bodies[body] = true
		}
	}
	return bodies
}
