// Package cli provides the command-line interface for ralph.
package cli

import (
	"os"

	"github.com/arvesolland/ralph/internal/log"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	configPath string
	verbose    bool
	quiet      bool
	noColor    bool
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "ralph",
	Short: "Autonomous AI development loop orchestration",
	Long: `Ralph is an autonomous AI development loop orchestration system
implementing the "Ralph Wiggum technique" - fresh context per iteration
with progress persisted in files and git.

Ralph manages plan-based development workflows where an AI agent executes
tasks iteratively, with each iteration getting a fresh context window
while progress is tracked in plan files and git commits.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Configure logging based on flags
		logger := log.Default().(*log.ConsoleLogger)

		if verbose {
			logger.SetLevel(log.LevelDebug)
		} else if quiet {
			logger.SetLevel(log.LevelWarn)
		}

		if noColor {
			logger.SetColorEnabled(false)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Global persistent flags
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", ".ralph/config.yaml", "config file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output (debug level)")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress informational output (warnings and errors only)")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable color output")
}

// GetConfigPath returns the config path from flags.
func GetConfigPath() string {
	return configPath
}
