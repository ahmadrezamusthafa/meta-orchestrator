package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestSliceTaskUsesUploadedDocsAsSkippedStageOutputs(t *testing.T) {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	body, _ := json.Marshal(map[string]interface{}{
		"title":        "Add refund endpoint",
		"active_slice": map[string]interface{}{"start_stage_id": "task_implementation", "halt_stage_id": "e2e_validation"},
		"external_docs": map[string]string{
			"PRD.md":       "# PRD\nRefunds within 30 days.",
			"TECH_DOC.md":  "# RFC\nPOST /refunds.",
			"TASK_PLAN.md": "# Plan\n1. Handler.",
			"notes.txt":    "scratch",
		},
	})
	task := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", string(body)).Body.Bytes())
	if task.CurrentStageID != "task_implementation" {
		t.Fatalf("task = %+v", task)
	}

	for stage, want := range map[string]string{"prd_discovery": "Refunds within 30 days.", "techdoc_rfc": "POST /refunds.", "task_breakdown": "1. Handler."} {
		p, _ := r.resolveArtifactPath(task.ID, stageDocName(stage))
		data, err := os.ReadFile(p)
		if err != nil || !strings.Contains(string(data), want) || !strings.Contains(string(data), "operator upload") {
			t.Fatalf("%s document = %q, %v", stage, data, err)
		}
	}
	if got := r.taskSnapshot(task.ID).Metadata[sliceDocsKey]; got != "prd_discovery,techdoc_rfc,task_breakdown" {
		t.Fatalf("metadata = %q", got)
	}
	r.console.mu.Lock()
	var log []string
	for _, e := range r.console.get(task.ID).entries {
		log = append(log, e.Content)
	}
	r.console.mu.Unlock()
	if all := strings.Join(log, "\n"); !strings.Contains(all, "PRD.md → output of") || !strings.Contains(all, "not used: notes.txt") {
		t.Fatalf("console:\n%s", all)
	}

	// The implementation stage's system prompt points the agent at them.
	brief := r.stageContext(r.taskSnapshot(task.ID))
	tech, _ := r.resolveArtifactPath(task.ID, stageDocName("techdoc_rfc"))
	if !strings.Contains(brief, "in place of the stages it skipped") || !strings.Contains(brief, tech) {
		t.Fatalf("brief = %q", brief)
	}
}

func TestFullPipelineTaskIgnoresUploadedStageDocs(t *testing.T) {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	task := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Refunds","external_prd":"# PRD"}`).Body.Bytes())
	p, _ := r.resolveArtifactPath(task.ID, stageDocName("prd_discovery"))
	if fileExists(p) || r.stageContext(&types.Task{ID: task.ID, CurrentStageID: "prd_discovery", Metadata: map[string]string{}}) != "" {
		t.Fatal("a task that runs PRD discovery itself must not get the upload as that stage's output")
	}
}
