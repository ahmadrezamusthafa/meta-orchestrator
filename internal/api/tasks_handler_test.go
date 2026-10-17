package api

import (
	"net/http"
	"strings"
	"sync"
	"testing"

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
