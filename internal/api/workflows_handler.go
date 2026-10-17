package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// workflowsMu serializes workflow changes with the write of .sdlc/workflows.json.
var workflowsMu sync.Mutex

// handleWorkflows lists workflows (GET) and registers a custom one (POST). Custom workflows are
// persisted and select the stages a task runs.
func (r *Router) handleWorkflows(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		list := []types.WorkflowDefinition{}
		for _, def := range r.cfg.WorkflowReg.List() {
			list = append(list, *def)
		}
		r.writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		r.saveWorkflow(w, req, "", false)

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleWorkflowItem serves GET /workflows/stages (the stage IDs a workflow may use) and
// PUT /workflows/{id} (replace a custom workflow).
func (r *Router) handleWorkflowItem(w http.ResponseWriter, req *http.Request) {
	id := strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/v1/workflows/"), "/")
	switch {
	case id == "stages" && req.Method == http.MethodGet:
		r.writeJSON(w, http.StatusOK, map[string]interface{}{"stages": runnableStages()})
	case id != "" && req.Method == http.MethodPut:
		r.saveWorkflow(w, req, id, true)
	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (r *Router) saveWorkflow(w http.ResponseWriter, req *http.Request, pathID string, replace bool) {
	var wf types.WorkflowDefinition
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&wf); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid workflow JSON")
		return
	}
	if pathID != "" {
		if wf.ID != "" && wf.ID != pathID {
			r.writeError(w, http.StatusBadRequest, "Workflow ID in the body does not match the URL")
			return
		}
		wf.ID = pathID
	}
	if wf.ID == "" || wf.Name == "" || len(wf.Stages) == 0 {
		r.writeError(w, http.StatusBadRequest, "Workflow requires ID, Name, and at least 1 Stage")
		return
	}
	if err := validateRunnable(&wf); err != nil {
		r.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	workflowsMu.Lock()
	if replace {
		if _, err := r.cfg.WorkflowReg.Get(wf.ID); err != nil {
			workflowsMu.Unlock()
			r.writeError(w, http.StatusNotFound, "Workflow "+wf.ID+" does not exist")
			return
		}
	}
	err := r.cfg.WorkflowReg.Add(&wf, replace)
	var saveErr error
	if err == nil {
		saveErr = r.saveWorkflows()
	}
	workflowsMu.Unlock()
	switch {
	case errors.Is(err, fsm.ErrWorkflowExists):
		r.writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		r.writeError(w, http.StatusBadRequest, err.Error())
		return
	case saveErr != nil:
		r.writeError(w, http.StatusInternalServerError, "Workflow registered but not saved: "+saveErr.Error())
		return
	}
	status, verb := http.StatusCreated, "created"
	if replace {
		status, verb = http.StatusOK, "updated"
	}
	r.writeJSON(w, status, map[string]interface{}{
		"status":      verb,
		"workflow_id": wf.ID,
		"timestamp":   time.Now(),
	})
}
