package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

	hiddenDir := filepath.Join(tempDir, ".github")
	_ = os.MkdirAll(hiddenDir, 0755)

	router := NewRouter(RouterConfig{
		RootDir: tempDir,
	})

	// 1. GET /api/v1/fs/browse without path (defaults to RootDir and shows hidden dirs by default)
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

	for _, d := range resp.Directories {
		t.Logf("Found dir: %s (hidden=%v)", d.Name, d.IsHidden)
	}

	if len(resp.Directories) != 4 {
		t.Fatalf("Expected 4 directories with hidden included, got %d", len(resp.Directories))
	}

	var foundHidden bool
	for _, d := range resp.Directories {
		if d.Name == ".github" {
			foundHidden = true
			if !d.IsHidden {
				t.Errorf("Expected .github to have IsHidden=true")
			}
		}
	}
	if !foundHidden {
		t.Errorf("Expected .github directory to be present")
	}

	// 1b. GET /api/v1/fs/browse?show_hidden=false
	reqHiddenOff := httptest.NewRequest(http.MethodGet, "/api/v1/fs/browse?show_hidden=false", nil)
	wHiddenOff := httptest.NewRecorder()
	router.ServeHTTP(wHiddenOff, reqHiddenOff)

	if wHiddenOff.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", wHiddenOff.Code)
	}

	var respHiddenOff BrowseFSResponse
	if err := json.NewDecoder(wHiddenOff.Body).Decode(&respHiddenOff); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(respHiddenOff.Directories) != 2 {
		t.Fatalf("Expected 2 directories when show_hidden=false, got %d", len(respHiddenOff.Directories))
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

func TestFSMkdirAPI(t *testing.T) {
	tempDir := t.TempDir()

	router := NewRouter(RouterConfig{
		RootDir: tempDir,
	})

	// 1. Success creation with parent_path and folder_name
	createBody := `{"parent_path":"` + tempDir + `","folder_name":"my-new-project"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fs/mkdir", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var resp CreateFolderResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	expectedPath := filepath.Join(tempDir, "my-new-project")
	if resp.Path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, resp.Path)
	}
	if resp.Name != "my-new-project" {
		t.Errorf("Expected name my-new-project, got %s", resp.Name)
	}

	// Verify it exists on disk
	fi, err := os.Stat(expectedPath)
	if err != nil || !fi.IsDir() {
		t.Fatalf("Expected directory to exist on disk: %v", err)
	}

	// 2. Conflict on existing directory
	reqConflict := httptest.NewRequest(http.MethodPost, "/api/v1/fs/mkdir", strings.NewReader(createBody))
	reqConflict.Header.Set("Content-Type", "application/json")
	wConflict := httptest.NewRecorder()
	router.ServeHTTP(wConflict, reqConflict)

	if wConflict.Code != http.StatusConflict {
		t.Errorf("Expected 409 Conflict, got %d", wConflict.Code)
	}

	// 3. Invalid folder name (empty or path traversal)
	badBody := `{"parent_path":"` + tempDir + `","folder_name":".."}`
	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/fs/mkdir", strings.NewReader(badBody))
	reqBad.Header.Set("Content-Type", "application/json")
	wBad := httptest.NewRecorder()
	router.ServeHTTP(wBad, reqBad)

	if wBad.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request, got %d", wBad.Code)
	}

	// 4. Method not allowed
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/fs/mkdir", nil)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 Method Not Allowed, got %d", wGet.Code)
	}
}

