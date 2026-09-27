package skills

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// MCPToolDefinition represents an individual tool advertised by an MCP server.
type MCPToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// MCPServerConfig defines configuration for stdio/SSE MCP server endpoints.
type MCPServerConfig struct {
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Environment map[string]string `json:"env,omitempty"`
	SSEEndpoint string            `json:"sse_endpoint,omitempty"`
}

// MCPSkillAdapter bridges Model Context Protocol tools into UniversalSkillContracts.
type MCPSkillAdapter struct{}

// NewMCPSkillAdapter creates a new MCP adapter.
func NewMCPSkillAdapter() *MCPSkillAdapter {
	return &MCPSkillAdapter{}
}

// ParseConfigFile parses an MCP server manifest file.
func (a *MCPSkillAdapter) ParseConfigFile(filePath string) ([]*types.UniversalSkillContract, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read MCP config: %w", err)
	}

	var root struct {
		MCPServers map[string]MCPServerConfig `json:"mcpServers"`
		Tools      []MCPToolDefinition        `json:"tools,omitempty"`
	}

	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MCP configuration: %w", err)
	}

	var contracts []*types.UniversalSkillContract
	if len(root.Tools) > 0 {
		for _, tool := range root.Tools {
			contracts = append(contracts, &types.UniversalSkillContract{
				Name:            tool.Name,
				Description:     tool.Description,
				SourceFormat:    types.SkillFormatMCP,
				SourceLocation:  filePath,
				Isolation:       types.IsolationSubprocess,
				InputSchema:     tool.InputSchema,
				TimeoutSeconds:  120,
				RequiresNetwork: true,
			})
		}
	} else {
		for serverName, conf := range root.MCPServers {
			contracts = append(contracts, a.ConvertServerConfig(serverName, conf, filePath))
		}
	}

	return contracts, nil
}

// ConvertServerConfig converts an MCPServerConfig to a UniversalSkillContract.
func (a *MCPSkillAdapter) ConvertServerConfig(serverName string, conf MCPServerConfig, sourceLocation string) *types.UniversalSkillContract {
	return &types.UniversalSkillContract{
		Name:            serverName,
		Description:     fmt.Sprintf("MCP Server tool proxy for %s (%s)", serverName, conf.Command),
		SourceFormat:    types.SkillFormatMCP,
		SourceLocation:  sourceLocation,
		Command:         conf.Command,
		Args:            conf.Args,
		Environment:     conf.Environment,
		Isolation:       types.IsolationSubprocess,
		TimeoutSeconds:  180,
		RequiresNetwork: true,
	}
}

// ConvertMCPTool converts an individual MCPToolDefinition to UniversalSkillContract.
func (a *MCPSkillAdapter) ConvertMCPTool(tool *MCPToolDefinition, sourceLocation string) *types.UniversalSkillContract {
	return &types.UniversalSkillContract{
		Name:            tool.Name,
		Description:     tool.Description,
		SourceFormat:    types.SkillFormatMCP,
		SourceLocation:  sourceLocation,
		Isolation:       types.IsolationSubprocess,
		InputSchema:     tool.InputSchema,
		TimeoutSeconds:  120,
		RequiresNetwork: true,
	}
}
