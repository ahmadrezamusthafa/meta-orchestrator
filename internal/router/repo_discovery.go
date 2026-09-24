package router

import (
	"os"
	"path/filepath"
	"strings"
)

// DiscoveredRepo details an identified codebase inside the organization workspace.
type DiscoveredRepo struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Category     string   `json:"category"` // "frontend", "backend", "contracts", "infra", "library"
	ManifestType string   `json:"manifest_type"`
	Languages    []string `json:"languages"`
}

// RepoDiscoveryEngine automatically discovers repositories and classifications within a workspace.
type RepoDiscoveryEngine struct {
	workspaceRoot string
}

// NewRepoDiscoveryEngine creates a new discovery engine.
func NewRepoDiscoveryEngine(workspaceRoot string) *RepoDiscoveryEngine {
	return &RepoDiscoveryEngine{workspaceRoot: workspaceRoot}
}

// Discover scans directories in workspaceRoot and identifies active codebases.
func (e *RepoDiscoveryEngine) Discover() ([]*DiscoveredRepo, error) {
	var repos []*DiscoveredRepo

	entries, err := os.ReadDir(e.workspaceRoot)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		repoPath := filepath.Join(e.workspaceRoot, entry.Name())
		repo := e.classifyRepo(entry.Name(), repoPath)
		if repo != nil {
			repos = append(repos, repo)
		}
	}

	// If no subdirectories matched, check if the workspaceRoot itself is a single repo
	if len(repos) == 0 {
		baseName := filepath.Base(e.workspaceRoot)
		repo := e.classifyRepo(baseName, e.workspaceRoot)
		if repo != nil {
			repos = append(repos, repo)
		}
	}

	return repos, nil
}

func (e *RepoDiscoveryEngine) classifyRepo(name string, path string) *DiscoveredRepo {
	category := "library"
	manifestType := "unknown"
	var languages []string

	// Check package.json (Frontend or Node backend)
	if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
		manifestType = "package.json"
		languages = append(languages, "typescript", "javascript")
		if strings.Contains(strings.ToLower(name), "front") || strings.Contains(strings.ToLower(name), "ui") || strings.Contains(strings.ToLower(name), "web") {
			category = "frontend"
		} else {
			category = "backend"
		}
	}

	// Check go.mod (Backend Go)
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		manifestType = "go.mod"
		languages = append(languages, "go")
		category = "backend"
	}

	// Check composer.json (PHP/Laravel)
	if _, err := os.Stat(filepath.Join(path, "composer.json")); err == nil {
		manifestType = "composer.json"
		languages = append(languages, "php")
		category = "backend"
	}

	// Check OpenAPI / Protobuf
	if _, err := os.Stat(filepath.Join(path, "openapi.yaml")); err == nil {
		category = "contracts"
	}

	if len(languages) == 0 {
		return nil
	}

	return &DiscoveredRepo{
		Name:         name,
		Path:         path,
		Category:     category,
		ManifestType: manifestType,
		Languages:    languages,
	}
}

// FindImpactedRepos returns repos whose files or names correlate with the task description.
func (e *RepoDiscoveryEngine) FindImpactedRepos(allRepos []*DiscoveredRepo, description string) []string {
	lowerDesc := strings.ToLower(description)
	var impacted []string

	for _, r := range allRepos {
		lowerName := strings.ToLower(r.Name)
		if strings.Contains(lowerDesc, lowerName) ||
			(r.Category == "frontend" && (strings.Contains(lowerDesc, "ui") || strings.Contains(lowerDesc, "frontend") || strings.Contains(lowerDesc, "page"))) ||
			(r.Category == "backend" && (strings.Contains(lowerDesc, "api") || strings.Contains(lowerDesc, "endpoint") || strings.Contains(lowerDesc, "backend") || strings.Contains(lowerDesc, "db"))) {
			impacted = append(impacted, r.Name)
		}
	}

	// If no specific match, default to all discovered repos
	if len(impacted) == 0 {
		for _, r := range allRepos {
			impacted = append(impacted, r.Name)
		}
	}

	return impacted
}
