// Package plan handles plan parsing and queue management.
package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProgressPath returns the path to the progress file for a plan.
// The progress file is named "<plan-name>.progress.md" in the same directory as the plan.
// Example: "plans/current/go-rewrite.md" → "plans/current/go-rewrite.progress.md"
func ProgressPath(plan *Plan) string {
	ext := filepath.Ext(plan.Path)
	return strings.TrimSuffix(plan.Path, ext) + ".progress.md"
}

// ReadProgress reads the existing progress file content for a plan.
// Returns an empty string if the file doesn't exist.
// Returns an error only if the file exists but cannot be read.
func ReadProgress(plan *Plan) (string, error) {
	path := ProgressPath(plan)

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading progress file: %w", err)
	}

	return string(content), nil
}

// AppendProgress appends a new timestamped entry to the progress file.
// Creates the file if it doesn't exist.
// Entry format:
//
//	## Iteration N (YYYY-MM-DD HH:MM)
//	{content}
func AppendProgress(plan *Plan, iteration int, content string) error {
	path := ProgressPath(plan)

	// Read existing content (or empty string if file doesn't exist)
	existing, err := ReadProgress(plan)
	if err != nil {
		return err
	}

	// Generate timestamp
	timestamp := time.Now().Format("2006-01-02 15:04")

	// Build new entry
	entry := fmt.Sprintf("\n## Iteration %d (%s)\n%s\n", iteration, timestamp, content)

	// Append to existing content
	newContent := existing + entry

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating progress directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("writing progress file: %w", err)
	}

	return nil
}

// AppendProgressWithTime is like AppendProgress but allows specifying the timestamp.
// Useful for testing.
func AppendProgressWithTime(plan *Plan, iteration int, content string, timestamp time.Time) error {
	path := ProgressPath(plan)

	// Read existing content (or empty string if file doesn't exist)
	existing, err := ReadProgress(plan)
	if err != nil {
		return err
	}

	// Format timestamp
	ts := timestamp.Format("2006-01-02 15:04")

	// Build new entry
	entry := fmt.Sprintf("\n## Iteration %d (%s)\n%s\n", iteration, ts, content)

	// Append to existing content
	newContent := existing + entry

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating progress directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("writing progress file: %w", err)
	}

	return nil
}

// CreateProgressFile creates a new progress file with a header if it doesn't exist.
// If the file already exists, does nothing.
func CreateProgressFile(plan *Plan) error {
	path := ProgressPath(plan)

	// Check if file exists
	if _, err := os.Stat(path); err == nil {
		// File exists, do nothing
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking progress file: %w", err)
	}

	// Create with header
	header := fmt.Sprintf("# Progress: %s\n\nIteration log - what was done, gotchas, and next steps.\n", plan.Name)

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating progress directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(header), 0644); err != nil {
		return fmt.Errorf("creating progress file: %w", err)
	}

	return nil
}
