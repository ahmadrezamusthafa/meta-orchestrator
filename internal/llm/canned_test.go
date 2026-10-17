package llm

import (
	"context"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

func init() { cannedForTests = true }

func TestCannedKeysAreOffOutsideTests(t *testing.T) {
	cannedForTests = false
	defer func() { cannedForTests = true }()
	_, err := NewOpenAIDriver("mock-key", "").Complete(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err == nil {
		t.Fatal("a mock-looking key must not get a canned answer in the daemon")
	}
}

func TestCredentialResolverKeepsConfiguredBaseURLAndKey(t *testing.T) {
	cfg := config.GetDefaultConfig()
	cfg.Providers["openai"] = config.AIProviderConfig{Enabled: true, APIKey: "sk-config", BaseURL: "http://gateway.local/v1"}
	f := NewClientFactory(cfg)
	f.SetCredentialResolver(func(string) (string, string, string) { return "", "", "" })
	c, _, err := f.GetClient("openai/gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	d := c.(*OpenAIDriver)
	if d.baseURL != "http://gateway.local/v1" || d.apiKey != "sk-config" {
		t.Fatalf("driver lost its configuration: baseURL=%q", d.baseURL)
	}
}

func TestDisabledProviderIsNotRegistered(t *testing.T) {
	cfg := config.GetDefaultConfig() // opencode is disabled by default
	if _, _, err := NewClientFactory(cfg).GetClient("opencode/qwen"); err == nil {
		t.Fatal("a disabled provider must not resolve")
	}
}
