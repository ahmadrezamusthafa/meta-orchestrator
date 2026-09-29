package api

import (
	"encoding/json"
	"net/http"
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
