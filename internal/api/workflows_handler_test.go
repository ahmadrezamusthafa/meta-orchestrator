package api

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
)

func TestWorkflowRegistrationIsRaceFreeAndRejectsDuplicates(t *testing.T) {
	r := hermeticRouter(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			do(r, http.MethodPost, "/api/v1/workflows", fmt.Sprintf(`{"id":"wf_race_%d","name":"n","stages":[{"id":"task_breakdown"}]}`, i))
		}(i)
		go func() {
			defer wg.Done()
			do(r, http.MethodGet, "/api/v1/workflows", "")
		}()
	}
	wg.Wait()
	if w := do(r, http.MethodPost, "/api/v1/workflows", `{"id":"wf_race_1","name":"again","stages":[{"id":"task_breakdown"}]}`); w.Code != http.StatusConflict {
		t.Fatalf("duplicate workflow id → %d, want 409", w.Code)
	}
}
