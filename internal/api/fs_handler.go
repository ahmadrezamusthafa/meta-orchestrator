package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// BreadcrumbItem represents a single clickable path component.
type BreadcrumbItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// DirectoryItem represents a browsable child directory with repo classification.
type DirectoryItem struct {
	Name          string            `json:"name"`
	Path          string            `json:"path"`
	IsRepo        bool              `json:"is_repo"`
	Manifest      string            `json:"manifest"`
	SuggestedRole types.ProjectRole `json:"suggested_role"`
	HasChildren   bool              `json:"has_children"`
}

// QuickBookmark represents a shortcut bookmark to common project folders.
type QuickBookmark struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Icon string `json:"icon"`
}

// BrowseFSResponse encapsulates the directory tree response.
type BrowseFSResponse struct {
	CurrentPath    string           `json:"current_path"`
	ParentPath     string           `json:"parent_path"`
	Breadcrumbs    []BreadcrumbItem `json:"breadcrumbs"`
	Directories    []DirectoryItem  `json:"directories"`
	QuickBookmarks []QuickBookmark  `json:"quick_bookmarks"`
}

func (r *Router) handleFSBrowse(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	targetPath := strings.TrimSpace(req.URL.Query().Get("path"))
	if targetPath == "" {
		targetPath = r.cfg.RootDir
	}

	// Clean and resolve path
	targetPath = filepath.Clean(targetPath)
	fileInfo, err := os.Stat(targetPath)
	if err != nil || !fileInfo.IsDir() {
		// Fallback to RootDir
		targetPath = r.cfg.RootDir
		fileInfo, err = os.Stat(targetPath)
		if err != nil {
			targetPath = "/"
		}
	}

	// Generate Parent Path
	parentPath := filepath.Dir(targetPath)
	if parentPath == targetPath {
		parentPath = ""
	}

	// Generate Breadcrumbs
	breadcrumbs := generateBreadcrumbs(targetPath)

	// Read child directories
	entries, err := os.ReadDir(targetPath)
	var dirs []DirectoryItem
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "bin" || name == "dist" {
				continue
			}

			childPath := filepath.Join(targetPath, name)
			manifest := checkManifest(childPath)
			suggestedRole := classifyRole(name, manifest, childPath)
			isRepo := manifest != "unknown"

			hasChildren := false
			if subEntries, subErr := os.ReadDir(childPath); subErr == nil {
				for _, sub := range subEntries {
					if sub.IsDir() && !strings.HasPrefix(sub.Name(), ".") && sub.Name() != "node_modules" {
						hasChildren = true
						break
					}
				}
			}

			dirs = append(dirs, DirectoryItem{
				Name:          name,
				Path:          childPath,
				IsRepo:        isRepo,
				Manifest:      manifest,
				SuggestedRole: suggestedRole,
				HasChildren:   hasChildren,
			})
		}
	}

	// Sort directories alphabetically
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	// Quick Bookmarks
	homeDir, _ := os.UserHomeDir()
	bookmarks := []QuickBookmark{
		{
			Name: "App Workspace",
			Path: r.cfg.RootDir,
			Icon: "layers",
		},
		{
			Name: "Workspaces Directory",
			Path: filepath.Join(r.cfg.RootDir, "workspaces"),
			Icon: "folder-git",
		},
	}
	if homeDir != "" {
		bookmarks = append(bookmarks, QuickBookmark{
			Name: "Home Directory",
			Path: homeDir,
			Icon: "home",
		})
	}
	parentOfRoot := filepath.Dir(r.cfg.RootDir)
	if parentOfRoot != "" && parentOfRoot != r.cfg.RootDir {
		bookmarks = append(bookmarks, QuickBookmark{
			Name: "Parent Projects Folder",
			Path: parentOfRoot,
			Icon: "folder",
		})
	}

	resp := BrowseFSResponse{
		CurrentPath:    targetPath,
		ParentPath:     parentPath,
		Breadcrumbs:    breadcrumbs,
		Directories:    dirs,
		QuickBookmarks: bookmarks,
	}

	r.writeJSON(w, http.StatusOK, resp)
}

func generateBreadcrumbs(fullPath string) []BreadcrumbItem {
	clean := filepath.Clean(fullPath)
	if clean == "/" {
		return []BreadcrumbItem{{Name: "Root (/)", Path: "/"}}
	}

	parts := strings.Split(clean, string(filepath.Separator))
	var items []BreadcrumbItem

	accumulated := ""
	for i, part := range parts {
		if i == 0 && part == "" {
			accumulated = "/"
			items = append(items, BreadcrumbItem{Name: "Root", Path: "/"})
			continue
		}
		if part == "" {
			continue
		}

		if accumulated == "/" {
			accumulated = "/" + part
		} else {
			accumulated = accumulated + "/" + part
		}

		items = append(items, BreadcrumbItem{
			Name: part,
			Path: accumulated,
		})
	}

	return items
}

func checkManifest(path string) string {
	if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
		return "package.json"
	}
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		return "go.mod"
	}
	if _, err := os.Stat(filepath.Join(path, "playwright.config.ts")); err == nil {
		return "playwright.config.ts"
	}
	if _, err := os.Stat(filepath.Join(path, "playwright.config.js")); err == nil {
		return "playwright.config.js"
	}
	if _, err := os.Stat(filepath.Join(path, "openapi.yaml")); err == nil {
		return "openapi.yaml"
	}
	if _, err := os.Stat(filepath.Join(path, "composer.json")); err == nil {
		return "composer.json"
	}
	if _, err := os.Stat(filepath.Join(path, "Cargo.toml")); err == nil {
		return "Cargo.toml"
	}
	if _, err := os.Stat(filepath.Join(path, "requirements.txt")); err == nil {
		return "requirements.txt"
	}
	return "unknown"
}

func classifyRole(name, manifest, path string) types.ProjectRole {
	lowerName := strings.ToLower(name)

	// 1. Tests
	if strings.Contains(manifest, "playwright") || strings.Contains(lowerName, "test") || strings.Contains(lowerName, "e2e") || strings.Contains(lowerName, "atdd") {
		return types.RoleAutomationTest
	}

	// 2. Contracts
	if manifest == "openapi.yaml" || strings.Contains(lowerName, "contract") || strings.Contains(lowerName, "schema") || strings.Contains(lowerName, "proto") {
		return types.RoleContracts
	}

	// 3. Artifacts / Docs
	if strings.Contains(lowerName, "artifact") || strings.Contains(lowerName, "doc") || strings.Contains(lowerName, "prd") {
		return types.RoleArtifact
	}

	// 4. Frontend
	if manifest == "package.json" {
		data, _ := os.ReadFile(filepath.Join(path, "package.json"))
		mStr := strings.ToLower(string(data))
		if strings.Contains(lowerName, "front") || strings.Contains(lowerName, "ui") || strings.Contains(lowerName, "web") ||
			strings.Contains(mStr, "vue") || strings.Contains(mStr, "react") || strings.Contains(mStr, "vite") {
			return types.RoleFrontend
		}
	}

	// 5. Backend
	if manifest == "go.mod" || manifest == "composer.json" || manifest == "Cargo.toml" || strings.Contains(lowerName, "service") || strings.Contains(lowerName, "api") || strings.Contains(lowerName, "backend") {
		return types.RoleBackend
	}

	return types.RoleOtherService
}
