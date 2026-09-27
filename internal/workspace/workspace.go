package workspace

import "time"

// WorkspaceStatus denotes lifecycle state of an ephemeral workspace.
type WorkspaceStatus string

const (
	WorkspaceStatusProvisioning WorkspaceStatus = "PROVISIONING"
	WorkspaceStatusActive       WorkspaceStatus = "ACTIVE"
	WorkspaceStatusLocked       WorkspaceStatus = "LOCKED"
	WorkspaceStatusTornDown     WorkspaceStatus = "TORN_DOWN"
)

// Workspace encapsulates the isolated directory environment for a task.
type Workspace struct {
	TaskID           string            `json:"task_id"`
	BasePath         string            `json:"base_path"`
	RepoPaths        map[string]string `json:"repo_paths"` // repo_name -> absolute path inside workspace
	ScratchDir       string            `json:"scratch_dir"`
	Status           WorkspaceStatus   `json:"status"`
	IsWorktree       bool              `json:"is_worktree"`
	WorktreeBranches map[string]string `json:"worktree_branches,omitempty"` // repo_name -> worktree branch name
	CreatedAt        time.Time         `json:"created_at"`
	LastUpdated      time.Time         `json:"last_updated"`
}
