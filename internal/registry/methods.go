package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// MethodDefinition details execution semantics and constraints for a methodology.
type MethodDefinition struct {
	ID                       string   `json:"id"`
	Name                     string   `json:"name"`
	Description              string   `json:"description"`
	ExecutionPattern         string   `json:"execution_pattern"`
	AllowedRoles             []string `json:"allowed_roles"`
	MaxConsecutiveIterations int      `json:"max_consecutive_iterations"`
}

// MethodRegistry manages registered methodology engines.
type MethodRegistry struct {
	mu      sync.RWMutex
	methods map[string]*MethodDefinition
}

// NewMethodRegistry creates a new method registry.
func NewMethodRegistry() *MethodRegistry {
	return &MethodRegistry{
		methods: make(map[string]*MethodDefinition),
	}
}

// LoadFromFile loads method definitions from a JSON file.
func (r *MethodRegistry) LoadFromFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read methods file: %w", err)
	}

	var list []MethodDefinition
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("failed to parse methods JSON: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range list {
		m := list[i]
		r.methods[m.ID] = &m
	}

	return nil
}

// Get retrieves a method by ID.
func (r *MethodRegistry) Get(id string) (*MethodDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, exists := r.methods[id]
	if !exists {
		return nil, fmt.Errorf("method '%s' not registered", id)
	}
	return m, nil
}

// ValidateRole verifies if a role is permitted to run under the method.
func (r *MethodRegistry) ValidateRole(methodID string, role string) (bool, error) {
	m, err := r.Get(methodID)
	if err != nil {
		return false, err
	}

	for _, allowed := range m.AllowedRoles {
		if allowed == role || allowed == "*" {
			return true, nil
		}
	}
	return false, nil
}
