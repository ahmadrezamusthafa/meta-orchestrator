package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// ManualInstall tells the operator how to install a tool the orchestrator cannot install itself.
type ManualInstall struct {
	Docs     string   `json:"docs"`
	Commands []string `json:"commands,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// ToolPackageDefinition is a command-line tool the orchestrator itself relies on.
type ToolPackageDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	// RequiredRuntimes maps a runtime (node, docker, go, java) to a ">=X.Y.Z" constraint.
	RequiredRuntimes map[string]string `json:"required_runtimes,omitempty"`
	// HealthCheckCmd is run without a shell (split on spaces); success means installed and its
	// output carries the version.
	HealthCheckCmd string `json:"health_check_cmd"`
	// ToolchainID names the internal/toolchain catalog entry that installs this tool, if any.
	ToolchainID string        `json:"toolchain_id,omitempty"`
	Install     ManualInstall `json:"install"`
}

// HealthCheckArgv splits HealthCheckCmd into argv.
func (p *ToolPackageDefinition) HealthCheckArgv() []string {
	return strings.Fields(p.HealthCheckCmd)
}

// ToolRegistry manages registered tool package manifests.
type ToolRegistry struct {
	mu       sync.RWMutex
	packages map[string]*ToolPackageDefinition
}

// NewToolRegistry creates a new tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		packages: make(map[string]*ToolPackageDefinition),
	}
}

// LoadFromFile loads tool packages from a JSON manifest.
func (r *ToolRegistry) LoadFromFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read tools manifest: %w", err)
	}

	var list []ToolPackageDefinition
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("failed to parse tools JSON: %w", err)
	}
	for i, p := range list {
		if p.ID == "" || p.Name == "" || len(p.HealthCheckArgv()) == 0 {
			return fmt.Errorf("invalid tool manifest at index %d: id, name and health_check_cmd are required", i)
		}
		if strings.ContainsAny(p.HealthCheckCmd, "|&;<>$`\"'") {
			return fmt.Errorf("tool %s: health_check_cmd runs without a shell, so it cannot use shell syntax", p.ID)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range list {
		p := list[i]
		r.packages[p.ID] = &p
	}
	return nil
}

// GetPackage retrieves a package by ID.
func (r *ToolRegistry) GetPackage(id string) (*ToolPackageDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.packages[id]
	if !exists {
		return nil, fmt.Errorf("tool package '%s' not found", id)
	}
	return p, nil
}

// ListPackages returns all registered tool definitions, sorted by ID.
func (r *ToolRegistry) ListPackages() []*ToolPackageDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*ToolPackageDefinition, 0, len(r.packages))
	for _, p := range r.packages {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}
