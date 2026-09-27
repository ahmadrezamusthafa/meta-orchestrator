package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/hooks"
)

func TestWorkspaceProvisionerLifecycle(t *testing.T) {
	tmpWorkspaces := t.TempDir()
	hookEngine := hooks.NewHookEngine("")

	var hookFired bool
	hookEngine.RegisterHook(hooks.HookDefinition{
		ID:         "hook-test-prep",
		Event:      hooks.HookPreStage,
		Driver:     hooks.HookDriverShell,
		Target:     "echo 'Workspace prep executed'",
		IsBlocking: true,
	})

	provisioner := NewWorkspaceProvisioner(tmpWorkspaces, hookEngine)
	ctx := context.Background()

	// Provision multi-repo workspace: frontend-portal and backend-core
	taskID := "task-ws-001"
	repos := []string{"frontend-portal", "backend-core"}

	ws, err := provisioner.Provision(ctx, taskID, repos, nil)
	if err != nil {
		t.Fatalf("failed to provision workspace: %v", err)
	}

	if ws.Status != WorkspaceStatusActive {
		t.Errorf("expected status ACTIVE, got %s", ws.Status)
	}
	if len(ws.RepoPaths) != 2 {
		t.Errorf("expected 2 repo sandboxes, got %d", len(ws.RepoPaths))
	}

	// Verify paths on disk
	for _, repo := range repos {
		repoPath := ws.RepoPaths[repo]
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			t.Errorf("repo path does not exist on disk: %s", repoPath)
		}
	}
	if _, err := os.Stat(ws.ScratchDir); os.IsNotExist(err) {
		t.Errorf("scratch dir does not exist on disk: %s", ws.ScratchDir)
	}

	// Verify Teardown
	err = provisioner.Teardown(ctx, taskID)
	if err != nil {
		t.Fatalf("failed to teardown workspace: %v", err)
	}

	if _, err := os.Stat(ws.BasePath); !os.IsNotExist(err) {
		t.Errorf("workspace base path was not removed after teardown: %s", ws.BasePath)
	}
	_ = hookFired
}

func TestWorkspaceProvisionerCopySource(t *testing.T) {
	tmpWorkspaces := t.TempDir()
	tmpSource := t.TempDir()

	// Create source files
	srcBackend := filepath.Join(tmpSource, "backend")
	_ = os.MkdirAll(srcBackend, 0755)
	_ = os.WriteFile(filepath.Join(srcBackend, "main.go"), []byte("package main"), 0644)

	provisioner := NewWorkspaceProvisioner(tmpWorkspaces, nil)
	ctx := context.Background()

	sourceMap := map[string]string{
		"backend": srcBackend,
	}
	ws, err := provisioner.Provision(ctx, "task-copy-1", []string{"backend"}, sourceMap)
	if err != nil {
		t.Fatalf("provision failed: %v", err)
	}

	copiedFile := filepath.Join(ws.RepoPaths["backend"], "main.go")
	data, err := os.ReadFile(copiedFile)
	if err != nil || string(data) != "package main" {
		t.Errorf("source file was not copied into workspace sandbox")
	}
}

func TestWorkspaceProvisionerWorktree(t *testing.T) {
	tmpWorkspaces := t.TempDir()
	tmpGitRepo := t.TempDir()

	// Initialize git repository with initial commit
	runCmd := func(dir string, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command failed: %s %v: %v\noutput: %s", name, args, err, string(out))
		}
	}

	runCmd(tmpGitRepo, "git", "init")
	runCmd(tmpGitRepo, "git", "config", "user.name", "Test Agent")
	runCmd(tmpGitRepo, "git", "config", "user.email", "test@orchestrator.local")
	_ = os.WriteFile(filepath.Join(tmpGitRepo, "README.md"), []byte("# Test Repo"), 0644)
	runCmd(tmpGitRepo, "git", "add", "README.md")
	runCmd(tmpGitRepo, "git", "commit", "-m", "Initial commit")

	provisioner := NewWorkspaceProvisioner(tmpWorkspaces, nil)
	ctx := context.Background()

	taskID := "task-wt-001"
	repoMap := map[string]string{
		"repo-a": tmpGitRepo,
	}

	ws, err := provisioner.Provision(ctx, taskID, []string{"repo-a"}, repoMap)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	if !ws.IsWorktree {
		t.Errorf("expected ws.IsWorktree to be true")
	}

	wtPath := ws.RepoPaths["repo-a"]
	if _, err := os.Stat(filepath.Join(wtPath, "README.md")); os.IsNotExist(err) {
		t.Errorf("README.md not found in worktree path: %s", wtPath)
	}

	// Teardown should remove worktree cleanly
	err = provisioner.Teardown(ctx, taskID)
	if err != nil {
		t.Fatalf("Teardown failed: %v", err)
	}
}

