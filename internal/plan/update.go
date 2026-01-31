// Package plan handles plan parsing and queue management.
package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrNoCheckbox is returned when the target line doesn't contain a checkbox.
var ErrNoCheckbox = errors.New("line does not contain a checkbox")

// ErrInvalidLine is returned when the line number is out of range.
var ErrInvalidLine = errors.New("line number out of range")

// checkboxUpdateRegex matches the checkbox portion of a line for updating.
// It captures everything before and after the [ ] or [x] to preserve formatting.
// Group 1: everything before the bracket (e.g., "  - ")
// Group 2: the checkbox character (space or x/X)
// Group 3: everything after the bracket (e.g., "] Task text")
var checkboxUpdateRegex = regexp.MustCompile(`^(.*-\s*\[)([ xX])(\].*)$`)

// UpdateCheckbox modifies a specific checkbox in the plan content.
// lineNum is 1-indexed (first line is line 1).
// complete=true changes [ ] to [x], complete=false changes [x] to [ ].
// Returns the modified content, or an error if the line doesn't contain a checkbox.
// Preserves exact whitespace and formatting around the checkbox.
func UpdateCheckbox(content string, lineNum int, complete bool) (string, error) {
	lines := strings.Split(content, "\n")

	// Validate line number (1-indexed)
	if lineNum < 1 || lineNum > len(lines) {
		return "", fmt.Errorf("%w: %d (valid range 1-%d)", ErrInvalidLine, lineNum, len(lines))
	}

	lineIdx := lineNum - 1
	line := lines[lineIdx]

	// Match checkbox pattern
	match := checkboxUpdateRegex.FindStringSubmatch(line)
	if match == nil {
		return "", fmt.Errorf("%w: line %d", ErrNoCheckbox, lineNum)
	}

	// Determine new checkbox character
	newChar := " "
	if complete {
		newChar = "x"
	}

	// Reconstruct line with new checkbox state
	lines[lineIdx] = match[1] + newChar + match[3]

	return strings.Join(lines, "\n"), nil
}

// SetCheckbox is a convenience method that updates a checkbox in the plan
// and updates the Plan's Content field.
func (p *Plan) SetCheckbox(lineNum int, complete bool) error {
	newContent, err := UpdateCheckbox(p.Content, lineNum, complete)
	if err != nil {
		return err
	}
	p.Content = newContent
	// Re-extract tasks to keep Tasks in sync with Content
	p.Tasks = ExtractTasks(p.Content)
	return nil
}

// Save writes the plan content to its file path.
// Uses atomic write (write to temp file, then rename) to prevent corruption on crash.
func Save(plan *Plan) error {
	if plan == nil {
		return errors.New("plan is nil")
	}
	if plan.Path == "" {
		return errors.New("plan path is empty")
	}

	// Create temp file in same directory for atomic rename
	dir := filepath.Dir(plan.Path)
	tmpFile, err := os.CreateTemp(dir, ".plan-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file if something goes wrong
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	// Write content to temp file
	_, err = tmpFile.WriteString(plan.Content)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Close temp file before rename
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Preserve original file permissions if file exists
	info, err := os.Stat(plan.Path)
	if err == nil {
		// File exists, use its permissions
		if err := os.Chmod(tmpPath, info.Mode()); err != nil {
			return fmt.Errorf("failed to set file permissions: %w", err)
		}
	}

	// Atomic rename
	if err := os.Rename(tmpPath, plan.Path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	success = true
	return nil
}
