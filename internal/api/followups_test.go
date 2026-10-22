package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/readiness"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

const observabilityReport = "## Signals added\nStructured logs.\n\n## Follow-up tasks\n```followups\n" + `[
  {"title": "Add distributed tracing to the order flow", "summary": "Requests cannot be followed across services.",
   "repos": ["order-svc", "not-assigned"],
   "items": [
     {"title": "Propagate traceparent through the queue consumer", "detail": "Extract the context\nin consume()", "priority": "HIGH", "evidence": "consumer.go:42"},
     {"title": "Span outgoing DB calls", "priority": "p3"},
     {"title": ""}
   ]},
  {"title": "Alert on the payment journey", "repos": "order-svc", "items": [{"title": "Burn-rate alert for checkout", "priority": "nope"}]},
  {"title": "   "}
]` + "\n```\n\n## Summary\nDone.\n"

func TestParseFollowUpsReadsTheBlock(t *testing.T) {
	got, ok := parseFollowUps("observability_alerting", observabilityReport)
	if !ok || len(got) != 2 {
		t.Fatalf("parsed %d proposals (ok %v), want 2", len(got), ok)
	}
	p := got[0]
	if p.StageName != "Watch the System & Send Alerts" || len(p.Items) != 2 || p.Items[0].Priority != "high" || p.Items[1].Priority != "low" {
		t.Fatalf("first proposal = %+v", p)
	}
	if strings.Join(got[1].Repos, ",") != "order-svc" || got[1].Items[0].Priority != "medium" {
		t.Fatalf("second proposal = %+v", got[1])
	}
	again, _ := parseFollowUps("observability_alerting", observabilityReport)
	if again[0].ID != p.ID || again[0].Items[0].ID != p.Items[0].ID || p.ID == got[1].ID {
		t.Fatal("ids must be stable across reads and distinct between proposals")
	}

	for name, report := range map[string]string{
		"no block":     "## Summary\nnothing",
		"not an array": "```followups\n{\"title\": \"x\"}\n```",
		"broken json":  "```followups\n[{\"title\": \n```",
	} {
		if list, ok := parseFollowUps("load_resilience", report); ok || len(list) != 0 {
			t.Errorf("%s: parsed %v (ok %v)", name, list, ok)
		}
	}
	if list, ok := parseFollowUps("load_resilience", "```json followups\n[]\n```"); !ok || len(list) != 0 {
		t.Fatalf("an empty list is a listed report with no proposals: %v %v", list, ok)
	}
	dup := "```followups\n[{\"title\":\"Same\"},{\"title\":\"same\"}]\n```"
	if list, _ := parseFollowUps("load_resilience", dup); len(list) != 2 || list[0].ID == list[1].ID {
		t.Fatalf("duplicate titles must get distinct ids: %+v", list)
	}
}

func TestReadinessBriefAsksForFollowUps(t *testing.T) {
	for _, s := range readiness.Steps {
		if !strings.Contains(stageInstructions[s.ID], "```"+readiness.FollowUpFence) || !strings.Contains(stageInstructions[s.ID], s.FollowUps) {
			t.Errorf("%s: the stage brief does not ask for its follow-up tasks", s.ID)
		}
	}
	// The example in the brief is itself a valid block.
	if list, ok := parseFollowUps("load_resilience", readiness.FollowUpBrief("x")); !ok || len(list) != 1 || len(list[0].Items) != 1 {
		t.Fatalf("the brief's example does not parse: %v %v", list, ok)
	}
}

type followUpsResp struct {
	Proposals []followUpProposal `json:"proposals"`
	Stages    []followUpStage    `json:"stages"`
}

func listFollowUps(t *testing.T, r *Router, id string) followUpsResp {
	t.Helper()
	w := do(r, http.MethodGet, "/api/v1/tasks/"+id+"/followups", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET followups → %d %s", w.Code, w.Body.String())
	}
	var out followUpsResp
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFollowUpsBecomeOneTaskOnTheOperatorsRequest(t *testing.T) {
	r := hermeticRouter(t)
	r.mu.Lock()
	r.tasks["TASK-LR"] = &types.Task{ID: "TASK-LR", Title: "Launch orders", WorkflowID: fsm.LaunchReadinessWorkflowID,
		CurrentStageID: "observability_alerting", State: types.TaskStateRunning, AssignedRepos: []string{"order-svc", "web"},
		ArtifactDir: ".sdlc/artifacts/TASK-LR", Metadata: map[string]string{}}
	r.mu.Unlock()

	taskCount := func() int {
		r.mu.RLock()
		defer r.mu.RUnlock()
		return len(r.tasks)
	}
	before := taskCount()
	r.saveStageDocument("TASK-LR", "observability_alerting", "test-model", observabilityReport)
	r.saveStageDocument("TASK-LR", "release_audit", "test-model", "## Summary\nAn older report without follow-ups.")
	if taskCount() != before {
		t.Fatal("saving a report must never create tasks by itself")
	}
	got := listFollowUps(t, r, "TASK-LR")
	if len(got.Proposals) != 2 || len(got.Stages) != len(readiness.Steps) {
		t.Fatalf("proposals = %d, stages = %d", len(got.Proposals), len(got.Stages))
	}
	if s := got.Stages[0]; s.ID != "release_audit" || !s.HasReport || s.Listed {
		t.Fatalf("release_audit stage = %+v", s)
	}
	if !strings.Contains(consoleText(r, "TASK-LR"), "proposed 2 follow-up tasks (3 items)") {
		t.Fatal("the operator is not told about the proposals")
	}

	tracing, alerting := got.Proposals[0], got.Proposals[1]
	body := `{"title":"Observability follow-ups","selections":[{"proposal_id":"` + tracing.ID + `","item_ids":["` + tracing.Items[0].ID +
		`"]},{"proposal_id":"` + alerting.ID + `"}]}`
	w := do(r, http.MethodPost, "/api/v1/tasks/TASK-LR/followups", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST followups → %d %s", w.Code, w.Body.String())
	}
	var created types.Task
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.WorkflowID != fsm.DefaultWorkflowID || created.Title != "Observability follow-ups" || created.Metadata["followup_of"] != "TASK-LR" {
		t.Fatalf("created = %+v", created)
	}
	if strings.Join(created.AssignedRepos, ",") != "order-svc" {
		t.Fatalf("repos = %v, want only the named repository the source task has", created.AssignedRepos)
	}
	for _, want := range []string{"Propagate traceparent", "Extract the context in consume()", "(evidence: consumer.go:42)", "Burn-rate alert",
		".sdlc/artifacts/TASK-LR/stages/observability_alerting.md"} {
		if !strings.Contains(created.Description, want) {
			t.Errorf("description lacks %q:\n%s", want, created.Description)
		}
	}
	if strings.Contains(created.Description, "Span outgoing DB calls") {
		t.Error("an unselected item was included")
	}

	got = listFollowUps(t, r, "TASK-LR")
	if got.Proposals[0].CreatedTaskID != created.ID || got.Proposals[1].CreatedTaskID != created.ID {
		t.Fatalf("links = %q, %q", got.Proposals[0].CreatedTaskID, got.Proposals[1].CreatedTaskID)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-LR/followups", `{"selections":[{"proposal_id":"`+alerting.ID+`"}]}`); w.Code != http.StatusConflict {
		t.Fatalf("adding a proposal twice → %d", w.Code)
	}

	// Once the follow-up task is removed from the board, the proposal can be added again.
	if w := do(r, http.MethodDelete, "/api/v1/tasks/"+created.ID, ""); w.Code != http.StatusOK {
		t.Fatalf("delete → %d", w.Code)
	}
	w = do(r, http.MethodPost, "/api/v1/tasks/TASK-LR/followups", `{"selections":[{"proposal_id":"`+alerting.ID+`"}]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("re-adding after delete → %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.Title != "Alert on the payment journey" {
		t.Fatalf("default title = %q", created.Title)
	}
}

func TestFollowUpRequestsAreValidated(t *testing.T) {
	r := hermeticRouter(t)
	r.mu.Lock()
	r.tasks["TASK-LV"] = &types.Task{ID: "TASK-LV", Title: "Launch", WorkflowID: fsm.LaunchReadinessWorkflowID,
		CurrentStageID: "observability_alerting", State: types.TaskStateWaitingGateApproval, Metadata: map[string]string{}}
	r.mu.Unlock()
	r.saveStageDocument("TASK-LV", "observability_alerting", "m", observabilityReport)
	p := listFollowUps(t, r, "TASK-LV").Proposals[0]

	for body, want := range map[string]int{
		`not json`:          http.StatusBadRequest,
		`{"selections":[]}`: http.StatusBadRequest,
		`{"selections":[{"proposal_id":"observability_alerting:gone"}]}`:             http.StatusNotFound,
		`{"selections":[{"proposal_id":"` + p.ID + `","item_ids":["not-an-item"]}]}`: http.StatusBadRequest,
	} {
		if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-LV/followups", body); w.Code != want {
			t.Errorf("%s → %d, want %d", body, w.Code, want)
		}
	}
	if w := do(r, http.MethodPut, "/api/v1/tasks/TASK-LV/followups", "{}"); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT → %d", w.Code)
	}
	// A task without readiness stages has nothing to propose.
	r.mu.Lock()
	r.tasks["TASK-GEN"] = &types.Task{ID: "TASK-GEN", Title: "Plain", State: types.TaskStatePending, Metadata: map[string]string{}}
	r.mu.Unlock()
	if got := listFollowUps(t, r, "TASK-GEN"); len(got.Proposals) != 0 || len(got.Stages) != 0 {
		t.Fatalf("general task follow-ups = %+v", got)
	}
}

// Concurrent clicks on the same proposal add it to the board once.
func TestFollowUpIsAddedOnceUnderConcurrentRequests(t *testing.T) {
	r := hermeticRouter(t)
	r.mu.Lock()
	r.tasks["TASK-LC"] = &types.Task{ID: "TASK-LC", Title: "Launch", WorkflowID: fsm.LaunchReadinessWorkflowID,
		CurrentStageID: "observability_alerting", State: types.TaskStateWaitingGateApproval, Metadata: map[string]string{}}
	r.mu.Unlock()
	r.saveStageDocument("TASK-LC", "observability_alerting", "m", observabilityReport)
	p := listFollowUps(t, r, "TASK-LC").Proposals[0]

	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = do(r, http.MethodPost, "/api/v1/tasks/TASK-LC/followups", `{"selections":[{"proposal_id":"`+p.ID+`"}]}`).Code
		}(i)
	}
	wg.Wait()
	createdN := 0
	for _, c := range codes {
		if c == http.StatusCreated {
			createdN++
		} else if c != http.StatusConflict {
			t.Fatalf("unexpected status %d", c)
		}
	}
	if createdN != 1 {
		t.Fatalf("created %d tasks, want 1", createdN)
	}
}
