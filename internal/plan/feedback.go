// Package plan handles plan parsing and queue management.
package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FeedbackPath returns the path to the feedback file for a plan.
// The feedback file is named "<plan-name>.feedback.md" in the same directory as the plan.
// Example: "plans/current/go-rewrite.md" → "plans/current/go-rewrite.feedback.md"
func FeedbackPath(plan *Plan) string {
	ext := filepath.Ext(plan.Path)
	return strings.TrimSuffix(plan.Path, ext) + ".feedback.md"
}

// feedbackEntryRegex matches a feedback entry line like "- [2024-01-30 14:32] content"
var feedbackEntryRegex = regexp.MustCompile(`^- \[\d{4}-\d{2}-\d{2} \d{2}:\d{2}\] .+`)

// ReadFeedback reads the pending feedback entries from a plan's feedback file.
// Returns an empty string if the file doesn't exist or has no pending entries.
// Returns only the content of the "## Pending" section.
func ReadFeedback(plan *Plan) (string, error) {
	path := FeedbackPath(plan)

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading feedback file: %w", err)
	}

	return extractPendingSection(string(content)), nil
}

// extractPendingSection extracts the content of the "## Pending" section from feedback file content.
// Returns an empty string if the section doesn't exist or is empty.
func extractPendingSection(content string) string {
	lines := strings.Split(content, "\n")
	var pendingLines []string
	inPending := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for section headers
		if strings.HasPrefix(trimmed, "## ") {
			if strings.EqualFold(trimmed, "## Pending") {
				inPending = true
				continue
			} else {
				// Another section starts, stop collecting
				if inPending {
					break
				}
				continue
			}
		}

		// Collect lines in pending section (skip empty lines and comments)
		if inPending && trimmed != "" && !strings.HasPrefix(trimmed, "<!--") {
			pendingLines = append(pendingLines, line)
		}
	}

	if len(pendingLines) == 0 {
		return ""
	}

	return strings.Join(pendingLines, "\n")
}

// AppendFeedback appends a new timestamped entry to the Pending section of the feedback file.
// Creates the file with proper structure if it doesn't exist.
// Entry format: - [YYYY-MM-DD HH:MM] source: content
func AppendFeedback(plan *Plan, source string, content string) error {
	return AppendFeedbackWithTime(plan, source, content, time.Now())
}

// AppendFeedbackWithTime is like AppendFeedback but allows specifying the timestamp.
// Useful for testing.
func AppendFeedbackWithTime(plan *Plan, source string, content string, timestamp time.Time) error {
	path := FeedbackPath(plan)

	// Read existing content (or create default structure)
	existing, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("reading feedback file: %w", err)
		}
		// File doesn't exist, will create with default structure
		existing = []byte("")
	}

	// Format timestamp
	ts := timestamp.Format("2006-01-02 15:04")

	// Build entry line
	var entry string
	if source != "" {
		entry = fmt.Sprintf("- [%s] %s: %s", ts, source, content)
	} else {
		entry = fmt.Sprintf("- [%s] %s", ts, content)
	}

	// Update file content
	newContent := insertIntoPendingSection(string(existing), entry, plan.Name)

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating feedback directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("writing feedback file: %w", err)
	}

	return nil
}

// insertIntoPendingSection inserts an entry into the Pending section.
// Creates the file structure if it doesn't exist.
func insertIntoPendingSection(content string, entry string, planName string) string {
	// If file is empty or doesn't have proper structure, create it
	if content == "" || !strings.Contains(content, "## Pending") {
		return fmt.Sprintf("# Feedback: %s\n\n## Pending\n%s\n\n## Processed\n", planName, entry)
	}

	lines := strings.Split(content, "\n")
	var result []string
	inserted := false
	inPending := false

	for i, line := range lines {
		result = append(result, line)
		trimmed := strings.TrimSpace(line)

		// Check for Pending header
		if strings.EqualFold(trimmed, "## Pending") {
			inPending = true
			continue
		}

		// When we hit another section header after Pending, insert before it
		if inPending && strings.HasPrefix(trimmed, "## ") {
			// Insert entry before this header (before the last appended line)
			result = result[:len(result)-1]
			result = append(result, entry)
			result = append(result, "")
			result = append(result, line)
			inserted = true
			inPending = false
			continue
		}

		// If we're at end of pending and haven't hit another section,
		// insert after the last entry in pending
		if inPending && i == len(lines)-1 && !inserted {
			result = append(result, entry)
			inserted = true
		}
	}

	// If we were in pending but never found the end, add at the end
	if inPending && !inserted {
		result = append(result, entry)
	}

	return strings.Join(result, "\n")
}

// MarkProcessed moves an entry from the Pending section to the Processed section.
// The entry parameter should be the full text of the entry line to move (including timestamp).
// Returns an error if the entry is not found in Pending.
func MarkProcessed(plan *Plan, entry string) error {
	path := FeedbackPath(plan)

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("feedback file does not exist")
		}
		return fmt.Errorf("reading feedback file: %w", err)
	}

	newContent, found := moveEntryToProcessed(string(content), entry)
	if !found {
		return fmt.Errorf("entry not found in Pending section")
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("writing feedback file: %w", err)
	}

	return nil
}

// moveEntryToProcessed moves an entry from Pending to Processed section.
// Returns the new content and whether the entry was found.
func moveEntryToProcessed(content string, entry string) (string, bool) {
	lines := strings.Split(content, "\n")
	var result []string
	var processedLines []string
	found := false
	inPending := false
	inProcessed := false
	processedIndex := -1

	// Normalize entry for comparison (trim whitespace)
	entryNormalized := strings.TrimSpace(entry)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track sections
		if strings.EqualFold(trimmed, "## Pending") {
			inPending = true
			inProcessed = false
			result = append(result, line)
			continue
		}
		if strings.EqualFold(trimmed, "## Processed") {
			inPending = false
			inProcessed = true
			processedIndex = len(result)
			result = append(result, line)
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			inPending = false
			inProcessed = false
		}

		// In pending section, look for the entry to remove
		if inPending && strings.TrimSpace(line) == entryNormalized {
			found = true
			// Don't add this line to result (removing from pending)
			continue
		}

		// Collect processed lines separately to ensure entry is added there
		if inProcessed {
			processedLines = append(processedLines, line)
		} else {
			result = append(result, line)
		}

		// Last line handling
		if i == len(lines)-1 && !inProcessed && processedIndex == -1 {
			// No Processed section exists, we need to create one
		}
	}

	if !found {
		return content, false
	}

	// If no Processed section exists, create one
	if processedIndex == -1 {
		result = append(result, "")
		result = append(result, "## Processed")
		result = append(result, entryNormalized)
		return strings.Join(result, "\n"), true
	}

	// Insert the entry at the beginning of processed section
	finalResult := make([]string, 0, len(result)+len(processedLines)+1)
	finalResult = append(finalResult, result[:processedIndex+1]...)
	finalResult = append(finalResult, entryNormalized)
	finalResult = append(finalResult, processedLines...)
	finalResult = append(finalResult, result[processedIndex+1:]...)

	return strings.Join(finalResult, "\n"), true
}

// CreateFeedbackFile creates a new feedback file with proper structure if it doesn't exist.
// If the file already exists, does nothing.
func CreateFeedbackFile(plan *Plan) error {
	path := FeedbackPath(plan)

	// Check if file exists
	if _, err := os.Stat(path); err == nil {
		// File exists, do nothing
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking feedback file: %w", err)
	}

	// Create with structure
	content := fmt.Sprintf("# Feedback: %s\n\n## Pending\n\n## Processed\n", plan.Name)

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating feedback directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("creating feedback file: %w", err)
	}

	return nil
}
