package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// SkillSource represents an external directory or bundle registered as a skill source.
type SkillSource struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	Format      types.SkillFormat `json:"format"` // "claude", "mcp", "auto", "bmad"
	Enabled     bool              `json:"enabled"`
	SkillCount  int               `json:"skill_count"`
	AddedAt     string            `json:"added_at"`
	Description string            `json:"description,omitempty"`
}

// SkillStateConfig stores per-skill activation and configuration overrides.
type SkillStateConfig struct {
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	CustomRoles     []string `json:"custom_roles,omitempty"`
	TimeoutOverride int      `json:"timeout_override,omitempty"`
}

// SkillsConfigData stores the persistent configuration on disk (.sdlc/skills_config.json).
type SkillsConfigData struct {
	RegisteredSources []SkillSource               `json:"registered_sources"`
	SkillStates       map[string]SkillStateConfig `json:"skill_states"`
}

// ConfigManager handles thread-safe loading, saving, and querying of skills configuration.
type ConfigManager struct {
	mu         sync.RWMutex
	filePath   string
	data       SkillsConfigData
	onUpdateFn func()
}

// NewConfigManager initializes the configuration manager.
func NewConfigManager(projectDir string, userHomeDir string) *ConfigManager {
	filePath := ""
	if projectDir != "" {
		filePath = filepath.Join(projectDir, ".sdlc", "skills_config.json")
	} else if userHomeDir != "" {
		filePath = filepath.Join(userHomeDir, ".config", "meta-orchestrator", "skills_config.json")
	}

	cm := &ConfigManager{
		filePath: filePath,
		data: SkillsConfigData{
			RegisteredSources: make([]SkillSource, 0),
			SkillStates:       make(map[string]SkillStateConfig),
		},
	}
	cm.load()
	return cm
}

// SetOnUpdateCallback sets a notification callback whenever configuration changes.
func (cm *ConfigManager) SetOnUpdateCallback(fn func()) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.onUpdateFn = fn
}

func (cm *ConfigManager) load() {
	if cm.filePath == "" {
		return
	}
	bytes, err := os.ReadFile(cm.filePath)
	if err != nil {
		return
	}
	var data SkillsConfigData
	if err := json.Unmarshal(bytes, &data); err == nil {
		if data.RegisteredSources == nil {
			data.RegisteredSources = make([]SkillSource, 0)
		}
		if data.SkillStates == nil {
			data.SkillStates = make(map[string]SkillStateConfig)
		}
		cm.data = data
	}
}

func (cm *ConfigManager) saveLocked() error {
	if cm.filePath == "" {
		return nil
	}
	dir := filepath.Dir(cm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}
	bytes, err := json.MarshalIndent(cm.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal skills configuration: %w", err)
	}
	if err := os.WriteFile(cm.filePath, bytes, 0644); err != nil {
		return fmt.Errorf("failed to write skills configuration to %s: %w", cm.filePath, err)
	}
	if cm.onUpdateFn != nil {
		go cm.onUpdateFn()
	}
	return nil
}

// IsSkillEnabled returns true unless the skill has been explicitly disabled.
func (cm *ConfigManager) IsSkillEnabled(skillName string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cfg, ok := cm.data.SkillStates[skillName]; ok {
		return cfg.Enabled
	}
	return true // Default enabled
}

// SetSkillEnabled toggles a skill's active status and persists the change.
func (cm *ConfigManager) SetSkillEnabled(skillName string, enabled bool) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cfg := cm.data.SkillStates[skillName]
	cfg.Name = skillName
	cfg.Enabled = enabled
	cm.data.SkillStates[skillName] = cfg
	return cm.saveLocked()
}

// ListSources returns all registered skill source directories.
func (cm *ConfigManager) ListSources() []SkillSource {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]SkillSource, len(cm.data.RegisteredSources))
	copy(result, cm.data.RegisteredSources)
	return result
}

// AddSource registers a new skill source directory or path.
func (cm *ConfigManager) AddSource(src SkillSource) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Avoid duplicates
	for i, existing := range cm.data.RegisteredSources {
		if existing.Path == src.Path {
			cm.data.RegisteredSources[i] = src
			return cm.saveLocked()
		}
	}

	cm.data.RegisteredSources = append(cm.data.RegisteredSources, src)
	return cm.saveLocked()
}

// RemoveSource unregisters a skill source by ID.
func (cm *ConfigManager) RemoveSource(sourceID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	idx := -1
	for i, src := range cm.data.RegisteredSources {
		if src.ID == sourceID || src.Path == sourceID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("skill source '%s' not found", sourceID)
	}

	cm.data.RegisteredSources = append(cm.data.RegisteredSources[:idx], cm.data.RegisteredSources[idx+1:]...)
	return cm.saveLocked()
}
