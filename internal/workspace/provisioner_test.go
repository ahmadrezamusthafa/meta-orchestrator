package workspace

import (
	"context"
	"os"
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
