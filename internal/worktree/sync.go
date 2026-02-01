// Package worktree manages git worktrees for plan execution.
package worktree

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
)

// SyncToWorktree copies plan, progress, and feedback files from the main worktree
// to the execution worktree. Also copies .env files based on config.worktree.copy_env_files.
//
// For bundles, files are copied to {worktree}/plans/current/{name}/.
// For legacy flat files, uses filepath.Rel() to preserve path structure.
//
// Missing source files are silently skipped (not an error).
func SyncToWorktree(p *plan.Plan, worktreePath string, cfg *config.Config, mainWorktreePath string) error {
	log.Debug("Syncing files to worktree: %s", worktreePath)

	// Files to sync: plan file, progress file, feedback file
	planPath := p.Path
	progressPath := plan.ProgressPath(p)
	feedbackPath := plan.FeedbackPath(p)

	var planDstPath, progressDstPath, feedbackDstPath string

	if p.IsBundle() {
		// Bundle: use p.Name to build destination paths
		// e.g., {worktree}/plans/current/{name}/plan.md
		bundleDst := filepath.Join(worktreePath, "plans", "current", p.Name)
		planDstPath = filepath.Join(bundleDst, "plan.md")
		progressDstPath = filepath.Join(bundleDst, "progress.md")
		feedbackDstPath = filepath.Join(bundleDst, "feedback.md")
	} else {
		// Legacy flat file: use filepath.Rel() to preserve path structure
		planRelPath, err := filepath.Rel(mainWorktreePath, planPath)
		if err != nil {
			planRelPath = filepath.Join("plans", "current", filepath.Base(planPath))
		}
		planDstPath = filepath.Join(worktreePath, planRelPath)

		progressRelPath, err := filepath.Rel(mainWorktreePath, progressPath)
		if err != nil {
			progressRelPath = filepath.Join("plans", "current", filepath.Base(progressPath))
		}
		progressDstPath = filepath.Join(worktreePath, progressRelPath)

		feedbackRelPath, err := filepath.Rel(mainWorktreePath, feedbackPath)
		if err != nil {
			feedbackRelPath = filepath.Join("plans", "current", filepath.Base(feedbackPath))
		}
		feedbackDstPath = filepath.Join(worktreePath, feedbackRelPath)
	}

	// Copy plan file (required)
	if err := copyFile(planPath, planDstPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("copying plan file: %w", err)
		}
		log.Debug("Plan file not found, skipping: %s", planPath)
	} else {
		log.Debug("Copied plan file: %s -> %s", planPath, planDstPath)
	}

	// Copy progress file (optional)
	if err := copyFile(progressPath, progressDstPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("copying progress file: %w", err)
		}
		log.Debug("Progress file not found, skipping: %s", progressPath)
	} else {
		log.Debug("Copied progress file: %s -> %s", progressPath, progressDstPath)
	}

	// Copy feedback file (optional)
	if err := copyFile(feedbackPath, feedbackDstPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("copying feedback file: %w", err)
		}
		log.Debug("Feedback file not found, skipping: %s", feedbackPath)
	} else {
		log.Debug("Copied feedback file: %s -> %s", feedbackPath, feedbackDstPath)
	}

	// Copy .env files based on config
	if cfg != nil && cfg.Worktree.CopyEnvFiles != "" {
		envFiles := parseEnvFileList(cfg.Worktree.CopyEnvFiles)
		for _, envFile := range envFiles {
			srcPath := filepath.Join(mainWorktreePath, envFile)
			dstPath := filepath.Join(worktreePath, envFile)
			if err := copyFile(srcPath, dstPath); err != nil {
				if !os.IsNotExist(err) {
					return fmt.Errorf("copying env file %s: %w", envFile, err)
				}
				log.Debug("Env file not found, skipping: %s", srcPath)
			} else {
				log.Debug("Copied env file: %s -> %s", srcPath, dstPath)
			}
		}
	}

	return nil
}

// SyncFromWorktree copies plan and progress files from the execution worktree
// back to the main worktree. This syncs changes made by the agent back to the queue.
//
// For bundles, files are read from {worktree}/plans/current/{name}/.
// For legacy flat files, uses filepath.Rel() to locate files.
//
// Missing source files are silently skipped (not an error).
// Feedback file is NOT synced back (human input comes from main worktree).
func SyncFromWorktree(p *plan.Plan, worktreePath string, mainWorktreePath string) error {
	log.Debug("Syncing files from worktree: %s", worktreePath)

	// Files to sync back: plan file, progress file (NOT feedback - that's human input)
	planPath := p.Path
	progressPath := plan.ProgressPath(p)

	var planSrcPath, progressSrcPath string

	if p.IsBundle() {
		// Bundle: use p.Name to build source paths
		// e.g., {worktree}/plans/current/{name}/plan.md
		bundleSrc := filepath.Join(worktreePath, "plans", "current", p.Name)
		planSrcPath = filepath.Join(bundleSrc, "plan.md")
		progressSrcPath = filepath.Join(bundleSrc, "progress.md")
	} else {
		// Legacy flat file: use filepath.Rel() to locate files
		planRelPath, err := filepath.Rel(mainWorktreePath, planPath)
		if err != nil {
			planRelPath = filepath.Join("plans", "current", filepath.Base(planPath))
		}
		planSrcPath = filepath.Join(worktreePath, planRelPath)

		progressRelPath, err := filepath.Rel(mainWorktreePath, progressPath)
		if err != nil {
			progressRelPath = filepath.Join("plans", "current", filepath.Base(progressPath))
		}
		progressSrcPath = filepath.Join(worktreePath, progressRelPath)
	}

	// Copy plan file back
	if err := copyFile(planSrcPath, planPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("copying plan file back: %w", err)
		}
		log.Debug("Plan file not found in worktree, skipping: %s", planSrcPath)
	} else {
		log.Debug("Copied plan file back: %s -> %s", planSrcPath, planPath)
	}

	// Copy progress file back
	if err := copyFile(progressSrcPath, progressPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("copying progress file back: %w", err)
		}
		log.Debug("Progress file not found in worktree, skipping: %s", progressSrcPath)
	} else {
		log.Debug("Copied progress file back: %s -> %s", progressSrcPath, progressPath)
	}

	return nil
}

// copyFile copies a file from src to dst, preserving file permissions.
// Creates destination directory if it doesn't exist.
// Returns os.ErrNotExist if source file doesn't exist.
func copyFile(src, dst string) error {
	// Check if source exists
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err // Will be os.ErrNotExist if file doesn't exist
	}

	// Create destination directory if needed
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create destination file with same permissions
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return nil
}

// parseEnvFileList parses a comma-separated list of env file names.
// Trims whitespace from each entry.
// Example: ".env, .env.local" -> [".env", ".env.local"]
func parseEnvFileList(list string) []string {
	if list == "" {
		return nil
	}

	parts := strings.Split(list, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}
