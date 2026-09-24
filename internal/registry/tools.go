package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ToolPackageDefinition represents declarative manifest of an installable tool package.
type ToolPackageDefinition struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Category          string            `json:"category"`
	ActiveVersion     string            `json:"active_version"`
	AvailableVersions []string          `json:"available_versions"`
	RequiredRuntimes  map[string]string `json:"required_runtimes"`
	HealthCheckCmd    string            `json:"health_check_cmd"`
	RollbackStrategy  string            `json:"rollback_strategy"`
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

	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range list {
		p := list[i]
		if p.ID == "" || p.ActiveVersion == "" || p.HealthCheckCmd == "" {
			return fmt.Errorf("invalid tool manifest at index %d: missing required fields", i)
		}
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

// ListPackages returns all registered tool definitions.
func (r *ToolRegistry) ListPackages() []*ToolPackageDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*ToolPackageDefinition, 0, len(r.packages))
	for _, p := range r.packages {
		list = append(list, p)
	}
	return list
}
