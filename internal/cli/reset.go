// Package cli provides the command-line interface for ralph.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/worktree"
	"github.com/spf13/cobra"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Move current plan back to pending",
	Long: `Reset the current plan by moving it from current/ back to pending/.

This is useful when you want to restart a plan from scratch, or when
a plan was interrupted and you want to start over.

If a worktree exists for the plan, it will be removed (unless --keep-worktree
is specified).

Use --hard to fully reset the plan:
  - Delete the feature branch
  - Reset progress.md and feedback.md to their initial templates
  - Preserves the original plan.md file

By default, prompts for confirmation before resetting.`,
	RunE: runReset,
}

var (
	resetForce        bool
	resetKeepWorktree bool
	resetHard         bool
)

func init() {
	rootCmd.AddCommand(resetCmd)
	resetCmd.Flags().BoolVarP(&resetForce, "force", "f", false, "Skip confirmation prompt")
	resetCmd.Flags().BoolVar(&resetKeepWorktree, "keep-worktree", false, "Don't remove the worktree")
	resetCmd.Flags().BoolVar(&resetHard, "hard", false, "Delete feature branch and reset progress/feedback files")
}

func runReset(cmd *cobra.Command, args []string) error {
	// Initialize git to find repo root
	g := git.NewGit(".")
	repoRoot, err := g.RepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Create queue
	plansDir := "plans"
	queue := plan.NewQueue(plansDir)

	// Get current plan
	current, err := queue.Current()
	if err != nil {
		return fmt.Errorf("checking current plan: %w", err)
	}

	if current == nil {
		return fmt.Errorf("no current plan to reset")
	}

	// Confirm unless --force
	if !resetForce {
		if resetHard {
			fmt.Printf("Hard reset plan '%s' back to pending?\n", current.Name)
		} else {
			fmt.Printf("Reset plan '%s' back to pending?\n", current.Name)
		}
		fmt.Printf("Branch: %s\n", current.Branch)

		// Check if worktree exists
		worktreesDir := ".ralph/worktrees"
		manager, err := worktree.NewManager(g, worktreesDir)
		if err == nil && manager.Exists(current) {
			if resetKeepWorktree {
				fmt.Println("Worktree will be kept")
			} else {
				fmt.Printf("Worktree at %s will be removed\n", manager.Path(current))
			}
		}

		if resetHard {
			fmt.Printf("Branch '%s' will be deleted\n", current.Branch)
			if current.IsBundle() {
				fmt.Println("progress.md and feedback.md will be reset")
			}
		}

		fmt.Print("\nContinue? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Remove worktree if it exists and --keep-worktree is not set
	if !resetKeepWorktree {
		worktreesDir := ".ralph/worktrees"
		manager, err := worktree.NewManager(g, worktreesDir)
		if err == nil && manager.Exists(current) {
			log.Info("Removing worktree...")
			// Delete branch only if --hard is set
			if err := manager.Remove(current, resetHard); err != nil {
				log.Warn("Failed to remove worktree: %v", err)
				// Continue anyway - the reset itself is more important
			} else {
				log.Success("Worktree removed")
			}
		}
	}

	// Delete branch if --hard and worktree was kept (branch wasn't deleted above)
	if resetHard && resetKeepWorktree {
		log.Info("Deleting branch '%s'...", current.Branch)
		if err := g.DeleteBranch(current.Branch, true); err != nil {
			log.Warn("Failed to delete branch: %v", err)
		} else {
			log.Success("Branch deleted")
		}
	}

	// Reset progress.md and feedback.md if --hard and plan is a bundle
	if resetHard && current.IsBundle() {
		log.Info("Resetting progress.md and feedback.md...")
		if err := plan.ResetBundleState(current.BundleDir, current.Name); err != nil {
			log.Warn("Failed to reset state files: %v", err)
		} else {
			log.Success("State files reset")
		}
	}

	// Reset the plan (move to pending)
	if err := queue.Reset(current); err != nil {
		return fmt.Errorf("resetting plan: %w", err)
	}

	if resetHard {
		log.Success("Plan '%s' hard reset to pending", current.Name)
	} else {
		log.Success("Plan '%s' reset to pending", current.Name)
	}

	// Store repo root for reference (unused but avoids warning)
	_ = repoRoot

	return nil
}
