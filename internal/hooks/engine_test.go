package hooks

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestShellHookExecutionAndBlocking(t *testing.T) {
	engine := NewHookEngine("")
	ctx := context.Background()

	// 1. Success hook
	engine.RegisterHook(HookDefinition{
		ID:         "test-echo",
		Event:      HookPreStage,
		Driver:     HookDriverShell,
		Target:     "echo \"Running task $META_TASK_ID in stage $META_STAGE\"",
		IsBlocking: true,
	})

	err := engine.TriggerEvent(ctx, HookPreStage, &HookExecutionContext{
		TaskID:        "task-hook-1",
		StageID:       "ATDD_RED_PHASE",
		WorkspacePath: "/tmp/workspace",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// 2. Blocking failure hook
	engine.RegisterHook(HookDefinition{
		ID:         "blocking-gate-fail",
		Event:      HookOnGate,
		Driver:     HookDriverShell,
		Target:     "exit 1",
		IsBlocking: true,
	})

	err = engine.TriggerEvent(ctx, HookOnGate, &HookExecutionContext{
		TaskID:  "task-hook-2",
		StageID: "GATE_TECH_DOC_REVIEW",
	})
	if err == nil {
		t.Fatalf("expected error from blocking hook with exit 1")
	}

	// 3. Non-blocking failure hook should not error out
	nonBlockingEngine := NewHookEngine("")
	nonBlockingEngine.RegisterHook(HookDefinition{
		ID:         "warn-only",
		Event:      HookPostStage,
		Driver:     HookDriverShell,
		Target:     "exit 1",
		IsBlocking: false,
	})
	err = nonBlockingEngine.TriggerEvent(ctx, HookPostStage, &HookExecutionContext{TaskID: "task-3"})
	if err != nil {
		t.Fatalf("expected non-blocking hook to not return error, got %v", err)
	}
}

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestWebhookHook(t *testing.T) {
	var receivedCount int32
	mockClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			atomic.AddInt32(&receivedCount, 1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
				Header:     make(http.Header),
			}
		}),
	}

	engine := NewHookEngine("")
	engine.SetHTTPClient(mockClient)
	engine.RegisterHook(HookDefinition{
		ID:         "slack-notifier",
		Event:      HookOnFailure,
		Driver:     HookDriverWebhook,
		Target:     "https://webhook.internal/slack",
		IsBlocking: false,
	})

	ctx := context.Background()
	err := engine.TriggerEvent(ctx, HookOnFailure, &HookExecutionContext{
		TaskID:  "task-fail-99",
		StageID: "IMPLEMENTATION_GREEN",
	})
	if err != nil {
		t.Fatalf("failed to trigger webhook: %v", err)
	}

	if atomic.LoadInt32(&receivedCount) != 1 {
		t.Errorf("expected 1 webhook request received, got %d", receivedCount)
	}
}

func TestConventionHookScript(t *testing.T) {
	tmpDir := t.TempDir()
	hooksDir := filepath.Join(tmpDir, ".sdlc", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}

	scriptPath := filepath.Join(hooksDir, "pre-commit.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho 'pre-commit convention check passed'\n"), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	engine := NewHookEngine(tmpDir)
	ctx := context.Background()

	err := engine.TriggerEvent(ctx, HookPreCommit, &HookExecutionContext{
		TaskID:  "task-commit-1",
		StageID: "IMPLEMENTATION_GREEN",
	})
	if err != nil {
		t.Fatalf("failed to run convention hook: %v", err)
	}
}
