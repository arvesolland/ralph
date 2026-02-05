package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/arvesolland/ralph/internal/state"
	"github.com/spf13/cobra"
)

var taskJSON bool

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage plan tasks",
	Long: `Manage tasks within a plan bundle's state.yaml.

Commands for task lifecycle: add, claim, complete, skip, criterion.
All commands operate on state.yaml inside the plan bundle directory.`,
}

// taskAddCmd adds a new task to the plan state.
var taskAddCmd = &cobra.Command{
	Use:   "add <plan-path>",
	Short: "Add a new task to the plan",
	Long: `Add a new task to the plan's state.yaml.

The task ID is auto-assigned as the next T{n}.

Examples:
  ralph task add plans/current/my-feature --title "Add auth middleware"
  ralph task add plans/current/my-feature --title "Add tests" --requires T1,T2
  ralph task add plans/current/my-feature --title "Deploy" --criteria "Tests pass;Linter clean"`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskAdd,
}

var taskAddTitle string
var taskAddRequires string
var taskAddCriteria string

// taskClaimCmd claims a task (sets status to doing).
var taskClaimCmd = &cobra.Command{
	Use:   "claim <plan-path> <task-id>",
	Short: "Claim a task (start working on it)",
	Long: `Claim a task by setting its status to doing.

The task must be in todo status and all dependencies must be met.

Examples:
  ralph task claim plans/current/my-feature T2`,
	Args: cobra.ExactArgs(2),
	RunE: runTaskClaim,
}

// taskCompleteCmd completes a task.
var taskCompleteCmd = &cobra.Command{
	Use:   "complete <plan-path> <task-id>",
	Short: "Complete a task",
	Long: `Complete a task by setting its status to done.

All criteria must be checked before a task can be completed.

Examples:
  ralph task complete plans/current/my-feature T1
  ralph task complete plans/current/my-feature T1 --commits abc123,def456`,
	Args: cobra.ExactArgs(2),
	RunE: runTaskComplete,
}

var taskCompleteCommits string

// taskSkipCmd skips a task.
var taskSkipCmd = &cobra.Command{
	Use:   "skip <plan-path> <task-id>",
	Short: "Skip a task",
	Long: `Skip a task with an optional reason.

Examples:
  ralph task skip plans/current/my-feature T3 --reason "Not needed for MVP"`,
	Args: cobra.ExactArgs(2),
	RunE: runTaskSkip,
}

var taskSkipReason string

// taskCriterionCmd is the parent command for criterion check/uncheck.
var taskCriterionCmd = &cobra.Command{
	Use:   "criterion",
	Short: "Manage task criteria",
	Long:  `Check or uncheck task criteria. Criteria are 1-indexed.`,
}

// taskCriterionCheckCmd checks a criterion.
var taskCriterionCheckCmd = &cobra.Command{
	Use:   "check <plan-path> <task-id> <index>",
	Short: "Check a criterion as done",
	Long: `Mark a task criterion as done. Index is 1-based.

Examples:
  ralph task criterion check plans/current/my-feature T1 1`,
	Args: cobra.ExactArgs(3),
	RunE: runTaskCriterionCheck,
}

// taskCriterionUncheckCmd unchecks a criterion.
var taskCriterionUncheckCmd = &cobra.Command{
	Use:   "uncheck <plan-path> <task-id> <index>",
	Short: "Uncheck a criterion",
	Long: `Mark a task criterion as not done. Index is 1-based.

Examples:
  ralph task criterion uncheck plans/current/my-feature T1 1`,
	Args: cobra.ExactArgs(3),
	RunE: runTaskCriterionUncheck,
}

func init() {
	rootCmd.AddCommand(taskCmd)

	// task add
	taskCmd.AddCommand(taskAddCmd)
	taskAddCmd.Flags().StringVar(&taskAddTitle, "title", "", "task title (required)")
	taskAddCmd.Flags().StringVar(&taskAddRequires, "requires", "", "comma-separated dependency task IDs (e.g. T1,T2)")
	taskAddCmd.Flags().StringVar(&taskAddCriteria, "criteria", "", "semicolon-separated criteria (e.g. \"Tests pass;Linter clean\")")
	taskAddCmd.MarkFlagRequired("title")

	// task claim
	taskCmd.AddCommand(taskClaimCmd)

	// task complete
	taskCmd.AddCommand(taskCompleteCmd)
	taskCompleteCmd.Flags().StringVar(&taskCompleteCommits, "commits", "", "comma-separated commit SHAs")

	// task skip
	taskCmd.AddCommand(taskSkipCmd)
	taskSkipCmd.Flags().StringVar(&taskSkipReason, "reason", "", "reason for skipping")

	// task criterion check/uncheck
	taskCmd.AddCommand(taskCriterionCmd)
	taskCriterionCmd.AddCommand(taskCriterionCheckCmd)
	taskCriterionCmd.AddCommand(taskCriterionUncheckCmd)

	// --json flag on all subcommands
	taskCmd.PersistentFlags().BoolVar(&taskJSON, "json", false, "output result as JSON")
}

// loadAndMutate loads state, calls the mutator, saves state, and outputs the result.
func loadAndMutate(planPath string, mutate func(st *state.PlanState) (any, error)) error {
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

	return outputResult(result)
}

func outputResult(result any) error {
	if taskJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	if msg, ok := result.(string); ok {
		fmt.Println(msg)
	}
	return nil
}

func runTaskAdd(cmd *cobra.Command, args []string) error {
	requires := parseCommaSep(taskAddRequires)
	criteria := parseSemicolonSep(taskAddCriteria)

	return loadAndMutate(args[0], func(st *state.PlanState) (any, error) {
		task, err := state.AddTask(st, taskAddTitle, requires, criteria)
		if err != nil {
			return nil, err
		}
		if taskJSON {
			return task, nil
		}
		return fmt.Sprintf("Added task %s: %s", task.ID, task.Title), nil
	})
}

func runTaskClaim(cmd *cobra.Command, args []string) error {
	taskID := args[1]
	return loadAndMutate(args[0], func(st *state.PlanState) (any, error) {
		if err := state.ClaimTask(st, taskID); err != nil {
			return nil, err
		}
		if taskJSON {
			return map[string]string{"task_id": taskID, "status": "doing"}, nil
		}
		return fmt.Sprintf("Claimed task %s (status: doing)", taskID), nil
	})
}

func runTaskComplete(cmd *cobra.Command, args []string) error {
	taskID := args[1]
	commits := parseCommaSep(taskCompleteCommits)

	return loadAndMutate(args[0], func(st *state.PlanState) (any, error) {
		if err := state.CompleteTask(st, taskID, commits, nil); err != nil {
			return nil, err
		}
		if taskJSON {
			return map[string]string{"task_id": taskID, "status": "done"}, nil
		}
		return fmt.Sprintf("Completed task %s (status: done)", taskID), nil
	})
}

func runTaskSkip(cmd *cobra.Command, args []string) error {
	taskID := args[1]
	return loadAndMutate(args[0], func(st *state.PlanState) (any, error) {
		if err := state.SkipTask(st, taskID, taskSkipReason); err != nil {
			return nil, err
		}
		if taskJSON {
			return map[string]string{"task_id": taskID, "status": "skipped"}, nil
		}
		return fmt.Sprintf("Skipped task %s", taskID), nil
	})
}

func runTaskCriterionCheck(cmd *cobra.Command, args []string) error {
	taskID := args[1]
	index, err := strconv.Atoi(args[2])
	if err != nil {
		return fmt.Errorf("invalid criterion index %q: must be an integer", args[2])
	}

	return loadAndMutate(args[0], func(st *state.PlanState) (any, error) {
		if err := state.CheckCriterion(st, taskID, index); err != nil {
			return nil, err
		}
		if taskJSON {
			return map[string]any{"task_id": taskID, "criterion": index, "done": true}, nil
		}
		return fmt.Sprintf("Checked criterion %d on task %s", index, taskID), nil
	})
}

func runTaskCriterionUncheck(cmd *cobra.Command, args []string) error {
	taskID := args[1]
	index, err := strconv.Atoi(args[2])
	if err != nil {
		return fmt.Errorf("invalid criterion index %q: must be an integer", args[2])
	}

	return loadAndMutate(args[0], func(st *state.PlanState) (any, error) {
		if err := state.UncheckCriterion(st, taskID, index); err != nil {
			return nil, err
		}
		if taskJSON {
			return map[string]any{"task_id": taskID, "criterion": index, "done": false}, nil
		}
		return fmt.Sprintf("Unchecked criterion %d on task %s", index, taskID), nil
	})
}

// parseCommaSep splits a comma-separated string into trimmed, non-empty parts.
func parseCommaSep(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// parseSemicolonSep splits a semicolon-separated string into trimmed, non-empty parts.
func parseSemicolonSep(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
