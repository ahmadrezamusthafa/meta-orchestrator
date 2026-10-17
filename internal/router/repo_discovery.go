package router

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// DiscoveredRepo details an identified codebase inside the organization workspace.
type DiscoveredRepo struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Category     string   `json:"category"` // "frontend", "backend", "contracts", "infra", "library"
	ManifestType string   `json:"manifest_type"`
	Languages    []string `json:"languages"` // detected technologies
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
	c := stack.Classify(name, path)
	if c.Manifest == stack.Unknown || c.Manifest == "markdown" {
		return nil
	}
	category := "library"
	switch c.Role {
	case types.RoleFrontend:
		category = "frontend"
	case types.RoleBackend:
		category = "backend"
	case types.RoleContracts:
		category = "contracts"
	case types.RoleInfrastructure:
		category = "infra"
	}
	return &DiscoveredRepo{
		Name:         name,
		Path:         path,
		Category:     category,
		ManifestType: c.Manifest,
		Languages:    c.Stack,
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
			(r.Category == "backend" && (strings.Contains(lowerDesc, "api") || strings.Contains(lowerDesc, "endpoint") || strings.Contains(lowerDesc, "backend") || strings.Contains(lowerDesc, "db"))) ||
			(r.Category == "infra" && (strings.Contains(lowerDesc, "infra") || strings.Contains(lowerDesc, "deploy") || strings.Contains(lowerDesc, "terragrunt") || strings.Contains(lowerDesc, "opentofu"))) {
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
