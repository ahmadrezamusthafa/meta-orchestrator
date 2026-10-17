package api

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// docStub answers every stage turn with a fixed stage output.
type docStub struct {
	stubProvider
	content string
}

func (s *docStub) StreamActivity(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Content: s.content, Provider: "anthropic", Model: "claude-x", FinishReason: "stop"}, nil
}

func runStageWith(t *testing.T, stage, content string) (*Router, string) {
	t.Helper()
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, &docStub{content: content})
	}
	r.mu.Lock()
	r.tasks["TASK-D1"] = &types.Task{ID: "TASK-D1", Title: "Invoice refunds", CurrentStageID: stage, State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"}}
	r.mu.Unlock()
	if w := do(r, "POST", "/api/v1/tasks/TASK-D1/execute", ""); w.Code != 200 {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-D1", types.TaskStateWaitingGateApproval)
	return r, consoleText(r, "TASK-D1")
}

func TestStageDocumentCheckWarnsOnIncompletePRD(t *testing.T) {
	r, log := runStageWith(t, "prd_discovery", "## Goals\n- Finance can refund a paid invoice.\n\n## Summary\nDone.")
	for _, want := range []string{"⚠ The prd_discovery document may be incomplete", "Non-goals", "Acceptance criteria", "Open questions"} {
		if !strings.Contains(log, want) {
			t.Fatalf("console is missing %q:\n%s", want, log)
		}
	}
	if got := r.taskSnapshot("TASK-D1").Metadata["doc_check_prd_discovery"]; got != "5 issues" {
		t.Fatalf("doc_check = %q", got)
	}
	if st := r.taskSnapshot("TASK-D1").State; st != types.TaskStateWaitingGateApproval {
		t.Fatalf("the check is advisory, state = %s", st)
	}
}

func TestStageDocumentCheckPassesAndFramesTheTechnicalDesign(t *testing.T) {
	rfc := "## Approach\nAdd a refund service.\n\n## Components\n- RefundService\n\n## Data / API changes\n- POST /api/v1/invoices/{id}/refunds\n\n" +
		"## Risks\n- Double refunds: idempotency key.\n\n## Alternatives considered\n- Gateway dashboard only: no audit trail.\n\n## Summary\nDesign ready."
	r, log := runStageWith(t, "techdoc_rfc", rfc)
	if strings.Contains(log, "may be incomplete") {
		t.Fatalf("a complete design must not warn:\n%s", log)
	}
	if got := r.taskSnapshot("TASK-D1").Metadata["doc_check_techdoc_rfc"]; got != "ok" {
		t.Fatalf("doc_check = %q", got)
	}
	path, _ := r.resolveArtifactPath("TASK-D1", stageDocName("techdoc_rfc"))
	raw, _ := os.ReadFile(path)
	doc := string(raw)
	if !strings.Contains(doc, "# Technical design: Invoice refunds\n\n> Task TASK-D1 · stage techdoc_rfc · repositories backend-core") ||
		!strings.Contains(doc, rfc) || strings.Contains(doc, "mermaid") {
		t.Fatalf("document:\n%s", doc)
	}

	// The artifact registry lists it.
	var list struct{ Artifacts []artifactEntry }
	_ = json.Unmarshal(do(r, "GET", "/api/v1/tasks/TASK-D1/artifacts", "").Body.Bytes(), &list)
	if len(list.Artifacts) != 1 || list.Artifacts[0].StageID != "techdoc_rfc" {
		t.Fatalf("artifacts = %+v", list.Artifacts)
	}
}
