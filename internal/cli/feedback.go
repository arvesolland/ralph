package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/arvesolland/ralph/internal/state"
	"github.com/spf13/cobra"
)

var feedbackJSON bool

var feedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Manage plan feedback",
	Long: `Manage structured feedback within a plan bundle's state.yaml.

Commands: add, resolve.
Feedback can be scoped to the whole plan or a specific task.`,
}

// feedbackAddCmd adds new feedback.
var feedbackAddCmd = &cobra.Command{
	Use:   "add <plan-path>",
	Short: "Add feedback to the plan",
	Long: `Add a new feedback entry to the plan's state.yaml.

The feedback ID is auto-assigned as the next F{n}.

Examples:
  ralph feedback add plans/current/my-feature --scope plan --message "Consider using JWT"
  ralph feedback add plans/current/my-feature --scope task:T2 --message "API needs auth" --author human`,
	Args: cobra.ExactArgs(1),
	RunE: runFeedbackAdd,
}

var feedbackAddScope string
var feedbackAddMessage string
var feedbackAddAuthor string

// feedbackResolveCmd resolves feedback.
var feedbackResolveCmd = &cobra.Command{
	Use:   "resolve <plan-path> <feedback-id>",
	Short: "Resolve a feedback entry",
	Long: `Mark a feedback entry as resolved.

Examples:
  ralph feedback resolve plans/current/my-feature F1`,
	Args: cobra.ExactArgs(2),
	RunE: runFeedbackResolve,
}

func init() {
	rootCmd.AddCommand(feedbackCmd)

	// feedback add
	feedbackCmd.AddCommand(feedbackAddCmd)
	feedbackAddCmd.Flags().StringVar(&feedbackAddScope, "scope", "plan", "feedback scope: 'plan' or 'task:Tn'")
	feedbackAddCmd.Flags().StringVar(&feedbackAddMessage, "message", "", "feedback message (required)")
	feedbackAddCmd.Flags().StringVar(&feedbackAddAuthor, "author", "agent", "author of the feedback")
	feedbackAddCmd.MarkFlagRequired("message")

	// feedback resolve
	feedbackCmd.AddCommand(feedbackResolveCmd)

	// --json flag on all subcommands
	feedbackCmd.PersistentFlags().BoolVar(&feedbackJSON, "json", false, "output result as JSON")
}

func runFeedbackAdd(cmd *cobra.Command, args []string) error {
	return loadAndMutateFeedback(args[0], func(st *state.PlanState) (any, error) {
		fb, err := state.AddFeedback(st, feedbackAddScope, feedbackAddAuthor, feedbackAddMessage)
		if err != nil {
			return nil, err
		}
		if feedbackJSON {
			return fb, nil
		}
		return fmt.Sprintf("Added feedback %s [%s]: %s", fb.ID, fb.Scope, fb.Message), nil
	})
}

func runFeedbackResolve(cmd *cobra.Command, args []string) error {
	feedbackID := args[1]
	return loadAndMutateFeedback(args[0], func(st *state.PlanState) (any, error) {
		if err := state.ResolveFeedback(st, feedbackID); err != nil {
			return nil, err
		}
		if feedbackJSON {
			return map[string]string{"feedback_id": feedbackID, "resolved": "true"}, nil
		}
		return fmt.Sprintf("Resolved feedback %s", feedbackID), nil
	})
}

// loadAndMutateFeedback is the feedback-specific variant of loadAndMutate.
func loadAndMutateFeedback(planPath string, mutate func(st *state.PlanState) (any, error)) error {
	bundleDir, err := resolveBundleDir(planPath)
	if err != nil {
		return err
	}

	st, err := state.LoadState(bundleDir)
	if err != nil {
		return fmt.Errorf("loading state.yaml: %w", err)
	}
	if st == nil {
		return fmt.Errorf("no state.yaml found in %s", bundleDir)
	}

	result, err := mutate(st)
	if err != nil {
		return err
	}

	if err := state.SaveState(st, bundleDir); err != nil {
		return fmt.Errorf("saving state.yaml: %w", err)
	}

	if feedbackJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	if msg, ok := result.(string); ok {
		fmt.Println(msg)
	}
	return nil
}
