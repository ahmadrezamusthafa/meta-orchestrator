package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFSBrowseAPI(t *testing.T) {
	tempDir := t.TempDir()

	// Create child folders with different manifests
	feDir := filepath.Join(tempDir, "frontend-app")
	_ = os.MkdirAll(feDir, 0755)
	_ = os.WriteFile(filepath.Join(feDir, "package.json"), []byte(`{"name":"fe"}`), 0644)

	beDir := filepath.Join(tempDir, "backend-service")
	_ = os.MkdirAll(beDir, 0755)
	_ = os.WriteFile(filepath.Join(beDir, "go.mod"), []byte(`module be`), 0644)

	router := NewRouter(RouterConfig{
		RootDir: tempDir,
	})

	// 1. GET /api/v1/fs/browse without path (defaults to RootDir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fs/browse", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	var resp BrowseFSResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.CurrentPath != tempDir {
		t.Errorf("Expected current_path to be %s, got %s", tempDir, resp.CurrentPath)
	}

	if len(resp.Directories) != 2 {
		t.Fatalf("Expected 2 directories, got %d", len(resp.Directories))
	}

	// 2. Test breadcrumbs
	if len(resp.Breadcrumbs) == 0 {
		t.Errorf("Expected breadcrumbs to not be empty")
	}

	// 3. Test quick bookmarks
	if len(resp.QuickBookmarks) == 0 {
		t.Errorf("Expected quick bookmarks to be populated")
	}
}
