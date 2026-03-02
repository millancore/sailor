package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/millancore/sailor/internal/docker"
	"github.com/millancore/sailor/internal/env"
	"github.com/millancore/sailor/internal/git"
	"github.com/millancore/sailor/internal/ui"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:     "remove <directory|branch>",
	Aliases: []string{"rm"},
	Short:   "Stop container, drop DB, and remove worktree",
	Args:    cobra.ExactArgs(1),
	RunE:    runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
	target := args[0]

	root, err := git.FindRoot()
	if err != nil {
		return err
	}

	worktrees, err := git.ListWorktrees()
	if err != nil {
		return err
	}

	// Resolve target to a worktree
	var found *git.Worktree
	for i, wt := range worktrees {
		if wt.Path == root {
			continue
		}
		absTarget, _ := filepath.Abs(target)
		if wt.Path == absTarget || wt.Branch == target {
			found = &worktrees[i]
			break
		}
	}

	if found == nil {
		for i, wt := range worktrees {
			if wt.Path == root {
				continue
			}
			if strings.ReplaceAll(wt.Branch, "/", "-") == strings.ReplaceAll(target, "/", "-") {
				found = &worktrees[i]
				break
			}
		}
	}

	if found == nil {
		return fmt.Errorf("worktree not found: '%s'", target)
	}

	envPath := filepath.Join(found.Path, ".env")
	dbName := env.Get(envPath, "DB_DATABASE", "")

	fmt.Println()
	fmt.Printf("  %s %s\n", ui.Dim("Directory:"), found.Path)
	fmt.Printf("  %s  %s\n", ui.Dim("Database:"), orNone(dbName))
	fmt.Println()

	confirmed, err := ui.Confirm(
		"Remove this worktree?",
		"This will stop the container, drop the database, and delete the worktree.",
	)
	if err != nil || !confirmed {
		ui.Info("Cancelled")
		return nil
	}

	// Stop container
	if err := ui.Spin("Stopping container", func() error {
		docker.ComposeDown(found.Path)
		return nil
	}); err != nil {
		return err
	}

	// Drop database
	if dbName != "" {
		dbInfo, dbErr := docker.DetectDB(root)
		if dbErr == nil && docker.DBIsReachable(dbInfo) {
			if err := ui.Spin(fmt.Sprintf("Dropping database: %s", dbName), func() error {
				return docker.DBDropDB(dbInfo, dbName)
			}); err != nil {
				ui.Warn("Could not drop database: %v", err)
			}
		}
	}

	// Remove override file (best-effort)
	os.Remove(filepath.Join(found.Path, "docker-compose.override.yml"))

	// Remove git worktree
	if err := ui.Spin("Removing git worktree", func() error {
		return git.Remove(root, found.Path)
	}); err != nil {
		ui.Warn("Failed to remove worktree: %v", err)
	}

	ui.Success("Removed: %s", target)
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
