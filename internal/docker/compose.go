package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const BackupSuffix = ".sailor-backup"

// ComposeFile represents a parsed docker-compose.yml as a yaml.Node tree.
type ComposeFile struct {
	Path string
	Root *yaml.Node // Document node
}

// ParseCompose reads and parses a docker-compose.yml.
func ParseCompose(path string) (*ComposeFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}

	return &ComposeFile{Path: path, Root: &doc}, nil
}

// Save writes the compose file back to disk.
func (c *ComposeFile) Save() error {
	data, err := yaml.Marshal(c.Root)
	if err != nil {
		return err
	}
	return os.WriteFile(c.Path, data, 0644)
}

// DetectAppService finds the app service name (usually laravel.test).
func (c *ComposeFile) DetectAppService() string {
	services := c.getServicesNode()
	if services == nil {
		return "laravel.test"
	}

	for i := 0; i < len(services.Content)-1; i += 2 {
		name := services.Content[i].Value
		if name == "laravel.test" {
			return name
		}
	}

	// Fallback: first service
	if len(services.Content) >= 2 {
		return services.Content[0].Value
	}

	return "laravel.test"
}

// DetectAppDependsOn returns the service names listed in the app service's depends_on.
func (c *ComposeFile) DetectAppDependsOn(appService string) []string {
	services := c.getServicesNode()
	if services == nil {
		return nil
	}
	for i := 0; i < len(services.Content)-1; i += 2 {
		if services.Content[i].Value != appService {
			continue
		}
		serviceBody := services.Content[i+1]
		for j := 0; j < len(serviceBody.Content)-1; j += 2 {
			if serviceBody.Content[j].Value != "depends_on" {
				continue
			}
			depNode := serviceBody.Content[j+1]
			var deps []string
			switch depNode.Kind {
			case yaml.SequenceNode:
				// depends_on: [mysql, redis]
				for _, item := range depNode.Content {
					deps = append(deps, item.Value)
				}
			case yaml.MappingNode:
				// depends_on: {mysql: {condition: ...}}
				for k := 0; k < len(depNode.Content)-1; k += 2 {
					deps = append(deps, depNode.Content[k].Value)
				}
			}
			return deps
		}
	}
	return nil
}

// DetectInfraServices returns all service names except the app service.
func (c *ComposeFile) DetectInfraServices(appService string) []string {
	services := c.getServicesNode()
	if services == nil {
		return nil
	}

	var infra []string
	for i := 0; i < len(services.Content)-1; i += 2 {
		name := services.Content[i].Value
		if name != appService {
			infra = append(infra, name)
		}
	}
	return infra
}

// detectFirstLocalNetwork returns the first non-external network key in the top-level networks section.
func (c *ComposeFile) detectFirstLocalNetwork() string {
	networks := c.getTopLevelMapping("networks")
	if networks == nil {
		return ""
	}
	for i := 0; i < len(networks.Content)-1; i += 2 {
		key := networks.Content[i].Value
		networkBody := networks.Content[i+1]
		// Skip networks marked as external
		isExternal := false
		if networkBody != nil {
			for j := 0; j < len(networkBody.Content)-1; j += 2 {
				if networkBody.Content[j].Value == "external" {
					val := networkBody.Content[j+1].Value
					if val == "true" {
						isExternal = true
					}
					break
				}
			}
		}
		if !isExternal {
			return key
		}
	}
	return ""
}

// WriteWorktreeOverride generates docker-compose.override.yml in dir.
// The override disables infra services, patches app ports, and redefines
// the sail network as external pointing at networkName.
func WriteWorktreeOverride(dir string, appService string, infraServices []string, networkName string) error {
	// Build services mapping node
	servicesMapping := &yaml.Node{Kind: yaml.MappingNode}

	// App service: clear depends_on (infra services are disabled via profiles).
	// Ports are handled by APP_PORT and VITE_PORT in .env.
	appServiceBody := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "depends_on"},
			{Kind: yaml.MappingNode, Tag: "!reset"},
		},
	}
	servicesMapping.Content = append(servicesMapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: appService},
		appServiceBody,
	)

	// Infra services: disable with profiles
	for _, svc := range infraServices {
		svcBody := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "profiles"},
				{
					Kind: yaml.SequenceNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "disabled"},
					},
				},
			},
		}
		servicesMapping.Content = append(servicesMapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: svc},
			svcBody,
		)
	}

	// Networks block: redefine sail as external
	networksMapping := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "sail"},
			{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "external"},
					{Kind: yaml.ScalarNode, Value: "true", Tag: "!!bool"},
					{Kind: yaml.ScalarNode, Value: "name"},
					{Kind: yaml.ScalarNode, Value: networkName},
				},
			},
		},
	}

	// Root document
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "services"},
			servicesMapping,
			{Kind: yaml.ScalarNode, Value: "networks"},
			networksMapping,
		},
	}

	doc := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{root},
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "docker-compose.override.yml"), data, 0644)
}

// getServicesNode returns the mapping node under "services:".
func (c *ComposeFile) getServicesNode() *yaml.Node {
	return c.getTopLevelMapping("services")
}

// getTopLevelMapping finds a top-level key in the YAML document.
func (c *ComposeFile) getTopLevelMapping(key string) *yaml.Node {
	if c.Root == nil || len(c.Root.Content) == 0 {
		return nil
	}
	root := c.Root.Content[0]
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1]
		}
	}
	return nil
}

// SanitizeDBName cleans a string for use as a database name.
func SanitizeDBName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "-", "_")
	var clean strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			clean.WriteRune(r)
		}
	}
	result := clean.String()
	if len(result) > 64 {
		result = result[:64]
	}
	return result
}
