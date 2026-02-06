// Package runner provides Claude CLI execution and iteration loop management.
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/state"
)

// IterationCooldown is the delay between iterations to avoid overwhelming the API.
const IterationCooldown = 3 * time.Second

// IterationTimeout is the default timeout for a single iteration.
const IterationTimeout = 60 * time.Minute

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

	// lastState is the most recently loaded state.yaml (nil if plan has no structured state).
	// Updated at the start and end of each iteration.
	lastState *state.PlanState
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

	// Auto-init state.yaml if plan is a bundle and state.yaml doesn't exist yet.
	l.autoInitState(ctx)

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

			if l.lastState != nil && len(l.lastState.Tasks) > 0 {
				// Criteria-gated verification: check if all tasks are done in state.yaml
				if l.isStateComplete() {
					log.Success("Plan verified complete (all tasks done in state.yaml)")
					result.Completed = true
					return result
				}
				// State exists but not all tasks done — fall back to LLM verification
				log.Warn("State.yaml incomplete, falling back to LLM verification...")
				verifyCtx, cancel := context.WithTimeout(ctx, VerificationTimeout)
				verifyResult, verifyErr := Verify(verifyCtx, l.plan, l.runner, l.config.Completion.VerificationModel)
				cancel()

				if verifyErr != nil {
					log.Warn("LLM verification failed: %v", verifyErr)
				} else if verifyResult.Verified {
					log.Success("Plan verified complete by LLM (state.yaml was stale)")
					result.Completed = true
					return result
				} else {
					log.Warn("LLM verification failed: %s", verifyResult.Reason)
					combinedReason := fmt.Sprintf(
						"LLM verification: %s\n\nstate.yaml also shows: %s\n\n"+
							"If `ralph task` commands are failing, edit state.yaml directly to update task statuses.",
						verifyResult.Reason, l.stateIncompleteReason())
					if err := l.writeFeedback(combinedReason); err != nil {
						log.Error("Failed to write verification feedback: %v", err)
					}
				}
			} else {
				// Fallback: LLM verification for plans without state.yaml
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
					if err := l.writeFeedback(verifyResult.Reason); err != nil {
						log.Error("Failed to write verification feedback: %v", err)
					}
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

	// Load state.yaml if plan is a bundle (for structured context injection)
	bundleDir := l.resolveBundleDir()
	if bundleDir != "" {
		st, loadErr := state.LoadState(bundleDir)
		if loadErr != nil {
			log.Warn("Failed to load state.yaml: %v", loadErr)
		} else if st != nil {
			l.lastState = st
			log.Debug("Loaded state.yaml with %d tasks", len(st.Tasks))
		}
	}

	// Build the prompt (includes structured context JSON if state.yaml exists)
	prompt, err := l.buildPrompt(l.lastState)
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

	// Reload the plan to get updated content from the working directory
	// The agent updates the plan file, so we must read it to get current task state
	// Use PlanDir from context - it's the loadable path for plan.Load()
	// Fallback to PlanFile for backward compatibility with old context.json files
	planLoadPath := l.ctx.PlanDir
	if planLoadPath == "" {
		planLoadPath = l.ctx.PlanFile
	}
	planLoadPath = filepath.Join(l.worktreePath, planLoadPath)
	updatedPlan, err := plan.Load(planLoadPath)
	if err != nil {
		log.Warn("Failed to reload plan from %s: %v", planLoadPath, err)
		// Continue with existing plan
	} else {
		l.plan = updatedPlan
	}

	// Reload state.yaml — agent may have updated it via ralph task/feedback CLI commands
	if bundleDir != "" {
		reloadedState, reloadErr := state.LoadState(bundleDir)
		if reloadErr != nil {
			log.Warn("Failed to reload state.yaml: %v", reloadErr)
		} else if reloadedState != nil {
			l.lastState = reloadedState
			log.Debug("Reloaded state.yaml after iteration")
		}
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
// If planState is non-nil, structured context JSON is injected via the {{CONTEXT_JSON}} placeholder.
func (l *IterationLoop) buildPrompt(planState *state.PlanState) (string, error) {
	// Compute PlanDir for prompt - fallback to parent of PlanFile for old contexts
	planDir := l.ctx.PlanDir
	if planDir == "" {
		// Backward compatibility: derive from PlanFile
		// For bundles, this would be wrong, but old contexts should only exist for flat files
		planDir = filepath.Dir(l.ctx.PlanFile)
	}

	// Build context overrides for placeholders
	overrides := map[string]string{
		"ITERATION":      fmt.Sprintf("%d", l.ctx.Iteration),
		"MAX_ITERATIONS": fmt.Sprintf("%d", l.ctx.MaxIterations),
		"FEATURE_BRANCH": l.ctx.FeatureBranch,
		"BASE_BRANCH":    l.ctx.BaseBranch,
		"PLAN_FILE":      l.ctx.PlanFile,
		"PLAN_DIR":       planDir,
	}

	// If structured state exists, build and inject context JSON
	if planState != nil {
		payload := state.BuildContext(planState)
		contextJSON, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			log.Warn("Failed to marshal context payload: %v", err)
		} else {
			overrides["CONTEXT_JSON"] = string(contextJSON)
		}
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

// resolveBundleDir returns the absolute bundle directory path inside the worktree,
// or empty string if the plan is not a bundle.
func (l *IterationLoop) resolveBundleDir() string {
	if !l.plan.IsBundle() {
		return ""
	}
	planDir := l.ctx.PlanDir
	if planDir == "" {
		return ""
	}
	return filepath.Join(l.worktreePath, planDir)
}

// autoInitState generates state.yaml from plan.md if the plan is a bundle
// and state.yaml doesn't exist yet. After regex-based init, it runs an LLM
// review loop to fix any gaps.
func (l *IterationLoop) autoInitState(ctx context.Context) {
	bundleDir := l.resolveBundleDir()
	if bundleDir == "" {
		return
	}

	// Check if state.yaml already exists with tasks
	existing, err := state.LoadState(bundleDir)
	if err != nil {
		log.Warn("Failed to check state.yaml: %v", err)
		return
	}
	if existing != nil && len(existing.Tasks) > 0 {
		return // Already has state with tasks
	}

	// Generate state.yaml from plan.md (first time, or re-init if tasks are empty)
	if existing != nil && len(existing.Tasks) == 0 {
		log.Info("Re-initializing state.yaml (exists but has 0 tasks)")
	} else {
		log.Info("Auto-generating state.yaml from plan.md")
	}
	st, err := state.InitStateFromPlan(l.plan.Content, l.plan.Name)
	if err != nil {
		log.Warn("Failed to init state from plan: %v", err)
		return
	}

	if err := state.SaveState(st, bundleDir); err != nil {
		log.Warn("Failed to save auto-generated state.yaml: %v", err)
		return
	}

	log.Info("Auto-generated state.yaml with %d tasks", len(st.Tasks))

	// Run LLM review loop to fix gaps in regex-parsed state
	reviewCfg := state.ReviewConfig{
		PromptBuilder: l.promptBuilder,
		Model:         l.config.Completion.VerificationModel,
		MaxAttempts:   5,
	}
	adapter := &reviewRunnerAdapter{runner: l.runner}
	result, reviewErr := state.ReviewState(ctx, adapter, l.plan.Content, bundleDir, reviewCfg)
	if reviewErr != nil {
		log.Warn("State review failed: %v (continuing with regex-parsed state)", reviewErr)
		return
	}
	log.Info("State review: %d iterations, aligned=%v, changes=%d",
		result.Iterations, result.Aligned, result.Changes)
}

// reviewRunnerAdapter adapts runner.Runner to state.ReviewRunner.
type reviewRunnerAdapter struct {
	runner Runner
}

func (a *reviewRunnerAdapter) Run(ctx context.Context, prompt string, opts state.ReviewRunnerOptions) (*state.ReviewRunnerResult, error) {
	runnerOpts := Options{
		Model:         opts.Model,
		Print:         opts.Print,
		OutputFormat:  opts.OutputFormat,
		NoPermissions: true,
	}
	result, err := a.runner.Run(ctx, prompt, runnerOpts)
	if err != nil {
		return nil, err
	}
	return &state.ReviewRunnerResult{
		TextContent: result.TextContent,
	}, nil
}

// isStateComplete returns true if the plan's state.yaml indicates all tasks are done or skipped.
func (l *IterationLoop) isStateComplete() bool {
	if l.lastState == nil || len(l.lastState.Tasks) == 0 {
		return false
	}
	for _, t := range l.lastState.Tasks {
		if t.Status != state.TaskStatusDone && t.Status != state.TaskStatusSkipped {
			return false
		}
	}
	return true
}

// stateIncompleteReason builds a human-readable reason explaining which tasks are not yet done.
func (l *IterationLoop) stateIncompleteReason() string {
	if l.lastState == nil {
		return "no state.yaml loaded"
	}
	var incomplete []string
	for _, t := range l.lastState.Tasks {
		if t.Status != state.TaskStatusDone && t.Status != state.TaskStatusSkipped {
			incomplete = append(incomplete, fmt.Sprintf("%s (%s)", t.ID, t.Status))
		}
	}
	if len(incomplete) == 0 {
		return ""
	}
	return fmt.Sprintf("Tasks not complete in state.yaml: %s. "+
		"Use `ralph task complete` after verifying all criteria, then output the completion marker.",
		fmt.Sprintf("%v", incomplete))
}

// writeFeedback writes verification failure reason to the feedback file.
func (l *IterationLoop) writeFeedback(reason string) error {
	content := fmt.Sprintf("**Verification failed:**\n%s", reason)
	return plan.AppendFeedback(l.plan, "verification", content)
}
