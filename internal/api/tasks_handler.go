package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

type CreateTaskRequest struct {
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	WorkflowID       string            `json:"workflow_id"`
	AssignedRepos    []string          `json:"assigned_repos"`
	Dependencies     []string          `json:"dependencies,omitempty"`
	SelectedMethod   string            `json:"selected_method"`
	RouterStrategy   string            `json:"router_strategy"`
	ActiveSlice      *types.StageSlice `json:"active_slice,omitempty"`
	SourceBranch     string            `json:"source_branch,omitempty"`
	ExternalTaskPlan string            `json:"external_task_plan,omitempty"`
	ExternalPRD      string            `json:"external_prd,omitempty"`
	MaxTokenBudget   int64             `json:"max_token_budget,omitempty"`
	Complexity       string            `json:"complexity,omitempty"`
	UseWorktree      bool              `json:"use_worktree,omitempty"`
	DisableWorktree  bool              `json:"disable_worktree,omitempty"` // work directly in the source checkout
}

type InjectContextRequest struct {
	Instruction string `json:"instruction"`
}

type GateApprovalRequest struct {
	Approved bool   `json:"approved"`
	Feedback string `json:"feedback,omitempty"`
}

func cloneTask(t *types.Task) *types.Task {
	if t == nil {
		return nil
	}
	cp := *t
	if t.Metadata != nil {
		cp.Metadata = make(map[string]string, len(t.Metadata))
		for k, v := range t.Metadata {
			cp.Metadata[k] = v
		}
	}
	if t.Dependencies != nil {
		cp.Dependencies = append([]string(nil), t.Dependencies...)
	}
	if t.AssignedRepos != nil {
		cp.AssignedRepos = append([]string(nil), t.AssignedRepos...)
	}
	return &cp
}

func (r *Router) handleTasks(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.mu.RLock()
		defer r.mu.RUnlock()

		search := strings.ToLower(req.URL.Query().Get("search"))
		method := req.URL.Query().Get("method")
		repo := req.URL.Query().Get("repo")

		var result []*types.Task
		for _, t := range r.tasks {
			if search != "" && !strings.Contains(strings.ToLower(t.Title), search) && !strings.Contains(strings.ToLower(t.ID), search) {
				continue
			}
			if method != "" && method != "All" && t.SelectedMethod != method {
				continue
			}
			if repo != "" && repo != "All" {
				matched := false
				for _, rName := range t.AssignedRepos {
					if rName == repo {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			result = append(result, t)
		}
		if result == nil {
			result = []*types.Task{}
		}
		sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
		r.writeJSON(w, http.StatusOK, result)

	case http.MethodPost:
		var body CreateTaskRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid request JSON payload")
			return
		}

		if body.Title == "" {
			r.writeError(w, http.StatusBadRequest, "Task title is required")
			return
		}

		r.mu.Lock()
		taskID := r.nextTaskIDLocked()

		now := time.Now()
		startStage := "prd_discovery"
		startIndex := 0

		// Handle Mid-Process Slicing
		if body.ActiveSlice != nil && body.ActiveSlice.StartStageID != "" {
			startStage = body.ActiveSlice.StartStageID
			// Set stage index based on starting stage
			switch startStage {
			case "atdd_creation":
				startIndex = 1
			case "techdoc_rfc":
				startIndex = 2
			case "task_breakdown":
				startIndex = 3
			case "task_implementation":
				startIndex = 4
			case "e2e_validation":
				startIndex = 5
			case "uat_verification":
				startIndex = 6
			case "signoff_merge":
				startIndex = 7
			}
		}

		selectedMethod := body.SelectedMethod
		if selectedMethod == "" || selectedMethod == "Auto" {
			selectedMethod = "BMAD"
		}

		workflowID := body.WorkflowID
		if workflowID == "" {
			workflowID = "general_ai_sdlc"
		}

		maxBudget := body.MaxTokenBudget
		if maxBudget == 0 {
			maxBudget = 50000
		}

		initialState := types.TaskStatePending
		var unmetDeps []string
		if len(body.Dependencies) > 0 {
			for _, depID := range body.Dependencies {
				dep, exists := r.tasks[depID]
				if !exists || dep.State != types.TaskStateCompleted {
					unmetDeps = append(unmetDeps, depID)
				}
			}
			if len(unmetDeps) > 0 {
				initialState = types.TaskStateWaitingDependency
			}
		}

		newTask := &types.Task{
			ID:                taskID,
			WorkflowID:        workflowID,
			Title:             body.Title,
			Description:       body.Description,
			CurrentStageID:    startStage,
			CurrentStageIndex: startIndex,
			State:             initialState,
			ActiveSlice:       body.ActiveSlice,
			AssignedRepos:     append([]string{}, body.AssignedRepos...),
			Dependencies:      body.Dependencies,
			ProfileName:       "orchestrator_agent",
			SelectedMethod:    selectedMethod,
			MaxTokenBudget:    maxBudget,
			ArtifactDir:       fmt.Sprintf(".sdlc/artifacts/%s", taskID),
			Metadata: map[string]string{
				"complexity":       body.Complexity,
				"router_strategy":  body.RouterStrategy,
				"source_branch":    body.SourceBranch,
				"router_source":    "BP",
				"router_rationale": fmt.Sprintf("Routed via %s to %s method", body.RouterStrategy, selectedMethod),
				"worktree_enabled": fmt.Sprint(body.UseWorktree || !body.DisableWorktree),
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		if len(unmetDeps) > 0 {
			newTask.Metadata["unmet_dependencies"] = strings.Join(unmetDeps, ",")
		}

		// Automatically detect JIRA key in title or description if connector is active
		if r.cfg.ConnectorsManager != nil && r.cfg.ConnectorsManager.GetConfig().Jira.AutoDetectKeys {
			if detectedKey := r.cfg.ConnectorsManager.DetectJiraKey(body.Title + " " + body.Description); detectedKey != "" {
				newTask.Metadata["jira_key"] = detectedKey
				newTask.Metadata["jira_url"] = r.cfg.ConnectorsManager.GetJiraURL(detectedKey)
			}
		}

		newTask.Metadata["worktree_branch"] = plannedBranch(newTask)

		if body.ActiveSlice != nil && body.ActiveSlice.ProduceVideo {
			newTask.Metadata["video_url"] = fmt.Sprintf("/api/v1/artifacts/%s/videos/run_final.mp4", taskID)
		}

		taskArtifactDir := filepath.Join(r.cfg.RootDir, ".sdlc", "artifacts", taskID)
		_ = os.MkdirAll(taskArtifactDir, 0755)

		if body.ExternalTaskPlan != "" {
			_ = os.WriteFile(filepath.Join(taskArtifactDir, "TASK_PLAN.md"), []byte(body.ExternalTaskPlan), 0644)
		}
		if body.ExternalPRD != "" {
			_ = os.WriteFile(filepath.Join(taskArtifactDir, "PRD.md"), []byte(body.ExternalPRD), 0644)
		}

		r.tasks[taskID] = newTask
		createdCopy := cloneTask(newTask)
		r.mu.Unlock()
		r.saveBoardNow()

		if len(unmetDeps) > 0 {
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
				"Task created at %s. Waiting for %s to complete — it starts automatically once they are done.", startStage, strings.Join(unmetDeps, ", "))})
		} else {
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
				"Task created at %s. Run the stage to start the agent, or ask it a question below.", startStage)})
		}

		// Broadcast new task event over WebSocket
		if r.cfg.WSHub != nil {
			r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{
				Type:      types.EventTaskStatus,
				TaskID:    taskID,
				StageID:   startStage,
				Timestamp: now,
				Payload:   createdCopy,
			})
		}

		r.writeJSON(w, http.StatusCreated, createdCopy)

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (r *Router) handleTaskItem(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/tasks/")
	parts := strings.Split(path, "/")
	taskID := parts[0]

	if taskID == "" {
		r.writeError(w, http.StatusBadRequest, "Missing task ID")
		return
	}

	r.mu.RLock()
	task, exists := r.tasks[taskID]
	var taskSnapshot *types.Task
	if exists {
		taskSnapshot = cloneTask(task)
	}
	r.mu.RUnlock()

	if !exists {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("Task %s not found", taskID))
		return
	}

	// Route sub-actions: /api/v1/tasks/{id}/inject, /api/v1/tasks/{id}/reset, /api/v1/tasks/{id}/gate, /api/v1/tasks/{id}/process, /api/v1/tasks/{id}/execute
	if len(parts) > 1 {
		subAction := parts[1]
		switch subAction {
		case "activity", "chat":
			r.handleTaskConsole(w, req, taskSnapshot, parts)
			return
		case "execute", "resume":
			if req.Method != http.MethodPost {
				r.writeError(w, http.StatusMethodNotAllowed, "POST required for "+subAction)
				return
			}
			trigger := "run"
			if subAction == "resume" {
				trigger = "resume"
			}
			r.respondStageStart(w, taskID, r.startStage(taskID, trigger, ""))
			return

		case "pause":
			if req.Method != http.MethodPost {
				r.writeError(w, http.StatusMethodNotAllowed, "POST required for pause")
				return
			}
			snap, err := r.pauseTask(taskID)
			if err != nil {
				r.writeError(w, http.StatusConflict, err.Error())
				return
			}
			r.writeJSON(w, http.StatusOK, map[string]interface{}{"status": "paused", "task_id": taskID, "task": snap})
			return

		case "process":
			if req.Method != http.MethodGet {
				r.writeError(w, http.StatusMethodNotAllowed, "GET required for process")
				return
			}
			r.mu.Lock()
			proc := r.getOrCreateTaskProcessLocked(taskID)
			if proc.Status == "RUNNING" {
				proc.DurationSeconds = int64(time.Since(proc.StartedAt).Seconds())
			}
			snapshot := *proc // encode a copy: the executor mutates proc concurrently
			snapshot.Logs = append([]string(nil), proc.Logs...)
			snapshot.SubProcesses = append([]types.TaskSubProcess(nil), proc.SubProcesses...)
			r.mu.Unlock()
			r.writeJSON(w, http.StatusOK, snapshot)
			return

		case "terminal", "logs":
			if req.Method != http.MethodGet {
				r.writeError(w, http.StatusMethodNotAllowed, "GET required for terminal logs")
				return
			}
			r.mu.Lock()
			logs := append([]string(nil), r.getOrCreateTaskProcessLocked(taskID).Logs...)
			r.mu.Unlock()
			r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": taskID, "logs": logs})
			return

		case "inject":
			if req.Method != http.MethodPost {
				r.writeError(w, http.StatusMethodNotAllowed, "POST required for inject")
				return
			}
			var body InjectContextRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.Instruction) == "" {
				r.writeError(w, http.StatusBadRequest, "Valid instruction string is required")
				return
			}
			instruction := strings.TrimSpace(body.Instruction)
			r.mu.Lock()
			if task.Metadata == nil {
				task.Metadata = map[string]string{}
			}
			if g := task.Metadata["operator_guidance"]; g != "" {
				instruction = g + "\n" + instruction
			}
			task.Metadata["operator_guidance"] = instruction
			blocked := task.State == types.TaskStateBlockedFrustration
			r.mu.Unlock()
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindUser, Content: "Guidance: " + strings.TrimSpace(body.Instruction)})
			if blocked {
				// A frustration-halted task restarts immediately with the new guidance.
				r.respondStageStart(w, taskID, r.startStage(taskID, "guidance", ""))
				return
			}
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: "Guidance saved — the agent will follow it on the next stage run."})
			r.writeJSON(w, http.StatusOK, map[string]interface{}{"status": "injected", "task_id": taskID, "task": r.taskSnapshot(taskID)})
			return

		case "reset":
			if req.Method != http.MethodPost {
				r.writeError(w, http.StatusMethodNotAllowed, "POST required for reset")
				return
			}
			r.mu.Lock()
			if task.Metadata == nil {
				task.Metadata = map[string]string{}
			}
			task.Metadata["frustration_count"] = "0"
			delete(task.Metadata, "failing_trace")
			delete(task.Metadata, "last_error")
			r.mu.Unlock()
			r.interruptAndWait(taskID, 5*time.Second) // reset restarts the stage even if a turn is live
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: "Failure counters cleared. Re-running " + taskSnapshot.CurrentStageID + " from a fresh attempt."})
			r.respondStageStart(w, taskID, r.startStage(taskID, "reset", ""))
			return

		case "gate":
			if req.Method != http.MethodPost {
				r.writeError(w, http.StatusMethodNotAllowed, "POST required for gate")
				return
			}
			var body GateApprovalRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				r.writeError(w, http.StatusBadRequest, "Invalid gate approval payload")
				return
			}
			snap, err := r.reviewStage(taskID, body.Approved, body.Feedback)
			if err != nil && snap == nil {
				r.writeError(w, http.StatusConflict, err.Error())
				return
			}
			r.writeJSON(w, http.StatusOK, map[string]interface{}{"status": "gate_updated", "approved": body.Approved, "task": snap})
			return

		case "worktree":
			r.handleTaskWorktree(w, req, taskSnapshot)
			return

		case "diff":
			r.handleTaskDiff(w, req, taskSnapshot)
			return

		case "artifacts":
			r.handleTaskArtifacts(w, req, taskID)
			return

		case "approvals":
			r.handleTaskApprovals(w, req, taskID, parts)
			return

		case "dependencies":
			if req.Method != http.MethodGet {
				r.writeError(w, http.StatusMethodNotAllowed, "GET required for dependencies")
				return
			}
			r.mu.Lock()
			var unmet []string
			type depDetail struct {
				ID             string          `json:"id"`
				Title          string          `json:"title"`
				State          types.TaskState `json:"state"`
				CurrentStageID string          `json:"current_stage_id"`
			}
			var details []depDetail
			for _, depID := range task.Dependencies {
				dep, exists := r.tasks[depID]
				if exists {
					details = append(details, depDetail{
						ID:             dep.ID,
						Title:          dep.Title,
						State:          dep.State,
						CurrentStageID: dep.CurrentStageID,
					})
					if dep.State != types.TaskStateCompleted {
						unmet = append(unmet, depID)
					}
				} else {
					unmet = append(unmet, depID)
				}
			}
			r.mu.Unlock()

			r.writeJSON(w, http.StatusOK, map[string]interface{}{
				"task_id":            taskID,
				"dependencies":       task.Dependencies,
				"all_satisfied":      len(unmet) == 0,
				"unmet_dependencies": unmet,
				"details":            details,
			})
			return

		default:
			r.writeError(w, http.StatusNotFound, "Unknown sub-action")
			return
		}
	}

	switch req.Method {
	case http.MethodGet:
		r.writeJSON(w, http.StatusOK, taskSnapshot)
	case http.MethodDelete:
		removed, err := r.deleteTask(taskID)
		if err != nil {
			r.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, map[string]interface{}{"status": "deleted", "task_id": taskID, "task": removed})
	case http.MethodPatch, http.MethodPut:
		var body struct {
			CurrentStageID string          `json:"current_stage_id,omitempty"`
			Stage          string          `json:"stage,omitempty"`
			State          types.TaskState `json:"state,omitempty"`
			AssignedRepos  *[]string       `json:"assigned_repos,omitempty"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		if body.AssignedRepos != nil {
			if err := r.assignRepos(taskID, *body.AssignedRepos); err != nil {
				r.writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if body.CurrentStageID == "" && body.Stage == "" && body.State == "" {
				r.saveBoardNow()
				snap := r.taskSnapshot(taskID)
				r.broadcastTask(snap)
				r.writeJSON(w, http.StatusOK, snap)
				return
			}
		}
		if body.CurrentStageID == "" && body.Stage != "" {
			body.CurrentStageID = body.Stage
		}

		// Moving a card changes its stage; it never pretends work is running. A move (or an explicit
		// RUNNING state) that should execute goes through the stage controller.
		r.mu.Lock()
		if body.CurrentStageID == "task_implementation" || body.CurrentStageID == "e2e_validation" {
			var unmet []string
			for _, depID := range task.Dependencies {
				if dep, ok := r.tasks[depID]; !ok || dep.State != types.TaskStateCompleted {
					unmet = append(unmet, depID)
				}
			}
			if len(unmet) > 0 {
				r.mu.Unlock()
				r.writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot advance task while dependencies are unmet: %s", strings.Join(unmet, ", ")))
				return
			}
		}
		stageChanged := body.CurrentStageID != "" && body.CurrentStageID != task.CurrentStageID
		prevStage := task.CurrentStageID
		if stageChanged {
			task.CurrentStageID = body.CurrentStageID
			if i := stageIndexOf(body.CurrentStageID); i >= 0 {
				task.CurrentStageIndex = i
			}
		}
		wantRun := body.State == types.TaskStateRunning
		switch {
		case body.State == types.TaskStateCompleted && task.State != types.TaskStateCompleted:
			r.setTaskState(task, types.TaskStateCompleted)
			r.onTaskCompleted(task)
		case body.State != "" && body.State != types.TaskStateRunning && body.State != task.State:
			r.setTaskState(task, body.State)
		case stageChanged && task.State != types.TaskStateCompleted && task.State != types.TaskStateRunning &&
			task.State != types.TaskStateWaitingDependency:
			r.setTaskState(task, types.TaskStatePending)
		}
		task.UpdatedAt = time.Now()
		completed := task.State == types.TaskStateCompleted
		r.mu.Unlock()

		if stageChanged {
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
				"Moved from %s to %s by operator. Run the stage to start the agent.", prevStage, body.CurrentStageID)})
		}
		if wantRun {
			if err := r.startStage(taskID, "run", ""); err != nil && !errors.Is(err, errTaskBusy) {
				var blocked *depsBlockedError
				if !errors.As(err, &blocked) {
					r.writeError(w, http.StatusConflict, err.Error())
					return
				}
			}
		}
		if completed {
			r.startUnblockedDependents(taskID)
		}
		r.saveBoardNow()
		taskCopy := r.taskSnapshot(taskID)
		r.broadcastTask(taskCopy)

		r.writeJSON(w, http.StatusOK, taskCopy)
	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
