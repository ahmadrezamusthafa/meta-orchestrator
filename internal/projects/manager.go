package projects

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// ProjectManager governs project registration, multi-repo topology, and symlink synthesis.
type ProjectManager struct {
	mu          sync.RWMutex
	rootDir     string
	storeFile   string
	projects    map[string]*types.Project
}

// NewProjectManager initializes the project manager.
func NewProjectManager(rootDir string) *ProjectManager {
	if rootDir == "" {
		rootDir, _ = os.Getwd()
	}

	storeDir := filepath.Join(rootDir, ".sdlc")
	_ = os.MkdirAll(storeDir, 0755)
	storeFile := filepath.Join(storeDir, "projects.json")

	mgr := &ProjectManager{
		rootDir:   rootDir,
		storeFile: storeFile,
		projects:  make(map[string]*types.Project),
	}

	mgr.loadProjects()
	if len(mgr.projects) == 0 {
		mgr.seedDefaultProjects()
	}

	return mgr
}

func (m *ProjectManager) loadProjects() {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.storeFile)
	if err != nil {
		return
	}

	var list []*types.Project
	if err := json.Unmarshal(data, &list); err == nil {
		for _, p := range list {
			m.projects[p.ID] = p
		}
	}
}

func (m *ProjectManager) saveProjectsLocked() error {
	list := make([]*types.Project, 0, len(m.projects))
	for _, p := range m.projects {
		list = append(list, p)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize projects: %w", err)
	}

	return os.WriteFile(m.storeFile, data, 0644)
}

func (m *ProjectManager) seedDefaultProjects() {
	now := time.Now()
	defaultRoot := filepath.Join(m.rootDir, "workspaces", "proj-core-platform")

	p := &types.Project{
		ID:          "proj-core-platform",
		Name:        "Enterprise Core Platform",
		Description: "Multi-repo ecosystem connecting Vue 3 frontend, Go microservices, shared OpenAPI contracts, and Playwright E2E suites.",
		RootDir:     defaultRoot,
		ActiveSDLC:  "general-ai-sdlc",
		Status:      "provisioned",
		CreatedAt:   now,
		UpdatedAt:   now,
		Repos: []types.ProjectRepo{
			{
				ID:           "repo-fe-portal",
				Name:         "frontend-portal",
				Path:         filepath.Join(m.rootDir, "web"),
				Role:         types.RoleFrontend,
				ManifestType: "package.json",
				SymlinkPath:  filepath.Join(defaultRoot, "frontend-portal"),
				Status:       "linked",
				CreatedAt:    now,
			},
			{
				ID:           "repo-be-core",
				Name:         "backend-core",
				Path:         m.rootDir,
				Role:         types.RoleBackend,
				ManifestType: "go.mod",
				SymlinkPath:  filepath.Join(defaultRoot, "backend-core"),
				Status:       "linked",
				CreatedAt:    now,
			},
			{
				ID:           "repo-contracts",
				Name:         "api-contracts",
				Path:         filepath.Join(m.rootDir, "schemas"),
				Role:         types.RoleContracts,
				ManifestType: "openapi.yaml",
				SymlinkPath:  filepath.Join(defaultRoot, "api-contracts"),
				Status:       "linked",
				CreatedAt:    now,
			},
			{
				ID:           "repo-e2e",
				Name:         "automation-test",
				Path:         filepath.Join(m.rootDir, "e2e"),
				Role:         types.RoleAutomationTest,
				ManifestType: "playwright.config.ts",
				SymlinkPath:  filepath.Join(defaultRoot, "automation-test"),
				Status:       "linked",
				CreatedAt:    now,
			},
			{
				ID:           "repo-docs",
				Name:         "artifacts-docs",
				Path:         filepath.Join(m.rootDir, ".sdlc", "prds"),
				Role:         types.RoleArtifact,
				ManifestType: "markdown",
				SymlinkPath:  filepath.Join(defaultRoot, "artifacts-docs"),
				Status:       "linked",
				CreatedAt:    now,
			},
		},
	}

	_, _ = m.Create(p)
}

// List returns all registered projects.
func (m *ProjectManager) List() []*types.Project {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*types.Project, 0, len(m.projects))
	for _, p := range m.projects {
		result = append(result, p)
	}
	return result
}

// Get retrieves a project by ID.
func (m *ProjectManager) Get(id string) (*types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.projects[id]
	if !exists {
		return nil, fmt.Errorf("project '%s' not found", id)
	}
	return p, nil
}

// Create registers a project, provisions its root folder, and establishes symlinks.
func (m *ProjectManager) Create(p *types.Project) (*types.Project, error) {
	if p.Name == "" {
		return nil, fmt.Errorf("project name is required")
	}

	if p.ID == "" {
		p.ID = slugify(p.Name)
	}

	if p.RootDir == "" {
		p.RootDir = filepath.Join(m.rootDir, "workspaces", p.ID)
	} else if !filepath.IsAbs(p.RootDir) {
		p.RootDir = filepath.Join(m.rootDir, p.RootDir)
	}

	if p.ActiveSDLC == "" {
		p.ActiveSDLC = "general-ai-sdlc"
	}

	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.Status = "provisioned"

	// Create root directory
	if err := os.MkdirAll(p.RootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create project root directory %s: %w", p.RootDir, err)
	}

	// Create .sdlc directory inside project root
	projSDLCDir := filepath.Join(p.RootDir, ".sdlc")
	_ = os.MkdirAll(projSDLCDir, 0755)

	// Process and link each repository
	hasErrors := false
	for i := range p.Repos {
		repo := &p.Repos[i]
		if repo.ID == "" {
			repo.ID = fmt.Sprintf("repo-%s", slugify(repo.Name))
		}
		repo.CreatedAt = now

		// Detect manifest if empty
		if repo.ManifestType == "" {
			repo.ManifestType = detectManifest(repo.Path)
		}

		symlinkTarget := filepath.Join(p.RootDir, repo.Name)
		repo.SymlinkPath = symlinkTarget

		// Clean up existing symlink or path at destination
		_ = os.Remove(symlinkTarget)

		// Check if source path exists
		if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
			repo.Status = "missing_source"
			repo.Error = fmt.Sprintf("Source path does not exist: %s", repo.Path)
			hasErrors = true
			continue
		}

		// Create atomic symlink
		if err := os.Symlink(repo.Path, symlinkTarget); err != nil {
			repo.Status = "error"
			repo.Error = fmt.Sprintf("Failed to create symlink: %v", err)
			hasErrors = true
		} else {
			repo.Status = "linked"
			repo.Error = ""
		}
	}

	if hasErrors {
		p.Status = "degraded"
	}

	// Write project.json inside project root .sdlc
	metaData, _ := json.MarshalIndent(p, "", "  ")
	_ = os.WriteFile(filepath.Join(projSDLCDir, "project.json"), metaData, 0644)

	m.mu.Lock()
	m.projects[p.ID] = p
	_ = m.saveProjectsLocked()
	m.mu.Unlock()

	return p, nil
}

// ResyncSymlinks re-establishes filesystem symlinks for an existing project.
func (m *ProjectManager) ResyncSymlinks(id string) (*types.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.projects[id]
	if !exists {
		return nil, fmt.Errorf("project '%s' not found", id)
	}

	if err := os.MkdirAll(p.RootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create project root directory %s: %w", p.RootDir, err)
	}

	hasErrors := false
	for i := range p.Repos {
		repo := &p.Repos[i]
		if repo.ManifestType == "" {
			repo.ManifestType = detectManifest(repo.Path)
		}

		symlinkTarget := filepath.Join(p.RootDir, repo.Name)
		repo.SymlinkPath = symlinkTarget

		_ = os.Remove(symlinkTarget)

		if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
			repo.Status = "missing_source"
			repo.Error = fmt.Sprintf("Source path not found: %s", repo.Path)
			hasErrors = true
			continue
		}

		if err := os.Symlink(repo.Path, symlinkTarget); err != nil {
			repo.Status = "error"
			repo.Error = fmt.Sprintf("Symlink failed: %v", err)
			hasErrors = true
		} else {
			repo.Status = "linked"
			repo.Error = ""
		}
	}

	p.UpdatedAt = time.Now()
	if hasErrors {
		p.Status = "degraded"
	} else {
		p.Status = "provisioned"
	}

	projSDLCDir := filepath.Join(p.RootDir, ".sdlc")
	_ = os.MkdirAll(projSDLCDir, 0755)
	metaData, _ := json.MarshalIndent(p, "", "  ")
	_ = os.WriteFile(filepath.Join(projSDLCDir, "project.json"), metaData, 0644)

	_ = m.saveProjectsLocked()
	return p, nil
}

// Update updates project metadata and re-links repos.
func (m *ProjectManager) Update(id string, updated *types.Project) (*types.Project, error) {
	m.mu.Lock()
	_, exists := m.projects[id]
	m.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("project '%s' not found", id)
	}

	updated.ID = id
	return m.Create(updated)
}

// Delete removes project registration and its root symlink directory if desired.
func (m *ProjectManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.projects[id]
	if !exists {
		return fmt.Errorf("project '%s' not found", id)
	}

	// Remove project directory
	if p.RootDir != "" && strings.Contains(p.RootDir, "workspaces") {
		_ = os.RemoveAll(p.RootDir)
	}

	delete(m.projects, id)
	return m.saveProjectsLocked()
}

// ScanDirectory scans a local folder for subdirectories and suggests functional roles.
func (m *ProjectManager) ScanDirectory(targetPath string) (*types.ScanDirResult, error) {
	if targetPath == "" {
		targetPath = m.rootDir
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory '%s': %w", targetPath, err)
	}

	var detected []types.DetectedRepo
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" || entry.Name() == "vendor" || entry.Name() == "bin" {
			continue
		}

		childPath := filepath.Join(targetPath, entry.Name())
		role, manifest, count := classifyDirectory(entry.Name(), childPath)

		detected = append(detected, types.DetectedRepo{
			Name:          entry.Name(),
			Path:          childPath,
			SuggestedRole: role,
			ManifestType:  manifest,
			FilesCount:    count,
		})
	}

	// If no subdirectories matched, test if targetPath itself is a single repo
	if len(detected) == 0 {
		baseName := filepath.Base(targetPath)
		role, manifest, count := classifyDirectory(baseName, targetPath)
		if manifest != "unknown" {
			detected = append(detected, types.DetectedRepo{
				Name:          baseName,
				Path:          targetPath,
				SuggestedRole: role,
				ManifestType:  manifest,
				FilesCount:    count,
			})
		}
	}

	return &types.ScanDirResult{
		ScannedPath:   targetPath,
		DetectedRepos: detected,
	}, nil
}

func classifyDirectory(name string, path string) (types.ProjectRole, string, int) {
	lowerName := strings.ToLower(name)
	entries, _ := os.ReadDir(path)
	fileCount := len(entries)

	// 1. Check for Playwright / Cypress / E2E test suite
	if _, err := os.Stat(filepath.Join(path, "playwright.config.ts")); err == nil {
		return types.RoleAutomationTest, "playwright.config.ts", fileCount
	}
	if _, err := os.Stat(filepath.Join(path, "playwright.config.js")); err == nil {
		return types.RoleAutomationTest, "playwright.config.js", fileCount
	}
	if _, err := os.Stat(filepath.Join(path, "cypress.config.ts")); err == nil {
		return types.RoleAutomationTest, "cypress.config.ts", fileCount
	}
	if strings.Contains(lowerName, "test") || strings.Contains(lowerName, "e2e") || strings.Contains(lowerName, "atdd") || strings.Contains(lowerName, "automation") {
		return types.RoleAutomationTest, detectManifest(path), fileCount
	}

	// 2. Check for Contracts (OpenAPI, Protobuf, Schemas)
	if _, err := os.Stat(filepath.Join(path, "openapi.yaml")); err == nil {
		return types.RoleContracts, "openapi.yaml", fileCount
	}
	if _, err := os.Stat(filepath.Join(path, "openapi.json")); err == nil {
		return types.RoleContracts, "openapi.json", fileCount
	}
	if strings.Contains(lowerName, "contract") || strings.Contains(lowerName, "schema") || strings.Contains(lowerName, "proto") {
		return types.RoleContracts, detectManifest(path), fileCount
	}

	// 3. Check for Docs / Artifacts
	if strings.Contains(lowerName, "artifact") || strings.Contains(lowerName, "doc") || strings.Contains(lowerName, "prd") || strings.Contains(lowerName, "spec") {
		return types.RoleArtifact, "markdown", fileCount
	}

	// 4. Check for Frontend (Vite, Vue, React, Next, Nuxt, Svelte, Angular)
	if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
		manifestData, _ := os.ReadFile(filepath.Join(path, "package.json"))
		mStr := strings.ToLower(string(manifestData))

		if strings.Contains(lowerName, "front") || strings.Contains(lowerName, "ui") || strings.Contains(lowerName, "web") || strings.Contains(lowerName, "client") ||
			strings.Contains(mStr, "vue") || strings.Contains(mStr, "react") || strings.Contains(mStr, "next") || strings.Contains(mStr, "nuxt") || strings.Contains(mStr, "vite") {
			return types.RoleFrontend, "package.json", fileCount
		}
		// If it's a node backend
		return types.RoleBackend, "package.json", fileCount
	}

	// 5. Check for Backend languages
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		return types.RoleBackend, "go.mod", fileCount
	}
	if _, err := os.Stat(filepath.Join(path, "composer.json")); err == nil {
		return types.RoleBackend, "composer.json", fileCount
	}
	if _, err := os.Stat(filepath.Join(path, "pom.xml")); err == nil {
		return types.RoleBackend, "pom.xml", fileCount
	}
	if _, err := os.Stat(filepath.Join(path, "Cargo.toml")); err == nil {
		return types.RoleBackend, "Cargo.toml", fileCount
	}
	if _, err := os.Stat(filepath.Join(path, "requirements.txt")); err == nil {
		return types.RoleBackend, "requirements.txt", fileCount
	}

	return types.RoleOtherService, "unknown", fileCount
}

func detectManifest(path string) string {
	if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
		return "package.json"
	}
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		return "go.mod"
	}
	if _, err := os.Stat(filepath.Join(path, "composer.json")); err == nil {
		return "composer.json"
	}
	if _, err := os.Stat(filepath.Join(path, "openapi.yaml")); err == nil {
		return "openapi.yaml"
	}
	if _, err := os.Stat(filepath.Join(path, "playwright.config.ts")); err == nil {
		return "playwright.config.ts"
	}
	if _, err := os.Stat(filepath.Join(path, "Cargo.toml")); err == nil {
		return "Cargo.toml"
	}
	return "unknown"
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' || r == '-' {
			b.WriteRune('-')
		}
	}
	res := b.String()
	res = strings.Trim(res, "-")
	if res == "" {
		return "project"
	}
	return res
}
