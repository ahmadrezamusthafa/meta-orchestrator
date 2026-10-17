package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `daemon packages …` runs without starting the server, against MO_HOME.
func TestPackagesSubcommand(t *testing.T) {
	cwd, tmp := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "configs"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `[{"id": "media", "name": "Media", "category": "Media", "kind": "system", "binaries": ["fake-media-tool"], "system": {"brew": "fake-media-tool"}}]`
	if err := os.WriteFile(filepath.Join(cwd, "configs", "packages.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MO_HOME", filepath.Join(tmp, "mo"))
	t.Setenv("HOME", tmp)
	t.Setenv("PATH", "/usr/bin:/bin")

	if code := runPackagesCLI(cwd, []string{"list"}); code != 0 {
		t.Fatalf("list exit %d", code)
	}
	if !strings.HasPrefix(os.Getenv("PATH"), filepath.Join(tmp, "mo", "bin")) {
		t.Fatalf("the managed bin folder is not first on PATH: %s", os.Getenv("PATH"))
	}
	if code := runPackagesCLI(cwd, []string{"install", "media"}); code != 1 {
		t.Fatalf("install without an unattended method exit %d", code)
	}
	if code := runPackagesCLI(cwd, []string{"bogus"}); code != 2 {
		t.Fatalf("unknown subcommand exit %d", code)
	}
}
