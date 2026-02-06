package state

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/prompt"
	"gopkg.in/yaml.v3"
)

// ReviewRunner defines the interface for LLM execution needed by the review loop.
// This matches runner.Runner but avoids a circular dependency.
type ReviewRunner interface {
	Run(ctx context.Context, prompt string, opts ReviewRunnerOptions) (*ReviewRunnerResult, error)
}

// ReviewRunnerOptions mirrors runner.Options for the subset needed by review.
type ReviewRunnerOptions struct {
	Model        string
	Print        bool
	OutputFormat string
}

// ReviewRunnerResult mirrors runner.Result for the subset needed by review.
type ReviewRunnerResult struct {
	TextContent string
}

// ReviewConfig holds configuration for the state review loop.
type ReviewConfig struct {
	PromptBuilder *prompt.Builder // Prompt builder for loading templates
	Model         string         // Model to use (default: claude-opus-4-5-latest)
	MaxAttempts   int            // Max review iterations (default: 5)
}

// ReviewResult holds the outcome of the review loop.
type ReviewResult struct {
	Aligned    bool // True if state matches plan
	Iterations int  // Number of review iterations performed
	Changes    int  // Number of iterations that produced changes
}

// DefaultReviewModel is the default model for state review.
const DefaultReviewModel = "opus"

// yamlFenceRegex extracts YAML content from markdown code fences.
var yamlFenceRegex = regexp.MustCompile("(?s)```ya?ml\n(.*?)```")

// ReviewState runs the LLM review loop comparing plan.md against state.yaml.
// It iterates up to cfg.MaxAttempts times, asking the LLM to verify alignment
// and fix any gaps. Returns early if the LLM reports ALIGNED.
func ReviewState(ctx context.Context, runner ReviewRunner, planContent string, bundleDir string, cfg ReviewConfig) (*ReviewResult, error) {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.Model == "" {
		cfg.Model = DefaultReviewModel
	}

	result := &ReviewResult{}
	var validationError string

	for i := 0; i < cfg.MaxAttempts; i++ {
		result.Iterations = i + 1

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Load current state
		currentState, err := LoadState(bundleDir)
		if err != nil {
			return result, fmt.Errorf("loading state.yaml: %w", err)
		}
		if currentState == nil {
			return result, fmt.Errorf("state.yaml not found in %s", bundleDir)
		}

		// Marshal current state to YAML for the prompt
		stateYAML, err := yaml.Marshal(currentState)
		if err != nil {
			return result, fmt.Errorf("marshaling state: %w", err)
		}

		// Build the review prompt
		reviewPrompt, err := buildReviewPrompt(cfg.PromptBuilder, planContent, string(stateYAML), validationError)
		if err != nil {
			return result, fmt.Errorf("building review prompt: %w", err)
		}

		// Call the LLM
		opts := ReviewRunnerOptions{
			Model:        cfg.Model,
			Print:        true,
			OutputFormat: "text",
		}

		llmResult, err := runner.Run(ctx, reviewPrompt, opts)
		if err != nil {
			return result, fmt.Errorf("LLM review call failed: %w", err)
		}

		response := strings.TrimSpace(llmResult.TextContent)
		log.Debug("State review iteration %d response: %s", i+1, truncateStr(response, 200))

		// Check for ALIGNED
		if isAligned(response) {
			result.Aligned = true
			log.Info("State review: aligned after %d iteration(s)", i+1)
			return result, nil
		}

		// Extract YAML from response
		newState, parseErr := parseStateFromResponse(response)
		if parseErr != nil {
			log.Warn("State review iteration %d: failed to parse YAML: %v", i+1, parseErr)
			validationError = fmt.Sprintf("Previous response had invalid YAML: %v. Please provide valid YAML.", parseErr)
			continue
		}

		// Validate dependencies
		if valErr := ValidateDependencies(newState.Tasks); valErr != nil {
			log.Warn("State review iteration %d: validation failed: %v", i+1, valErr)
			validationError = fmt.Sprintf("Previous response had validation error: %v. Please fix.", valErr)
			continue
		}

		// Merge: preserve runtime state from existing tasks
		merged := mergeStates(currentState, newState)

		// Save the merged state
		if err := SaveState(merged, bundleDir); err != nil {
			return result, fmt.Errorf("saving reviewed state: %w", err)
		}

		result.Changes++
		validationError = "" // Clear any previous error
		log.Info("State review iteration %d: applied changes (%d tasks)", i+1, len(merged.Tasks))
	}

	// Exhausted max attempts without ALIGNED
	log.Warn("State review: max attempts (%d) reached without alignment", cfg.MaxAttempts)
	return result, nil
}

// buildReviewPrompt constructs the prompt for a review iteration.
func buildReviewPrompt(builder *prompt.Builder, planContent, stateYAML, validationError string) (string, error) {
	overrides := map[string]string{
		"PLAN_CONTENT": planContent,
		"STATE_YAML":   stateYAML,
	}
	if validationError != "" {
		overrides["VALIDATION_ERROR"] = fmt.Sprintf("\n## Previous Error\n\n%s\n", validationError)
	} else {
		overrides["VALIDATION_ERROR"] = ""
	}

	if builder != nil {
		return builder.Build("state_review_prompt.md", overrides)
	}

	// Fallback: build prompt inline without template builder
	return buildReviewPromptInline(planContent, stateYAML, validationError), nil
}

// buildReviewPromptInline builds the review prompt without a template builder.
func buildReviewPromptInline(planContent, stateYAML, validationError string) string {
	var sb strings.Builder
	sb.WriteString("Compare this plan against the state.yaml and identify gaps.\n\n")
	sb.WriteString("## Plan Content\n\n")
	sb.WriteString(planContent)
	sb.WriteString("\n\n## Current State YAML\n\n")
	sb.WriteString(stateYAML)
	if validationError != "" {
		sb.WriteString("\n\n## Previous Error\n\n")
		sb.WriteString(validationError)
	}
	sb.WriteString("\n\nIf aligned, respond with: ALIGNED\n")
	sb.WriteString("If not, respond with corrected state.yaml in ```yaml fences.\n")
	return sb.String()
}

// isAligned checks if the LLM response indicates the state is aligned.
func isAligned(response string) bool {
	trimmed := strings.TrimSpace(response)
	// Strip backticks — LLMs sometimes wrap the response in `ALIGNED`
	trimmed = strings.Trim(trimmed, "`")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "ALIGNED" {
		return true
	}
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "`")
		line = strings.TrimSpace(line)
		if line == "ALIGNED" {
			return true
		}
	}
	return false
}

// parseStateFromResponse extracts and parses a PlanState from the LLM response.
func parseStateFromResponse(response string) (*PlanState, error) {
	// Extract YAML from code fences
	yamlContent := extractYAML(response)
	if yamlContent == "" {
		return nil, fmt.Errorf("no YAML code fence found in response")
	}

	var state PlanState
	if err := yaml.Unmarshal([]byte(yamlContent), &state); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if len(state.Tasks) == 0 {
		return nil, fmt.Errorf("parsed state has no tasks")
	}

	return &state, nil
}

// extractYAML extracts YAML content from markdown code fences.
func extractYAML(response string) string {
	match := yamlFenceRegex.FindStringSubmatch(response)
	if match == nil {
		return ""
	}
	return strings.TrimSpace(match[1])
}

// mergeStates preserves runtime state (Status, StartedAt, DoneAt, Artifacts, Notes)
// from the old state while taking structural changes (new tasks, criteria, dependencies)
// from the new state.
func mergeStates(old, new *PlanState) *PlanState {
	// Build a lookup of old tasks by ID
	oldTasks := make(map[string]*TaskState, len(old.Tasks))
	for i := range old.Tasks {
		oldTasks[old.Tasks[i].ID] = &old.Tasks[i]
	}

	// Build a set of new task IDs
	newTaskIDs := make(map[string]bool, len(new.Tasks))
	for _, t := range new.Tasks {
		newTaskIDs[t.ID] = true
	}

	// Merge tasks from new state, preserving runtime fields from old
	merged := make([]TaskState, 0, len(new.Tasks))
	for _, newTask := range new.Tasks {
		if oldTask, exists := oldTasks[newTask.ID]; exists {
			// Task exists in both: preserve runtime state, take structural changes
			newTask.Status = oldTask.Status
			newTask.StartedAt = oldTask.StartedAt
			newTask.DoneAt = oldTask.DoneAt
			newTask.Artifacts = oldTask.Artifacts
			newTask.Notes = oldTask.Notes

			// Merge criteria: preserve Done/DoneAt from old criteria that match by text
			newTask.Criteria = mergeCriteria(oldTask.Criteria, newTask.Criteria)
		}
		merged = append(merged, newTask)
	}

	// Keep old tasks that were removed by the LLM (log a warning)
	for _, oldTask := range old.Tasks {
		if !newTaskIDs[oldTask.ID] {
			log.Warn("State review: keeping task %s removed by LLM", oldTask.ID)
			merged = append(merged, oldTask)
		}
	}

	// Preserve plan-level fields from old state
	result := &PlanState{
		ID:        old.ID,
		Title:     old.Title,
		Status:    old.Status,
		CreatedAt: old.CreatedAt,
		Tasks:     merged,
		Feedback:  old.Feedback,
	}

	return result
}

// mergeCriteria preserves Done/DoneAt from old criteria that match by text,
// while taking new criteria from the LLM output.
func mergeCriteria(old, new []Criterion) []Criterion {
	oldByText := make(map[string]*Criterion, len(old))
	for i := range old {
		oldByText[old[i].Text] = &old[i]
	}

	result := make([]Criterion, len(new))
	for i, c := range new {
		result[i] = c
		if oldC, exists := oldByText[c.Text]; exists {
			result[i].Done = oldC.Done
			result[i].DoneAt = oldC.DoneAt
		}
	}
	return result
}

// truncateStr shortens a string to maxLen, adding "..." if truncated.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
