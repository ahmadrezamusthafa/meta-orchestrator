package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectConfigKeepsProviderEnabledWhenOnlyModelChanges(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // keep the operator's system config out of the test
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".sdlc"), 0o755); err != nil {
		t.Fatal(err)
	}
	yml := "providers:\n  claude:\n    model: claude-sonnet-5-5\n  openai:\n    enabled: false\n"
	if err := os.WriteFile(filepath.Join(root, ".sdlc", "config.yaml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewCascadingConfigResolver(root).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if p := cfg.Providers["claude"]; !p.Enabled || p.Model != "claude-sonnet-5-5" {
		t.Fatalf("claude = %+v, want enabled with the overridden model", p)
	}
	if cfg.Providers["openai"].Enabled {
		t.Fatal("an explicit enabled: false must still disable the provider")
	}
}
