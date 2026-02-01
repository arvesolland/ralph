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
// For bundles: returns "{bundleDir}/progress.md"
// For flat files: returns "<plan-name>.progress.md" in the same directory as the plan.
// Example (bundle): "plans/current/my-plan/" → "plans/current/my-plan/progress.md"
// Example (flat): "plans/current/go-rewrite.md" → "plans/current/go-rewrite.progress.md"
func ProgressPath(plan *Plan) string {
	if plan.IsBundle() {
		return filepath.Join(plan.BundleDir, "progress.md")
	}
	// Legacy flat file path
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

// stripTemplateComments removes the HTML comment block containing format instructions
// from scaffolded progress files. This is called on first real entry to clean up the file.
func stripTemplateComments(content string) string {
	// Find and remove the template comment block
	start := strings.Index(content, "<!--\nFORMAT FOR EACH ITERATION:")
	if start == -1 {
		return content
	}
	end := strings.Index(content[start:], "-->")
	if end == -1 {
		return content
	}
	// Remove the comment block and any trailing newline
	end += start + 3 // 3 = len("-->")
	result := content[:start] + content[end:]
	// Trim excess whitespace but keep the header format
	result = strings.TrimRight(result, "\n") + "\n"
	return result
}

// AppendProgress appends a new timestamped entry to the progress file.
// Creates the file if it doesn't exist.
// Entry format:
//
//	## Iteration N (YYYY-MM-DD HH:MM) - X/Y (Z%)
//	{content}
func AppendProgress(plan *Plan, iteration int, content string) error {
	path := ProgressPath(plan)

	// Read existing content (or empty string if file doesn't exist)
	existing, err := ReadProgress(plan)
	if err != nil {
		return err
	}

	// Strip template comments on first real entry (iteration 1)
	if iteration == 1 {
		existing = stripTemplateComments(existing)
	}

	// Generate timestamp
	timestamp := time.Now().Format("2006-01-02 15:04")

	// Calculate progress
	progress := CalculateProgress(plan.Tasks)

	// Build new entry with progress
	entry := fmt.Sprintf("\n## Iteration %d (%s) - %s\n%s\n", iteration, timestamp, progress.String(), content)

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

	// Strip template comments on first real entry (iteration 1)
	if iteration == 1 {
		existing = stripTemplateComments(existing)
	}

	// Format timestamp
	ts := timestamp.Format("2006-01-02 15:04")

	// Calculate progress
	progress := CalculateProgress(plan.Tasks)

	// Build new entry with progress
	entry := fmt.Sprintf("\n## Iteration %d (%s) - %s\n%s\n", iteration, ts, progress.String(), content)

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
