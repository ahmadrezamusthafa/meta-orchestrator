package api

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Responses must be encoded from copies: deleteTask rewrites a dependent's Dependencies in place
// under r.mu, so encoding the live slice after unlocking races with it.
func TestDependenciesAndListResponsesDoNotRaceWithDeletes(t *testing.T) {
	r := hermeticRouter(t)
	r.mu.Lock()
	for _, id := range []string{"TASK-D1", "TASK-D2", "TASK-D3"} {
		r.tasks[id] = &types.Task{ID: id, Title: id, State: types.TaskStateCompleted, Metadata: map[string]string{}}
	}
	r.tasks["TASK-W"] = &types.Task{ID: "TASK-W", Title: "w", State: types.TaskStateWaitingDependency,
		Dependencies: []string{"TASK-D1", "TASK-D2", "TASK-D3"}, Metadata: map[string]string{}}
	r.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			do(r, http.MethodGet, "/api/v1/tasks/TASK-W/dependencies", "")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			do(r, http.MethodGet, "/api/v1/tasks", "")
		}
	}()
	go func() {
		defer wg.Done()
		for _, id := range []string{"TASK-D1", "TASK-D2"} {
			do(r, http.MethodDelete, "/api/v1/tasks/"+id, "")
		}
	}()
	wg.Wait()
	if deps := r.taskSnapshot("TASK-W").Dependencies; len(deps) != 1 || deps[0] != "TASK-D3" {
		t.Fatalf("dependencies after deletes = %v", deps)
	}
}

func TestOversizedTaskBodiesAreRejected(t *testing.T) {
	r := hermeticRouter(t)
	huge := `{"title":"x","description":"` + strings.Repeat("a", maxTaskBodyBytes+1) + `"}`
	if w := do(r, http.MethodPost, "/api/v1/tasks", huge); w.Code != http.StatusBadRequest {
		t.Fatalf("oversized create → %d, want 400", w.Code)
	}
	r.mu.Lock()
	r.tasks["TASK-B"] = &types.Task{ID: "TASK-B", Title: "b", State: types.TaskStatePending, Metadata: map[string]string{}}
	r.mu.Unlock()
	big := `{"instruction":"` + strings.Repeat("a", maxTaskBodyBytes+1) + `"}`
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-B/inject", big); w.Code != http.StatusBadRequest {
		t.Fatalf("oversized inject → %d, want 400", w.Code)
	}
}

func TestPatchRejectsUnknownState(t *testing.T) {
	r := hermeticRouter(t)
	r.mu.Lock()
	r.tasks["TASK-S"] = &types.Task{ID: "TASK-S", Title: "s", State: types.TaskStatePending, Metadata: map[string]string{}}
	r.mu.Unlock()
	if w := do(r, http.MethodPatch, "/api/v1/tasks/TASK-S", `{"state":"BOGUS"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown state → %d, want 400", w.Code)
	}
	if s := r.taskSnapshot("TASK-S"); s.State != types.TaskStatePending {
		t.Fatalf("state changed to %s", s.State)
	}
	if w := do(r, http.MethodPatch, "/api/v1/tasks/TASK-S", `{"state":"SUSPENDED"}`); w.Code != http.StatusOK {
		t.Fatalf("known state → %d %s", w.Code, w.Body.String())
	}
}

// Moving the card while its stage runs would settle the running turn's output as the review of
// the stage it was moved to, so approving would skip that stage.
func TestPatchRefusesStageMoveWhileRunning(t *testing.T) {
	stub := &streamingStub{block: make(chan struct{})}
	r := consoleRouter(t, stub)
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/execute", ""); w.Code != http.StatusOK {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodPatch, "/api/v1/tasks/TASK-C1", `{"current_stage_id":"e2e_validation"}`); w.Code != http.StatusConflict {
		t.Fatalf("stage move while running → %d, want 409", w.Code)
	}
	if s := r.taskSnapshot("TASK-C1"); s.CurrentStageID != "task_implementation" {
		t.Fatalf("stage moved to %s while running", s.CurrentStageID)
	}
	do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/pause", "")
	waitState(t, r, "TASK-C1", types.TaskStateSuspended)
	r.interruptAndWait("TASK-C1", 2*time.Second) // let the turn settle before the temp dir goes
}
