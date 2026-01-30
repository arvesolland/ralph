// Package cli provides the command-line interface for ralph.
package cli

import (
	"fmt"
	"os"

	"github.com/arvesolland/ralph/internal/plan"
	"github.com/spf13/cobra"
)

// ANSI color codes for status output
const (
	statusColorReset  = "\033[0m"
	statusColorGreen  = "\033[32m"
	statusColorYellow = "\033[33m"
	statusColorGray   = "\033[90m"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display queue status and worktree information",
	Long: `Display the current state of the plan queue and worktrees.

Shows:
- Count of plans in each queue (pending, current, complete)
- Current plan name and branch if one is active
- List of pending plans by name
- Worktree status (count, paths)`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Find plans directory (relative to current working directory)
	plansDir := "plans"

	// Check if plans directory exists
	if _, err := os.Stat(plansDir); os.IsNotExist(err) {
		fmt.Println("No plans directory found. Run 'ralph init' to initialize.")
		return nil
	}

	queue := plan.NewQueue(plansDir)
	status, err := queue.Status()
	if err != nil {
		return fmt.Errorf("getting queue status: %w", err)
	}

	// Determine if we should use colors
	useColor := !noColor && isTerminalFd(os.Stdout)

	// Print header
	fmt.Println("Queue Status")
	fmt.Println("============")
	fmt.Println()

	// Current plan (green)
	if status.CurrentPlan != "" {
		if useColor {
			fmt.Printf("%sCurrent:%s %s (branch: feat/%s)\n",
				statusColorGreen, statusColorReset,
				status.CurrentPlan, status.CurrentPlan)
		} else {
			fmt.Printf("Current: %s (branch: feat/%s)\n",
				status.CurrentPlan, status.CurrentPlan)
		}
	} else {
		if useColor {
			fmt.Printf("%sCurrent:%s (none)\n", statusColorGray, statusColorReset)
		} else {
			fmt.Println("Current: (none)")
		}
	}
	fmt.Println()

	// Pending plans (yellow)
	if useColor {
		fmt.Printf("%sPending:%s %d plan(s)\n", statusColorYellow, statusColorReset, status.PendingCount)
	} else {
		fmt.Printf("Pending: %d plan(s)\n", status.PendingCount)
	}
	if len(status.PendingPlans) > 0 {
		for _, name := range status.PendingPlans {
			fmt.Printf("  - %s\n", name)
		}
	}
	fmt.Println()

	// Complete count
	fmt.Printf("Complete: %d plan(s)\n", status.CompleteCount)
	fmt.Println()

	// Worktree status (placeholder until worktree module is implemented)
	fmt.Println("Worktrees")
	fmt.Println("---------")
	fmt.Printf("  (worktree status not yet implemented)\n")

	return nil
}

// isTerminalFd checks if the given file is a terminal.
func isTerminalFd(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
