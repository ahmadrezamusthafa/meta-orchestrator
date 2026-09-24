package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
	"gopkg.in/yaml.v3"
)

// SuperpowerToolConfig defines structure of superpower tool yaml.
type SuperpowerToolConfig struct {
	Name            string                 `yaml:"name"`
	Description     string                 `yaml:"description"`
	Command         string                 `yaml:"command"`
	Arguments       []string               `yaml:"arguments"`
	Environment     map[string]string      `yaml:"environment"`
	AllowHostAccess bool                   `yaml:"allow_host_access"`
	TimeoutSeconds  int                    `yaml:"timeout_seconds"`
	InputSchema     map[string]interface{} `yaml:"input_schema"`
}

// SuperpowerSkillAdapter ingests bare-metal and shell automation tools.
type SuperpowerSkillAdapter struct{}

// NewSuperpowerSkillAdapter creates a new Superpower adapter.
func NewSuperpowerSkillAdapter() *SuperpowerSkillAdapter {
	return &SuperpowerSkillAdapter{}
}

// ParseDirectory parses a Superpower skill directory or executable script.
func (a *SuperpowerSkillAdapter) ParseDirectory(skillDirPath string) (*types.UniversalSkillContract, error) {
	configFile := filepath.Join(skillDirPath, "superpower.yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		// Check if there is an executable script directly
		scriptName := filepath.Base(skillDirPath) + ".sh"
		scriptPath := filepath.Join(skillDirPath, scriptName)
		if _, statErr := os.Stat(scriptPath); statErr == nil {
			return &types.UniversalSkillContract{
				Name:            strings.TrimSuffix(filepath.Base(skillDirPath), ".sh"),
				Description:     fmt.Sprintf("Superpower shell executable tool %s", scriptName),
				SourceFormat:    types.SkillFormatSuperpower,
				SourceLocation:  scriptPath,
				Isolation:       types.IsolationHost,
				TimeoutSeconds:  300,
				RequiresNetwork: true,
			}, nil
		}
		return nil, fmt.Errorf("failed to read superpower.yaml in %s: %w", skillDirPath, err)
	}

	var conf SuperpowerToolConfig
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("failed to parse superpower.yaml: %w", err)
	}

	isolation := types.IsolationDocker
	if conf.AllowHostAccess {
		isolation = types.IsolationHost
	}

	timeout := conf.TimeoutSeconds
	if timeout <= 0 {
		timeout = 300
	}

	name := conf.Name
	if name == "" {
		name = filepath.Base(skillDirPath)
	}

	return &types.UniversalSkillContract{
		Name:            name,
		Description:     conf.Description,
		SourceFormat:    types.SkillFormatSuperpower,
		SourceLocation:  skillDirPath,
		Isolation:       isolation,
		InputSchema:     conf.InputSchema,
		TimeoutSeconds:  timeout,
		RequiresNetwork: true,
	}, nil
}
