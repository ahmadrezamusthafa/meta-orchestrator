package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
)

const hubManifest = `[
  {"id": "alpha", "name": "Alpha", "category": "Agents", "health_check_cmd": "alpha --version",
   "required_runtimes": {"node": ">=18.0.0"}, "install": {"docs": "https://example.invalid/alpha", "commands": ["npm install -g alpha"]}},
  {"id": "beta", "name": "Beta", "category": "Runtimes", "health_check_cmd": "beta version",
   "required_runtimes": {"node": ">=22"}, "install": {"docs": "https://example.invalid/beta"}},
  {"id": "gamma", "name": "Gamma", "category": "Runtimes", "health_check_cmd": "gamma -v", "install": {"docs": "https://example.invalid/gamma"}},
  {"id": "node", "name": "Node.js", "category": "Runtimes", "health_check_cmd": "node --version", "toolchain_id": "node", "install": {"docs": ""}},
  {"id": "docker", "name": "Docker", "category": "Runtimes", "health_check_cmd": "docker --version", "toolchain_id": "docker", "install": {"docs": ""}}
]`

// fakeHost is a machine with some binaries; runs records every version command it answers.
type fakeHost struct {
	mu   sync.Mutex
	bins map[string]string // binary → version output; "!fail" fails the command
	runs []string
	// marker, once it exists, adds "node" (what the fake brew install drops).
	marker string
}

func (h *fakeHost) lookPath(name string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.bins[name]; ok {
		return "/fake/bin/" + name, nil
	}
	if name == "node" && h.marker != "" {
		if _, err := os.Stat(h.marker); err == nil {
			return "/fake/bin/node", nil
		}
	}
	return "", errors.New("not found")
}

func (h *fakeHost) run(_ context.Context, name string, args ...string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.runs = append(h.runs, strings.Join(append([]string{name}, args...), " "))
	out, ok := h.bins[filepath.Base(name)]
	if !ok && filepath.Base(name) == "node" {
		out, ok = "v20.11.1", true
	}
	if !ok || out == "!fail" {
		return "segmentation fault", errors.New("exit status 139")
	}
	return out, nil
}

func (h *fakeHost) runCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.runs)
}

func hubRouter(t *testing.T, h *fakeHost) *Router {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "configs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "configs", "tools.json"), []byte(hubManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewRouter(RouterConfig{RootDir: root, Toolchain: toolchain.NewWithHost("darwin", h.lookPath, h.run)})
}

func hubTools(t *testing.T, r *Router) map[string]map[string]interface{} {
	t.Helper()
	w, _ := tcDo(t, r, http.MethodGet, "/api/v1/tools", "")
	var list []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || w.Code != http.StatusOK {
		t.Fatalf("GET /tools = %d %s", w.Code, w.Body.String())
	}
	out := map[string]map[string]interface{}{}
	for _, it := range list {
		out[it["id"].(string)] = it
	}
	return out
}

func TestToolHubReportsHealthCheckResults(t *testing.T) {
	h := &fakeHost{bins: map[string]string{"alpha": "alpha 2.3.4 (build 9)", "gamma": "!fail", "node": "v20.11.1"}}
	r := hubRouter(t, h)

	got := hubTools(t, r)
	if len(got) != 5 {
		t.Fatalf("the list comes from the manifest: %v", got)
	}
	if a := got["alpha"]; a["status"] != "INSTALLED" || a["installed_version"] != "2.3.4" {
		t.Fatalf("alpha = %v", a)
	}
	if b := got["beta"]; b["status"] != "NOT_INSTALLED" || b["installed_version"] != nil {
		t.Fatalf("beta = %v", b)
	}
	if g := got["gamma"]; g["status"] != "ERROR" || !strings.Contains(g["problem"].(string), "failed") {
		t.Fatalf("gamma = %v", g)
	}
	// argv, not a shell line: the binary resolved on PATH, then its own arguments.
	found := false
	for _, run := range h.runs {
		found = found || run == "/fake/bin/alpha --version"
	}
	if !found {
		t.Fatalf("health check runs = %v", h.runs)
	}

	n := h.runCount()
	hubTools(t, r)
	if h.runCount() != n {
		t.Fatal("a second listing within the cache window re-ran the health checks")
	}
	if w, _ := tcDo(t, r, http.MethodPost, "/api/v1/tools/recheck", ""); w.Code != http.StatusOK || h.runCount() == n {
		t.Fatalf("recheck = %d, runs %d → %d", w.Code, n, h.runCount())
	}

	w, _ := tcDo(t, r, http.MethodGet, "/api/v1/tools?category=Agents", "")
	var only []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &only)
	if len(only) != 1 || only[0]["id"] != "alpha" {
		t.Fatalf("category filter = %v", only)
	}
}

func TestToolHubInstallWithoutInstallerReturnsInstructions(t *testing.T) {
	r := hubRouter(t, &fakeHost{bins: map[string]string{"brew": "Homebrew 4.0.0"}})

	w, out := tcDo(t, r, http.MethodPost, "/api/v1/tools/install", `{"tool_id":"alpha"}`)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("install alpha = %d %v", w.Code, out)
	}
	inst := out["install"].(map[string]interface{})
	if inst["docs"] != "https://example.invalid/alpha" || inst["commands"].([]interface{})[0] != "npm install -g alpha" {
		t.Fatalf("instructions = %v", inst)
	}

	// Docker's catalog entry only has a Linux method; on macOS the operator installs it.
	w, out = tcDo(t, r, http.MethodPost, "/api/v1/tools/install", `{"tool_id":"docker"}`)
	if w.Code != http.StatusNotImplemented || out["install"].(map[string]interface{})["docs"] == "" {
		t.Fatalf("install docker = %d %v", w.Code, out)
	}

	for _, v := range []string{"../../etc", "1.0/../../x", "a/b", ".."} {
		body, _ := json.Marshal(map[string]string{"tool_id": "node", "version": v})
		if w, _ := tcDo(t, r, http.MethodPost, "/api/v1/tools/install", string(body)); w.Code != http.StatusBadRequest {
			t.Fatalf("version %q = %d", v, w.Code)
		}
	}
	if w, _ := tcDo(t, r, http.MethodPost, "/api/v1/tools/install", `{"tool_id":"nope"}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown tool = %d", w.Code)
	}
	if w, _ := tcDo(t, r, http.MethodPost, "/api/v1/tools/rollback", `{"tool_id":"alpha","target_version":"1.0"}`); w.Code != http.StatusNotImplemented {
		t.Fatalf("rollback = %d", w.Code)
	}
}

// A tool in the toolchain catalog installs through the toolchain's job. The package manager on
// PATH is a fake script, so nothing is installed on this machine.
func TestToolHubInstallDelegatesToToolchain(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "installed")
	script := "#!/bin/sh\necho \"fake brew $*\"\n/usr/bin/touch " + marker + "\n"
	if err := os.WriteFile(filepath.Join(bin, "brew"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	h := &fakeHost{bins: map[string]string{"brew": "Homebrew 4.0.0"}, marker: marker}
	r := hubRouter(t, h)

	if got := hubTools(t, r)["node"]; got["status"] != "NOT_INSTALLED" {
		t.Fatalf("node before install = %v", got)
	}
	w, out := tcDo(t, r, http.MethodPost, "/api/v1/tools/install", `{"tool_id":"node"}`)
	if w.Code != http.StatusAccepted || out["command"] != "brew install node" {
		t.Fatalf("install node = %d %v", w.Code, out)
	}
	id := out["id"].(string)
	var job *toolchain.Job
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if job = r.toolchain.Installer.Job(id); job.State != toolchain.JobRunning {
			break
		}
	}
	if job.State != toolchain.JobSucceeded || !strings.Contains(strings.Join(job.Log, "\n"), "fake brew install node") {
		t.Fatalf("job = %+v", job)
	}
	got := hubTools(t, r)["node"]
	if got["status"] != "INSTALLED" || got["installed_version"] != "20.11.1" || got["job"] == nil {
		t.Fatalf("node after install = %v", got)
	}
}

func TestToolHubMatrixChecksRuntimesOnThisHost(t *testing.T) {
	r := hubRouter(t, &fakeHost{bins: map[string]string{"alpha": "2.0.0", "node": "v20.11.1"}})

	w, out := tcDo(t, r, http.MethodPost, "/api/v1/tools/resolve-matrix", "")
	if w.Code != http.StatusOK || out["host_os"] != runtime.GOOS || out["host_arch"] != runtime.GOARCH {
		t.Fatalf("matrix = %d %v", w.Code, out)
	}
	if out["matrix_status"] != "conflicts" {
		t.Fatalf("beta needs node >=22, but 20.11.1 is installed: %v", out)
	}
	byID := map[string]map[string]interface{}{}
	for _, it := range out["tools"].([]interface{}) {
		m := it.(map[string]interface{})
		byID[m["tool_id"].(string)] = m
	}
	alpha := byID["alpha"]["runtimes"].([]interface{})[0].(map[string]interface{})
	if byID["alpha"]["compatible"] != true || alpha["found"] != "20.11.1" || alpha["satisfied"] != true {
		t.Fatalf("alpha = %v", byID["alpha"])
	}
	if byID["beta"]["compatible"] != false {
		t.Fatalf("beta = %v", byID["beta"])
	}
}
