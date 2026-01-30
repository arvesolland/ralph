// Package plan handles plan parsing and queue management.
package plan

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Plan represents a parsed plan file.
type Plan struct {
	// Path is the absolute path to the plan file.
	Path string

	// Name is derived from the filename without extension (e.g., "go-rewrite" from "go-rewrite.md").
	Name string

	// Content is the raw markdown content of the plan.
	Content string

	// Tasks will be populated by ExtractTasks (implemented in T9).
	Tasks []Task

	// Status is extracted from the plan content (e.g., "pending", "open", "complete").
	// Defaults to "pending" if not found.
	Status string

	// Branch is the git branch name for this plan (e.g., "feat/go-rewrite").
	Branch string
}

// Task is a placeholder for task extraction (implemented in T9).
type Task struct {
	Line     int
	Text     string
	Complete bool
	Requires []string
	Subtasks []Task
}

// statusRegex matches **Status:** value patterns in markdown.
var statusRegex = regexp.MustCompile(`(?m)^\*\*Status:\*\*\s*(\S+)`)

// Load reads and parses a plan file from the given path.
// It extracts the name, status, and branch from the content.
// Returns an error if the file cannot be read.
func Load(path string) (*Plan, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	name := deriveName(absPath)
	status := extractStatus(string(content))
	branch := deriveBranch(name)

	return &Plan{
		Path:    absPath,
		Name:    name,
		Content: string(content),
		Status:  status,
		Branch:  branch,
	}, nil
}

// deriveName extracts the plan name from the file path.
// "go-rewrite.md" → "go-rewrite"
// "plans/current/my-plan.md" → "my-plan"
func deriveName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// extractStatus finds the **Status:** value in the plan content.
// Returns "pending" if not found.
func extractStatus(content string) string {
	matches := statusRegex.FindStringSubmatch(content)
	if len(matches) >= 2 {
		return strings.ToLower(matches[1])
	}
	return "pending"
}

// deriveBranch creates a git branch name from the plan name.
// "go-rewrite" → "feat/go-rewrite"
// "my plan (v2)" → "feat/my-plan-v2"
func deriveBranch(name string) string {
	sanitized := sanitizeBranchName(name)
	return "feat/" + sanitized
}

// sanitizeBranchName converts a plan name to a valid git branch name.
// - Converts to lowercase
// - Replaces spaces with hyphens
// - Removes special characters except hyphens and alphanumerics
// - Collapses multiple hyphens to single hyphen
// - Trims leading/trailing hyphens
func sanitizeBranchName(name string) string {
	// Convert to lowercase
	result := strings.ToLower(name)

	// Replace spaces and underscores with hyphens
	result = strings.ReplaceAll(result, " ", "-")
	result = strings.ReplaceAll(result, "_", "-")

	// Remove special characters (keep only alphanumeric and hyphen)
	var cleaned strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			cleaned.WriteRune(r)
		}
	}
	result = cleaned.String()

	// Collapse multiple hyphens to single hyphen
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}

	// Trim leading/trailing hyphens
	result = strings.Trim(result, "-")

	return result
}
