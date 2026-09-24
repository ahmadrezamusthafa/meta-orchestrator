package skills

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// OpenAIToolDefinition represents standard OpenAI function specification.
type OpenAIToolDefinition struct {
	Type     string `json:"type"` // "function"
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

// OpenAISkillAdapter adapts OpenAI function tools into UniversalSkillContracts.
type OpenAISkillAdapter struct{}

// NewOpenAISkillAdapter creates a new OpenAI adapter.
func NewOpenAISkillAdapter() *OpenAISkillAdapter {
	return &OpenAISkillAdapter{}
}

// ParseJSONFile parses OpenAI tool definitions from a JSON file.
func (a *OpenAISkillAdapter) ParseJSONFile(filePath string) ([]*types.UniversalSkillContract, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read OpenAI tool file: %w", err)
	}

	var tools []OpenAIToolDefinition
	if err := json.Unmarshal(data, &tools); err != nil {
		// Try single tool format
		var single OpenAIToolDefinition
		if singleErr := json.Unmarshal(data, &single); singleErr == nil && single.Function.Name != "" {
			tools = []OpenAIToolDefinition{single}
		} else {
			return nil, fmt.Errorf("failed to parse OpenAI tools JSON: %w", err)
		}
	}

	var contracts []*types.UniversalSkillContract
	for _, tool := range tools {
		contracts = append(contracts, &types.UniversalSkillContract{
			Name:           tool.Function.Name,
			Description:    tool.Function.Description,
			SourceFormat:   types.SkillFormatOpenAI,
			SourceLocation: filePath,
			Isolation:      types.IsolationSubprocess,
			InputSchema:    tool.Function.Parameters,
			TimeoutSeconds: 120,
		})
	}

	return contracts, nil
}
