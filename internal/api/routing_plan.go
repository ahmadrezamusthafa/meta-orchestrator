package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Routing plan flow: the operator describes a task, the AI grades its complexity, the router
// proposes a method and model per stage, and the operator confirms or changes them before the
// task is created. The confirmed plan is what each stage runs with.

const analysisTimeout = 25 * time.Second

// methodOption describes an execution method for the plan editor.
type methodOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

var planMethods = []methodOption{
	{ID: "react", Name: "ReAct", Description: "One agent reasons and acts in a fast loop. Cheapest; best for small, focused changes."},
	{ID: "bmad", Name: "BMAD", Description: "PM, QA, architect and developer roles hand off in sequence. Thorough; best for features with real design work."},
	{ID: "supervisor", Name: "Supervisor", Description: "A coordinator splits work across parallel sub-agents per repository. Best for multi-repo changes."},
	{ID: "superpower", Name: "Superpower", Description: "Plan-and-execute with environment control. Best for CI, containers, migrations and E2E runs."},
}

func validMethod(m string) bool {
	for _, o := range planMethods {
		if o.ID == strings.ToLower(m) {
			return true
		}
	}
	return false
}

type analyzeRequest struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	AssignedRepos []string `json:"assigned_repos"`
	// Complexity, when set, is the operator's own grading: the plan is rebuilt without asking the AI.
	Complexity string `json:"complexity,omitempty"`
	TaskType   string `json:"task_type,omitempty"`
	StartStage string `json:"start_stage,omitempty"`
	HaltStage  string `json:"halt_stage,omitempty"`
}

type analyzeResponse struct {
	Assessment router.ComplexityAssessment `json:"assessment"`
	Plan       []router.PlannedStage       `json:"plan"`
	Methods    []methodOption              `json:"methods"`
	Models     []router.ModelOption        `json:"models"`
}

// planStages is the slice of the pipeline a task runs, from start to halt inclusive.
func planStages(start, halt string) []string {
	from, to := 0, len(stagePipeline)-1
	if i := stageIndexOf(start); i >= 0 {
		from = i
	}
	if i := stageIndexOf(halt); i >= from {
		to = i
	}
	return append([]string(nil), stagePipeline[from:to+1]...)
}

// classifier sends the complexity prompt to the analysis model, trying its fallbacks in order.
func (r *Router) classifier() router.Classifier {
	if r.clientFactory == nil || r.strategyRouter == nil {
		return nil
	}
	return func(ctx context.Context, system, prompt string) (string, string, error) {
		primary, chain := r.strategyRouter.AnalysisModel()
		models := chain
		if len(models) == 0 && primary != "" {
			models = []string{primary}
		}
		if len(models) == 0 {
			return "", "", fmt.Errorf("no model configured")
		}
		var lastErr error
		for _, model := range models {
			client, modelID, err := r.clientFactory.GetClient(model)
			if err != nil {
				lastErr = err
				continue
			}
			if r.telemetry != nil {
				client = r.telemetry.tracker.Instrument(client, telemetry.CallMeta{TaskID: "complexity-analysis",
					StageID: "complexity_analysis", Model: model, Tier: router.TierLogParse})
			}
			resp, err := client.Complete(ctx, &llm.LLMRequest{Model: modelID, MaxTokens: 600, Timeout: analysisTimeout,
				Messages: []llm.Message{{Role: llm.RoleSystem, Content: system}, {Role: llm.RoleUser, Content: prompt}}})
			if err != nil {
				lastErr = err
				continue
			}
			return resp.Content, model, nil
		}
		return "", "", lastErr
	}
}

// assess grades a task: the operator's complexity when given, otherwise the AI (with heuristic fallback).
func (r *Router) assess(ctx context.Context, title, description, complexity, taskType string, repos []string) router.ComplexityAssessment {
	if complexity != "" {
		a := router.ComplexityAssessment{Complexity: router.NormalizeComplexity(complexity), TaskType: taskType,
			Source: router.SourceOperator, Rationale: "Set by the operator."}
		if a.TaskType == "" {
			a.TaskType = router.ClassifyTaskType(title, description)
		}
		return a
	}
	ctx, cancel := context.WithTimeout(ctx, analysisTimeout)
	defer cancel()
	return router.AnalyzeComplexity(ctx, r.classifier(), title, description, repos)
}

// handleTaskAnalyze: POST /api/v1/tasks/analyze grades a draft task and proposes its routing plan.
func (r *Router) handleTaskAnalyze(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body analyzeRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.Title) == "" {
		r.writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	a := r.assess(req.Context(), body.Title, body.Description, body.Complexity, body.TaskType, body.AssignedRepos)
	r.writeJSON(w, http.StatusOK, analyzeResponse{
		Assessment: a,
		Plan:       r.strategyRouter.Plan(planStages(body.StartStage, body.HaltStage), a.Complexity, a.TaskType, body.AssignedRepos),
		Methods:    planMethods,
		Models:     r.strategyRouter.ModelOptions(),
	})
}

// validatePlan normalizes a submitted plan and rejects unknown stages, methods and models.
func (r *Router) validatePlan(plan []types.StageRoute) ([]types.StageRoute, error) {
	allowed := map[string]bool{}
	for _, o := range r.strategyRouter.ModelOptions() {
		allowed[o.Model] = true
	}
	seen := map[string]bool{}
	out := make([]types.StageRoute, 0, len(plan))
	for _, s := range plan {
		if stageIndexOf(s.StageID) < 0 {
			return nil, fmt.Errorf("unknown stage %q", s.StageID)
		}
		if seen[s.StageID] {
			return nil, fmt.Errorf("stage %q appears twice", s.StageID)
		}
		seen[s.StageID] = true
		s.Method = strings.ToLower(strings.TrimSpace(s.Method))
		if s.Method != "" && !validMethod(s.Method) {
			return nil, fmt.Errorf("unknown method %q for %s", s.Method, s.StageID)
		}
		s.Model = strings.TrimSpace(s.Model)
		if s.Model != "" && len(allowed) > 0 && !allowed[s.Model] {
			return nil, fmt.Errorf("model %q for %s is not enabled in your router chain", s.Model, s.StageID)
		}
		out = append(out, s)
	}
	return out, nil
}

// routeTask is the routing of the task's current stage: the live router decision, replaced by
// the stage's confirmed plan entry when there is one.
func (r *Router) routeTask(task *types.Task, complexity, taskType string) *router.RoutingDecision {
	d := r.strategyRouter.RouteForTask(task.CurrentStageID, complexity, taskType, task.AssignedRepos)
	if rt := task.RouteFor(task.CurrentStageID); rt != nil {
		d = router.ApplyPlan(d, rt.Method, rt.Model, rt.Tier, rt.Overridden)
	}
	return d
}

type routingUpdate struct {
	Complexity  string             `json:"complexity,omitempty"`
	RoutingPlan []types.StageRoute `json:"routing_plan"`
}

// handleTaskRouting: GET returns the task's routing plan; PUT replaces it (and optionally the
// complexity). Changes apply from the next stage run.
func (r *Router) handleTaskRouting(w http.ResponseWriter, req *http.Request, taskID string) {
	switch req.Method {
	case http.MethodGet:
		r.mu.Lock()
		t, ok := r.tasks[taskID]
		var cp *types.Task
		if ok {
			cp = cloneTask(t)
		}
		r.mu.Unlock()
		if !ok {
			r.writeError(w, http.StatusNotFound, "task not found")
			return
		}
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"complexity": cp.Metadata["complexity"], "complexity_source": cp.Metadata["complexity_source"],
			"complexity_rationale": cp.Metadata["complexity_rationale"], "routing_plan": cp.RoutingPlan,
			"methods": planMethods, "models": r.strategyRouter.ModelOptions(),
		})
	case http.MethodPut:
		var body routingUpdate
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		plan, err := r.validatePlan(body.RoutingPlan)
		if err != nil {
			r.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.mu.Lock()
		t, ok := r.tasks[taskID]
		if !ok {
			r.mu.Unlock()
			r.writeError(w, http.StatusNotFound, "task not found")
			return
		}
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		if body.Complexity != "" {
			t.Metadata["complexity"] = router.NormalizeComplexity(body.Complexity)
			t.Metadata["complexity_source"] = router.SourceOperator
		}
		t.RoutingPlan = plan
		t.Metadata["routing_confirmed"] = "true"
		t.UpdatedAt = time.Now()
		cp := cloneTask(t)
		r.mu.Unlock()
		r.saveBoardNow()
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: "Routing plan updated: " + planSummary(plan) + ". It applies from the next stage run."})
		if r.cfg.WSHub != nil {
			r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{Type: types.EventTaskStatus, TaskID: taskID, StageID: cp.CurrentStageID,
				Timestamp: time.Now(), Payload: cp})
		}
		r.writeJSON(w, http.StatusOK, cp)
	default:
		r.writeError(w, http.StatusMethodNotAllowed, "GET or PUT required")
	}
}

func planSummary(plan []types.StageRoute) string {
	parts := make([]string, 0, len(plan))
	for _, s := range plan {
		p := fmt.Sprintf("%s → %s on %s", s.StageID, s.Method, s.Model)
		if s.Overridden {
			p += " (your choice)"
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return "routed live per stage"
	}
	return strings.Join(parts, "; ")
}

// proposedPlan converts the router's proposal into a plan to store on a task.
func proposedPlan(stages []router.PlannedStage) []types.StageRoute {
	out := make([]types.StageRoute, 0, len(stages))
	for _, s := range stages {
		out = append(out, types.StageRoute{StageID: s.StageID, Method: s.Method, Model: s.Model, Tier: s.Tier, Reasoning: s.Reasoning})
	}
	return out
}
