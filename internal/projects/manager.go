package projects

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// ProjectManager governs project registration, multi-repo topology, and symlink synthesis.
type ProjectManager struct {
	mu        sync.RWMutex
	rootDir   string
	storeFile string
	projects  map[string]*types.Project
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

	// Write a sibling file and rename it over the store, so a crash never leaves it half written.
	tmp, err := os.CreateTemp(filepath.Dir(m.storeFile), ".projects-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), m.storeFile)
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
		result = append(result, cloneProject(p))
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
	return cloneProject(p), nil
}

// Create registers a project, provisions its root folder, and establishes symlinks.
func (m *ProjectManager) Create(p *types.Project) (*types.Project, error) {
	if p.Name == "" {
		return nil, fmt.Errorf("project name is required")
	}

	if p.ID == "" {
		p.ID = slugify(p.Name)
	}
	for _, r := range p.Repos {
		if err := validRepoName(r.Name); err != nil {
			return nil, err
		}
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
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	p.Status = "provisioned"

	// Create root directory
	if err := os.MkdirAll(p.RootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create project root directory %s: %w", p.RootDir, err)
	}

	// Create .sdlc directory inside project root
	projSDLCDir := filepath.Join(p.RootDir, ".sdlc")
	_ = os.MkdirAll(projSDLCDir, 0755)

	removeStaleLinks(p)

	// Process and link each repository
	hasErrors := false
	for i := range p.Repos {
		repo := &p.Repos[i]
		if repo.ID == "" {
			repo.ID = fmt.Sprintf("repo-%s", slugify(repo.Name))
		}
		if repo.CreatedAt.IsZero() {
			repo.CreatedAt = now
		}

		// Detect manifest if empty
		if repo.ManifestType == "" {
			repo.ManifestType = detectManifest(repo.Path)
		}

		// Detect Git branch, file count and tech stack
		repo.GitBranch = readGitBranch(repo.Path)
		repo.FilesCount = countFiles(repo.Path)
		repo.Stack = stack.For(repo.Path).Names()

		symlinkTarget := filepath.Join(p.RootDir, repo.Name)
		repo.SymlinkPath = symlinkTarget

		// Check if source path exists
		if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
			repo.Status = "missing_source"
			repo.Error = fmt.Sprintf("Source path does not exist: %s", repo.Path)
			hasErrors = true
			continue
		}

		if err := linkRepo(repo.Path, symlinkTarget); err != nil {
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
	out := cloneProject(p)
	m.mu.Unlock()

	return out, nil
}

// validRepoName accepts a repository name that is one entry inside the project folder.
func validRepoName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || !filepath.IsLocal(name) {
		return fmt.Errorf("invalid repository name %q: it names the link inside the project folder", name)
	}
	return nil
}

// removeStaleLinks deletes the links in the project folder that no repository uses any more.
// Only links are removed: the folder may also hold the operator's own files.
func removeStaleLinks(p *types.Project) {
	active := make(map[string]bool)
	for _, r := range p.Repos {
		active[r.Name] = true
	}
	entries, err := os.ReadDir(p.RootDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || active[entry.Name()] || entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		_ = os.Remove(filepath.Join(p.RootDir, entry.Name()))
	}
}

// linkRepo points link at the repository. It replaces an earlier link but never a real file or
// folder; a repository that already lives at link needs no link.
func linkRepo(source, link string) error {
	if err := validRepoName(filepath.Base(link)); err != nil {
		return err
	}
	if fi, err := os.Lstat(link); err == nil {
		if fi.Mode()&os.ModeSymlink == 0 {
			if filepath.Clean(source) == filepath.Clean(link) {
				return nil
			}
			return fmt.Errorf("%s already exists and is not a link; rename it or the repository", link)
		}
		if err := os.Remove(link); err != nil {
			return err
		}
	}
	return os.Symlink(source, link)
}

// managedRoot reports whether dir is a project folder under the manager's workspaces folder —
// the only kind of project folder it deletes.
func (m *ProjectManager) managedRoot(dir string) bool {
	rel, err := filepath.Rel(filepath.Join(m.rootDir, "workspaces"), dir)
	return err == nil && rel != "." && filepath.IsLocal(rel)
}

// cloneProject copies p, so callers never share the manager's project.
func cloneProject(p *types.Project) *types.Project {
	cp := *p
	cp.Repos = slices.Clone(p.Repos)
	for i := range cp.Repos {
		cp.Repos[i].Stack = slices.Clone(cp.Repos[i].Stack)
	}
	return &cp
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

	removeStaleLinks(p)

	hasErrors := false
	for i := range p.Repos {
		repo := &p.Repos[i]
		if repo.ManifestType == "" {
			repo.ManifestType = detectManifest(repo.Path)
		}
		repo.GitBranch = readGitBranch(repo.Path)
		repo.FilesCount = countFiles(repo.Path)
		repo.Stack = stack.For(repo.Path).Names()

		symlinkTarget := filepath.Join(p.RootDir, repo.Name)
		repo.SymlinkPath = symlinkTarget

		if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
			repo.Status = "missing_source"
			repo.Error = fmt.Sprintf("Source path not found: %s", repo.Path)
			hasErrors = true
			continue
		}

		if err := linkRepo(repo.Path, symlinkTarget); err != nil {
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
	return cloneProject(p), nil
}

// Update updates project metadata and re-links repos.
func (m *ProjectManager) Update(id string, updated *types.Project) (*types.Project, error) {
	m.mu.Lock()
	existing, exists := m.projects[id]
	m.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("project '%s' not found", id)
	}

	for _, r := range updated.Repos {
		if err := validRepoName(r.Name); err != nil {
			return nil, err // before the old folder is touched
		}
	}
	updated.ID = id
	if updated.CreatedAt.IsZero() {
		updated.CreatedAt = existing.CreatedAt
	}
	if updated.RootDir == "" {
		updated.RootDir = existing.RootDir
	}

	// If root directory changed, clean up old root dir if it was under workspaces/
	if existing.RootDir != "" && updated.RootDir != existing.RootDir && m.managedRoot(existing.RootDir) {
		_ = os.RemoveAll(existing.RootDir)
	}

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
	if p.RootDir != "" && m.managedRoot(p.RootDir) {
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
		role, manifest, techs, count := classifyDirectory(entry.Name(), childPath)

		detected = append(detected, types.DetectedRepo{
			Name:          entry.Name(),
			Path:          childPath,
			SuggestedRole: role,
			ManifestType:  manifest,
			FilesCount:    count,
			Stack:         techs,
		})
	}

	// If no subdirectories matched, test if targetPath itself is a single repo
	if len(detected) == 0 {
		baseName := filepath.Base(targetPath)
		role, manifest, techs, count := classifyDirectory(baseName, targetPath)
		if manifest != stack.Unknown {
			detected = append(detected, types.DetectedRepo{
				Name:          baseName,
				Path:          targetPath,
				SuggestedRole: role,
				ManifestType:  manifest,
				FilesCount:    count,
				Stack:         techs,
			})
		}
	}

	return &types.ScanDirResult{
		ScannedPath:   targetPath,
		DetectedRepos: detected,
	}, nil
}

// classifyDirectory suggests a role and manifest from the folder's files (see stack.Classify).
func classifyDirectory(name string, path string) (types.ProjectRole, string, []string, int) {
	c := stack.Classify(name, path)
	return c.Role, c.Manifest, c.Stack, countFiles(path)
}

func detectManifest(path string) string {
	return stack.Classify(filepath.Base(path), path).Manifest
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

func readGitBranch(repoPath string) string {
	headFile := filepath.Join(repoPath, ".git", "HEAD")
	data, err := os.ReadFile(headFile)
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	if strings.HasPrefix(content, "ref: refs/heads/") {
		return strings.TrimPrefix(content, "ref: refs/heads/")
	}
	if len(content) > 8 {
		return content[:8]
	}
	return content
}

func countFiles(repoPath string) int {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return 0
	}
	return len(entries)
}
