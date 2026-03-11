package cmd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/millancore/sailor/internal/deps"
	"github.com/millancore/sailor/internal/docker"
	"github.com/millancore/sailor/internal/env"
	"github.com/millancore/sailor/internal/git"
	"github.com/millancore/sailor/internal/ui"
	"github.com/spf13/cobra"
)

const (
	baseAppPort  = 8080
	baseVitePort = 5174
	maxPort      = 65535
)

var addCmd = &cobra.Command{
	Use:   "add <branch> [directory]",
	Short: "Create worktree with DB, deps, and compose config",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runAdd,
}

func runAdd(cmd *cobra.Command, args []string) error {
	branch := args[0]

	root, err := git.FindRoot()
	if err != nil {
		return err
	}

	// Parse main compose to detect services
	composePath := filepath.Join(root, "docker-compose.yml")
	compose, err := docker.ParseCompose(composePath)
	if err != nil {
		return err
	}

	appService := compose.DetectAppService()
	infraServices := compose.DetectInfraServices(appService)

	// Resolve target directory
	var targetDir string
	if len(args) > 1 {
		targetDir = args[1]
	} else {
		safeBranch := strings.ReplaceAll(branch, "/", "-")
		targetDir = filepath.Join(filepath.Dir(root), fmt.Sprintf("%s-%s", filepath.Base(root), safeBranch))
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}

	// Duplicate check
	worktrees, err := git.ListWorktrees()
	if err != nil {
		return err
	}
	for _, wt := range worktrees {
		if wt.Path == absTarget {
			return fmt.Errorf("worktree already exists: %s", absTarget)
		}
	}

	// Branch check
	if !git.BranchExists(root, branch) {
		ui.Warn("Branch '%s' does not exist.", branch)
		confirmed, err := ui.Confirm("Create from current HEAD?", "")
		if err != nil || !confirmed {
			return fmt.Errorf("cancelled")
		}
		if err := git.CreateBranch(root, branch); err != nil {
			return fmt.Errorf("failed to create branch: %w", err)
		}
		ui.Success("Created branch '%s'", branch)
	}

	// ── 1. Git worktree ──
	ui.Step(1, 5, "Creating git worktree")
	if err := ui.Spin("Creating git worktree", func() error {
		return git.Add(root, absTarget, branch)
	}); err != nil {
		return fmt.Errorf("git worktree add failed: %w", err)
	}

	// ── 2. Copy dependencies ──
	ui.Step(2, 5, "Copying dependencies")
	var needComposer, needNpm bool
	if err := ui.Spin("Copying dependencies", func() error {
		needComposer, needNpm = deps.CopyDeps(root, absTarget)
		return nil
	}); err != nil {
		return err
	}

	if !needComposer {
		ui.Success("vendor/ copied — lock files match")
	} else {
		ui.Info("vendor/ needs install")
		runInstall(absTarget, "composer")
	}
	if !needNpm {
		ui.Success("node_modules/ copied — lock files match")
	} else {
		ui.Info("node_modules/ needs install")
		runInstall(absTarget, "npm")
	}

	deps.EnsureStorageDirs(absTarget)

	// ── 3. Database ──
	ui.Step(3, 5, "Setting up database")

	mainEnvPath := filepath.Join(root, ".env")
	sourceDB := env.Get(mainEnvPath, "DB_DATABASE", "laravel")
	dbName := docker.SanitizeDBName(fmt.Sprintf("%s_%s", sourceDB, strings.ReplaceAll(branch, "/", "_")))

	runMigrateLater := false

	dbInfo, dbErr := docker.DetectDB(root)
	if dbErr == nil && docker.DBIsReachable(dbInfo) {
		sourceHasTables := docker.DBHasTables(dbInfo, sourceDB)

		var opts []ui.SelectOption
		if sourceHasTables {
			opts = []ui.SelectOption{
				{Label: "migrate --seed", Description: "Fresh migrations and seeders, validates your migration files", Value: "1"},
				{Label: fmt.Sprintf("Snapshot from '%s'", sourceDB), Description: "Copy table structure and all data", Value: "2"},
				{Label: fmt.Sprintf("Schema only from '%s'", sourceDB), Description: "Copy table structure only, no data", Value: "3"},
				{Label: fmt.Sprintf("Share '%s'", sourceDB), Description: "Reuse the main branch database, no copy", Value: "4"},
				{Label: "Skip", Description: "Leave the database empty", Value: "5"},
			}
		} else {
			opts = []ui.SelectOption{
				{Label: "migrate --seed", Description: "Fresh migrations and seeders, validates your migration files", Value: "1"},
				{Label: fmt.Sprintf("Snapshot from '%s'", sourceDB), Description: "Source has no tables — will run on an empty database", Value: "2"},
				{Label: fmt.Sprintf("Schema only from '%s'", sourceDB), Description: "Source has no tables — will run on an empty database", Value: "3"},
				{Label: fmt.Sprintf("Share '%s'", sourceDB), Description: "Reuse the main branch database, no copy", Value: "4"},
				{Label: "Skip", Description: "Leave the database empty", Value: "5"},
			}
		}
		dbChoice, _ := ui.Select("How to populate the database?", opts, "1")

		if dbChoice == "4" {
			dbName = sourceDB
			ui.Success("Using shared database: %s", sourceDB)
		} else {
			ui.Info("Creating database: %s", dbName)
			if err := docker.DBCreateDB(dbInfo, dbName); err != nil {
				ui.Error("Failed to create database: %v", err)
			}

			switch dbChoice {
			case "1":
				runMigrateLater = true
			case "2":
				if sourceHasTables {
					if err := ui.Spin("Copying schema + data", func() error {
						dump, err := docker.DBDump(dbInfo, sourceDB, false)
						if err != nil {
							return err
						}
						return docker.DBImport(dbInfo, dbName, dump)
					}); err != nil {
						ui.Error("Failed to copy data: %v", err)
					} else {
						ui.Success("Full copy done")
					}
				} else {
					ui.Warn("No tables in source — will migrate --seed after start")
					runMigrateLater = true
				}
			case "3":
				if sourceHasTables {
					if err := ui.Spin("Copying schema", func() error {
						dump, err := docker.DBDump(dbInfo, sourceDB, true)
						if err != nil {
							return err
						}
						return docker.DBImport(dbInfo, dbName, dump)
					}); err != nil {
						ui.Error("Failed to copy schema: %v", err)
					} else {
						ui.Success("Schema copied")
					}
				} else {
					ui.Warn("No tables in source — will migrate --seed after start")
					runMigrateLater = true
				}
			case "5":
				ui.Info("Skipped")
			}
		}

		if runMigrateLater {
			os.WriteFile(filepath.Join(absTarget, ".sailor-migrate"), []byte(""), 0644)
		}
	} else {
		ui.Warn("Database not reachable — is the main branch running? (sail up -d)")
		ui.Warn("Skipping DB setup.")
	}

	// ── 4. Configure .env ──
	ui.Step(4, 5, "Configuring .env")

	appPort, vitePort, err := allocatePorts(root)
	if err != nil {
		return fmt.Errorf("port allocation failed: %w", err)
	}

	envSrc := filepath.Join(root, ".env")
	envDst := filepath.Join(absTarget, ".env")
	if _, err := os.Stat(envSrc); err == nil {
		env.Copy(envSrc, envDst)
	} else if _, err := os.Stat(filepath.Join(absTarget, ".env.example")); err == nil {
		env.Copy(filepath.Join(absTarget, ".env.example"), envDst)
	}

	if _, err := os.Stat(envDst); err == nil {
		updates := map[string]string{
			"APP_PORT":     fmt.Sprintf("%d", appPort),
			"APP_URL":      fmt.Sprintf("http://localhost:%d", appPort),
			"APP_NAME":     fmt.Sprintf("\"%s-%s\"", filepath.Base(root), branch),
			"DB_DATABASE":  dbName,
			"REDIS_PREFIX": docker.SanitizeDBName(fmt.Sprintf("%s_%s", sourceDB, branch)) + "_",
			"VITE_PORT":    fmt.Sprintf("%d", vitePort),
		}
		if err := env.Write(envDst, updates); err != nil {
			ui.Warn("Failed to update .env: %v", err)
		} else {
			ui.Success(".env configured (port=%d, db=%s)", appPort, dbName)
		}
	} else {
		ui.Warn("No .env created — configure manually")
	}

	// ── 5. Write docker-compose.override.yml ──
	ui.Step(5, 5, "Writing docker-compose.override.yml")

	networkName, netErr := docker.DetectSailNetwork(root)
	if netErr != nil {
		ui.Warn("Could not detect Sail network: %v", netErr)
		ui.Warn("Make sure your main branch is running: sail up -d")
	} else {
		if err := docker.WriteWorktreeOverride(absTarget, appService, infraServices, networkName); err != nil {
			ui.Warn("Failed to write override: %v", err)
		} else {
			ui.Success("Created docker-compose.override.yml")
			addToGitignore(absTarget, "docker-compose.override.yml")
		}
	}

	// ── Done — summary box ──
	fmt.Println()
	fmt.Println(ui.SummaryBox("Ready!", []string{
		fmt.Sprintf("%s    %s", ui.Dim("Branch:"), branch),
		fmt.Sprintf("%s %s", ui.Dim("Directory:"), absTarget),
		fmt.Sprintf("%s  %s", ui.Dim("Database:"), dbName),
		fmt.Sprintf("%s   %s", ui.Dim("App URL:"), fmt.Sprintf("http://localhost:%d", appPort)),
		fmt.Sprintf("%s      %s", ui.Dim("Vite:"), fmt.Sprintf("localhost:%d", vitePort)),
		"",
		fmt.Sprintf("%s cd %s && sailor up", ui.Bold("Start:"), absTarget),
	}))

	return nil
}

// allocatePorts scans existing worktree .env files and checks system
// availability to find unused ports within the valid range.
func allocatePorts(root string) (appPort, vitePort int, err error) {
	usedApp := make(map[int]bool)
	usedVite := make(map[int]bool)

	worktrees, wtErr := git.ListWorktrees()
	if wtErr == nil {
		for _, wt := range worktrees {
			if wt.Path == root {
				continue
			}
			envPath := filepath.Join(wt.Path, ".env")
			vals, err := env.Read(envPath)
			if err != nil {
				continue
			}
			if p := parsePort(vals["APP_PORT"]); p > 0 {
				usedApp[p] = true
			}
			if p := parsePort(vals["VITE_PORT"]); p > 0 {
				usedVite[p] = true
			}
		}
	}

	appPort, err = findAvailablePort(baseAppPort, usedApp)
	if err != nil {
		return 0, 0, fmt.Errorf("APP_PORT: %w", err)
	}
	vitePort, err = findAvailablePort(baseVitePort, usedVite)
	if err != nil {
		return 0, 0, fmt.Errorf("VITE_PORT: %w", err)
	}
	return appPort, vitePort, nil
}

// findAvailablePort returns the first port starting from base that is not in
// the used set and is not already bound on the system.
func findAvailablePort(base int, used map[int]bool) (int, error) {
	for port := base; port <= maxPort; port++ {
		if used[port] {
			continue
		}
		if portAvailable(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found in range %d–%d", base, maxPort)
}

// portAvailable checks whether a TCP port is free on localhost.
func portAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

func parsePort(s string) int {
	p, _ := strconv.Atoi(strings.TrimSpace(s))
	return p
}

func runInstall(dir, tool string) {
	switch tool {
	case "composer":
		if _, err := os.Stat(filepath.Join(dir, "composer.json")); err == nil {
			if err := ui.Spin("Running composer install", func() error {
				return execIn(dir, "composer", "install", "--quiet")
			}); err != nil {
				ui.Warn("composer install failed — run manually")
			}
		}
	case "npm":
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			if err := ui.Spin("Running npm install", func() error {
				return execIn(dir, "npm", "install", "--silent")
			}); err != nil {
				ui.Warn("npm install failed — run manually")
			}
		}
	}
}

func execIn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Run()
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
