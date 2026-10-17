package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestArtifactsServeOnlyRealFilesInsideTheTaskDir(t *testing.T) {
	r := hermeticRouter(t)
	if w := do(r, http.MethodGet, "/api/v1/artifacts/TASK-1/PRD.md", ""); w.Code != http.StatusNotFound {
		t.Fatalf("a missing artifact must be 404, never synthesized: %d %s", w.Code, w.Body.String())
	}
	for _, bad := range []string{"TASK-1/..%2f..%2fetc%2fpasswd", "..%2fTASK-2/x.md", "TASK-1/..%2f..%2f..%2fgo.mod"} {
		if w := do(r, http.MethodGet, "/api/v1/artifacts/"+bad, ""); w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Fatalf("%s → %d, want rejection", bad, w.Code)
		}
	}
	if _, err := r.resolveArtifactPath("TASK-1", "../../secret"); err != nil {
		t.Fatalf("cleaned relative path should stay inside the task dir: %v", err)
	}
	if p, _ := r.resolveArtifactPath("TASK-1", "../../secret"); !strings.HasPrefix(p, r.artifactDir("TASK-1")) {
		t.Fatalf("escaped the artifact dir: %s", p)
	}
	if _, err := r.resolveArtifactPath("../x", "a.md"); err == nil {
		t.Fatal("task id with path separators must be rejected")
	}
}

func TestFinishedStageIsSavedAsAReviewableDocument(t *testing.T) {
	r := hermeticRouter(t)
	task := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Add refund endpoint"}`).Body.Bytes())

	proc := do(r, http.MethodGet, "/api/v1/tasks/"+task.ID+"/process", "")
	var p types.TaskProcessInfo
	_ = json.Unmarshal(proc.Body.Bytes(), &p)
	if p.Status != "IDLE" || p.ProcessID != 0 || len(p.Logs) != 0 {
		t.Fatalf("a task that never ran must not report a process: %+v", p)
	}

	do(r, http.MethodPost, "/api/v1/tasks/"+task.ID+"/execute", "")
	waitState(t, r, task.ID, types.TaskStateWaitingGateApproval)

	var list struct {
		Artifacts []artifactEntry `json:"artifacts"`
	}
	_ = json.Unmarshal(do(r, http.MethodGet, "/api/v1/tasks/"+task.ID+"/artifacts", "").Body.Bytes(), &list)
	if len(list.Artifacts) != 1 || list.Artifacts[0].Kind != "stage_output" || list.Artifacts[0].StageID != "prd_discovery" {
		t.Fatalf("stage output not saved: %+v", list.Artifacts)
	}
	if w := do(r, http.MethodGet, "/api/v1/artifacts/"+task.ID+"/"+list.Artifacts[0].Path, ""); w.Code != http.StatusOK {
		t.Fatalf("stage document not readable: %d", w.Code)
	}
}

// An agent writes into the artifact directory (evidence/ is handed to it as an absolute path), so a
// symlink planted there must not let the artifacts endpoint read files outside it.
func TestArtifactsRefuseSymlinksLeavingTheTaskDir(t *testing.T) {
	r := hermeticRouter(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("OUTSIDE-CONTENT"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(r.artifactDir("TASK-1"), "evidence")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "leak.md")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"evidence/leak.md", "evidence/linkdir/outside.md"} {
		w := do(r, http.MethodGet, "/api/v1/artifacts/TASK-1/"+p, "")
		if w.Code == http.StatusOK || strings.Contains(w.Body.String(), "OUTSIDE-CONTENT") {
			t.Fatalf("%s followed a symlink out of the artifact dir: %d %s", p, w.Code, w.Body.String())
		}
	}
	// A symlink that stays inside the task's directory is still served.
	if err := os.WriteFile(filepath.Join(dir, "real.md"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.md", filepath.Join(dir, "alias.md")); err != nil {
		t.Fatal(err)
	}
	if w := do(r, http.MethodGet, "/api/v1/artifacts/TASK-1/evidence/alias.md", ""); w.Code != http.StatusOK {
		t.Fatalf("in-dir symlink should be served: %d %s", w.Code, w.Body.String())
	}
}
