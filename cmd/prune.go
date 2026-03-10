package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/millancore/sailor/internal/docker"
	"github.com/millancore/sailor/internal/env"
	"github.com/millancore/sailor/internal/git"
	"github.com/millancore/sailor/internal/ui"
	"github.com/spf13/cobra"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Stop all worktree containers and remove all worktrees",
	RunE:  runPrune,
}

func runPrune(cmd *cobra.Command, args []string) error {
	root, err := git.FindRoot()
	if err != nil {
		return err
	}

	worktrees, err := git.ListWorktrees()
	if err != nil {
		return err
	}

	// Collect non-main worktrees
	var targets []git.Worktree
	for _, wt := range worktrees {
		if wt.Path != root {
			targets = append(targets, wt)
		}
	}

	if len(targets) == 0 {
		ui.Info("No worktrees to prune")
		return nil
	}

	fmt.Println()
	fmt.Printf("  %s\n", ui.Dim("Worktrees to remove:"))
	for _, wt := range targets {
		fmt.Printf("    • %s (%s)\n", wt.Branch, wt.Path)
	}
	fmt.Println()

	confirmed, err := ui.Confirm(
		fmt.Sprintf("Prune all %d worktree(s)?", len(targets)),
		"This will stop containers, drop databases, and delete all worktrees.",
	)
	if err != nil || !confirmed {
		ui.Info("Cancelled")
		return nil
	}

	mainDB := env.Get(filepath.Join(root, ".env"), "DB_DATABASE", "")

	dbInfo, dbErr := docker.DetectDB(root)
	dbReachable := dbErr == nil && docker.DBIsReachable(dbInfo)

	for _, wt := range targets {
		branch := wt.Branch
		path := wt.Path

		if err := ui.Spin(fmt.Sprintf("Stopping container: %s", branch), func() error {
			docker.ComposeDown(path)
			return nil
		}); err != nil {
			ui.Warn("Could not stop container for %s: %v", branch, err)
		}

		dbName := env.Get(filepath.Join(path, ".env"), "DB_DATABASE", "")
		if dbName != "" && dbName != mainDB && dbReachable {
			if err := ui.Spin(fmt.Sprintf("Dropping database: %s", dbName), func() error {
				return docker.DBDropDB(dbInfo, dbName)
			}); err != nil {
				ui.Warn("Could not drop database %s: %v", dbName, err)
			}
		}

		os.Remove(filepath.Join(path, "docker-compose.override.yml"))

		if err := ui.Spin(fmt.Sprintf("Removing worktree: %s", branch), func() error {
			return git.Remove(root, path)
		}); err != nil {
			ui.Warn("Failed to remove worktree %s: %v", branch, err)
		}
	}

	ui.Success("Pruned %d worktree(s)", len(targets))
	return nil
}
