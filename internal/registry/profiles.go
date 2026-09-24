package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// AgentProfile represents a configured agent persona and its security boundaries.
type AgentProfile struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Role                 string   `json:"role"`
	Description          string   `json:"description"`
	AllowedSkills        []string `json:"allowed_skills"`
	DeniedSkills         []string `json:"denied_skills"`
	SystemPromptTemplate string   `json:"system_prompt_template"`
	MaxTokenBudget       int64    `json:"max_token_budget"`
}

// ProfileRegistry manages agent profiles and enforces role-based skill permissions.
type ProfileRegistry struct {
	mu       sync.RWMutex
	profiles map[string]*AgentProfile
}

// NewProfileRegistry creates an empty registry.
func NewProfileRegistry() *ProfileRegistry {
	return &ProfileRegistry{
		profiles: make(map[string]*AgentProfile),
	}
}

// LoadFromFile loads profiles from a JSON configuration file.
func (r *ProfileRegistry) LoadFromFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read profiles file: %w", err)
	}

	var list []AgentProfile
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("failed to parse profiles JSON: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range list {
		p := list[i]
		r.profiles[p.Role] = &p
		r.profiles[p.ID] = &p
	}

	return nil
}

// GetProfile retrieves a profile by role or ID.
func (r *ProfileRegistry) GetProfile(roleOrID string) (*AgentProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.profiles[roleOrID]
	if !exists {
		return nil, fmt.Errorf("profile '%s' not registered", roleOrID)
	}
	return p, nil
}

// CheckPermission verifies whether a profile is allowed to execute a given skill.
func (r *ProfileRegistry) CheckPermission(roleOrID string, skillName string) (bool, error) {
	profile, err := r.GetProfile(roleOrID)
	if err != nil {
		return false, err
	}

	// 1. Explicit deny takes precedence
	for _, denied := range profile.DeniedSkills {
		if denied == skillName || denied == "*" {
			return false, nil
		}
	}

	// 2. DevOps superpower can execute any non-denied skill
	if profile.Role == "devops_superpower" {
		return true, nil
	}

	// 3. Explicit allow
	for _, allowed := range profile.AllowedSkills {
		if allowed == skillName || allowed == "*" {
			return true, nil
		}
	}

	return false, nil
}
