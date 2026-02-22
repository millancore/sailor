package docker

import (
	"fmt"
	"os/exec"
	"strings"
)

// FindPostgresContainer detects the PostgreSQL container name from docker compose ps.
func FindPostgresContainer(mainDir string) (string, error) {
	containers, err := ComposePS(mainDir)
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %w", err)
	}

	for _, c := range containers {
		svc := strings.ToLower(c.Service)
		if svc == "postgres" || svc == "postgresql" || svc == "pgsql" || svc == "db" {
			return c.Name, nil
		}
	}

	return "", fmt.Errorf("no PostgreSQL container found — is the main branch running?")
}

// PostgresIsReachable checks if the PostgreSQL container is ready.
func PostgresIsReachable(container, user string) bool {
	if container == "" {
		return false
	}
	err := exec.Command("docker", "exec", container, "pg_isready", "-U", user).Run()
	return err == nil
}

// PostgresExec runs a SQL command via psql connected to the maintenance database.
func PostgresExec(container, user, password, sql string) error {
	args := []string{"exec", "-e", "PGPASSWORD=" + password, container, "psql", "-U", user, "-d", "postgres", "-tAc", sql}
	cmd := exec.Command("docker", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// PostgresCreateDB creates a database.
func PostgresCreateDB(container, user, password, dbName string) error {
	sql := fmt.Sprintf("CREATE DATABASE \"%s\";", dbName)
	return PostgresExec(container, user, password, sql)
}

// PostgresDropDB drops a database, terminating any active connections first.
func PostgresDropDB(container, user, password, dbName string) error {
	terminate := fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();",
		dbName,
	)
	_ = PostgresExec(container, user, password, terminate)
	sql := fmt.Sprintf("DROP DATABASE IF EXISTS \"%s\";", dbName)
	return PostgresExec(container, user, password, sql)
}

// PostgresHasTables checks if a database has any tables in the public schema.
func PostgresHasTables(container, user, password, dbName string) bool {
	args := []string{
		"exec", "-e", "PGPASSWORD=" + password, container,
		"psql", "-U", user, "-d", dbName, "-tAc",
		"SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' LIMIT 1;",
	}
	out, err := exec.Command("docker", args...).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// PostgresDump dumps a database using pg_dump.
func PostgresDump(container, user, password, dbName string, schemaOnly bool) (string, error) {
	args := []string{"exec", "-e", "PGPASSWORD=" + password, container, "pg_dump", "-U", user}
	if schemaOnly {
		args = append(args, "-s")
	}
	args = append(args, dbName)
	out, err := exec.Command("docker", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// PostgresImport imports SQL into a database via psql stdin.
func PostgresImport(container, user, password, dbName, sql string) error {
	args := []string{"exec", "-i", "-e", "PGPASSWORD=" + password, container, "psql", "-U", user, "-d", dbName}
	cmd := exec.Command("docker", args...)
	cmd.Stdin = strings.NewReader(sql)
	return cmd.Run()
}
