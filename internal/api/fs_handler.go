package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
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
	Stack         []string          `json:"stack,omitempty"`
	HasChildren   bool              `json:"has_children"`
	IsHidden      bool              `json:"is_hidden"`
}

// FileItem is a file of the browsed directory (listed only when files are requested).
type FileItem struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	IsHidden bool   `json:"is_hidden"`
}

// maxBrowseFiles caps the files listed for one directory.
const maxBrowseFiles = 2000

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
	Files          []FileItem       `json:"files,omitempty"`
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

	// Check whether hidden directories should be visible (default: true)
	showHiddenParam := req.URL.Query().Get("show_hidden")
	if showHiddenParam == "" {
		showHiddenParam = req.URL.Query().Get("hidden")
	}
	showHidden := showHiddenParam != "false" && showHiddenParam != "0"

	// Files are listed only for file pickers (files=true), e.g. attaching a file to a console prompt.
	withFiles := req.URL.Query().Get("files") == "true"

	// Read child directories
	entries, err := os.ReadDir(targetPath)
	var dirs []DirectoryItem
	var files []FileItem
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				if withFiles && len(files) < maxBrowseFiles && (showHidden || !strings.HasPrefix(entry.Name(), ".")) {
					var size int64
					if info, infoErr := entry.Info(); infoErr == nil {
						size = info.Size()
					}
					files = append(files, FileItem{Name: entry.Name(), Path: filepath.Join(targetPath, entry.Name()), Size: size,
						IsHidden: strings.HasPrefix(entry.Name(), ".")})
				}
				continue
			}
			name := entry.Name()
			isHidden := strings.HasPrefix(name, ".")

			// Filter out hidden folders if show_hidden is explicitly disabled
			if isHidden && !showHidden {
				continue
			}

			// Always skip internal git repository database and large package caches
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "bin" || name == "dist" {
				continue
			}

			childPath := filepath.Join(targetPath, name)
			class := stack.Classify(name, childPath)
			manifest, suggestedRole := class.Manifest, class.Role
			isRepo := (manifest != stack.Unknown && manifest != "markdown") || dirExists(filepath.Join(childPath, ".git"))

			hasChildren := false
			if subEntries, subErr := os.ReadDir(childPath); subErr == nil {
				for _, sub := range subEntries {
					if !sub.IsDir() {
						continue
					}
					subName := sub.Name()
					if subName == ".git" || subName == "node_modules" || subName == "vendor" {
						continue
					}
					if strings.HasPrefix(subName, ".") && !showHidden {
						continue
					}
					hasChildren = true
					break
				}
			}

			dirs = append(dirs, DirectoryItem{
				Name:          name,
				Path:          childPath,
				IsRepo:        isRepo,
				Manifest:      manifest,
				SuggestedRole: suggestedRole,
				Stack:         class.Stack,
				HasChildren:   hasChildren,
				IsHidden:      isHidden,
			})
		}
	}

	// Sort directories alphabetically
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
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
		Files:          files,
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

// CreateFolderRequest holds parameters to create a new folder.
type CreateFolderRequest struct {
	ParentPath string `json:"parent_path"`
	FolderName string `json:"folder_name"`
	Path       string `json:"path"`
}

// CreateFolderResponse represents response after folder creation.
type CreateFolderResponse struct {
	Success    bool   `json:"success"`
	Path       string `json:"path"`
	Name       string `json:"name"`
	ParentPath string `json:"parent_path"`
}

func (r *Router) handleFSMkdir(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var payload CreateFolderRequest
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	var targetPath string
	var folderName string
	var parentDir string

	if strings.TrimSpace(payload.Path) != "" {
		targetPath = filepath.Clean(strings.TrimSpace(payload.Path))
		folderName = filepath.Base(targetPath)
		parentDir = filepath.Dir(targetPath)
	} else {
		folderName = strings.TrimSpace(payload.FolderName)
		parentDir = strings.TrimSpace(payload.ParentPath)
		if parentDir == "" {
			parentDir = r.cfg.RootDir
		}
		parentDir = filepath.Clean(parentDir)

		if folderName == "" {
			r.writeError(w, http.StatusBadRequest, "Folder name is required")
			return
		}

		if strings.Contains(folderName, "/") || strings.Contains(folderName, "\\") || folderName == ".." || folderName == "." {
			r.writeError(w, http.StatusBadRequest, "Invalid folder name")
			return
		}

		targetPath = filepath.Join(parentDir, folderName)
	}

	// Check if already exists
	if _, err := os.Stat(targetPath); err == nil {
		r.writeError(w, http.StatusConflict, "Directory already exists")
		return
	}

	// Create directory
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		r.writeError(w, http.StatusInternalServerError, "Failed to create directory: "+err.Error())
		return
	}

	r.writeJSON(w, http.StatusCreated, CreateFolderResponse{
		Success:    true,
		Path:       targetPath,
		Name:       folderName,
		ParentPath: parentDir,
	})
}
