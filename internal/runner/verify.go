// Package runner provides Claude CLI execution and verification.
package runner

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/arvesolland/ralph/internal/plan"
)

// VerificationTimeout is the default timeout for verification requests.
// Verification uses a shorter timeout since Haiku is fast.
const VerificationTimeout = 60 * time.Second

// DefaultVerificationModel is the default model used for verification (fast, cheap).
const DefaultVerificationModel = "claude-3-5-haiku-latest"

// VerificationResult holds the result of plan completion verification.
type VerificationResult struct {
	// Verified is true if the plan is confirmed complete.
	Verified bool

	// Reason explains why verification failed (empty if Verified is true).
	Reason string

	// RawResponse is the raw response from the verification model.
	RawResponse string
}

// verificationPromptTemplate is the prompt used to verify plan completion.
const verificationPromptTemplate = `You are a verification assistant. Your job is to determine if a plan has been completed.

Below is a plan with tasks and checkboxes. Analyze the plan and determine if ALL tasks are complete.

A task is complete when:
1. Its status is explicitly marked as "complete" (e.g., **Status:** complete)
2. All of its "Done when" checkboxes are checked ([x])
3. All of its subtask checkboxes are checked ([x])

PLAN CONTENT:
%s

Based on the plan above, answer with EXACTLY one of:
- "YES" if ALL tasks are complete
- "NO: <reason>" if any tasks are incomplete, explaining what is not complete

Your response must start with either "YES" or "NO:". Be specific about what is incomplete if answering NO.`

// yesNoRegex matches YES or NO: patterns at the start of the response.
var yesNoRegex = regexp.MustCompile(`(?im)^(YES|NO)\s*:?\s*(.*)`)

// Verify checks if a plan is complete using a fast model for verification.
// It builds a prompt with the plan state and asks the model to verify completion.
// The model parameter specifies which model to use; if empty, uses DefaultVerificationModel.
// Returns (true, "", nil) if verified complete.
// Returns (false, reason, nil) if not complete, with an explanation.
// Returns (false, "", err) on execution errors.
func Verify(ctx context.Context, p *plan.Plan, runner Runner, model string) (*VerificationResult, error) {
	// Build the verification prompt with plan content
	prompt := buildVerificationPrompt(p)

	// Use default model if not specified
	if model == "" {
		model = DefaultVerificationModel
	}

	// Set up options for verification model
	opts := DefaultOptions()
	opts.Model = model
	opts.Print = true          // Use --print mode for simple prompt/response
	opts.OutputFormat = "text" // Use text format for verification (stream-json requires --verbose with --print)

	// Use shorter timeout for verification if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, VerificationTimeout)
		defer cancel()
	}

	// Run verification
	result, err := runner.Run(ctx, prompt, opts)
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	// Parse the response
	verified, reason := parseVerificationResponse(result.TextContent)

	return &VerificationResult{
		Verified:    verified,
		Reason:      reason,
		RawResponse: result.TextContent,
	}, nil
}

// buildVerificationPrompt creates the prompt for plan verification.
func buildVerificationPrompt(p *plan.Plan) string {
	return fmt.Sprintf(verificationPromptTemplate, p.Content)
}

// parseVerificationResponse extracts the yes/no answer and reason from the model response.
// Returns (true, "") for YES responses.
// Returns (false, reason) for NO responses.
// Returns (false, "unclear response") if the response doesn't match expected format.
func parseVerificationResponse(response string) (bool, string) {
	response = strings.TrimSpace(response)
	if response == "" {
		return false, "empty response from verification model"
	}

	// Try to match YES/NO pattern
	match := yesNoRegex.FindStringSubmatch(response)
	if match == nil {
		// Check for simple YES/NO at start without regex
		upper := strings.ToUpper(response)
		if strings.HasPrefix(upper, "YES") {
			return true, ""
		}
		if strings.HasPrefix(upper, "NO") {
			// Extract reason from rest of response
			reason := strings.TrimPrefix(upper, "NO")
			reason = strings.TrimPrefix(reason, ":")
			reason = strings.TrimSpace(reason)
			if reason == "" {
				reason = "verification indicated incomplete but no reason given"
			}
			return false, reason
		}

		// Response doesn't clearly indicate YES or NO
		return false, fmt.Sprintf("unclear response from verification model: %s", truncate(response, 200))
	}

	answer := strings.ToUpper(match[1])
	if answer == "YES" {
		return true, ""
	}

	// NO response - extract reason
	reason := strings.TrimSpace(match[2])
	if reason == "" {
		// Try to get reason from rest of response
		afterMatch := strings.TrimPrefix(response, match[0])
		reason = strings.TrimSpace(afterMatch)
	}

	if reason == "" {
		reason = "verification indicated incomplete but no reason given"
	}

	return false, reason
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// BuildPlanSummary creates a summary of plan state for verification.
// This can be used when the full plan content is too large.
func BuildPlanSummary(p *plan.Plan) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Plan: %s\n", p.Name))
	sb.WriteString(fmt.Sprintf("Status: %s\n", p.Status))
	sb.WriteString(fmt.Sprintf("Branch: %s\n\n", p.Branch))

	complete := plan.CountComplete(p.Tasks)
	total := plan.CountTotal(p.Tasks)
	sb.WriteString(fmt.Sprintf("Tasks: %d/%d complete\n\n", complete, total))

	// List incomplete tasks
	sb.WriteString("Incomplete tasks:\n")
	incomplete := findIncompleteTasks(p.Tasks, "")
	if len(incomplete) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, task := range incomplete {
			sb.WriteString(fmt.Sprintf("  - %s\n", task))
		}
	}

	return sb.String()
}

// findIncompleteTasks recursively finds all incomplete task texts with their path.
func findIncompleteTasks(tasks []plan.Task, prefix string) []string {
	var result []string
	for _, t := range tasks {
		taskName := t.Text
		if prefix != "" {
			taskName = prefix + " > " + taskName
		}

		if !t.Complete {
			result = append(result, taskName)
		}

		// Recurse into subtasks
		result = append(result, findIncompleteTasks(t.Subtasks, taskName)...)
	}
	return result
}
