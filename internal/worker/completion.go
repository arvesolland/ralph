// Package worker implements the queue processing loop for Ralph.
package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/arvesolland/ralph/internal/git"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/arvesolland/ralph/internal/worktree"
)

// Common completion errors.
var (
	// ErrGHNotInstalled is returned when the GitHub CLI is not available.
	ErrGHNotInstalled = errors.New("gh CLI not installed")

	// ErrPushFailed is returned when pushing the branch fails.
	ErrPushFailed = errors.New("failed to push branch")

	// ErrPRCreateFailed is returned when creating the PR fails.
	ErrPRCreateFailed = errors.New("failed to create PR")

	// ErrMergeConflict is returned when merge has conflicts.
	ErrMergeConflict = errors.New("merge conflict")

	// ErrCheckoutFailed is returned when checkout fails.
	ErrCheckoutFailed = errors.New("failed to checkout branch")

	// ErrMergeFailed is returned when merge fails (non-conflict).
	ErrMergeFailed = errors.New("failed to merge branch")
)

// prURLRegex matches the PR URL from gh pr create output.
// gh outputs: https://github.com/owner/repo/pull/123
var prURLRegex = regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/pull/\d+`)

// PRCreationTimeout is the timeout for Claude to create the PR.
const PRCreationTimeout = 5 * time.Minute

// PRCreationConfig holds configuration for PR creation.
type PRCreationConfig struct {
	Plan          *plan.Plan
	Worktree      *worktree.Worktree
	Git           git.Git
	Runner        runner.Runner
	PromptBuilder *prompt.Builder
	BaseBranch    string
}

// CompletePR handles PR mode completion:
// 1. Push branch to origin
// 2. Use Claude to create PR with good description
// Returns the PR URL on success.
func CompletePR(cfg PRCreationConfig) (string, error) {
	p := cfg.Plan
	g := cfg.Git

	// Step 1: Push the branch to origin
	log.Info("Pushing branch %s to origin...", p.Branch)
	if err := pushBranch(g, p.Branch); err != nil {
		return "", fmt.Errorf("%w: %v", ErrPushFailed, err)
	}
	log.Success("Branch pushed successfully")

	// Step 2: Check if gh is installed
	if !isGHInstalled() {
		logManualPRInstructions(p)
		return "", ErrGHNotInstalled
	}

	// Step 3: Use Claude to create PR with good description
	log.Info("Creating PR with Claude...")
	prURL, err := createPRWithClaude(cfg)
	if err != nil {
		// Fall back to programmatic PR creation
		log.Warn("Claude PR creation failed: %v, falling back to basic PR", err)
		prURL, err = createPR(p, g.WorkDir())
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrPRCreateFailed, err)
		}
	}

	log.Success("PR created: %s", prURL)
	return prURL, nil
}

// CompleteBranch handles branch mode completion:
// Push branch to origin only, without creating a PR.
// This is useful when you want to manually create a PR later or review the branch first.
func CompleteBranch(p *plan.Plan, g git.Git) error {
	log.Info("Pushing branch %s to origin...", p.Branch)
	if err := pushBranch(g, p.Branch); err != nil {
		return fmt.Errorf("%w: %v", ErrPushFailed, err)
	}
	log.Success("Branch %s pushed to origin (no PR created)", p.Branch)
	return nil
}

// createPRWithClaude uses Claude Code CLI to create a PR with a good description.
func createPRWithClaude(cfg PRCreationConfig) (string, error) {
	// Build the prompt
	baseBranch := cfg.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	// Get relative plan path for the prompt
	planPath := cfg.Plan.Path

	overrides := map[string]string{
		"FEATURE_BRANCH": cfg.Plan.Branch,
		"BASE_BRANCH":    baseBranch,
		"PLAN_FILE":      planPath,
	}

	promptContent, err := cfg.PromptBuilder.Build("pr_creation_prompt.md", overrides)
	if err != nil {
		return "", fmt.Errorf("building PR prompt: %w", err)
	}

	// Run Claude with gh access
	opts := runner.DefaultOptions()
	opts.WorkDir = cfg.Worktree.Path
	opts.NoPermissions = true
	opts.AllowedTools = []string{"Bash", "Read", "Glob", "Grep"}

	ctx, cancel := context.WithTimeout(context.Background(), PRCreationTimeout)
	defer cancel()

	result, err := cfg.Runner.Run(ctx, promptContent, opts)
	if err != nil {
		return "", fmt.Errorf("claude execution: %w", err)
	}

	// Extract PR URL from Claude's output
	prURL := extractPRURL(result.TextContent)
	if prURL == "" {
		// Try to get the PR URL from gh if Claude created it but we couldn't parse the URL
		prURL, _ = getExistingPRURL(cfg.Worktree.Path)
	}

	if prURL == "" {
		return "", fmt.Errorf("PR may have been created but URL not found in output")
	}

	return prURL, nil
}

// pushBranch pushes the branch to origin with upstream tracking.
func pushBranch(g git.Git, branch string) error {
	return g.PushWithUpstream("origin", branch)
}

// createPR creates a PR using the gh CLI (fallback when Claude fails).
// Returns the PR URL or an error.
func createPR(p *plan.Plan, workDir string) (string, error) {
	// Check if gh is installed
	if !isGHInstalled() {
		return "", ErrGHNotInstalled
	}

	// Build basic PR title and body (fallback)
	title := fmt.Sprintf("feat: %s", p.Name)
	body := fmt.Sprintf("## Summary\n\nImplements: %s\n\n---\n\nGenerated by [Ralph](https://github.com/arvesolland/ralph)", p.Name)

	// Run gh pr create
	cmd := exec.Command("gh", "pr", "create",
		"--title", title,
		"--body", body,
	)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Check for specific error conditions
		errOutput := stderr.String()

		// If PR already exists, try to get its URL
		if strings.Contains(errOutput, "already exists") {
			return getExistingPRURL(workDir)
		}

		return "", fmt.Errorf("gh pr create: %s: %w", errOutput, err)
	}

	// Parse PR URL from output
	output := stdout.String()
	prURL := extractPRURL(output)
	if prURL == "" {
		// gh also outputs to stderr sometimes, try there
		prURL = extractPRURL(stderr.String())
	}

	if prURL == "" {
		// If we can't extract the URL but the command succeeded, return the raw output
		prURL = strings.TrimSpace(output)
	}

	return prURL, nil
}

// isGHInstalled checks if the GitHub CLI is available.
func isGHInstalled() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// extractPRURL extracts a GitHub PR URL from text.
func extractPRURL(text string) string {
	match := prURLRegex.FindString(text)
	return match
}

// getExistingPRURL gets the URL of an existing PR for the current branch.
func getExistingPRURL(workDir string) (string, error) {
	cmd := exec.Command("gh", "pr", "view", "--json", "url", "-q", ".url")
	cmd.Dir = workDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("getting existing PR: %w", err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// logManualPRInstructions logs instructions for creating PR manually.
func logManualPRInstructions(p *plan.Plan) {
	log.Warn("GitHub CLI (gh) not installed. Please create PR manually:")
	log.Info("  1. Go to your repository on GitHub")
	log.Info("  2. Create a new pull request for branch: %s", p.Branch)
	log.Info("  3. Or install gh: https://cli.github.com/")
}

// CompleteMerge handles merge mode completion:
// 1. Check out base branch in main worktree
// 2. Merge feature branch with --no-ff
// 3. Push base branch to origin
// 4. Delete feature branch (local and remote)
// The mainGit should be a Git instance for the main worktree (not the feature worktree).
func CompleteMerge(p *plan.Plan, baseBranch string, mainGit git.Git) error {
	featureBranch := p.Branch

	// Step 1: Checkout base branch in main worktree
	log.Info("Checking out base branch %s...", baseBranch)
	if err := mainGit.Checkout(baseBranch); err != nil {
		return fmt.Errorf("%w: %v", ErrCheckoutFailed, err)
	}
	log.Debug("Checked out %s", baseBranch)

	// Step 2: Merge feature branch with --no-ff
	log.Info("Merging %s into %s...", featureBranch, baseBranch)
	if err := mainGit.Merge(featureBranch, true); err != nil {
		if errors.Is(err, git.ErrMergeConflict) {
			return fmt.Errorf("%w: resolve conflicts in %s and try again", ErrMergeConflict, baseBranch)
		}
		return fmt.Errorf("%w: %v", ErrMergeFailed, err)
	}
	log.Success("Merged %s into %s", featureBranch, baseBranch)

	// Step 3: Push base branch to origin
	log.Info("Pushing %s to origin...", baseBranch)
	if err := mainGit.Push(); err != nil {
		return fmt.Errorf("%w: %v", ErrPushFailed, err)
	}
	log.Success("Pushed %s to origin", baseBranch)

	// Step 4: Delete feature branch (local)
	log.Info("Deleting local branch %s...", featureBranch)
	if err := mainGit.DeleteBranch(featureBranch, true); err != nil {
		// Log warning but don't fail - the merge was successful
		log.Warn("Failed to delete local branch %s: %v", featureBranch, err)
	} else {
		log.Debug("Deleted local branch %s", featureBranch)
	}

	// Step 5: Delete feature branch (remote)
	log.Info("Deleting remote branch %s...", featureBranch)
	if err := mainGit.DeleteRemoteBranch("origin", featureBranch); err != nil {
		// Log warning but don't fail - the merge was successful
		// Remote branch might not exist if we never pushed it
		log.Warn("Failed to delete remote branch %s: %v", featureBranch, err)
	} else {
		log.Debug("Deleted remote branch %s", featureBranch)
	}

	log.Success("Merge complete: %s merged into %s", featureBranch, baseBranch)
	return nil
}
