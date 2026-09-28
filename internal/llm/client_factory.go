package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

// CredentialResolver resolves live auth credentials for a given provider.
type CredentialResolver func(providerID string) (apiKey string, sessionToken string, authMethod string)

// ClientFactory manages instantiation and failover between LLM providers.
type ClientFactory struct {
	mu           sync.RWMutex
	cfg          *config.OrchestratorConfig
	providers    map[string]ProviderClient
	credResolver CredentialResolver
	usageObs     UsageObserver
}

// UsageObserver is notified after every successful completion (telemetry instrumentation hook).
type UsageObserver func(modelStr string, resp *LLMResponse)

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

// SetCredentialResolver configures dynamic credential resolution from runtime store.
func (f *ClientFactory) SetCredentialResolver(resolver CredentialResolver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.credResolver = resolver
}

// SetUsageObserver registers a hook invoked with each successful completion's response.
func (f *ClientFactory) SetUsageObserver(obs UsageObserver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.usageObs = obs
}

func (f *ClientFactory) initializeDrivers() {
	for name, provCfg := range f.cfg.Providers {
		switch name {
		case "claude":
			f.providers["claude"] = NewAnthropicDriver(provCfg.APIKey, provCfg.BaseURL)
		case "antigravity":
			f.providers["antigravity"] = NewAntigravityDriver(provCfg.APIKey, provCfg.BaseURL)
		case "openai", "chatgpt":
			f.providers["openai"] = NewOpenAIDriver(provCfg.APIKey, provCfg.BaseURL)
		case "opencode":
			f.providers["opencode"] = NewOpenCodeDriver(provCfg.BaseURL, provCfg.Model)
		}
	}
}

// GetClient resolves provider by model string (e.g. "claude/claude-3-5-sonnet" or "openai/gpt-4o").
func (f *ClientFactory) GetClient(modelStr string) (ProviderClient, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

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

	// Normalize provider name aliases
	if providerName == "chatgpt" {
		providerName = "openai"
	}

	// If credential resolver is provided, update client with latest credentials
	if f.credResolver != nil {
		apiKey, sessionToken, _ := f.credResolver(providerName)
		switch providerName {
		case "claude":
			d := NewAnthropicDriver(apiKey, "")
			d.SetSessionToken(sessionToken)
			f.providers["claude"] = d
		case "antigravity":
			f.providers["antigravity"] = NewAntigravityDriver(apiKey, "")
		case "openai":
			f.providers["openai"] = NewOpenAIDriver(apiKey, "")
		}
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

// ExecuteWithFallbackChain executes through an ordered priority chain of models (9router pattern).
// It attempts each model sequentially. If one fails, it executes onFailover callback and routes to the next.
func (f *ClientFactory) ExecuteWithFallbackChain(
	ctx context.Context,
	models []string,
	req *LLMRequest,
	onFailover func(failedModel string, nextModel string, err error),
) (*LLMResponse, string, error) {
	if len(models) == 0 {
		return nil, "", fmt.Errorf("no models provided in priority chain")
	}

	var errList []string
	for i, modelStr := range models {
		client, modelID, err := f.GetClient(modelStr)
		if err != nil {
			errList = append(errList, fmt.Sprintf("%s (resolve failed: %v)", modelStr, err))
			if i < len(models)-1 && onFailover != nil {
				onFailover(modelStr, models[i+1], err)
			}
			continue
		}

		// Set model on request
		req.Model = modelID
		resp, completeErr := client.Complete(ctx, req)
		if completeErr == nil {
			f.mu.RLock()
			obs := f.usageObs
			f.mu.RUnlock()
			if obs != nil && resp != nil {
				obs(modelStr, resp)
			}
			return resp, modelStr, nil
		}

		// Log failure and notify failover
		errList = append(errList, fmt.Sprintf("%s (error: %v)", modelStr, completeErr))
		if i < len(models)-1 && onFailover != nil {
			onFailover(modelStr, models[i+1], completeErr)
		}
	}

	return nil, "", fmt.Errorf("all models in 9router priority chain failed: %s", strings.Join(errList, "; "))
}

// ExecuteWithFallback executes the request and automatically fails over if primary returns error.
func (f *ClientFactory) ExecuteWithFallback(ctx context.Context, primaryModel string, fallbackModel string, req *LLMRequest) (*LLMResponse, error) {
	chain := []string{primaryModel}
	if fallbackModel != "" {
		chain = append(chain, fallbackModel)
	}
	resp, _, err := f.ExecuteWithFallbackChain(ctx, chain, req, nil)
	return resp, err
}

