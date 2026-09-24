package llm

import (
	"context"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

func TestClientFactoryResolutionAndFallback(t *testing.T) {
	cfg := config.GetDefaultConfig()
	factory := NewClientFactory(cfg)

	// Test 1: Resolve Anthropic
	client, modelID, err := factory.GetClient("claude/claude-3-5-sonnet")
	if err != nil || client == nil {
		t.Fatalf("failed to resolve claude client: %v", err)
	}
	if modelID != "claude-3-5-sonnet" {
		t.Errorf("expected modelID 'claude-3-5-sonnet', got '%s'", modelID)
	}

	// Test 2: Resolve OpenAI
	client, modelID, err = factory.GetClient("openai/gpt-4o")
	if err != nil || client == nil {
		t.Fatalf("failed to resolve openai client: %v", err)
	}
	if modelID != "gpt-4o" {
		t.Errorf("expected modelID 'gpt-4o', got '%s'", modelID)
	}

	// Test 3: Execute with fallback
	req := &LLMRequest{
		Messages: []Message{
			{Role: RoleUser, Content: "Hello AI"},
		},
	}
	resp, err := factory.ExecuteWithFallback(context.Background(), "claude/claude-3-5-sonnet", "openai/gpt-4o", req)
	if err != nil {
		t.Fatalf("ExecuteWithFallback failed: %v", err)
	}
	if resp.Provider != "anthropic" {
		t.Errorf("expected primary anthropic response, got %s", resp.Provider)
	}
}
