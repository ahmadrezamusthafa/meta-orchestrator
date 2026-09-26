package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestProjectsAPI(t *testing.T) {
	tempDir := t.TempDir()

	dummyRepo := filepath.Join(tempDir, "services", "auth-service")
	_ = os.MkdirAll(dummyRepo, 0755)
	_ = os.WriteFile(filepath.Join(dummyRepo, "go.mod"), []byte(`module auth-service`), 0644)

	projMgr := projects.NewProjectManager(tempDir)
	router := NewRouter(RouterConfig{
		ProjectManager: projMgr,
		RootDir:        tempDir,
	})

	// 1. GET /api/v1/projects
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	var list []*types.Project
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// 2. POST /api/v1/projects/scan
	scanBody, _ := json.Marshal(ScanDirectoryRequest{Path: filepath.Join(tempDir, "services")})
	reqScan := httptest.NewRequest(http.MethodPost, "/api/v1/projects/scan", bytes.NewReader(scanBody))
	wScan := httptest.NewRecorder()
	router.ServeHTTP(wScan, reqScan)

	if wScan.Code != http.StatusOK {
		t.Fatalf("Expected 200 from scan, got %d: %s", wScan.Code, wScan.Body.String())
	}

	var scanRes types.ScanDirResult
	if err := json.NewDecoder(wScan.Body).Decode(&scanRes); err != nil {
		t.Fatalf("Failed to decode scan result: %v", err)
	}
	if len(scanRes.DetectedRepos) == 0 {
		t.Fatalf("Expected at least 1 detected repo, got 0")
	}

	// 3. POST /api/v1/projects
	newProj := types.Project{
		ID:          "custom-proj",
		Name:        "Custom Microservices",
		Description: "Multi-repo setup test",
		RootDir:     filepath.Join(tempDir, "workspaces", "custom-proj"),
		Repos: []types.ProjectRepo{
			{
				Name: "auth-service",
				Path: dummyRepo,
				Role: types.RoleBackend,
			},
		},
	}
	body, _ := json.Marshal(newProj)
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	wCreate := httptest.NewRecorder()
	router.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created, got %d: %s", wCreate.Code, wCreate.Body.String())
	}

	// 4. POST /api/v1/projects/custom-proj/resync
	reqResync := httptest.NewRequest(http.MethodPost, "/api/v1/projects/custom-proj/resync", nil)
	wResync := httptest.NewRecorder()
	router.ServeHTTP(wResync, reqResync)

	if wResync.Code != http.StatusOK {
		t.Fatalf("Expected 200 from resync, got %d: %s", wResync.Code, wResync.Body.String())
	}
}
