package registry

import (
	"path/filepath"
	"testing"
)

func TestToolRegistryManifest(t *testing.T) {
	reg := NewToolRegistry()
	toolsPath := filepath.Join("..", "..", "configs", "tools.json")

	err := reg.LoadFromFile(toolsPath)
	if err != nil {
		t.Fatalf("failed to load tools manifest: %v", err)
	}

	packages := reg.ListPackages()
	if len(packages) < 5 {
		t.Fatalf("expected at least 5 tools registered, got %d", len(packages))
	}

	// Verify Playwright package
	pw, err := reg.GetPackage("playwright-engine")
	if err != nil || pw == nil {
		t.Fatalf("expected playwright-engine to exist: %v", err)
	}
	if pw.Category != "runtime" {
		t.Errorf("expected runtime category, got %s", pw.Category)
	}
	if pw.HealthCheckCmd != "npx playwright --version" {
		t.Errorf("unexpected health check cmd: %s", pw.HealthCheckCmd)
	}

	// Verify Tree-sitter package
	ts, err := reg.GetPackage("treesitter-parsers")
	if err != nil || ts.Category != "parser" {
		t.Errorf("expected parser category for treesitter")
	}
}
