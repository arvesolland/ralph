// Package cli provides the command-line interface for ralph.
package cli

import (
	"fmt"
	"os"

	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/worktree"
	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove orphaned worktrees",
	Long: `Remove worktrees that no longer have associated plans.

A worktree is considered orphaned if it exists in .ralph/worktrees/ but
has no matching plan in pending/ or current/.

For safety, worktrees with uncommitted changes are NOT removed.
Use --dry-run to see what would be removed without actually removing anything.`,
	RunE: runCleanup,
}

var (
	cleanupDryRun bool
)

func init() {
	rootCmd.AddCommand(cleanupCmd)
	cleanupCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Show what would be removed without removing anything")
}

func runCleanup(cmd *cobra.Command, args []string) error {
	// Initialize git to find repo root
	g := git.NewGit(".")
	repoRoot, err := g.RepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Check if ralph is initialized
	worktreesDir := ".ralph/worktrees"
	if _, err := os.Stat(worktreesDir); os.IsNotExist(err) {
		fmt.Println("No worktrees directory found. Nothing to clean up.")
		return nil
	}

	// Create worktree manager
	manager, err := worktree.NewManager(g, worktreesDir)
	if err != nil {
		return fmt.Errorf("creating worktree manager: %w", err)
	}

	// Create queue for active plan lookup
	plansDir := "plans"
	queue := plan.NewQueue(plansDir)

	if cleanupDryRun {
		fmt.Println("Dry run - no changes will be made")
		fmt.Println()
	}

	// Run cleanup
	results, err := manager.Cleanup(queue)
	if err != nil {
		return fmt.Errorf("cleaning up worktrees: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No orphaned worktrees found.")
		return nil
	}

	// Report results
	removedCount := 0
	skippedCount := 0

	for _, result := range results {
		if result.Skipped {
			skippedCount++
			if cleanupDryRun {
				log.Warn("Would skip: %s (%s)", result.Path, result.SkipReason)
			} else {
				log.Warn("Skipped: %s (%s)", result.Path, result.SkipReason)
			}
		} else {
			removedCount++
			if cleanupDryRun {
				log.Info("Would remove: %s", result.Path)
			} else {
				log.Success("Removed: %s", result.Path)
			}
		}
	}

	fmt.Println()
	if cleanupDryRun {
		fmt.Printf("Would remove %d worktree(s), skip %d\n", removedCount, skippedCount)
	} else {
		fmt.Printf("Removed %d worktree(s), skipped %d\n", removedCount, skippedCount)
	}

	// Store repo root for reference (unused but avoids warning)
	_ = repoRoot

	return nil
}
