// Package cli provides the command-line interface for ralph.
package cli

import (
	"fmt"

	"github.com/arvesolland/ralph/internal/plan"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Manage plan bundles",
	Long: `Manage plan bundles for development workflows.

Plan bundles are self-contained directories containing:
  - plan.md: The plan with tasks and acceptance criteria
  - progress.md: Iteration log tracking progress
  - feedback.md: Human input for blockers

Commands:
  create   Create a new plan bundle in pending/
  migrate  Convert legacy flat files to bundles`,
}

var planCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new plan bundle",
	Long: `Create a new plan bundle in plans/pending/.

The bundle will be created with scaffolded files:
  - plan.md: Template with overview, tasks, and rules sections
  - progress.md: Header with format instructions
  - feedback.md: Pending and Processed sections

The name will be sanitized to a valid directory name (lowercase,
hyphens, no special characters).

Examples:
  ralph plan create my-feature
  ralph plan create "Add User Authentication"`,
	Args: cobra.ExactArgs(1),
	RunE: runPlanCreate,
}

var planMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate legacy flat files to bundles",
	Long: `Convert all legacy flat plan files to bundle directories.

This command scans pending/, current/, and complete/ directories
for flat .md files (not .progress.md or .feedback.md) and converts
each to a bundle:

  my-plan.md              -> my-plan/plan.md
  my-plan.progress.md     -> my-plan/progress.md
  my-plan.feedback.md     -> my-plan/feedback.md

Existing bundles (directories) are skipped. If associated files
(progress, feedback) don't exist, scaffolded versions are created.

This migration is safe to run multiple times - it only affects
flat files that haven't been converted yet.`,
	RunE: runPlanMigrate,
}

func init() {
	rootCmd.AddCommand(planCmd)
	planCmd.AddCommand(planCreateCmd)
	planCmd.AddCommand(planMigrateCmd)
}

func runPlanCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	plansDir := "plans"

	p, err := plan.CreateBundle(plansDir, name)
	if err != nil {
		return fmt.Errorf("creating plan bundle: %w", err)
	}

	fmt.Printf("Created plan bundle: %s\n", p.BundleDir)
	fmt.Printf("  - plan.md: Task planning template\n")
	fmt.Printf("  - progress.md: Iteration logging\n")
	fmt.Printf("  - feedback.md: Human input for blockers\n")
	fmt.Println()
	fmt.Printf("Next step: Edit %s/plan.md to define your tasks\n", p.BundleDir)

	return nil
}

func runPlanMigrate(cmd *cobra.Command, args []string) error {
	plansDir := "plans"

	if err := plan.MigrateToBundles(plansDir); err != nil {
		return fmt.Errorf("migrating to bundles: %w", err)
	}

	fmt.Println("Migration complete.")
	return nil
}
