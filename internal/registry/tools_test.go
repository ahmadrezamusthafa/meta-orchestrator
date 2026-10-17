package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToolRegistryManifest(t *testing.T) {
	reg := NewToolRegistry()
	if err := reg.LoadFromFile(filepath.Join("..", "..", "configs", "tools.json")); err != nil {
		t.Fatalf("failed to load tools manifest: %v", err)
	}
	pkgs := reg.ListPackages()
	if len(pkgs) == 0 {
		t.Fatal("expected tools in the shipped manifest")
	}
	for _, p := range pkgs {
		if p.Install.Docs == "" && p.ToolchainID == "" {
			t.Errorf("%s: a tool without a toolchain installer needs install docs", p.ID)
		}
	}
	pw, err := reg.GetPackage("playwright")
	if err != nil {
		t.Fatal(err)
	}
	if argv := pw.HealthCheckArgv(); len(argv) != 4 || argv[0] != "npx" || argv[1] != "--no-install" {
		t.Errorf("playwright health check argv = %v", argv)
	}
}

func TestToolRegistryRejectsShellSyntax(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.json")
	_ = os.WriteFile(path, []byte(`[{"id":"x","name":"X","health_check_cmd":"x --version && rm -rf /"}]`), 0o644)
	if err := NewToolRegistry().LoadFromFile(path); err == nil {
		t.Fatal("a health check with shell syntax must be rejected")
	}
}
