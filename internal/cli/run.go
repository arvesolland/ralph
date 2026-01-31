// Package cli provides the command-line interface for ralph.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/spf13/cobra"
)

var (
	maxIterations int
	reviewPlan    bool
)

var runCmd = &cobra.Command{
	Use:   "run <plan-file>",
	Short: "Run the iteration loop on a plan",
	Long: `Execute the iteration loop on a specified plan file.

The iteration loop will:
1. Build a prompt from the plan and context
2. Execute Claude to work on the plan
3. Check for completion markers
4. Verify completion with a secondary model (Haiku)
5. Commit changes after each iteration
6. Repeat until plan is complete or max iterations reached

Example:
  ralph run plans/current/my-feature.md
  ralph run plans/pending/fix-bug.md --max 50`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().IntVar(&maxIterations, "max", runner.DefaultMaxIterations, "maximum iterations before stopping")
	runCmd.Flags().BoolVar(&reviewPlan, "review", false, "run plan review before execution (not yet implemented)")
}

func runRun(cmd *cobra.Command, args []string) error {
	planPath := args[0]

	// Validate plan file exists
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		return fmt.Errorf("plan file does not exist: %s", planPath)
	}

	// Make path absolute
	absPlanPath, err := filepath.Abs(planPath)
	if err != nil {
		return fmt.Errorf("resolving plan path: %w", err)
	}

	// Load the plan
	p, err := plan.Load(absPlanPath)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}

	log.Info("Running plan: %s", p.Name)
	log.Info("Branch: %s", p.Branch)
	log.Info("Max iterations: %d", maxIterations)

	// Load configuration
	cfg, err := config.LoadWithDefaults(GetConfigPath())
	if err != nil {
		log.Warn("Failed to load config, using defaults: %v", err)
		cfg = config.Defaults()
	}

	// Determine worktree path (current directory for now - worker will handle actual worktree)
	worktreePath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Initialize git
	g := git.NewGit(worktreePath)

	// Verify we're in a git repo
	_, err = g.RepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Get current branch for context
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Handle --review flag (placeholder)
	if reviewPlan {
		log.Warn("Plan review not yet implemented - skipping")
	}

	// Create execution context
	execCtx := runner.NewContext(p, cfg.Git.BaseBranch, maxIterations)
	execCtx.FeatureBranch = currentBranch

	// Initialize prompt builder
	configDir := filepath.Dir(GetConfigPath())
	promptsDir := filepath.Join(configDir, "prompts")
	promptBuilder := prompt.NewBuilder(cfg, configDir, promptsDir)

	// Create CLI runner
	claudeRunner := runner.NewCLIRunner()

	// Create iteration loop
	loop := runner.NewIterationLoop(runner.LoopConfig{
		Plan:          p,
		Context:       execCtx,
		Config:        cfg,
		Runner:        claudeRunner,
		Git:           g,
		PromptBuilder: promptBuilder,
		WorktreePath:  worktreePath,
		OnIteration: func(iteration int, result *runner.Result) {
			log.Info("Iteration %d/%d complete", iteration, maxIterations)
			if result.IsComplete {
				log.Info("Completion marker detected")
			}
		},
		OnBlocker: func(blocker *runner.Blocker) {
			log.Warn("Blocker detected: %s", blocker.Description)
			if blocker.Action != "" {
				log.Info("Action required: %s", blocker.Action)
			}
		},
	})

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Warn("Received signal %v, stopping after current iteration...", sig)
		cancel()
	}()

	// Run the iteration loop
	result := loop.Run(ctx)

	// Report results
	fmt.Println()
	fmt.Println("==============================")
	fmt.Printf("Iterations completed: %d/%d\n", result.Iterations, maxIterations)

	if result.Completed {
		log.Success("Plan completed successfully!")
		return nil
	}

	if result.Error != nil {
		if result.Error == context.Canceled {
			log.Warn("Execution interrupted by user")
			return nil // Exit 0 on user interruption
		}
		return fmt.Errorf("execution failed: %w", result.Error)
	}

	if result.FinalBlocker != nil {
		log.Warn("Execution stopped on blocker: %s", result.FinalBlocker.Description)
		return nil // Exit 0 - blockers are not failures
	}

	return fmt.Errorf("plan not completed after %d iterations", result.Iterations)
}
