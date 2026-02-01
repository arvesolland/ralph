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
	// For bundles, this is the directory name.
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

	// BundleDir is the absolute path to the bundle directory.
	// Empty string indicates a legacy flat file (not a bundle).
	BundleDir string
}

// statusRegex matches **Status:** value patterns in markdown.
var statusRegex = regexp.MustCompile(`(?m)^\*\*Status:\*\*\s*(\S+)`)

// IsBundle returns true if the plan is a bundle (directory-based),
// false if it's a legacy flat file.
func (p *Plan) IsBundle() bool {
	return p.BundleDir != ""
}

// Load reads and parses a plan file from the given path.
// It extracts the name, status, and branch from the content.
// If path is a directory (bundle), it loads plan.md from inside.
// Returns an error if the file cannot be read.
func Load(path string) (*Plan, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	// Check if path is a directory (bundle) or file
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	var bundleDir string
	var planPath string

	if info.IsDir() {
		// Bundle: directory containing plan.md
		bundleDir = absPath
		planPath = filepath.Join(absPath, "plan.md")
	} else {
		// Legacy flat file
		bundleDir = ""
		planPath = absPath
	}

	content, err := os.ReadFile(planPath)
	if err != nil {
		return nil, err
	}

	name := deriveName(absPath, bundleDir != "")
	status := extractStatus(string(content))
	branch := deriveBranch(name)
	tasks := ExtractTasks(string(content))

	return &Plan{
		Path:      planPath,
		Name:      name,
		Content:   string(content),
		Tasks:     tasks,
		Status:    status,
		Branch:    branch,
		BundleDir: bundleDir,
	}, nil
}

// deriveName extracts the plan name from the path.
// For bundles (isBundle=true): uses directory name as-is.
// For flat files: removes .md extension from filename.
//
// Examples:
//   - "plans/pending/my-bundle" (bundle) → "my-bundle"
//   - "go-rewrite.md" (flat file) → "go-rewrite"
//   - "plans/current/my-plan.md" (flat file) → "my-plan"
func deriveName(path string, isBundle bool) string {
	base := filepath.Base(path)
	if isBundle {
		// Bundle: directory name is the plan name
		return base
	}
	// Flat file: remove extension
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
