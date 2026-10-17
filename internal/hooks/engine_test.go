package hooks

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

	_, err := engine.TriggerEvent(ctx, HookPreStage, &HookExecutionContext{
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

	_, err = engine.TriggerEvent(ctx, HookOnGate, &HookExecutionContext{
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
	warnings, err := nonBlockingEngine.TriggerEvent(ctx, HookPostStage, &HookExecutionContext{TaskID: "task-3"})
	if err != nil {
		t.Fatalf("expected non-blocking hook to not return error, got %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "warn-only") {
		t.Fatalf("a non-blocking failure must be reported to the caller, got %v", warnings)
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
	_, err := engine.TriggerEvent(ctx, HookOnFailure, &HookExecutionContext{
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

	_, err := engine.TriggerEvent(ctx, HookPreCommit, &HookExecutionContext{
		TaskID:  "task-commit-1",
		StageID: "IMPLEMENTATION_GREEN",
	})
	if err != nil {
		t.Fatalf("failed to run convention hook: %v", err)
	}
}

func TestShellHookRunsInWorkspaceAndCapsOutput(t *testing.T) {
	ws := t.TempDir()
	engine := NewHookEngine("")
	engine.RegisterHook(HookDefinition{ID: "where", Event: HookPreStage, Driver: HookDriverShell, IsBlocking: true,
		Target: `pwd > where.txt; echo "$META_EVENT $META_TASK_ID" >> where.txt`})
	engine.RegisterHook(HookDefinition{ID: "noisy", Event: HookPostStage, Driver: HookDriverShell, IsBlocking: true,
		Target: `i=0; while [ $i -lt 2000 ]; do echo "line $i"; i=$((i+1)); done; exit 3`})
	if _, err := engine.TriggerEvent(context.Background(), HookPreStage, &HookExecutionContext{TaskID: "T1", WorkspacePath: ws}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(ws, "where.txt"))
	if !strings.Contains(string(got), "pre-stage T1") {
		t.Fatalf("hook should run in the workspace with META_* set, got %q", got)
	}
	_, err := engine.TriggerEvent(context.Background(), HookPostStage, &HookExecutionContext{TaskID: "T1", WorkspacePath: ws})
	if err == nil || len(err.Error()) > maxHookOutput+300 || !strings.Contains(err.Error(), "line 1999") {
		t.Fatalf("error should keep only the tail of the output (len %d)", len(err.Error()))
	}
}

func TestDockerHookRunsImageWithWorkspaceMounted(t *testing.T) {
	ws := t.TempDir()
	bin := filepath.Join(t.TempDir(), "docker")
	argsFile := filepath.Join(t.TempDir(), "args")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho \"$@\" > "+argsFile+"\n[ \"$1\" = rm ] || exit $FAKE_DOCKER_EXIT\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_DOCKER_EXIT", "0")
	engine := NewHookEngine("")
	engine.SetDockerBinary(bin)
	engine.RegisterHook(HookDefinition{ID: "lint", Event: HookPreCommit, Driver: HookDriverDocker, Target: "golangci/golangci-lint:v1", IsBlocking: true})
	if _, err := engine.TriggerEvent(context.Background(), HookPreCommit, &HookExecutionContext{TaskID: "T9", StageID: "task_implementation", WorkspacePath: ws}); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argsFile)
	for _, want := range []string{"run --rm --name meta-hook-", "-v " + ws + ":/workspace", "-w /workspace", "-e META_TASK_ID=T9", "-e META_WORKSPACE_PATH=/workspace", "golangci/golangci-lint:v1"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("docker args %q missing %q", args, want)
		}
	}

	t.Setenv("FAKE_DOCKER_EXIT", "2")
	if _, err := engine.TriggerEvent(context.Background(), HookPreCommit, &HookExecutionContext{TaskID: "T9", WorkspacePath: ws}); err == nil {
		t.Fatal("a failing container must fail the blocking hook")
	}
	if _, err := engine.TriggerEvent(context.Background(), HookPreCommit, &HookExecutionContext{TaskID: "T9"}); err == nil {
		t.Fatal("a docker hook without a workspace must fail, not succeed silently")
	}
}

func TestLoadDefinitionsValidates(t *testing.T) {
	dir := t.TempDir()
	if defs, err := LoadDefinitions(filepath.Join(dir, "missing.json")); err != nil || defs != nil {
		t.Fatalf("missing file is no hooks: %v %v", defs, err)
	}
	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte(`[{"id":"x","event":"pre-nothing","driver":"shell","target":"true"}]`), 0o600)
	if _, err := LoadDefinitions(bad); err == nil {
		t.Fatal("unknown event must be rejected")
	}
	good := filepath.Join(dir, "good.json")
	_ = os.WriteFile(good, []byte(`[{"id":"x","event":"pre-stage","driver":"shell","target":"true","is_blocking":true}]`), 0o600)
	defs, err := LoadDefinitions(good)
	if err != nil || len(defs) != 1 || !defs[0].IsBlocking {
		t.Fatalf("LoadDefinitions = %+v, %v", defs, err)
	}
}

func TestWebhookFailureNeverEchoesTheURL(t *testing.T) {
	engine := NewHookEngine("")
	engine.SetHTTPClient(&http.Client{Transport: failingTransport{}})
	engine.RegisterHook(HookDefinition{ID: "notify", Event: HookOnFailure, Driver: HookDriverWebhook, Target: "https://hooks.example.test/services/SECRET-PATH", IsBlocking: true})
	_, err := engine.TriggerEvent(context.Background(), HookOnFailure, &HookExecutionContext{TaskID: "T"})
	if err == nil || strings.Contains(err.Error(), "SECRET-PATH") {
		t.Fatalf("webhook errors must not carry the target URL: %v", err)
	}
	if red := engine.List()[0].Redacted(); strings.Contains(red.Target, "SECRET-PATH") {
		t.Fatalf("Redacted kept the webhook path: %s", red.Target)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}
