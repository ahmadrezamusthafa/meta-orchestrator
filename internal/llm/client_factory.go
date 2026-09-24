package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

// ClientFactory manages instantiation and failover between LLM providers.
type ClientFactory struct {
	mu        sync.RWMutex
	cfg       *config.OrchestratorConfig
	providers map[string]ProviderClient
}

// NewClientFactory creates a factory wired to the active configuration.
func NewClientFactory(cfg *config.OrchestratorConfig) *ClientFactory {
	if cfg == nil {
		cfg = config.GetDefaultConfig()
	}

	f := &ClientFactory{
		cfg:       cfg,
		providers: make(map[string]ProviderClient),
	}
	f.initializeDrivers()
	return f
}

func (f *ClientFactory) initializeDrivers() {
	for name, provCfg := range f.cfg.Providers {
		switch name {
		case "claude":
			f.providers["claude"] = NewAnthropicDriver(provCfg.APIKey, provCfg.BaseURL)
		case "antigravity":
			f.providers["antigravity"] = NewAntigravityDriver(provCfg.APIKey, provCfg.BaseURL)
		case "openai":
			f.providers["openai"] = NewOpenAIDriver(provCfg.APIKey, provCfg.BaseURL)
		case "opencode":
			f.providers["opencode"] = NewOpenCodeDriver(provCfg.BaseURL, provCfg.Model)
		}
	}
}

// GetClient resolves provider by model string (e.g. "claude/claude-3-5-sonnet" or "openai/gpt-4o").
func (f *ClientFactory) GetClient(modelStr string) (ProviderClient, string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	parts := strings.Split(modelStr, "/")
	var providerName string
	var modelID string

	if len(parts) == 2 {
		providerName = parts[0]
		modelID = parts[1]
	} else {
		providerName = "claude"
		modelID = modelStr
	}

	client, exists := f.providers[providerName]
	if !exists {
		// Fallback to claude
		client = f.providers["claude"]
		if client == nil {
			return nil, "", fmt.Errorf("no provider available for %s", modelStr)
		}
	}

	return client, modelID, nil
}

// ExecuteWithFallback executes the request and automatically fails over if primary returns error.
func (f *ClientFactory) ExecuteWithFallback(ctx context.Context, primaryModel string, fallbackModel string, req *LLMRequest) (*LLMResponse, error) {
	client, modelID, err := f.GetClient(primaryModel)
	if err == nil {
		req.Model = modelID
		resp, err := client.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
		// Primary failed, log and try fallback
		fmt.Printf("WARNING: Primary model %s failed (%v), executing fallback to %s\n", primaryModel, err, fallbackModel)
	}

	if fallbackModel != "" {
		fbClient, fbModelID, fbErr := f.GetClient(fallbackModel)
		if fbErr == nil {
			req.Model = fbModelID
			return fbClient.Complete(ctx, req)
		}
	}

	return nil, fmt.Errorf("both primary (%s) and fallback (%s) failed", primaryModel, fallbackModel)
}
