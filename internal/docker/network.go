package docker

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/millancore/sailor/internal/env"
)

// sanitizeProjectName mirrors Docker Compose's project name sanitization:
// lowercase, strip everything except [a-z0-9-].
func sanitizeProjectName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NetworkExists checks if a Docker network exists.
func NetworkExists(name string) bool {
	return exec.Command("docker", "network", "inspect", name).Run() == nil
}

// DetectSailNetwork finds the Docker network that Laravel Sail created.
// It parses the compose file to find the first non-external network key,
// then constructs the full name as <project>_<network>.
func DetectSailNetwork(rootDir string) (string, error) {
	composePath := filepath.Join(rootDir, "docker-compose.yml")
	compose, err := ParseCompose(composePath)
	if err != nil {
		return "", err
	}

	networkKey := compose.detectFirstLocalNetwork()
	if networkKey == "" {
		return "", fmt.Errorf("no local network found in docker-compose.yml")
	}

	// Project name: COMPOSE_PROJECT_NAME from .env, fallback to directory basename
	// Docker Compose sanitizes the project name: lowercase, keep only [a-z0-9-].
	envPath := filepath.Join(rootDir, ".env")
	projectName := env.Get(envPath, "COMPOSE_PROJECT_NAME", "")
	if projectName == "" {
		projectName = sanitizeProjectName(filepath.Base(rootDir))
	}

	fullName := projectName + "_" + networkKey
	if !NetworkExists(fullName) {
		return "", fmt.Errorf("network '%s' not found — make sure your main branch is running: sail up -d", fullName)
	}

	return fullName, nil
}
