package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// These variables are set at build time using -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// versionCmd represents the version command.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Long:  `Display the version, git commit, and build date of this ralph binary.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ralph version %s\n", Version)
		fmt.Printf("  commit:  %s\n", Commit)
		fmt.Printf("  built:   %s\n", BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
