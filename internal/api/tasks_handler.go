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
	SelectedMethod   string            `json:"selected_method"`
	RouterStrategy   string            `json:"router_strategy"`
	ActiveSlice      *types.StageSlice `json:"active_slice,omitempty"`
	SourceBranch     string            `json:"source_branch,omitempty"`
	ExternalTaskPlan string            `json:"external_task_plan,omitempty"`
	ExternalPRD      string            `json:"external_prd,omitempty"`
	MaxTokenBudget   int64             `json:"max_token_budget,omitempty"`
	Complexity       string            `json:"complexity,omitempty"`
}

type InjectContextRequest struct {
	Instruction string `json:"instruction"`
}

type GateApprovalRequest struct {
	Approved bool   `json:"approved"`
	Feedback string `json:"feedback,omitempty"`
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

		newTask := &types.Task{
			ID:                taskID,
			WorkflowID:        workflowID,
			Title:             body.Title,
			Description:       body.Description,
			CurrentStageID:    startStage,
			CurrentStageIndex: startIndex,
			State:             types.TaskStateRunning,
			ActiveSlice:       body.ActiveSlice,
			AssignedRepos:     body.AssignedRepos,
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
			},
			CreatedAt: now,
			UpdatedAt: now,
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
		r.mu.Unlock()

		// Broadcast new task event over WebSocket
		if r.cfg.WSHub != nil {
			r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{
				Type:      types.EventTaskStatus,
				TaskID:    taskID,
				StageID:   startStage,
				Timestamp: now,
				Payload:   newTask,
			})
		}

		r.writeJSON(w, http.StatusCreated, newTask)

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

	r.mu.Lock()
	task, exists := r.tasks[taskID]
	r.mu.Unlock()

	if !exists {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("Task %s not found", taskID))
		return
	}

	// Route sub-actions: /api/v1/tasks/{id}/inject, /api/v1/tasks/{id}/reset, /api/v1/tasks/{id}/gate
	if len(parts) > 1 {
		subAction := parts[1]
		switch subAction {
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
				}
			} else {
				task.State = types.TaskStateSuspended
			}
			task.UpdatedAt = time.Now()
			r.mu.Unlock()

			r.writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":   "gate_updated",
				"approved": body.Approved,
				"task":     task,
			})
			return

		default:
			r.writeError(w, http.StatusNotFound, "Unknown sub-action")
			return
		}
	}

	switch req.Method {
	case http.MethodGet:
		r.writeJSON(w, http.StatusOK, task)
	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
