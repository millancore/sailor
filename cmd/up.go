package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/millancore/sailor/internal/docker"
	"github.com/millancore/sailor/internal/env"
	"github.com/millancore/sailor/internal/git"
	"github.com/millancore/sailor/internal/ui"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up [directory]",
	Short: "Start app container (default: current directory)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUp,
}

func runUp(cmd *cobra.Command, args []string) error {
	target := "."
	if len(args) > 0 {
		target = args[0]
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(absTarget, "docker-compose.yml")); os.IsNotExist(err) {
		return fmt.Errorf("no docker-compose.yml in %s", absTarget)
	}

	// Check MySQL reachability
	root, err := git.FindRoot()
	if err != nil {
		return err
	}

	// Verify the sail network exists
	if _, err := docker.DetectSailNetwork(root); err != nil {
		return fmt.Errorf("sail network not found. Is your main branch running? (sail up -d)")
	}

	dbInfo, dbErr := docker.DetectDB(root)
	if dbErr != nil || !docker.DBIsReachable(dbInfo) {
		ui.Warn("Database is not reachable. Is your main branch running?")
		mainDir := root
		fmt.Printf("  %s Start it with: cd %s && sail up -d\n", ui.Dim("→"), mainDir)
		fmt.Println()
		fmt.Print("  Continue anyway? [y/N] ")
		answer := readLine()
		if answer == "" || answer[0] != 'y' && answer[0] != 'Y' {
			return nil
		}
	}

	ui.Header("Starting app container")

	mainComposePath := filepath.Join(root, "docker-compose.yml")
	mainCompose, err := docker.ParseCompose(mainComposePath)
	if err != nil {
		return err
	}
	appService := mainCompose.DetectAppService()

	if err := docker.ComposeUp(absTarget, appService); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}

	// Run pending migrations
	migrateMarker := filepath.Join(absTarget, ".sailor-migrate")
	if _, err := os.Stat(migrateMarker); err == nil {
		ui.Info("Running migrate --seed...")
		time.Sleep(3 * time.Second)
		if err := docker.ComposeExec(absTarget, appService, "php", "artisan", "migrate", "--seed", "--force"); err != nil {
			ui.Warn("Migration failed — run manually")
		}
		os.Remove(migrateMarker)
	}

	ui.Success("App is running")
	envPath := filepath.Join(absTarget, ".env")
	if url := env.Get(envPath, "APP_URL", ""); url != "" {
		fmt.Printf("  %s %s\n", ui.Dim("→"), url)
	}

	return nil
}
