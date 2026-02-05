// Package runner provides Claude CLI execution and iteration loop management.
package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/prompt"
)

// IterationCooldown is the delay between iterations to avoid overwhelming the API.
const IterationCooldown = 3 * time.Second

// IterationTimeout is the default timeout for a single iteration.
const IterationTimeout = 30 * time.Minute

// LoopResult represents the outcome of the iteration loop.
type LoopResult struct {
	// Completed is true if the plan was verified complete.
	Completed bool

	// Iterations is the number of iterations executed.
	Iterations int

	// FinalBlocker is the last blocker encountered, if any.
	FinalBlocker *Blocker

	// Error is the error that caused termination, if any.
	Error error
}

// IterationLoop manages the main execution loop for plan completion.
// It orchestrates: prompt building → Claude execution → verification → commit.
type IterationLoop struct {
	// plan is the plan being executed
	plan *plan.Plan

	// ctx is the execution context
	ctx *Context

	// config is the loaded configuration
	config *config.Config

	// runner executes Claude CLI
	runner Runner

	// git handles git operations
	git git.Git

	// promptBuilder builds prompts from templates
	promptBuilder *prompt.Builder

	// worktreePath is the path to the execution worktree
	worktreePath string

	// iterationTimeout is the timeout for each iteration
	iterationTimeout time.Duration

	// onBeforeIteration is called before each iteration (for syncing files)
	onBeforeIteration func()

	// onIteration is called after each iteration (for testing/hooks)
	onIteration func(iteration int, result *Result)

	// onBlocker is called when a blocker is detected
	onBlocker func(blocker *Blocker)

	// onAfterCommit is called after a successful commit (for pushing to remote)
	onAfterCommit func()
}

// LoopConfig holds configuration for creating an IterationLoop.
type LoopConfig struct {
	Plan             *plan.Plan
	Context          *Context
	Config           *config.Config
	Runner           Runner
	Git              git.Git
	PromptBuilder    *prompt.Builder
	WorktreePath     string
	IterationTimeout  time.Duration
	OnBeforeIteration func()
	OnIteration       func(iteration int, result *Result)
	OnBlocker         func(blocker *Blocker)
	OnAfterCommit     func()
}

// NewIterationLoop creates a new iteration loop with the given configuration.
func NewIterationLoop(cfg LoopConfig) *IterationLoop {
	timeout := cfg.IterationTimeout
	if timeout == 0 {
		timeout = IterationTimeout
	}

	return &IterationLoop{
		plan:              cfg.Plan,
		ctx:               cfg.Context,
		config:            cfg.Config,
		runner:            cfg.Runner,
		git:               cfg.Git,
		promptBuilder:     cfg.PromptBuilder,
		worktreePath:      cfg.WorktreePath,
		iterationTimeout:  timeout,
		onBeforeIteration: cfg.OnBeforeIteration,
		onIteration:       cfg.OnIteration,
		onBlocker:         cfg.OnBlocker,
		onAfterCommit:     cfg.OnAfterCommit,
	}
}

// Run executes the iteration loop until the plan is complete or max iterations reached.
// Returns a LoopResult indicating the outcome.
func (l *IterationLoop) Run(ctx context.Context) *LoopResult {
	result := &LoopResult{}

	for !l.ctx.IsMaxReached() {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result
		default:
		}

		log.Info("Starting iteration %d/%d", l.ctx.Iteration, l.ctx.MaxIterations)

		// Call before-iteration hook (for syncing feedback files)
		if l.onBeforeIteration != nil {
			l.onBeforeIteration()
		}

		// Run single iteration
		iterResult, err := l.runIteration(ctx)
		result.Iterations = l.ctx.Iteration

		if err != nil {
			log.Error("Iteration %d failed: %v", l.ctx.Iteration, err)
			result.Error = err
			return result
		}

		// Call iteration hook if set
		if l.onIteration != nil {
			l.onIteration(l.ctx.Iteration, iterResult)
		}

		// Handle blocker if detected
		if iterResult.Blocker != nil {
			log.Warn("Blocker detected: %s", iterResult.Blocker.Description)
			result.FinalBlocker = iterResult.Blocker
			if l.onBlocker != nil {
				l.onBlocker(iterResult.Blocker)
			}
			// Continue - agent may have worked on other tasks
		}

		// Check for completion
		if iterResult.IsComplete {
			log.Info("Completion marker detected, verifying...")

			// Verify completion with configured model
			verifyCtx, cancel := context.WithTimeout(ctx, VerificationTimeout)
			verifyResult, verifyErr := Verify(verifyCtx, l.plan, l.runner, l.config.Completion.VerificationModel)
			cancel()

			if verifyErr != nil {
				log.Warn("Verification failed: %v", verifyErr)
				// Continue anyway - let next iteration try again
			} else if verifyResult.Verified {
				log.Success("Plan verified complete!")
				result.Completed = true
				return result
			} else {
				log.Warn("Verification failed: %s", verifyResult.Reason)
				// Write feedback for next iteration
				if err := l.writeFeedback(verifyResult.Reason); err != nil {
					log.Error("Failed to write verification feedback: %v", err)
				}
			}
		}

		// Increment iteration for next round
		l.ctx = l.ctx.Increment()

		// Save updated context
		ctxPath := ContextPath(l.worktreePath)
		if err := SaveContext(l.ctx, ctxPath); err != nil {
			log.Error("Failed to save context: %v", err)
			// Non-fatal, continue
		}

		// Cooldown between iterations
		log.Debug("Cooling down for %v before next iteration", IterationCooldown)
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result
		case <-time.After(IterationCooldown):
		}
	}

	// Max iterations reached
	log.Error("Max iterations (%d) reached without completion", l.ctx.MaxIterations)
	result.Error = fmt.Errorf("max iterations (%d) reached without completion", l.ctx.MaxIterations)
	return result
}

// runIteration executes a single iteration of the loop.
// Debug logging throughout helps diagnose hangs and performance issues in production.
func (l *IterationLoop) runIteration(ctx context.Context) (*Result, error) {
	log.Debug("Building prompt for iteration %d", l.ctx.Iteration)

	// Build the prompt
	prompt, err := l.buildPrompt()
	if err != nil {
		return nil, fmt.Errorf("building prompt: %w", err)
	}

	log.Debug("Prompt built successfully (%d bytes)", len(prompt))

	// Set up options for Claude
	opts := DefaultOptions()
	opts.WorkDir = l.worktreePath
	opts.NoPermissions = true // Skip interactive permission prompts

	log.Debug("Starting Claude CLI execution (timeout: %v)", l.iterationTimeout)

	// Create timeout context for this iteration
	iterCtx, cancel := context.WithTimeout(ctx, l.iterationTimeout)
	defer cancel()

	// Run Claude
	result, err := l.runner.Run(iterCtx, prompt, opts)
	if err != nil {
		log.Debug("Claude execution failed: %v", err)
		return result, fmt.Errorf("claude execution: %w", err)
	}

	log.Debug("Claude execution completed (duration: %v, complete: %v)", result.Duration, result.IsComplete)

	// Reload the plan to get updated content from the worktree (not main repo)
	// The agent updates the plan file in the worktree, so we must read from there
	worktreePlanPath := filepath.Join(l.worktreePath, l.ctx.PlanFile)
	// For bundles, PlanFile is "plans/current/foo/plan.md" - we need to load the
	// bundle directory so that BundleDir is set correctly for progress/feedback paths
	if filepath.Base(worktreePlanPath) == "plan.md" {
		worktreePlanPath = filepath.Dir(worktreePlanPath)
	}
	updatedPlan, err := plan.Load(worktreePlanPath)
	if err != nil {
		log.Warn("Failed to reload plan: %v", err)
		// Continue with existing plan
	} else {
		l.plan = updatedPlan
	}

	// Append to progress file
	if err := l.appendProgress(result); err != nil {
		log.Error("Failed to append progress: %v", err)
		// Non-fatal, continue
	}

	// Commit changes
	if err := l.commitChanges(); err != nil {
		log.Error("Failed to commit changes: %v", err)
		// Non-fatal, continue
	}

	return result, nil
}

// buildPrompt builds the prompt for Claude using the template builder.
func (l *IterationLoop) buildPrompt() (string, error) {
	// Build context overrides for placeholders
	overrides := map[string]string{
		"ITERATION":      fmt.Sprintf("%d", l.ctx.Iteration),
		"MAX_ITERATIONS": fmt.Sprintf("%d", l.ctx.MaxIterations),
		"FEATURE_BRANCH": l.ctx.FeatureBranch,
		"BASE_BRANCH":    l.ctx.BaseBranch,
		"PLAN_FILE":      l.ctx.PlanFile,
	}

	// Build the main prompt
	content, err := l.promptBuilder.Build("prompt.md", overrides)
	if err != nil {
		return "", fmt.Errorf("building prompt: %w", err)
	}

	return content, nil
}

// appendProgress appends iteration results to the progress file.
func (l *IterationLoop) appendProgress(result *Result) error {
	// Build progress entry
	content := fmt.Sprintf("Claude execution completed in %v.\n", result.Duration)

	if result.IsComplete {
		content += "Completion marker detected.\n"
	}

	if result.Blocker != nil {
		content += fmt.Sprintf("Blocker: %s\n", result.Blocker.Description)
	}

	return plan.AppendProgress(l.plan, l.ctx.Iteration, content)
}

// commitChanges commits all changes after an iteration.
func (l *IterationLoop) commitChanges() error {
	// Check if there are changes to commit
	status, err := l.git.Status()
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}

	if status.IsClean() {
		log.Debug("No changes to commit")
		return nil
	}

	// Stage all changes
	allFiles := append(append(status.Staged, status.Unstaged...), status.Untracked...)
	if err := l.git.Add(allFiles...); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}

	// Build commit message
	message := fmt.Sprintf("ralph: iteration %d", l.ctx.Iteration)

	// Commit
	if err := l.git.Commit(message); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	log.Debug("Committed iteration %d changes", l.ctx.Iteration)

	// Call after-commit callback (for pushing to remote)
	if l.onAfterCommit != nil {
		l.onAfterCommit()
	}

	return nil
}

// writeFeedback writes verification failure reason to the feedback file.
func (l *IterationLoop) writeFeedback(reason string) error {
	content := fmt.Sprintf("**Verification failed:**\n%s", reason)
	return plan.AppendFeedback(l.plan, "verification", content)
}
