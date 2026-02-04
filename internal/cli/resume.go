// Package cli provides the command-line interface for ralph.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/arvesolland/ralph/internal/worktree"
	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <plan-name>",
	Short: "Resume an archived plan from complete/",
	Long: `Resume a plan that was archived (moved to complete/).

This is useful when a plan was archived due to max iterations being reached
and you want to continue working on it.

The command will:
1. Find the plan in complete/ (supports partial name matching)
2. Move it back to pending/
3. Optionally reset the iteration counter

Example:
  ralph resume my-feature              # Resume plan named my-feature
  ralph resume my-feature-20240201     # Resume with full archived name
  ralph resume my-feature --reset      # Resume and reset iteration counter`,
	Args: cobra.ExactArgs(1),
	RunE: runResume,
}

var (
	resumeForce      bool
	resumeResetIter  bool
	resumeListOnly   bool
)

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().BoolVarP(&resumeForce, "force", "f", false, "Skip confirmation prompt")
	resumeCmd.Flags().BoolVar(&resumeResetIter, "reset", false, "Reset iteration counter to 1")
	resumeCmd.Flags().BoolVarP(&resumeListOnly, "list", "l", false, "List archived plans and exit")
}

func runResume(cmd *cobra.Command, args []string) error {
	// Initialize git to find repo root
	g := git.NewGit(".")
	repoRoot, err := g.RepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Create queue
	plansDir := filepath.Join(repoRoot, "plans")
	queue := plan.NewQueue(plansDir)

	// List mode
	if resumeListOnly {
		return listArchivedPlans(queue)
	}

	planName := args[0]

	// Find the plan in complete/
	completeDir := filepath.Join(plansDir, "complete")
	matchedPlan, err := findArchivedPlan(completeDir, planName)
	if err != nil {
		return err
	}

	// Confirm unless --force
	if !resumeForce {
		fmt.Printf("Resume plan '%s' from complete/ to pending/?\n", matchedPlan.Name)
		fmt.Printf("Branch: %s\n", matchedPlan.Branch)
		if resumeResetIter {
			fmt.Println("Iteration counter will be reset to 1")
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

	// Move from complete/ to pending/
	if err := queue.Resume(matchedPlan); err != nil {
		return fmt.Errorf("resuming plan: %w", err)
	}

	log.Success("Plan '%s' moved to pending", matchedPlan.Name)

	// Reset iteration counter if requested
	if resumeResetIter {
		// Check if worktree exists and has context.json
		worktreesDir := filepath.Join(repoRoot, ".ralph", "worktrees")
		manager, err := worktree.NewManager(g, worktreesDir)
		if err == nil && manager.Exists(matchedPlan) {
			wtPath := manager.Path(matchedPlan)
			ctxPath := runner.ContextPath(wtPath)

			// Load and reset context
			ctx, err := runner.LoadContext(ctxPath)
			if err == nil {
				ctx.Iteration = 1
				if err := runner.SaveContext(ctx, ctxPath); err != nil {
					log.Warn("Failed to reset iteration counter: %v", err)
				} else {
					log.Success("Iteration counter reset to 1")
				}
			}
		}
	}

	return nil
}

// findArchivedPlan finds a plan in the complete/ directory.
// Supports exact match, prefix match, and partial match.
func findArchivedPlan(completeDir string, name string) (*plan.Plan, error) {
	entries, err := os.ReadDir(completeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no archived plans found (complete/ directory doesn't exist)")
		}
		return nil, fmt.Errorf("reading complete directory: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no archived plans found")
	}

	var matches []os.DirEntry
	var exactMatch os.DirEntry

	for _, entry := range entries {
		entryName := entry.Name()

		// Exact match
		if entryName == name {
			exactMatch = entry
			break
		}

		// Prefix match (e.g., "my-feature" matches "my-feature-20240201")
		if strings.HasPrefix(entryName, name+"-") || strings.HasPrefix(entryName, name) {
			matches = append(matches, entry)
		}
	}

	// Exact match takes precedence
	if exactMatch != nil {
		return loadPlanFromComplete(completeDir, exactMatch)
	}

	// No matches
	if len(matches) == 0 {
		// List available plans
		var available []string
		for _, e := range entries {
			available = append(available, e.Name())
		}
		return nil, fmt.Errorf("no archived plan matching '%s' found.\nAvailable: %s",
			name, strings.Join(available, ", "))
	}

	// Multiple matches - ask user to be more specific
	if len(matches) > 1 {
		var matchNames []string
		for _, m := range matches {
			matchNames = append(matchNames, m.Name())
		}
		return nil, fmt.Errorf("multiple plans match '%s': %s\nPlease be more specific.",
			name, strings.Join(matchNames, ", "))
	}

	// Single match
	return loadPlanFromComplete(completeDir, matches[0])
}

// loadPlanFromComplete loads a plan from an entry in complete/
func loadPlanFromComplete(completeDir string, entry os.DirEntry) (*plan.Plan, error) {
	path := filepath.Join(completeDir, entry.Name())

	if entry.IsDir() {
		// Bundle - load plan.md from the bundle
		planPath := filepath.Join(path, "plan.md")
		p, err := plan.Load(planPath)
		if err != nil {
			return nil, fmt.Errorf("loading plan from bundle: %w", err)
		}
		p.BundleDir = path
		return p, nil
	}

	// Legacy flat file
	return plan.Load(path)
}

// listArchivedPlans lists all plans in complete/
func listArchivedPlans(queue *plan.Queue) error {
	plans, err := queue.Completed()
	if err != nil {
		return fmt.Errorf("listing archived plans: %w", err)
	}

	if len(plans) == 0 {
		log.Info("No archived plans found")
		return nil
	}

	fmt.Println("Archived plans:")
	for _, p := range plans {
		fmt.Printf("  %s (branch: %s)\n", p.Name, p.Branch)
	}

	return nil
}
