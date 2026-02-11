// Package cli provides the command-line interface for ralph.
package cli

import (
	"fmt"
	"os"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/spf13/cobra"
)

// ANSI color codes for status output
const (
	statusColorReset  = "\033[0m"
	statusColorGreen  = "\033[32m"
	statusColorYellow = "\033[33m"
	statusColorGray   = "\033[90m"
	statusColorCyan   = "\033[36m"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display project status from Board",
	Long: `Display the current project status from the Board task management service.

Shows:
- Project information
- Active plan (if any) with task statistics
- Available and blocked tasks
- Recent progress entries`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.LoadWithDefaults(GetConfigPath())
	if err != nil {
		log.Warn("Failed to load config, using defaults: %v", err)
		cfg = config.Defaults()
	}

	// Determine project slug
	projectSlug := cfg.Board.ProjectSlug
	if projectSlug == "" {
		return fmt.Errorf("board.project_slug not configured; run 'ralph init' or set it in .ralph/config.yaml")
	}

	// Create Board client
	boardClient := cfg.BoardClient()

	// Fetch project context
	agentCtx, err := boardClient.ProjectContext(projectSlug)
	if err != nil {
		return fmt.Errorf("fetching project context: %w", err)
	}

	// Determine if we should use colors
	useColor := !noColor && isTerminalFd(os.Stdout)

	// Print project info
	if useColor {
		fmt.Printf("%sProject:%s %s (%s)\n", statusColorCyan, statusColorReset, agentCtx.Project.Name, agentCtx.Project.Slug)
	} else {
		fmt.Printf("Project: %s (%s)\n", agentCtx.Project.Name, agentCtx.Project.Slug)
	}
	if agentCtx.Project.Description != "" {
		fmt.Printf("  %s\n", agentCtx.Project.Description)
	}
	fmt.Println()

	// Print active plan
	if agentCtx.Plan.ID > 0 {
		if useColor {
			fmt.Printf("%sActive Plan:%s %s (ID: %d, status: %s)\n",
				statusColorGreen, statusColorReset,
				agentCtx.Plan.Title, agentCtx.Plan.ID, agentCtx.Plan.Status)
		} else {
			fmt.Printf("Active Plan: %s (ID: %d, status: %s)\n",
				agentCtx.Plan.Title, agentCtx.Plan.ID, agentCtx.Plan.Status)
		}
		if agentCtx.Plan.FeatureBranch != "" {
			fmt.Printf("  Branch: %s\n", agentCtx.Plan.FeatureBranch)
		}

		// Task stats
		stats := agentCtx.Stats
		if stats.TotalTasks > 0 {
			completed := stats.Done + stats.Skipped
			pct := 0
			if stats.TotalTasks > 0 {
				pct = completed * 100 / stats.TotalTasks
			}
			fmt.Printf("  Tasks: %d/%d completed (%d%%)\n", completed, stats.TotalTasks, pct)
			fmt.Printf("    Done: %d  Doing: %d  Claimed: %d  Blocked: %d  Available: %d  Skipped: %d\n",
				stats.Done, stats.Doing, stats.Claimed, stats.Blocked, stats.Available, stats.Skipped)
		}
	} else {
		if useColor {
			fmt.Printf("%sActive Plan:%s (none)\n", statusColorGray, statusColorReset)
		} else {
			fmt.Println("Active Plan: (none)")
		}
	}
	fmt.Println()

	// Available tasks
	if len(agentCtx.AvailableTasks) > 0 {
		if useColor {
			fmt.Printf("%sAvailable Tasks:%s %d\n", statusColorYellow, statusColorReset, len(agentCtx.AvailableTasks))
		} else {
			fmt.Printf("Available Tasks: %d\n", len(agentCtx.AvailableTasks))
		}
		for _, task := range agentCtx.AvailableTasks {
			fmt.Printf("  - [%d] %s\n", task.ID, task.Title)
		}
		fmt.Println()
	}

	// Blocked tasks
	if len(agentCtx.BlockedTasks) > 0 {
		fmt.Printf("Blocked Tasks: %d\n", len(agentCtx.BlockedTasks))
		for _, task := range agentCtx.BlockedTasks {
			fmt.Printf("  - [%d] %s\n", task.ID, task.Title)
		}
		fmt.Println()
	}

	// Recent progress
	if len(agentCtx.RecentProgress) > 0 {
		fmt.Println("Recent Progress:")
		limit := len(agentCtx.RecentProgress)
		if limit > 5 {
			limit = 5
		}
		for _, p := range agentCtx.RecentProgress[:limit] {
			fmt.Printf("  [%s] %s: %s\n", p.CreatedAt, p.Author, truncate(p.Body, 80))
		}
		fmt.Println()
	}

	// Recent feedback
	if len(agentCtx.RecentFeedback) > 0 {
		fmt.Println("Recent Feedback:")
		limit := len(agentCtx.RecentFeedback)
		if limit > 5 {
			limit = 5
		}
		for _, f := range agentCtx.RecentFeedback[:limit] {
			fmt.Printf("  [%s] %s: %s\n", f.CreatedAt, f.Author, truncate(f.Body, 80))
		}
	}

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

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
