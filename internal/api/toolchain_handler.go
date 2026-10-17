package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// toolchainAckKey stores the missing tools the operator chose to run a task's code stages without.
const toolchainAckKey = "toolchain_ack"

// toolsMissingError blocks a code stage until its required tools are installed or acknowledged.
type toolsMissingError struct{ Missing []string }

func (e *toolsMissingError) Error() string {
	return "this stage needs tools that are not installed on this machine: " + toolchain.Names(e.Missing)
}

// toolchainFor reports the tools the named repositories need (and every catalog tool with all).
func (r *Router) toolchainFor(repos []string, all bool) toolchain.Report {
	return r.toolchain.Report(toolchain.Requirements(r.repoStacks(repos)), all)
}

// toolchainGate stops a code stage whose verification tools are missing, unless the operator
// already chose to continue without exactly those tools.
func (r *Router) toolchainGate(taskID string) error {
	t := r.taskSnapshot(taskID)
	if t == nil || !codeStages[t.CurrentStageID] || len(t.AssignedRepos) == 0 {
		return nil
	}
	rep := r.toolchainFor(t.AssignedRepos, false)
	acked := map[string]bool{}
	for _, id := range strings.Split(t.Metadata[toolchainAckKey], ",") {
		acked[id] = true
	}
	var missing []string
	for _, id := range rep.Missing {
		if !acked[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return &toolsMissingError{Missing: missing}
	}
	return nil
}

// noteMissingTools tells the operator, in the console and through the install prompt, that a stage
// waits for tools. A start nobody clicked (a prerequisite task finishing) does not pop the prompt.
func (r *Router) noteMissingTools(taskID, trigger string, e *toolsMissingError) {
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
		"⚠ Not started: this stage verifies changes with %s, which this machine does not have. Install them from the prompt "+
			"or the Toolchain section of Workspace & Repos (one click where possible), or choose Run without them.",
		toolchain.Names(e.Missing))})
	if r.cfg.WSHub == nil || trigger == "dependency_cleared" {
		return
	}
	t := r.taskSnapshot(taskID)
	stage := ""
	if t != nil {
		stage = t.CurrentStageID
	}
	r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{Type: types.EventToolchainNeeded, TaskID: taskID, StageID: stage,
		Timestamp: time.Now(), Payload: map[string]interface{}{"missing": e.Missing, "trigger": trigger}})
}

// allRepoNames lists the repositories of one project, or of every project when id is empty.
func (r *Router) allRepoNames(projectID string) []string {
	var out []string
	if r.cfg.ProjectManager == nil {
		return out
	}
	seen := map[string]bool{}
	for _, p := range r.cfg.ProjectManager.List() {
		if projectID != "" && p.ID != projectID {
			continue
		}
		for _, repo := range p.Repos {
			if !seen[repo.Name] {
				seen[repo.Name] = true
				out = append(out, repo.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// scopeRepos resolves ?task= or ?project= (default: every registered repository).
func (r *Router) scopeRepos(req *http.Request) ([]string, error) {
	if id := req.URL.Query().Get("task"); id != "" {
		t := r.taskSnapshot(id)
		if t == nil {
			return nil, errTaskNotFound
		}
		return t.AssignedRepos, nil
	}
	return r.allRepoNames(req.URL.Query().Get("project")), nil
}

// handleToolchain: GET /api/v1/toolchain[?task=|?project=][&all=1] reports the tools the scope
// needs (with all, every tool the orchestrator knows) and how to install the missing ones.
func (r *Router) handleToolchain(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	r.handleToolchainReport(w, req)
}

type toolchainInstallRequest struct {
	Tool   string `json:"tool"`
	Method int    `json:"method"`
}

// handleToolchainAction serves POST /toolchain/recheck, POST /toolchain/install and
// GET /toolchain/jobs/{id}.
func (r *Router) handleToolchainAction(w http.ResponseWriter, req *http.Request) {
	path := strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/v1/toolchain/"), "/")
	switch {
	case path == "recheck":
		if req.Method != http.MethodPost {
			r.writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		toolchain.EnsurePath() // pick up tools installed from the operator's terminal
		r.toolchain.Checker.Invalidate()
		r.handleToolchainReport(w, req)

	case path == "install":
		if req.Method != http.MethodPost {
			r.writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		var body toolchainInstallRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Tool == "" {
			r.writeError(w, http.StatusBadRequest, `expected {"tool": "<id>", "method": <index>}`)
			return
		}
		job, err := r.toolchain.Installer.Start(body.Tool, body.Method)
		switch {
		case errors.Is(err, toolchain.ErrUnknownTool):
			r.writeError(w, http.StatusNotFound, fmt.Sprintf("unknown tool %q", body.Tool))
		case errors.Is(err, toolchain.ErrBusy):
			r.writeJSON(w, http.StatusConflict, map[string]interface{}{"error": err.Error(), "job": r.toolchain.Installer.Latest(body.Tool)})
		case err != nil:
			r.writeError(w, http.StatusBadRequest, err.Error())
		default:
			r.writeJSON(w, http.StatusAccepted, job)
		}

	case strings.HasPrefix(path, "jobs/"):
		if req.Method != http.MethodGet {
			r.writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}
		job := r.toolchain.Installer.Job(strings.TrimPrefix(path, "jobs/"))
		if job == nil {
			r.writeError(w, http.StatusNotFound, "no such install job")
			return
		}
		r.writeJSON(w, http.StatusOK, job)

	default:
		r.writeError(w, http.StatusNotFound, "toolchain action not found")
	}
}

func (r *Router) handleToolchainReport(w http.ResponseWriter, req *http.Request) {
	repos, err := r.scopeRepos(req)
	if err != nil {
		r.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	r.writeJSON(w, http.StatusOK, r.toolchainFor(repos, req.URL.Query().Get("all") == "1"))
}

// taskToolchainResponse is a task's toolchain with the tools the operator continued without.
type taskToolchainResponse struct {
	toolchain.Report
	Acknowledged []string `json:"acknowledged"`
}

// handleTaskToolchain: GET /tasks/{id}/toolchain reports the task's tools; POST
// /tasks/{id}/toolchain/ack lets its code stages run without the tools missing right now.
func (r *Router) handleTaskToolchain(w http.ResponseWriter, req *http.Request, t *types.Task, parts []string) {
	rep := r.toolchainFor(t.AssignedRepos, false)
	switch {
	case len(parts) == 2 && req.Method == http.MethodGet:
	case len(parts) == 3 && parts[2] == "ack" && req.Method == http.MethodPost:
		r.mu.Lock()
		if live, ok := r.tasks[t.ID]; ok {
			if live.Metadata == nil {
				live.Metadata = map[string]string{}
			}
			if len(rep.Missing) == 0 {
				delete(live.Metadata, toolchainAckKey)
			} else {
				live.Metadata[toolchainAckKey] = strings.Join(rep.Missing, ",")
			}
		}
		r.mu.Unlock()
		if len(rep.Missing) > 0 {
			r.addEntry(t.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
				"Continuing without %s. The agent skips the checks that need them and lists them in its summary.", toolchain.Names(rep.Missing))})
		}
		t = r.taskSnapshot(t.ID)
	default:
		r.writeError(w, http.StatusNotFound, "use GET /toolchain or POST /toolchain/ack")
		return
	}
	resp := taskToolchainResponse{Report: rep, Acknowledged: []string{}}
	if ack := t.Metadata[toolchainAckKey]; ack != "" {
		resp.Acknowledged = strings.Split(ack, ",")
	}
	r.writeJSON(w, http.StatusOK, resp)
}
