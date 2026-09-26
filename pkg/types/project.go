package types

import "time"

// ProjectRole defines the functional role of a registered repository or directory.
type ProjectRole string

const (
	RoleFrontend       ProjectRole = "frontend"
	RoleBackend        ProjectRole = "backend"
	RoleAutomationTest ProjectRole = "automation-test"
	RoleContracts      ProjectRole = "contracts"
	RoleArtifact       ProjectRole = "artifact"
	RoleOtherService   ProjectRole = "other"
)

// ProjectRepo encapsulates a single registered repository/path within a project.
type ProjectRepo struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Path         string      `json:"path"`          // Absolute source path on disk
	Role         ProjectRole `json:"role"`          // "backend", "frontend", "automation-test", etc.
	ManifestType string      `json:"manifest_type"` // "go.mod", "package.json", "composer.json", "openapi.yaml", etc.
	SymlinkPath  string      `json:"symlink_path"`  // Absolute symlink target inside project root
	Status       string      `json:"status"`        // "linked", "missing_source", "error"
	Error        string      `json:"error,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
}

// Project represents a multi-repo orchestration project with an established project root and symlinks.
type Project struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	RootDir     string        `json:"root_dir"`    // Where the symlinked project root is located
	ActiveSDLC  string        `json:"active_sdlc"` // e.g. "general-ai-sdlc"
	Repos       []ProjectRepo `json:"repos"`
	Status      string        `json:"status"` // "provisioned", "pending", "degraded"
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// DetectedRepo represents a repository discovered during directory scanning.
type DetectedRepo struct {
	Name          string      `json:"name"`
	Path          string      `json:"path"`
	SuggestedRole ProjectRole `json:"suggested_role"`
	ManifestType  string      `json:"manifest_type"`
	FilesCount    int         `json:"files_count"`
}

// ScanDirResult returns discovered repositories from a directory path.
type ScanDirResult struct {
	ScannedPath   string         `json:"scanned_path"`
	DetectedRepos []DetectedRepo `json:"detected_repos"`
}
