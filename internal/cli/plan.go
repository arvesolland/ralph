// Package cli provides the command-line interface for ralph.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/arvesolland/ralph/internal/state"
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
  migrate  Convert legacy flat files to bundles
  review   Review state.yaml against plan.md using LLM`,
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

var reviewModel string

var planReviewCmd = &cobra.Command{
	Use:   "review <plan-path>",
	Short: "Review state.yaml against plan.md using LLM",
	Long: `Review and fix state.yaml by comparing it against plan.md using an LLM.

If state.yaml doesn't exist, it will be generated from plan.md first.
The LLM reviews the state for missing tasks, dependencies, and criteria,
and iterates up to 5 times until the state is aligned with the plan.

Examples:
  ralph plan review plans/pending/my-feature
  ralph plan review plans/current/fix-bug --model claude-sonnet-4-5-20250929`,
	Args: cobra.ExactArgs(1),
	RunE: runPlanReview,
}

func init() {
	rootCmd.AddCommand(planCmd)
	planCmd.AddCommand(planCreateCmd)
	planCmd.AddCommand(planMigrateCmd)
	planCmd.AddCommand(planReviewCmd)
	planReviewCmd.Flags().StringVar(&reviewModel, "model", "", "model to use for review (default: verification model from config)")
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

func runPlanReview(cmd *cobra.Command, args []string) error {
	planPath := args[0]

	// Resolve absolute path
	absPlanPath, err := filepath.Abs(planPath)
	if err != nil {
		return fmt.Errorf("resolving plan path: %w", err)
	}

	// Validate path exists
	if _, err := os.Stat(absPlanPath); os.IsNotExist(err) {
		return fmt.Errorf("plan path does not exist: %s", planPath)
	}

	// Load the plan
	p, err := plan.Load(absPlanPath)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}

	if !p.IsBundle() {
		return fmt.Errorf("plan review requires a plan bundle (directory), not a flat file: %s", planPath)
	}

	bundleDir := p.BundleDir

	// If state.yaml doesn't exist, generate it first
	existing, err := state.LoadState(bundleDir)
	if err != nil {
		return fmt.Errorf("checking state.yaml: %w", err)
	}
	if existing == nil {
		log.Info("No state.yaml found, generating from plan.md...")
		st, initErr := state.InitStateFromPlan(p.Content, p.Name)
		if initErr != nil {
			return fmt.Errorf("initializing state from plan: %w", initErr)
		}
		if err := state.SaveState(st, bundleDir); err != nil {
			return fmt.Errorf("saving initial state: %w", err)
		}
		log.Info("Generated state.yaml with %d tasks", len(st.Tasks))
	}

	// Load config for model and prompt builder
	cfg, err := config.LoadWithDefaults(GetConfigPath())
	if err != nil {
		log.Warn("Failed to load config, using defaults: %v", err)
		cfg = config.Defaults()
	}

	// Determine model
	model := reviewModel
	if model == "" {
		model = cfg.Completion.VerificationModel
	}

	// Set up prompt builder
	configDir := filepath.Dir(GetConfigPath())
	promptsDir := filepath.Join(configDir, "prompts")
	promptBuilder := prompt.NewBuilder(cfg, configDir, promptsDir)

	// Create runner
	claudeRunner := runner.NewCLIRunner()
	adapter := &cliReviewRunnerAdapter{runner: claudeRunner}

	// Run review loop
	reviewCfg := state.ReviewConfig{
		PromptBuilder: promptBuilder,
		Model:         model,
		MaxAttempts:   5,
	}

	fmt.Printf("Reviewing state.yaml for plan: %s\n", p.Name)
	fmt.Printf("Model: %s\n", model)
	fmt.Println()

	result, err := state.ReviewState(context.Background(), adapter, p.Content, bundleDir, reviewCfg)
	if err != nil {
		return fmt.Errorf("state review failed: %w", err)
	}

	// Print summary
	fmt.Println()
	fmt.Printf("Review complete:\n")
	fmt.Printf("  Iterations: %d\n", result.Iterations)
	fmt.Printf("  Aligned: %v\n", result.Aligned)
	fmt.Printf("  Changes applied: %d\n", result.Changes)

	if result.Aligned {
		fmt.Println("\nState is aligned with plan.")
	} else {
		fmt.Println("\nState may still have gaps after max iterations.")
		fmt.Println("Run again or manually review state.yaml.")
	}

	return nil
}

// cliReviewRunnerAdapter adapts runner.CLIRunner to state.ReviewRunner.
type cliReviewRunnerAdapter struct {
	runner *runner.CLIRunner
}

func (a *cliReviewRunnerAdapter) Run(ctx context.Context, prompt string, opts state.ReviewRunnerOptions) (*state.ReviewRunnerResult, error) {
	runnerOpts := runner.Options{
		Model:        opts.Model,
		Print:        opts.Print,
		OutputFormat: opts.OutputFormat,
	}
	result, err := a.runner.Run(ctx, prompt, runnerOpts)
	if err != nil {
		return nil, err
	}
	return &state.ReviewRunnerResult{
		TextContent: result.TextContent,
	}, nil
}
