package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/state"
	"github.com/spf13/cobra"
)

var contextJSON bool

var contextCmd = &cobra.Command{
	Use:   "context <plan-path>",
	Short: "Display structured context for a plan",
	Long: `Display the structured state context for a plan bundle.

Reads state.yaml from the plan bundle and outputs the full context payload
including task status, selection, feedback, and summary statistics.

With --json, outputs machine-readable JSON suitable for agent consumption.
Without --json, outputs a human-readable summary.

Examples:
  ralph context plans/current/my-feature
  ralph context plans/current/my-feature --json`,
	Args: cobra.ExactArgs(1),
	RunE: runContext,
}

func init() {
	rootCmd.AddCommand(contextCmd)
	contextCmd.Flags().BoolVar(&contextJSON, "json", false, "output as JSON")
}

func runContext(cmd *cobra.Command, args []string) error {
	planPath := args[0]

	// Resolve the bundle directory
	bundleDir, err := resolveBundleDir(planPath)
	if err != nil {
		return err
	}

	// Load state.yaml from the bundle
	st, err := state.LoadState(bundleDir)
	if err != nil {
		return fmt.Errorf("loading state.yaml: %w", err)
	}

	if st == nil {
		return fmt.Errorf("no state.yaml found in %s (plan has no structured state)", bundleDir)
	}

	// Build the context payload
	payload := state.BuildContext(st)

	if contextJSON {
		return outputContextJSON(payload)
	}
	return outputContextHuman(payload)
}

// resolveBundleDir resolves a plan path argument to a bundle directory.
// Accepts: bundle directory, plan.md file, or plan file within a bundle.
func resolveBundleDir(planPath string) (string, error) {
	absPath, err := filepath.Abs(planPath)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("path does not exist: %s", planPath)
	}

	if info.IsDir() {
		return absPath, nil
	}

	// If it's a file named plan.md inside a bundle, use parent directory
	if filepath.Base(absPath) == "plan.md" {
		parentDir := filepath.Dir(absPath)
		return parentDir, nil
	}

	// Try loading as a plan to get the bundle dir
	p, err := plan.Load(absPath)
	if err != nil {
		return "", fmt.Errorf("loading plan: %w", err)
	}
	if p.BundleDir != "" {
		return p.BundleDir, nil
	}

	// Legacy flat file — no bundle directory
	return "", fmt.Errorf("plan at %s is not a bundle (no state.yaml support for flat files)", planPath)
}

func outputContextJSON(payload *state.ContextPayload) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func outputContextHuman(payload *state.ContextPayload) error {
	fmt.Printf("Plan: %s (%s)\n", payload.Plan.Title, payload.Plan.Status)
	fmt.Printf("ID:   %s\n", payload.Plan.ID)
	fmt.Println()

	// Summary
	fmt.Printf("Progress: %d/%d tasks done (%.0f%%)\n",
		payload.Summary.ByStatus["done"], payload.Summary.Total,
		payload.Summary.DoneRatio*100)
	fmt.Println()

	// Selection
	if payload.Selection.SuggestedNext != nil {
		fmt.Printf("Suggested next: %s — %s\n",
			payload.Selection.SuggestedNext.TaskID,
			payload.Selection.SuggestedNext.Reason)
	} else {
		fmt.Println("Suggested next: (none)")
	}
	fmt.Println()

	// Available tasks
	if len(payload.Selection.Available) > 0 {
		fmt.Println("Available tasks:")
		for _, pick := range payload.Selection.Available {
			fmt.Printf("  %s — %s\n", pick.TaskID, pick.Reason)
		}
	}

	// Blocked tasks
	if len(payload.Selection.Blocked) > 0 {
		fmt.Println("Blocked tasks:")
		for _, pick := range payload.Selection.Blocked {
			fmt.Printf("  %s — %s\n", pick.TaskID, pick.Reason)
		}
	}

	// Unresolved feedback
	if len(payload.Feedback.Unresolved) > 0 {
		fmt.Println()
		fmt.Println("Unresolved feedback:")
		for _, fb := range payload.Feedback.Unresolved {
			fmt.Printf("  %s [%s] (%s): %s\n", fb.ID, fb.Scope, fb.Author, fb.Message)
		}
	}

	// Task details
	fmt.Println()
	fmt.Println("Tasks:")
	for _, task := range payload.Tasks.Items {
		criteriaCount := 0
		criteriaDone := 0
		for _, c := range task.Criteria {
			criteriaCount++
			if c.Done {
				criteriaDone++
			}
		}
		if criteriaCount > 0 {
			fmt.Printf("  %s [%s] %s (%d/%d criteria)\n",
				task.ID, task.Status, task.Title, criteriaDone, criteriaCount)
		} else {
			fmt.Printf("  %s [%s] %s\n", task.ID, task.Status, task.Title)
		}
	}

	return nil
}
