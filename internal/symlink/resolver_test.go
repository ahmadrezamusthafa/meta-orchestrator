package symlink

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutoSymlinkResolutionAndCollision(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create provider: shared-ui v1.5.0
	providerDir := filepath.Join(tmpDir, "shared-ui")
	_ = os.MkdirAll(providerDir, 0755)
	providerJSON := `{"name": "@myorg/shared-ui", "version": "1.5.0"}`
	_ = os.WriteFile(filepath.Join(providerDir, "package.json"), []byte(providerJSON), 0644)

	// 2. Create consumer: frontend-portal requiring @myorg/shared-ui ^1.0.0
	consumerDir := filepath.Join(tmpDir, "frontend-portal")
	_ = os.MkdirAll(consumerDir, 0755)
	consumerJSON := `{
  "name": "frontend-portal",
  "version": "1.0.0",
  "dependencies": {
    "@myorg/shared-ui": "^1.0.0"
  }
}`
	_ = os.WriteFile(filepath.Join(consumerDir, "package.json"), []byte(consumerJSON), 0644)

	resolver := NewAutoSymlinkResolver()
	repoMap := map[string]string{
		"@myorg/shared-ui": providerDir,
		"frontend-portal":  consumerDir,
	}

	results, err := resolver.ResolveWorkspace(repoMap)
	if err != nil {
		t.Fatalf("unexpected error resolving symlinks: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 symlink created, got %d", len(results))
	}

	linkTarget, err := os.Readlink(results[0].TargetLink)
	if err != nil || linkTarget != providerDir {
		t.Errorf("expected link to point to %s, got %s", providerDir, linkTarget)
	}

	// 3. Test Major Version Collision (Consumer requires ^2.0.0 while provider is 1.5.0)
	conflictJSON := `{
  "name": "frontend-portal",
  "version": "1.0.0",
  "dependencies": {
    "@myorg/shared-ui": "^2.0.0"
  }
}`
	_ = os.WriteFile(filepath.Join(consumerDir, "package.json"), []byte(conflictJSON), 0644)

	_, err = resolver.ResolveWorkspace(repoMap)
	if err == nil {
		t.Fatalf("expected collision error for version mismatch ^2.0.0 vs 1.5.0")
	}
}
