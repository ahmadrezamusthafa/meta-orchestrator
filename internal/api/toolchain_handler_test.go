package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// bareMachine has only the given binaries, all working.
func bareMachine(bins ...string) *toolchain.Toolchain {
	have := map[string]bool{}
	for _, b := range bins {
		have[b] = true
	}
	return toolchain.NewWithHost("darwin",
		func(name string) (string, error) {
			if have[name] {
				return "/opt/homebrew/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		func(ctx context.Context, name string, args ...string) (string, error) {
			return filepath.Base(name) + " 1.0.0", nil
		})
}

func toolchainRouter(t *testing.T, tc *toolchain.Toolchain) *Router {
	t.Helper()
	root, infra := t.TempDir(), t.TempDir()
	_ = os.WriteFile(filepath.Join(infra, "terragrunt.hcl"), []byte("terraform {}"), 0o644)
	_ = os.WriteFile(filepath.Join(infra, "main.tf"), []byte(`resource "x" "y" {}`), 0o644)
	pm := projects.NewProjectManager(root)
	if _, err := pm.Create(&types.Project{Name: "Platform", Repos: []types.ProjectRepo{{Name: "infra", Path: infra}}}); err != nil {
		t.Fatal(err)
	}
	r := NewRouter(RouterConfig{RootDir: root, ProjectManager: pm, Toolchain: tc})
	r.mu.Lock()
	r.tasks["TASK-9"] = &types.Task{ID: "TASK-9", Title: "Add a unit", CurrentStageID: "task_implementation",
		State: types.TaskStatePending, AssignedRepos: []string{"infra"}, Metadata: map[string]string{}}
	r.tasks["TASK-10"] = &types.Task{ID: "TASK-10", Title: "Plan", CurrentStageID: "techdoc_rfc",
		State: types.TaskStatePending, AssignedRepos: []string{"infra"}, Metadata: map[string]string{}}
	r.mu.Unlock()
	return r
}

func tcDo(t *testing.T, r *Router, method, url, body string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, url, strings.NewReader(body)))
	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func TestCodeStageAsksForMissingToolsFirst(t *testing.T) {
	r := toolchainRouter(t, bareMachine("brew"))

	w, out := tcDo(t, r, http.MethodPost, "/api/v1/tasks/TASK-9/execute", "")
	if w.Code != http.StatusConflict || out["status"] != "needs_tools" {
		t.Fatalf("execute = %d %v", w.Code, out)
	}
	if got := out["missing"].([]interface{}); len(got) != 2 || got[0] != "tofu" || got[1] != "terragrunt" {
		t.Fatalf("missing = %v", got)
	}
	items := out["toolchain"].(map[string]interface{})["items"].([]interface{})
	first := items[0].(map[string]interface{})
	if first["id"] != "tofu" || first["recommended"].(float64) < 0 {
		t.Fatalf("the response carries one-click install options: %v", first)
	}
	if r.taskSnapshot("TASK-9").State != types.TaskStatePending {
		t.Fatal("a blocked stage does not start")
	}

	// Stages that do not run code checks are never blocked.
	if err := r.toolchainGate("TASK-10"); err != nil {
		t.Fatalf("techdoc stage gated: %v", err)
	}

	// Continue anyway: the gate opens for exactly the tools missing now.
	w, out = tcDo(t, r, http.MethodPost, "/api/v1/tasks/TASK-9/toolchain/ack", "")
	if w.Code != http.StatusOK || len(out["acknowledged"].([]interface{})) != 2 {
		t.Fatalf("ack = %d %v", w.Code, out)
	}
	if err := r.toolchainGate("TASK-9"); err != nil {
		t.Fatalf("acknowledged tools still block: %v", err)
	}
	if brief := r.stackBrief(r.taskSnapshot("TASK-9")); !strings.Contains(brief, "Not installed on this machine: OpenTofu (`tofu`), Terragrunt") {
		t.Errorf("the agent is told which tools are missing:\n%s", brief)
	}
}

func TestToolchainEndpoints(t *testing.T) {
	r := toolchainRouter(t, bareMachine("brew", "tofu", "terragrunt"))
	if err := r.toolchainGate("TASK-9"); err != nil {
		t.Fatalf("installed tools pass the gate: %v", err)
	}

	w, out := tcDo(t, r, http.MethodGet, "/api/v1/toolchain?all=1", "")
	if w.Code != http.StatusOK || len(out["items"].([]interface{})) != len(toolchain.Catalog) || out["ready"] != true {
		t.Fatalf("catalog view = %d %v", w.Code, out)
	}
	w, out = tcDo(t, r, http.MethodGet, "/api/v1/tasks/TASK-9/toolchain", "")
	stackTools := 0
	for _, raw := range out["items"].([]interface{}) {
		if id := raw.(map[string]interface{})["id"]; id == "tofu" || id == "terragrunt" {
			stackTools++
		}
	}
	if w.Code != http.StatusOK || stackTools != 2 || out["ready"] != true {
		t.Fatalf("task view = %d %v", w.Code, out)
	}
	if w, _ := tcDo(t, r, http.MethodPost, "/api/v1/toolchain/recheck?project=platform", ""); w.Code != http.StatusOK {
		t.Fatalf("recheck = %d", w.Code)
	}

	cases := map[string]int{
		`{"tool":"nope","method":0}`:  http.StatusNotFound,
		`{"tool":"java","method":0}`:  http.StatusBadRequest, // needs a password: copy-and-run only
		`{"tool":"pnpm","method":0}`:  http.StatusBadRequest, // npm is not installed
		`{"tool":"tofu","method":99}`: http.StatusBadRequest,
		`not json`:                    http.StatusBadRequest,
	}
	for body, want := range cases {
		if w, _ := tcDo(t, r, http.MethodPost, "/api/v1/toolchain/install", body); w.Code != want {
			t.Errorf("install %s = %d, want %d (%s)", body, w.Code, want, w.Body.String())
		}
	}
	if w, _ := tcDo(t, r, http.MethodGet, "/api/v1/toolchain/jobs/install-404", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown job = %d", w.Code)
	}
}

func TestMessageToBlockedStageBecomesAChat(t *testing.T) {
	r := toolchainRouter(t, bareMachine("brew"))
	stub := &streamingStub{}
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, stub)
	}
	w, out := tcDo(t, r, http.MethodPost, "/api/v1/tasks/TASK-9/chat", `{"message":"what does this unit deploy?"}`)
	if w.Code != http.StatusAccepted || out["mode"] != "chat" {
		t.Fatalf("chat = %d %v — the message must be answered, not dropped", w.Code, out)
	}
	if r.taskSnapshot("TASK-9").CurrentStageID != "task_implementation" {
		t.Fatal("the stage stays where it was")
	}
	r.console.mu.Lock()
	entries := append([]types.ConsoleEntry(nil), r.console.get("TASK-9").entries...)
	r.console.mu.Unlock()
	found := false
	for _, e := range entries {
		found = found || strings.Contains(e.Content, "Not started: this stage verifies changes with OpenTofu")
	}
	if !found {
		t.Fatal("the console says why the stage did not start")
	}
}
