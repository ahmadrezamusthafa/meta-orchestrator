package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/hooks"
)

// WorkspaceProvisioner manages ephemeral sandboxes and cloned repository directories.
type WorkspaceProvisioner struct {
	mu            sync.RWMutex
	rootWorkspacesDir string
	hookEngine    *hooks.HookEngine
	activeSpaces  map[string]*Workspace
}

// NewWorkspaceProvisioner initializes provisioner rooted at rootWorkspacesDir.
func NewWorkspaceProvisioner(rootWorkspacesDir string, hookEngine *hooks.HookEngine) *WorkspaceProvisioner {
	if rootWorkspacesDir == "" {
		rootWorkspacesDir = filepath.Join(os.TempDir(), "meta-orchestrator-workspaces")
	}
	return &WorkspaceProvisioner{
		rootWorkspacesDir: rootWorkspacesDir,
		hookEngine:        hookEngine,
		activeSpaces:      make(map[string]*Workspace),
	}
}

// Provision creates isolated directory tree for taskID and prepares identified repositories.
func (p *WorkspaceProvisioner) Provision(ctx context.Context, taskID string, repos []string, sourceDirMap map[string]string) (*Workspace, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	taskDir := filepath.Join(p.rootWorkspacesDir, taskID)
	scratchDir := filepath.Join(taskDir, ".scratch")

	if err := os.MkdirAll(scratchDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create task workspace directory: %w", err)
	}

	repoPaths := make(map[string]string)
	for _, repo := range repos {
		targetRepoDir := filepath.Join(taskDir, repo)
		if err := os.MkdirAll(targetRepoDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create repo sandbox dir %s: %w", repo, err)
		}

		// If a source directory is provided, simulate or copy files
		if sourceDirMap != nil {
			if srcPath, exists := sourceDirMap[repo]; exists {
				_ = copyDirMock(srcPath, targetRepoDir)
			}
		}

		repoPaths[repo] = targetRepoDir
	}

	ws := &Workspace{
		TaskID:      taskID,
		BasePath:    taskDir,
		RepoPaths:   repoPaths,
		ScratchDir:  scratchDir,
		Status:      WorkspaceStatusActive,
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}

	p.activeSpaces[taskID] = ws

	// Trigger pre-stage lifecycle hook if configured
	if p.hookEngine != nil {
		hookCtx := &hooks.HookExecutionContext{
			TaskID:        taskID,
			StageID:       "WORKSPACE_PROVISION",
			WorkspacePath: taskDir,
		}
		if err := p.hookEngine.TriggerEvent(ctx, hooks.HookPreStage, hookCtx); err != nil {
			return nil, fmt.Errorf("pre-stage hook failed during workspace provisioning: %w", err)
		}
	}

	return ws, nil
}

// GetWorkspace returns active workspace for a task.
func (p *WorkspaceProvisioner) GetWorkspace(taskID string) (*Workspace, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ws, exists := p.activeSpaces[taskID]
	if !exists {
		return nil, fmt.Errorf("workspace for task %s not found", taskID)
	}
	return ws, nil
}

// Teardown cleanly removes the ephemeral directory tree.
func (p *WorkspaceProvisioner) Teardown(ctx context.Context, taskID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ws, exists := p.activeSpaces[taskID]
	if !exists {
		return nil
	}

	if err := os.RemoveAll(ws.BasePath); err != nil {
		return fmt.Errorf("failed to remove workspace path %s: %w", ws.BasePath, err)
	}

	ws.Status = WorkspaceStatusTornDown
	delete(p.activeSpaces, taskID)
	return nil
}

func copyDirMock(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		d := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			_ = os.MkdirAll(d, 0755)
			_ = copyDirMock(s, d)
		} else {
			data, err := os.ReadFile(s)
			if err == nil {
				_ = os.WriteFile(d, data, 0644)
			}
		}
	}
	return nil
}
