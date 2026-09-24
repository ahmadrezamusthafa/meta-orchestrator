package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
	"gopkg.in/yaml.v3"
)

// BMADSkillManifest defines structure of bmad-skill.yaml or bmad-skill.json.
type BMADSkillManifest struct {
	ID             string                 `json:"id" yaml:"id"`
	Name           string                 `json:"name" yaml:"name"`
	Description    string                 `json:"description" yaml:"description"`
	Methodology    string                 `json:"methodology" yaml:"methodology"` // "bmad"
	AllowedRoles   []string               `json:"allowed_roles" yaml:"allowed_roles"`
	HandoffTo      string                 `json:"handoff_to" yaml:"handoff_to"`
	Isolation      string                 `json:"isolation" yaml:"isolation"`
	Parameters     map[string]interface{} `json:"parameters" yaml:"parameters"`
	TimeoutSeconds int                    `json:"timeout_seconds" yaml:"timeout_seconds"`
}

// BMADSkillAdapter ingests BMAD methodology skill definitions.
type BMADSkillAdapter struct{}

// NewBMADSkillAdapter creates a new BMAD adapter.
func NewBMADSkillAdapter() *BMADSkillAdapter {
	return &BMADSkillAdapter{}
}

// ParseDirectory parses a BMAD skill directory.
func (a *BMADSkillAdapter) ParseDirectory(skillDirPath string) (*types.UniversalSkillContract, error) {
	yamlPath := filepath.Join(skillDirPath, "bmad-skill.yaml")
	jsonPath := filepath.Join(skillDirPath, "bmad-skill.json")

	var data []byte
	var isYAML bool
	var err error

	if _, err = os.Stat(yamlPath); err == nil {
		data, err = os.ReadFile(yamlPath)
		isYAML = true
	} else if _, err = os.Stat(jsonPath); err == nil {
		data, err = os.ReadFile(jsonPath)
		isYAML = false
	} else {
		return nil, fmt.Errorf("no bmad-skill.yaml or bmad-skill.json found in %s", skillDirPath)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read BMAD manifest: %w", err)
	}

	var manifest BMADSkillManifest
	if isYAML {
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("failed to unmarshal BMAD YAML: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("failed to unmarshal BMAD JSON: %w", err)
		}
	}

	name := manifest.Name
	if name == "" {
		name = manifest.ID
	}
	if name == "" {
		name = filepath.Base(skillDirPath)
	}

	timeout := manifest.TimeoutSeconds
	if timeout <= 0 {
		timeout = 180
	}

	isolation := types.IsolationSubprocess
	if manifest.Isolation == "docker" {
		isolation = types.IsolationDocker
	}

	return &types.UniversalSkillContract{
		Name:            name,
		Description:     manifest.Description,
		SourceFormat:    types.SkillFormatBMAD,
		SourceLocation:  skillDirPath,
		Isolation:       isolation,
		InputSchema:     manifest.Parameters,
		RequiredRoles:   manifest.AllowedRoles,
		TimeoutSeconds:  timeout,
		RequiresNetwork: false,
	}, nil
}
