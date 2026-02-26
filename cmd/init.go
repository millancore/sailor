package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/millancore/sailor/internal/docker"
	"github.com/millancore/sailor/internal/git"
	"github.com/millancore/sailor/internal/ui"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Validate Sail is running and show usage instructions",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	root, err := git.FindRoot()
	if err != nil {
		return err
	}

	composePath := filepath.Join(root, "docker-compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("no docker-compose.yml found in %s", root)
	}

	ui.Header("Initializing sailor")

	// Parse compose to detect services (informational only)
	compose, err := docker.ParseCompose(composePath)
	if err != nil {
		return err
	}

	appService := compose.DetectAppService()
	infraServices := compose.DetectInfraServices(appService)

	ui.Info("App service detected: %s", appService)
	if len(infraServices) > 0 {
		ui.Info("Infra services detected: %s", strings.Join(infraServices, ", "))
	} else {
		ui.Warn("No infra services detected in docker-compose.yml")
	}

	// Detect Sail's existing network (validates that Sail is running)
	networkName, err := docker.DetectSailNetwork(root)
	if err != nil {
		return fmt.Errorf("could not detect Sail network: %w\n  Make sure your main branch is running: sail up -d", err)
	}
	ui.Info("Detected Sail network: %s", networkName)

	// Add docker-compose.override.yml to .gitignore
	addToGitignore(root, "docker-compose.override.yml")

	ui.Success("Ready!")
	fmt.Println()
	fmt.Printf("  %s\n", ui.Bold("How to use:"))
	fmt.Println()
	fmt.Printf("  %s Start your main branch:\n", ui.Dim("1."))
	fmt.Printf("     %s\n", ui.Cyan("sail up -d"))
	fmt.Println()
	fmt.Printf("  %s Add worktrees:\n", ui.Dim("2."))
	fmt.Printf("     %s\n", ui.Cyan("sailor add feature/payments"))
	fmt.Println()
	fmt.Printf("  %s Main branch must be running — it provides MySQL, Redis, etc.\n", ui.Dim("Note:"))

	return nil
}

func addToGitignore(root string, entries ...string) {
	gitignorePath := filepath.Join(root, ".gitignore")

	existing := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	}

	var toAdd []string
	for _, entry := range entries {
		if !strings.Contains(existing, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) > 0 {
		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()
		for _, entry := range toAdd {
			f.WriteString(entry + "\n")
		}
	}
}
