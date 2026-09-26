package projects

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestProjectManager_CreateAndSymlink(t *testing.T) {
	tempDir := t.TempDir()

	// Create dummy repositories
	frontendDir := filepath.Join(tempDir, "repos", "frontend-app")
	_ = os.MkdirAll(frontendDir, 0755)
	_ = os.WriteFile(filepath.Join(frontendDir, "package.json"), []byte(`{"name":"frontend-app"}`), 0644)

	backendDir := filepath.Join(tempDir, "repos", "backend-service")
	_ = os.MkdirAll(backendDir, 0755)
	_ = os.WriteFile(filepath.Join(backendDir, "go.mod"), []byte(`module backend-service`), 0644)

	e2eDir := filepath.Join(tempDir, "repos", "e2e-tests")
	_ = os.MkdirAll(e2eDir, 0755)
	_ = os.WriteFile(filepath.Join(e2eDir, "playwright.config.ts"), []byte(`export default {}`), 0644)

	mgr := NewProjectManager(tempDir)

	projectRoot := filepath.Join(tempDir, "workspaces", "test-project")
	p := &types.Project{
		ID:          "test-project",
		Name:        "Test Project",
		Description: "A multi-repo test setup",
		RootDir:     projectRoot,
		Repos: []types.ProjectRepo{
			{
				Name: "frontend-app",
				Path: frontendDir,
				Role: types.RoleFrontend,
			},
			{
				Name: "backend-service",
				Path: backendDir,
				Role: types.RoleBackend,
			},
			{
				Name: "e2e-tests",
				Path: e2eDir,
				Role: types.RoleAutomationTest,
			},
		},
	}

	created, err := mgr.Create(p)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	if created.Status != "provisioned" {
		t.Fatalf("Expected status provisioned, got %s", created.Status)
	}

	// Verify symlinks exist and resolve correctly
	feSymlink := filepath.Join(projectRoot, "frontend-app")
	if fi, err := os.Lstat(feSymlink); err != nil || (fi.Mode()&os.ModeSymlink == 0) {
		t.Errorf("frontend-app should be a symlink: %v", err)
	}

	beSymlink := filepath.Join(projectRoot, "backend-service")
	if fi, err := os.Lstat(beSymlink); err != nil || (fi.Mode()&os.ModeSymlink == 0) {
		t.Errorf("backend-service should be a symlink: %v", err)
	}

	// Test ScanDirectory
	scanRes, err := mgr.ScanDirectory(filepath.Join(tempDir, "repos"))
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	if len(scanRes.DetectedRepos) != 3 {
		t.Fatalf("Expected 3 detected repos, got %d", len(scanRes.DetectedRepos))
	}

	// Test ResyncSymlinks
	resynced, err := mgr.ResyncSymlinks(created.ID)
	if err != nil {
		t.Fatalf("ResyncSymlinks failed: %v", err)
	}
	if resynced.Status != "provisioned" {
		t.Fatalf("Expected provisioned after resync, got %s", resynced.Status)
	}
}
