package cmd

import (
	"os"

	"github.com/millancore/sailor/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sailor",
	Short: "Run multiple Laravel Sail branches in parallel",
	Long:  ui.Banner(),
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(portsCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(pruneCmd)
}
