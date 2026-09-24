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
	RequiredRoles   []string               `yaml:"required_roles,omitempty"`
	TimeoutSeconds  int                    `yaml:"timeout_seconds,omitempty"`
	RequiresNetwork bool                   `yaml:"requires_network,omitempty"`
	InputSchema     map[string]interface{} `yaml:"input_schema,omitempty"`
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

	return a.ParseContent(string(data), skillDirPath)
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

	return &types.UniversalSkillContract{
		Name:            name,
		Description:     description,
		SourceFormat:    types.SkillFormatClaude,
		SourceLocation:  sourceLocation,
		Isolation:       types.IsolationSubprocess,
		InputSchema:     schema,
		RequiredRoles:   meta.RequiredRoles,
		TimeoutSeconds:  timeout,
		RequiresNetwork: meta.RequiresNetwork,
	}, nil
}
