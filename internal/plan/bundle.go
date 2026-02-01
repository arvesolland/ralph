package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// planTemplate is the template for a new plan.md file.
const planTemplate = `# Plan: %s

**Spec:** <!-- Link to spec if applicable -->
**Created:** %s
**Status:** pending

## Context

<!-- Brief description of what this plan accomplishes -->

### Gotchas

<!-- Known challenges or things to watch out for -->

---

## Rules

1. **Pick task:** First task where status is not ` + "`complete`" + ` and all ` + "`Requires`" + ` are ` + "`complete`" + `
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" checked and verified, then set Status: ` + "`complete`" + `
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Tasks

### T1: First task
> Brief description of what this task accomplishes

**Requires:** —
**Status:** pending

**Done when:**
- [ ] First acceptance criterion
- [ ] Second acceptance criterion

**Subtasks:**
1. [ ] First subtask
2. [ ] Second subtask

---

## Discovered

<!-- Tasks found during implementation -->

---

## Completed

<!-- Completion dates will be added here -->
`

// progressTemplate is the template for a new progress.md file.
const progressTemplate = `# Progress: %s

Iteration log - what was done, gotchas, and next steps.

<!--
FORMAT FOR EACH ITERATION:
---
### Iteration N: Task identifier
**Completed:** What you actually did - be specific about files changed
**Gotcha:** Optional - surprises, edge cases, things that didn't work
**Next:** What the next iteration should tackle
-->
`

// feedbackTemplate is the template for a new feedback.md file.
const feedbackTemplate = `# Feedback: %s

Human input and responses to blockers.

## Pending

<!--
Add feedback items here in this format:
- [YYYY-MM-DD HH:MM] Your message here

The agent will read these and act on them in the next iteration.
-->

## Processed

<!-- Agent moves processed items here after acting on them -->
`

// CreateBundle creates a new plan bundle directory with all scaffolded files.
// It creates the directory in plansDir/pending/ with the given name.
// Returns the loaded Plan or an error if creation fails.
func CreateBundle(plansDir, name string) (*Plan, error) {
	// Validate name
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("plan name cannot be empty")
	}

	// Sanitize name for directory
	sanitizedName := sanitizeBundleName(name)
	if sanitizedName == "" {
		return nil, fmt.Errorf("plan name '%s' results in empty directory name after sanitization", name)
	}

	// Create bundle path in pending/
	bundleDir := filepath.Join(plansDir, "pending", sanitizedName)

	// Check if already exists
	if _, err := os.Stat(bundleDir); err == nil {
		return nil, fmt.Errorf("plan '%s' already exists at %s", name, bundleDir)
	}

	// Create the bundle directory
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bundle directory: %w", err)
	}

	// Scaffold all files
	if err := scaffoldPlan(bundleDir, name); err != nil {
		// Clean up on failure
		os.RemoveAll(bundleDir)
		return nil, fmt.Errorf("failed to scaffold plan.md: %w", err)
	}

	if err := scaffoldProgress(bundleDir, name); err != nil {
		os.RemoveAll(bundleDir)
		return nil, fmt.Errorf("failed to scaffold progress.md: %w", err)
	}

	if err := scaffoldFeedback(bundleDir, name); err != nil {
		os.RemoveAll(bundleDir)
		return nil, fmt.Errorf("failed to scaffold feedback.md: %w", err)
	}

	// Load and return the created plan
	return Load(bundleDir)
}

// scaffoldPlan creates the plan.md file with a template.
func scaffoldPlan(bundleDir, name string) error {
	path := filepath.Join(bundleDir, "plan.md")
	today := time.Now().Format("2006-01-02")
	content := fmt.Sprintf(planTemplate, name, today)
	return os.WriteFile(path, []byte(content), 0644)
}

// scaffoldProgress creates the progress.md file with a header and format instructions.
func scaffoldProgress(bundleDir, name string) error {
	path := filepath.Join(bundleDir, "progress.md")
	content := fmt.Sprintf(progressTemplate, name)
	return os.WriteFile(path, []byte(content), 0644)
}

// scaffoldFeedback creates the feedback.md file with Pending/Processed sections.
func scaffoldFeedback(bundleDir, name string) error {
	path := filepath.Join(bundleDir, "feedback.md")
	content := fmt.Sprintf(feedbackTemplate, name)
	return os.WriteFile(path, []byte(content), 0644)
}

// MigrateToBundles converts all flat plan files to bundle directories.
// It iterates through pending/, current/, and complete/ directories,
// converting each .md file (that isn't .progress.md or .feedback.md) into a bundle.
// Existing bundles (directories) are skipped.
// Associated .progress.md and .feedback.md files are moved into the bundle.
// If associated files are missing, scaffolded versions are created.
func MigrateToBundles(plansDir string) error {
	subdirs := []string{"pending", "current", "complete"}

	for _, subdir := range subdirs {
		dirPath := filepath.Join(plansDir, subdir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			// Directory doesn't exist, skip
			continue
		}

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return fmt.Errorf("failed to read %s directory: %w", subdir, err)
		}

		for _, entry := range entries {
			// Skip directories (already bundles or not plan files)
			if entry.IsDir() {
				continue
			}

			// Skip non-.md files
			if !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}

			// Skip .progress.md and .feedback.md files (associated files)
			if strings.HasSuffix(entry.Name(), ".progress.md") ||
				strings.HasSuffix(entry.Name(), ".feedback.md") {
				continue
			}

			// This is a flat plan file - migrate it
			planPath := filepath.Join(dirPath, entry.Name())
			if err := migratePlanFile(planPath); err != nil {
				return fmt.Errorf("failed to migrate %s: %w", planPath, err)
			}
		}
	}

	return nil
}

// migratePlanFile converts a single flat plan file to a bundle directory.
func migratePlanFile(planPath string) error {
	dir := filepath.Dir(planPath)
	filename := filepath.Base(planPath)

	// Derive bundle name from filename (remove .md extension)
	bundleName := strings.TrimSuffix(filename, ".md")

	// Create bundle directory
	bundleDir := filepath.Join(dir, bundleName)
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return fmt.Errorf("failed to create bundle directory: %w", err)
	}

	// Move plan file into bundle as plan.md
	destPlanPath := filepath.Join(bundleDir, "plan.md")
	if err := os.Rename(planPath, destPlanPath); err != nil {
		return fmt.Errorf("failed to move plan file: %w", err)
	}

	// Look for associated progress file and move/create it
	progressSrc := filepath.Join(dir, bundleName+".progress.md")
	progressDest := filepath.Join(bundleDir, "progress.md")
	if _, err := os.Stat(progressSrc); err == nil {
		// Progress file exists - move it
		if err := os.Rename(progressSrc, progressDest); err != nil {
			return fmt.Errorf("failed to move progress file: %w", err)
		}
	} else {
		// No progress file - create scaffolded version
		if err := scaffoldProgress(bundleDir, bundleName); err != nil {
			return fmt.Errorf("failed to scaffold progress file: %w", err)
		}
	}

	// Look for associated feedback file and move/create it
	feedbackSrc := filepath.Join(dir, bundleName+".feedback.md")
	feedbackDest := filepath.Join(bundleDir, "feedback.md")
	if _, err := os.Stat(feedbackSrc); err == nil {
		// Feedback file exists - move it
		if err := os.Rename(feedbackSrc, feedbackDest); err != nil {
			return fmt.Errorf("failed to move feedback file: %w", err)
		}
	} else {
		// No feedback file - create scaffolded version
		if err := scaffoldFeedback(bundleDir, bundleName); err != nil {
			return fmt.Errorf("failed to scaffold feedback file: %w", err)
		}
	}

	return nil
}

// sanitizeBundleName converts a plan name to a valid directory name.
// Similar to sanitizeBranchName but preserves case.
func sanitizeBundleName(name string) string {
	// Replace spaces and underscores with hyphens
	result := strings.ReplaceAll(name, " ", "-")
	result = strings.ReplaceAll(result, "_", "-")

	// Remove special characters (keep only alphanumeric, hyphen, and period)
	var cleaned strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
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

	// Convert to lowercase for consistency
	return strings.ToLower(result)
}
