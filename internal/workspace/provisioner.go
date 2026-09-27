package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/hooks"
)

// WorkspaceProvisioner manages ephemeral sandboxes, git worktrees, and cloned repository directories.
type WorkspaceProvisioner struct {
	mu            sync.RWMutex
	rootWorkspacesDir string
	hookEngine    *hooks.HookEngine
	activeSpaces  map[string]*Workspace
	sourceDirMap  map[string]map[string]string // taskID -> (repo -> srcPath)
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
		sourceDirMap:      make(map[string]map[string]string),
	}
}

func isGitDirectory(dir string) bool {
	if dir == "" {
		return false
	}
	gitPath := filepath.Join(dir, ".git")
	_, err := os.Stat(gitPath)
	return err == nil
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
	worktreeBranches := make(map[string]string)
	isWorktreeMode := false

	if sourceDirMap != nil {
		p.sourceDirMap[taskID] = sourceDirMap
	}

	for _, repo := range repos {
		targetRepoDir := filepath.Join(taskDir, repo)
		worktreeCreated := false

		// Check if source repo exists and has a .git directory to use worktrees
		if sourceDirMap != nil {
			if srcPath, exists := sourceDirMap[repo]; exists && isGitDirectory(srcPath) {
				branchName := fmt.Sprintf("feat/%s-%s", strings.ToLower(taskID), repo)
				_ = os.MkdirAll(filepath.Dir(targetRepoDir), 0755)

				// Create isolated git worktree for parallel execution
				cmd := exec.Command("git", "-C", srcPath, "worktree", "add", "-b", branchName, targetRepoDir)
				if err := cmd.Run(); err != nil {
					// If branch already exists, attach without -b
					cmdRetry := exec.Command("git", "-C", srcPath, "worktree", "add", targetRepoDir, branchName)
					if errRetry := cmdRetry.Run(); errRetry == nil {
						worktreeCreated = true
					}
				} else {
					worktreeCreated = true
				}

				if worktreeCreated {
					isWorktreeMode = true
					worktreeBranches[repo] = branchName
				}
			}
		}

		if !worktreeCreated {
			if err := os.MkdirAll(targetRepoDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create repo sandbox dir %s: %w", repo, err)
			}
			if sourceDirMap != nil {
				if srcPath, exists := sourceDirMap[repo]; exists {
					_ = copyDirMock(srcPath, targetRepoDir)
				}
			}
		}

		repoPaths[repo] = targetRepoDir
	}

	ws := &Workspace{
		TaskID:           taskID,
		BasePath:         taskDir,
		RepoPaths:        repoPaths,
		ScratchDir:       scratchDir,
		Status:           WorkspaceStatusActive,
		IsWorktree:       isWorktreeMode,
		WorktreeBranches: worktreeBranches,
		CreatedAt:        time.Now(),
		LastUpdated:      time.Now(),
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

// Teardown cleanly removes the ephemeral directory tree and detaches git worktrees.
func (p *WorkspaceProvisioner) Teardown(ctx context.Context, taskID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ws, exists := p.activeSpaces[taskID]
	if !exists {
		return nil
	}

	// Detach and clean up git worktrees if active
	if ws.IsWorktree && p.sourceDirMap != nil {
		if srcMap, ok := p.sourceDirMap[taskID]; ok {
			for repo := range ws.WorktreeBranches {
				if srcPath, hasSrc := srcMap[repo]; hasSrc {
					targetDir := ws.RepoPaths[repo]
					cmd := exec.Command("git", "-C", srcPath, "worktree", "remove", "--force", targetDir)
					_ = cmd.Run()
					cmdPrune := exec.Command("git", "-C", srcPath, "worktree", "prune")
					_ = cmdPrune.Run()
				}
			}
		}
		delete(p.sourceDirMap, taskID)
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
