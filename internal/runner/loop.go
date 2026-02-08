// Package runner provides Claude CLI execution and iteration loop management.
package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/arvesolland/ralph/internal/atm"
	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/prompt"
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

// MaxFalseCompletions is the number of consecutive false completion claims before halting.
const MaxFalseCompletions = 5

// IterationLoop manages the main execution loop for plan completion.
// It orchestrates: prompt building -> Claude execution -> verification -> commit.
type IterationLoop struct {
	// atm is the ATM client for task management
	atm atm.ATM

	// planID is the ATM plan ID being executed
	planID int

	// projectSlug is the ATM project slug
	projectSlug string

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

	// falseCompletions tracks consecutive false completion claims
	falseCompletions int

	// onBeforeIteration is called before each iteration
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
	ATM               atm.ATM
	PlanID            int
	ProjectSlug       string
	Context           *Context
	Config            *config.Config
	Runner            Runner
	Git               git.Git
	PromptBuilder     *prompt.Builder
	WorktreePath      string
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
		atm:               cfg.ATM,
		planID:            cfg.PlanID,
		projectSlug:       cfg.ProjectSlug,
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

	// Validate plan ID before entering loop
	if l.atm != nil && l.planID <= 0 {
		result.Error = fmt.Errorf("invalid plan ID: %d (must be > 0)", l.planID)
		return result
	}

	for !l.ctx.IsMaxReached() {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result
		default:
		}

		log.Info("Starting iteration %d/%d", l.ctx.Iteration, l.ctx.MaxIterations)

		// Call before-iteration hook
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

		// Check for completion via ATM stats
		if iterResult.IsComplete {
			log.Info("Completion marker detected, checking ATM stats...")

			if l.isATMComplete(ctx) {
				log.Success("Plan verified complete (all tasks done in ATM)")
				result.Completed = true
				return result
			}

			// Track consecutive false completions
			l.falseCompletions++
			log.Warn("ATM stats show plan is not yet complete (false completion %d/%d)", l.falseCompletions, MaxFalseCompletions)

			if l.falseCompletions >= MaxFalseCompletions {
				result.Error = fmt.Errorf("agent claimed completion %d times but ATM stats never confirmed — halting", l.falseCompletions)
				return result
			}

			feedback := fmt.Sprintf("Agent claimed completion but ATM stats show tasks remain. This is false completion attempt %d/%d. Use `atm-cli plan context %d --format text` to see which tasks are still incomplete.", l.falseCompletions, MaxFalseCompletions, l.planID)
			if err := l.addATMFeedback(ctx, feedback); err != nil {
				log.Error("Failed to add ATM feedback: %v", err)
			}
		} else {
			// Reset false completion counter on non-completion iterations
			l.falseCompletions = 0
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
func (l *IterationLoop) runIteration(ctx context.Context) (*Result, error) {
	log.Debug("Building prompt for iteration %d", l.ctx.Iteration)

	// Fetch fresh context from ATM as structured text (for prompt injection)
	var contextText string
	if l.atm != nil && l.planID > 0 {
		var err error
		contextText, err = l.atm.PlanContextText(l.planID)
		if err != nil {
			return nil, fmt.Errorf("fetching ATM plan context (plan %d): %w", l.planID, err)
		}
		log.Debug("Fetched ATM plan context text (%d bytes)", len(contextText))
	}

	// Build the prompt
	promptContent, err := l.buildPrompt(contextText)
	if err != nil {
		return nil, fmt.Errorf("building prompt: %w", err)
	}

	log.Debug("Prompt built successfully (%d bytes)", len(promptContent))

	// Set up options for Claude
	opts := DefaultOptions()
	opts.WorkDir = l.worktreePath
	opts.NoPermissions = true // Skip interactive permission prompts

	log.Debug("Starting Claude CLI execution (timeout: %v)", l.iterationTimeout)

	// Create timeout context for this iteration
	iterCtx, cancel := context.WithTimeout(ctx, l.iterationTimeout)
	defer cancel()

	// Run Claude
	result, err := l.runner.Run(iterCtx, promptContent, opts)
	if err != nil {
		log.Debug("Claude execution failed: %v", err)
		return result, fmt.Errorf("claude execution: %w", err)
	}

	log.Debug("Claude execution completed (duration: %v, complete: %v)", result.Duration, result.IsComplete)

	// Add progress to ATM
	if l.atm != nil && l.planID > 0 {
		progressBody := fmt.Sprintf("Iteration %d completed in %v.", l.ctx.Iteration, result.Duration)
		if result.IsComplete {
			progressBody += " Completion marker detected."
		}
		if result.Blocker != nil {
			progressBody += fmt.Sprintf(" Blocker: %s", result.Blocker.Description)
		}

		if _, err := l.atm.AddProgress(l.planID, "ralph", progressBody); err != nil {
			log.Warn("Failed to add ATM progress: %v", err)
		}
	}

	// Commit changes
	if err := l.commitChanges(); err != nil {
		log.Error("Failed to commit changes: %v", err)
		// Non-fatal, continue
	}

	return result, nil
}

// buildPrompt builds the prompt for Claude using the template builder.
func (l *IterationLoop) buildPrompt(contextText string) (string, error) {
	// Build context overrides for placeholders
	overrides := map[string]string{
		"ITERATION":      fmt.Sprintf("%d", l.ctx.Iteration),
		"MAX_ITERATIONS": fmt.Sprintf("%d", l.ctx.MaxIterations),
		"FEATURE_BRANCH": l.ctx.FeatureBranch,
		"BASE_BRANCH":    l.ctx.BaseBranch,
		"PLAN_ID":        fmt.Sprintf("%d", l.planID),
	}

	// Inject ATM plan context as structured text
	if contextText != "" {
		overrides["ATM_CONTEXT"] = contextText
	} else {
		overrides["ATM_CONTEXT"] = "[No plan context available. Run `atm-cli plan context " + fmt.Sprintf("%d", l.planID) + " --format text` to fetch it manually.]"
	}

	// Build the main prompt
	content, err := l.promptBuilder.Build("prompt.md", overrides)
	if err != nil {
		return "", fmt.Errorf("building prompt: %w", err)
	}

	return content, nil
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

	// Stage tracked changes only (not untracked -- they may be gitignored)
	allFiles := make([]string, 0, len(status.Staged)+len(status.Unstaged))
	allFiles = append(allFiles, status.Staged...)
	allFiles = append(allFiles, status.Unstaged...)
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

// isATMComplete checks if the plan is complete via ATM stats.
// Fail-closed: returns false if ATM is unreachable (retries 3 times).
func (l *IterationLoop) isATMComplete(ctx context.Context) bool {
	if l.atm == nil || l.planID == 0 {
		// No ATM configured, treat completion marker as sufficient
		return true
	}

	// Retry up to 3 times with backoff
	var agentCtx *atm.AgentContext
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		agentCtx, err = l.atm.PlanContext(l.planID)
		if err == nil {
			break
		}
		log.Warn("ATM completion check attempt %d/3 failed: %v", attempt, err)
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(time.Duration(attempt*5) * time.Second):
			}
		}
	}
	if err != nil {
		log.Error("Failed to verify ATM completion after 3 attempts: %v", err)
		// Fail-closed: do NOT trust the completion marker when ATM is unreachable
		return false
	}

	stats := agentCtx.Stats
	if stats.TotalTasks == 0 {
		// No tasks tracked in ATM, trust the completion marker
		return true
	}

	completed := stats.Done + stats.Skipped
	log.Debug("ATM stats: %d/%d tasks done (done=%d, skipped=%d)",
		completed, stats.TotalTasks, stats.Done, stats.Skipped)

	return completed == stats.TotalTasks
}

// addATMFeedback adds feedback to the plan via ATM.
func (l *IterationLoop) addATMFeedback(ctx context.Context, reason string) error {
	if l.atm == nil || l.planID == 0 {
		return nil
	}
	_, err := l.atm.AddFeedback(l.planID, "ralph-verification", reason)
	return err
}
