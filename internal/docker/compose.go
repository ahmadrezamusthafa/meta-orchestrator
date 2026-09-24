package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ComposeService defines a containerized service specification.
type ComposeService struct {
	Image         string            `yaml:"image"`
	ContainerName string            `yaml:"container_name"`
	Ports         []string          `yaml:"ports,omitempty"`
	Environment   map[string]string `yaml:"environment,omitempty"`
	Volumes       []string          `yaml:"volumes,omitempty"`
	Networks      []string          `yaml:"networks,omitempty"`
}

// DockerComposeFile represents the structure of docker-compose.generated.yml.
type DockerComposeFile struct {
	Version  string                    `yaml:"version"`
	Services map[string]ComposeService `yaml:"services"`
	Networks map[string]interface{}    `yaml:"networks"`
}

// ComposeOrchestrator manages generation and execution of task container environments.
type ComposeOrchestrator struct {
	mu           sync.Mutex
	basePortBase int
}

// NewComposeOrchestrator creates a new compose orchestrator.
func NewComposeOrchestrator() *ComposeOrchestrator {
	return &ComposeOrchestrator{
		basePortBase: 10000,
	}
}

// GenerateComposeFile creates an isolated Docker Compose manifest for a task.
func (o *ComposeOrchestrator) GenerateComposeFile(
	taskID string,
	workspaceDir string,
	skillMounts []SkillVolumeMount,
	requiredServices []string,
) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	networkName := fmt.Sprintf("meta_net_%s", taskID)
	composePath := filepath.Join(workspaceDir, "docker-compose.generated.yml")

	// Calculate deterministic port offset based on task hash or length
	portOffset := 5432 + (len(taskID) % 500)

	services := make(map[string]ComposeService)

	// Base App Container
	var appVolumes []string
	appVolumes = append(appVolumes, fmt.Sprintf("%s:/workspace", workspaceDir))
	for _, m := range skillMounts {
		mode := "ro"
		if !m.ReadOnly {
			mode = "rw"
		}
		appVolumes = append(appVolumes, fmt.Sprintf("%s:%s:%s", m.HostPath, m.ContainerPath, mode))
	}

	services["app_sandbox"] = ComposeService{
		Image:         "node:20-alpine",
		ContainerName: fmt.Sprintf("app_%s", taskID),
		Volumes:       appVolumes,
		Networks:      []string{networkName},
		Environment: map[string]string{
			"META_TASK_ID":   taskID,
			"WORKSPACE_PATH": "/workspace",
		},
	}

	// Add requested backing services (postgres, redis, mailpit)
	for _, s := range requiredServices {
		switch strings.ToLower(s) {
		case "postgres":
			services["postgres"] = ComposeService{
				Image:         "postgres:16-alpine",
				ContainerName: fmt.Sprintf("pg_%s", taskID),
				Ports:         []string{fmt.Sprintf("%d:5432", portOffset)},
				Networks:      []string{networkName},
				Environment: map[string]string{
					"POSTGRES_USER":     "meta",
					"POSTGRES_PASSWORD": "secret_password",
					"POSTGRES_DB":       "testdb",
				},
			}
		case "redis":
			services["redis"] = ComposeService{
				Image:         "redis:7-alpine",
				ContainerName: fmt.Sprintf("redis_%s", taskID),
				Ports:         []string{fmt.Sprintf("%d:6379", portOffset+1000)},
				Networks:      []string{networkName},
			}
		}
	}

	networks := map[string]interface{}{
		networkName: map[string]string{
			"driver": "bridge",
		},
	}

	doc := DockerComposeFile{
		Version:  "3.8",
		Services: services,
		Networks: networks,
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("failed to marshal docker-compose YAML: %w", err)
	}

	if err := os.WriteFile(composePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write generated docker-compose.yml: %w", err)
	}

	return composePath, nil
}
