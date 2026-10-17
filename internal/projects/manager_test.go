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

// A project folder may be an existing folder of the operator's: syncing links never deletes
// what the manager did not create, and repository names cannot leave the folder.
func TestProjectLinksNeverDeleteTheOperatorsFiles(t *testing.T) {
	tempDir := t.TempDir()
	repo := filepath.Join(tempDir, "src", "svc")
	_ = os.MkdirAll(repo, 0o755)
	_ = os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module svc"), 0o644)
	mgr := NewProjectManager(tempDir)

	root := filepath.Join(tempDir, "my-workspaces", "payments") // contains "workspaces", not managed
	_ = os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "notes.txt"), []byte("keep me"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "svc2"), []byte("a file named like a repo"), 0o644)
	_ = os.Symlink(repo, filepath.Join(root, "old-link"))

	p, err := mgr.Create(&types.Project{ID: "payments", Name: "Payments", RootDir: root,
		Repos: []types.ProjectRepo{{Name: "svc", Path: repo}, {Name: "svc2", Path: repo}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{"notes.txt", "docs", "svc2"} {
		if _, err := os.Lstat(filepath.Join(root, keep)); err != nil {
			t.Errorf("%s was deleted: %v", keep, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "old-link")); !os.IsNotExist(err) {
		t.Error("a stale link the project no longer lists is removed")
	}
	if fi, err := os.Lstat(filepath.Join(root, "svc")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("svc should be linked: %v", err)
	}
	if p.Repos[1].Status != "error" || p.Status != "degraded" {
		t.Errorf("a real file in the way is reported, not replaced: %+v", p.Repos[1])
	}
	if _, err := mgr.ResyncSymlinks("payments"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "notes.txt")); string(b) != "keep me" {
		t.Error("resync deleted the operator's file")
	}

	// Deleting a project outside the managed workspaces folder keeps the folder.
	if err := mgr.Delete("payments"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); err != nil {
		t.Fatalf("deleting the project removed the operator's folder: %v", err)
	}

	for _, bad := range []string{"../escape", "a/b", "..", "."} {
		if _, err := mgr.Create(&types.Project{Name: "Bad", Repos: []types.ProjectRepo{{Name: bad, Path: repo}}}); err == nil {
			t.Errorf("repository name %q must be rejected", bad)
		}
	}
	if _, err := os.Lstat(filepath.Join(tempDir, "workspaces", "escape")); !os.IsNotExist(err) {
		t.Error("a repository name escaped the project folder")
	}
}

func TestProjectReadsDoNotRaceWithResync(t *testing.T) {
	tempDir := t.TempDir()
	repo := filepath.Join(tempDir, "svc")
	_ = os.MkdirAll(repo, 0o755)
	mgr := NewProjectManager(tempDir)
	if _, err := mgr.Create(&types.Project{ID: "p", Name: "P", Repos: []types.ProjectRepo{{Name: "svc", Path: repo}}}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			_, _ = mgr.ResyncSymlinks("p")
		}
	}()
	for i := 0; i < 20; i++ {
		for _, p := range mgr.List() {
			_ = p.Status + p.Repos[0].Status
		}
		if p, err := mgr.Get("p"); err == nil {
			_ = p.UpdatedAt.String() + p.Repos[0].GitBranch
		}
	}
	<-done
}
