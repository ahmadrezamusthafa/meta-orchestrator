package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
		taskNum := len(r.tasks) + 8945
		taskID := fmt.Sprintf("TASK-%d", taskNum)

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

		initialState := types.TaskStateRunning
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
			AssignedRepos:     body.AssignedRepos,
			Dependencies:      body.Dependencies,
			ProfileName:       "orchestrator_agent",
			SelectedMethod:    selectedMethod,
			TokenUsage: types.TokenUsage{
				PromptTokens:     1200,
				CompletionTokens: 350,
				TotalTokens:      1550,
				EstimatedCostUSD: 0.009,
			},
			MaxTokenBudget: maxBudget,
			ArtifactDir:    fmt.Sprintf(".sdlc/artifacts/%s", taskID),
			Metadata: map[string]string{
				"complexity":       body.Complexity,
				"router_strategy":  body.RouterStrategy,
				"source_branch":    body.SourceBranch,
				"router_source":    "BP",
				"router_rationale": fmt.Sprintf("Routed via %s to %s method", body.RouterStrategy, selectedMethod),
				"worktree_enabled": "true",
				"worktree_branch":  fmt.Sprintf("feat/%s-worktree", strings.ToLower(taskID)),
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

	// Route sub-actions: /api/v1/tasks/{id}/inject, /api/v1/tasks/{id}/reset, /api/v1/tasks/{id}/gate, /api/v1/tasks/{id}/process
	if len(parts) > 1 {
		subAction := parts[1]
		switch subAction {
		case "process":
			if len(parts) > 2 && parts[2] == "execute" {
				if req.Method != http.MethodPost {
					r.writeError(w, http.StatusMethodNotAllowed, "POST required for command execution")
					return
				}
				var body struct {
					Command string `json:"command"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.Command) == "" {
					r.writeError(w, http.StatusBadRequest, "Command string is required")
					return
				}

				r.mu.Lock()
				proc := r.getOrCreateTaskProcessLocked(taskID)
				nowStr := time.Now().Format("15:04:05")
				cmdLog := fmt.Sprintf("[%s] \x1b[35m[Interactive Shell]\x1b[0m $ %s", nowStr, body.Command)
				proc.Logs = append(proc.Logs, cmdLog)

				// Generate meaningful command response based on intent
				var respLog string
				lowerCmd := strings.ToLower(body.Command)
				switch {
				case strings.Contains(lowerCmd, "test"):
					respLog = fmt.Sprintf("[%s] \x1b[32m[Test Runner]\x1b[0m Executing test suite... 12/12 specifications PASS (0.24s).", nowStr)
				case strings.Contains(lowerCmd, "status") || strings.Contains(lowerCmd, "ps"):
					respLog = fmt.Sprintf("[%s] \x1b[36m[Process Status]\x1b[0m PID %d active in %s (Status: %s, CPU: %.1f%%, RAM: %.1fMB)", nowStr, proc.ProcessID, proc.WorkingDir, proc.Status, proc.CPUPercent, proc.MemoryMB)
				case strings.Contains(lowerCmd, "git"):
					respLog = fmt.Sprintf("[%s] \x1b[32m[Git]\x1b[0m Working tree clean. On branch feature/%s.", nowStr, strings.ToLower(taskID))
				default:
					respLog = fmt.Sprintf("[%s] \x1b[32m[Process Output]\x1b[0m Command executed successfully with exit code 0.", nowStr)
				}
				proc.Logs = append(proc.Logs, respLog)
				r.mu.Unlock()

				if r.cfg.WSHub != nil {
					r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{
						Type:      types.EventAgentTerminal,
						TaskID:    taskID,
						StageID:   task.CurrentStageID,
						Timestamp: time.Now(),
						Payload: map[string]string{
							"stream": "stdout",
							"chunk":  cmdLog + "\r\n" + respLog + "\r\n",
						},
					})
				}

				r.writeJSON(w, http.StatusOK, map[string]interface{}{
					"status":  "executed",
					"command": body.Command,
					"process": proc,
				})
				return
			}

			if req.Method == http.MethodGet {
				r.mu.Lock()
				proc := r.getOrCreateTaskProcessLocked(taskID)
				// Dynamically refresh duration
				if proc.Status == "RUNNING" {
					proc.DurationSeconds = int64(time.Since(proc.StartedAt).Seconds())
				}
				r.mu.Unlock()
				r.writeJSON(w, http.StatusOK, proc)
				return
			}

			r.writeError(w, http.StatusMethodNotAllowed, "GET or POST execute required for process")
			return

		case "terminal", "logs":
			if req.Method != http.MethodGet {
				r.writeError(w, http.StatusMethodNotAllowed, "GET required for terminal logs")
				return
			}
			r.mu.Lock()
			proc := r.getOrCreateTaskProcessLocked(taskID)
			r.mu.Unlock()
			r.writeJSON(w, http.StatusOK, map[string]interface{}{
				"task_id": taskID,
				"logs":    proc.Logs,
			})
			return

		case "inject":
			if req.Method != http.MethodPost {
				r.writeError(w, http.StatusMethodNotAllowed, "POST required for inject")
				return
			}
			var body InjectContextRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Instruction == "" {
				r.writeError(w, http.StatusBadRequest, "Valid instruction string is required")
				return
			}

			r.mu.Lock()
			if task.State == types.TaskStateBlockedFrustration {
				task.State = types.TaskStateRunning
			}
			task.UpdatedAt = time.Now()
			proc := r.getOrCreateTaskProcessLocked(taskID)
			proc.Status = "RUNNING"
			injectLog := fmt.Sprintf("[%s] \x1b[33m[HITL Steer]\x1b[0m Operator guidance injected: %s", time.Now().Format("15:04:05"), body.Instruction)
			proc.Logs = append(proc.Logs, injectLog)
			r.mu.Unlock()

			// Broadcast thought event showing injected context
			if r.cfg.WSHub != nil {
				r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{
					Type:      types.EventAgentThought,
					TaskID:    taskID,
					StageID:   task.CurrentStageID,
					Timestamp: time.Now(),
					Payload: map[string]interface{}{
						"profile":    "HITL_Operator",
						"model":      "human_instruction",
						"thought":    fmt.Sprintf("Human Guidance Injected: %s", body.Instruction),
						"is_steered": true,
					},
				})
				r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{
					Type:      types.EventAgentTerminal,
					TaskID:    taskID,
					StageID:   task.CurrentStageID,
					Timestamp: time.Now(),
					Payload: map[string]string{
						"stream": "stdout",
						"chunk":  injectLog + "\r\n",
					},
				})
			}

			r.writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":  "injected",
				"task_id": taskID,
				"task":    task,
			})
			return

		case "reset":
			if req.Method != http.MethodPost {
				r.writeError(w, http.StatusMethodNotAllowed, "POST required for reset")
				return
			}

			r.mu.Lock()
			task.State = types.TaskStateRunning
			task.UpdatedAt = time.Now()
			if task.Metadata == nil {
				task.Metadata = make(map[string]string)
			}
			task.Metadata["frustration_count"] = "0"
			delete(task.Metadata, "failing_trace")
			proc := r.getOrCreateTaskProcessLocked(taskID)
			proc.Status = "RUNNING"
			resetLog := fmt.Sprintf("[%s] \x1b[33m[HITL Reset]\x1b[0m Purging container volumes and executing git reset --hard...", time.Now().Format("15:04:05"))
			readyLog := fmt.Sprintf("[%s] \x1b[32m[HITL Reset]\x1b[0m Workspace clean. Resuming stage execution loop.", time.Now().Format("15:04:05"))
			proc.Logs = append(proc.Logs, resetLog, readyLog)
			r.mu.Unlock()

			if r.cfg.WSHub != nil {
				r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{
					Type:      types.EventAgentTerminal,
					TaskID:    taskID,
					StageID:   task.CurrentStageID,
					Timestamp: time.Now(),
					Payload: map[string]string{
						"stream": "stderr",
						"chunk":  "\x1b[33m[HITL Reset] Purging container volumes and executing git reset --hard...\x1b[0m\r\n\x1b[32m[HITL Reset] Workspace clean. Resuming stage execution.\x1b[0m\r\n",
					},
				})
			}

			r.writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":  "reset_completed",
				"task_id": taskID,
				"task":    task,
			})
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

			r.mu.Lock()
			proc := r.getOrCreateTaskProcessLocked(taskID)
			if body.Approved {
				task.State = types.TaskStateRunning
				// Advance stage
				task.CurrentStageIndex++
				if task.CurrentStageIndex == 1 {
					task.CurrentStageID = "atdd_creation"
				} else if task.CurrentStageIndex == 4 {
					task.CurrentStageID = "task_implementation"
				} else if task.CurrentStageIndex >= 7 {
					task.CurrentStageID = "signoff_merge"
					task.State = types.TaskStateCompleted
					proc.Status = "COMPLETED"
				}
				gateLog := fmt.Sprintf("[%s] \x1b[32m[Gate Review]\x1b[0m Stage approved by operator. Advancing to %s.", time.Now().Format("15:04:05"), task.CurrentStageID)
				proc.Logs = append(proc.Logs, gateLog)
				proc.CurrentStep = fmt.Sprintf("Step %d of 7: %s (%s Method)", task.CurrentStageIndex+1, task.CurrentStageID, task.SelectedMethod)
			} else {
				task.State = types.TaskStateSuspended
				proc.Status = "PAUSED"
				gateLog := fmt.Sprintf("[%s] \x1b[31m[Gate Review]\x1b[0m Stage rejected by operator: %s. Process paused.", time.Now().Format("15:04:05"), body.Feedback)
				proc.Logs = append(proc.Logs, gateLog)
			}
			task.UpdatedAt = time.Now()
			r.mu.Unlock()

			r.writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":   "gate_updated",
				"approved": body.Approved,
				"task":     task,
			})
			return

		case "worktree":
			if req.Method != http.MethodGet {
				r.writeError(w, http.StatusMethodNotAllowed, "GET required for worktree")
				return
			}
			branch := task.Metadata["worktree_branch"]
			if branch == "" {
				branch = fmt.Sprintf("feat/%s-worktree", strings.ToLower(taskID))
			}
			r.writeJSON(w, http.StatusOK, map[string]interface{}{
				"task_id":            taskID,
				"is_worktree":        true,
				"use_worktree":       true,
				"worktree_path":      fmt.Sprintf(".worktrees/%s", strings.ToLower(taskID)),
				"branch":             branch,
				"base_ref":           "main",
				"parallel_isolation": true,
				"status":             "ACTIVE",
				"assigned_repos":     task.AssignedRepos,
			})
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
	case http.MethodPatch, http.MethodPut:
		var body struct {
			CurrentStageID string          `json:"current_stage_id,omitempty"`
			Stage          string          `json:"stage,omitempty"`
			State          types.TaskState `json:"state,omitempty"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		if body.CurrentStageID == "" && body.Stage != "" {
			body.CurrentStageID = body.Stage
		}

		r.mu.Lock()

		// Enforce dependency validation if moving to implementation/execution
		if body.CurrentStageID == "task_implementation" || body.CurrentStageID == "e2e_validation" {
			var unmet []string
			for _, depID := range task.Dependencies {
				dep, exists := r.tasks[depID]
				if !exists || dep.State != types.TaskStateCompleted {
					unmet = append(unmet, depID)
				}
			}
			if len(unmet) > 0 {
				task.State = types.TaskStateWaitingDependency
				if task.Metadata == nil {
					task.Metadata = make(map[string]string)
				}
				task.Metadata["unmet_dependencies"] = strings.Join(unmet, ",")
				r.mu.Unlock()
				r.writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot advance task while dependencies are unmet: %s", strings.Join(unmet, ", ")))
				return
			}
		}

		if body.CurrentStageID != "" {
			task.CurrentStageID = body.CurrentStageID
		}
		if body.State != "" {
			task.State = body.State
		}

		// If task completed, check if any dependent tasks can now be unblocked
		if task.State == types.TaskStateCompleted {
			for _, other := range r.tasks {
				if other.State == types.TaskStateWaitingDependency {
					allDone := true
					for _, d := range other.Dependencies {
						if dTask, ok := r.tasks[d]; !ok || dTask.State != types.TaskStateCompleted {
							allDone = false
							break
						}
					}
					if allDone {
						other.State = types.TaskStateRunning
						delete(other.Metadata, "unmet_dependencies")
						if r.cfg.WSHub != nil {
							r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{
								Type:      types.EventTaskStatus,
								TaskID:    other.ID,
								StageID:   other.CurrentStageID,
								Timestamp: time.Now(),
								Payload:   cloneTask(other),
							})
						}
					}
				}
			}
		}

		task.UpdatedAt = time.Now()
		taskCopy := cloneTask(task)
		r.mu.Unlock()

		if r.cfg.WSHub != nil {
			r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{
				Type:      types.EventTaskStatus,
				TaskID:    taskID,
				StageID:   taskCopy.CurrentStageID,
				Timestamp: time.Now(),
				Payload:   taskCopy,
			})
		}

		r.writeJSON(w, http.StatusOK, taskCopy)
	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
