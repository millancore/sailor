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

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List worktrees and their status",
	RunE:    runList,
}

func runList(cmd *cobra.Command, args []string) error {
	root, err := git.FindRoot()
	if err != nil {
		return err
	}

	worktrees, err := git.ListWorktrees()
	if err != nil {
		return err
	}

	// Separate head worktree from linked ones
	var head git.Worktree
	var wts []git.Worktree
	for _, wt := range worktrees {
		if wt.Path == root {
			head = wt
		} else {
			wts = append(wts, wt)
		}
	}

	// Print head branch info above the table
	headStatus := ui.Red("stopped")
	dbInfo, cErr := docker.DetectDB(root)
	if cErr == nil && docker.DBIsReachable(dbInfo) {
		headStatus = ui.Green("running (infra)")
	}
	fmt.Printf("\n  %s  %s  %s\n", ui.Bold(head.Branch), ui.Dim(shortenHome(head.Path)), headStatus)

	// Build table rows
	headers := []string{"BRANCH", "DIRECTORY", "PORT", "DATABASE", "STATUS", "URL"}

	var rows [][]string

	for _, wt := range wts {
		shortDir := shortenHome(wt.Path)
		envPath := filepath.Join(wt.Path, ".env")
		appPort := env.Get(envPath, "APP_PORT", "?")
		dbName := env.Get(envPath, "DB_DATABASE", "?")

		status := ui.Red("stopped")
		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			status = ui.Yellow("missing")
		} else {
			port := 0
			fmt.Sscanf(appPort, "%d", &port)
			if port > 0 && docker.IsPortInUse(port) {
				status = ui.Green("running")
			}
		}

		urlCell := "—"
		if appPort != "?" {
			rawURL := "http://localhost:" + appPort
			urlCell = ui.Link(rawURL, rawURL)
		}

		rows = append(rows, []string{wt.Branch, shortDir, appPort, dbName, status, urlCell})
	}

	fmt.Println()
	fmt.Println(ui.Table(headers, rows))

	if len(wts) == 0 {
		fmt.Println()
		ui.Info("No worktrees. Use 'sailor add <branch>'.")
	}

	return nil
}


func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
