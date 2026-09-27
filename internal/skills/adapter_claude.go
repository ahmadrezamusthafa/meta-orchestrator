package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
	"gopkg.in/yaml.v3"
)

// ClaudeSkillFrontmatter extracts YAML frontmatter from SKILL.md.
type ClaudeSkillFrontmatter struct {
	Name            string                 `yaml:"name"`
	Description     string                 `yaml:"description"`
	Command         string                 `yaml:"command,omitempty"`
	Args            []string               `yaml:"args,omitempty"`
	Env             map[string]string      `yaml:"env,omitempty"`
	RequiredRoles   []string               `yaml:"required_roles,omitempty"`
	AllowedRoles    []string               `yaml:"allowed_roles,omitempty"`
	TimeoutSeconds  int                    `yaml:"timeout_seconds,omitempty"`
	RequiresNetwork bool                   `yaml:"requires_network,omitempty"`
	InputSchema     map[string]interface{} `yaml:"input_schema,omitempty"`
	Isolation       string                 `yaml:"isolation,omitempty"`
	Metadata        map[string]interface{} `yaml:"metadata,omitempty"`
}

// ClaudeSkillAdapter ingests Anthropic SKILL.md directory bundles.
type ClaudeSkillAdapter struct{}

// NewClaudeSkillAdapter creates an adapter for Claude skills.
func NewClaudeSkillAdapter() *ClaudeSkillAdapter {
	return &ClaudeSkillAdapter{}
}

// ParseDirectory parses a Claude skill directory containing SKILL.md.
func (a *ClaudeSkillAdapter) ParseDirectory(skillDirPath string) (*types.UniversalSkillContract, error) {
	skillFile := filepath.Join(skillDirPath, "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read SKILL.md in %s: %w", skillDirPath, err)
	}

	contract, err := a.ParseContent(string(data), skillDirPath)
	if err != nil {
		return nil, err
	}

	// If no explicit command defined, auto-detect runnable entrypoint scripts in scripts/
	if contract.Command == "" {
		scriptsDir := filepath.Join(skillDirPath, "scripts")
		if stat, err := os.Stat(scriptsDir); err == nil && stat.IsDir() {
			if _, err := os.Stat(filepath.Join(scriptsDir, "run.sh")); err == nil {
				contract.Command = "bash"
				contract.Args = []string{filepath.Join(scriptsDir, "run.sh")}
			} else if _, err := os.Stat(filepath.Join(scriptsDir, "main.py")); err == nil {
				contract.Command = "python3"
				contract.Args = []string{filepath.Join(scriptsDir, "main.py")}
			} else if _, err := os.Stat(filepath.Join(scriptsDir, "index.js")); err == nil {
				contract.Command = "node"
				contract.Args = []string{filepath.Join(scriptsDir, "index.js")}
			}
		}
	}

	return contract, nil
}

// ParseContent parses raw SKILL.md content with optional frontmatter.
func (a *ClaudeSkillAdapter) ParseContent(content string, sourceLocation string) (*types.UniversalSkillContract, error) {
	lines := strings.Split(content, "\n")
	var frontmatterYAML bytes.Buffer
	var body bytes.Buffer

	inFrontmatter := false
	frontmatterDone := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" && !frontmatterDone {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			} else {
				inFrontmatter = false
				frontmatterDone = true
				continue
			}
		}

		if inFrontmatter {
			frontmatterYAML.WriteString(line + "\n")
		} else {
			body.WriteString(line + "\n")
		}
	}

	var meta ClaudeSkillFrontmatter
	if frontmatterYAML.Len() > 0 {
		if err := yaml.Unmarshal(frontmatterYAML.Bytes(), &meta); err != nil {
			return nil, fmt.Errorf("failed to parse Claude skill frontmatter YAML: %w", err)
		}
	}

	// Fallback to directory name or first heading if name is empty
	name := meta.Name
	if name == "" {
		if sourceLocation != "" {
			name = filepath.Base(sourceLocation)
		} else {
			name = "unnamed_claude_skill"
		}
	}

	description := meta.Description
	if description == "" {
		description = strings.TrimSpace(body.String())
		if len(description) > 200 {
			description = description[:197] + "..."
		}
	}

	timeout := meta.TimeoutSeconds
	if timeout <= 0 {
		timeout = 120
	}

	schema := meta.InputSchema
	if schema == nil {
		schema = map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "Input instruction or query for this skill",
				},
			},
		}
	}

	roles := meta.RequiredRoles
	if len(roles) == 0 && len(meta.AllowedRoles) > 0 {
		roles = meta.AllowedRoles
	}

	isolation := types.IsolationSubprocess
	if meta.Isolation == "docker" {
		isolation = types.IsolationDocker
	} else if meta.Isolation == "host" {
		isolation = types.IsolationHost
	}

	metadata := meta.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	bodyStr := strings.TrimSpace(body.String())
	if bodyStr != "" {
		metadata["instructions"] = bodyStr
	}
	if sourceLocation != "" {
		metadata["skill_dir"] = sourceLocation
	}

	return &types.UniversalSkillContract{
		Name:            name,
		Description:     description,
		SourceFormat:    types.SkillFormatClaude,
		SourceLocation:  sourceLocation,
		SourceType:      "claude",
		Enabled:         true,
		Isolation:       isolation,
		InputSchema:     schema,
		RequiredRoles:   roles,
		TimeoutSeconds:  timeout,
		RequiresNetwork: meta.RequiresNetwork,
		Command:         meta.Command,
		Args:            meta.Args,
		Environment:     meta.Env,
		Metadata:        metadata,
	}, nil
}
