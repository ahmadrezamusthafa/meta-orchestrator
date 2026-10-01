package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/shadow"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ws"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func setupTestRouter() *Router {
	hub := ws.NewHub()
	go hub.Run()
	r := NewRouter(RouterConfig{
		WSHub:   hub,
		RootDir: "/tmp",
	})
	// Tests trigger real stage executions: keep them off live providers.
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, stubProvider{})
	}
	r.seedFixtureTasks()
	r.reconcileIdleRunning()
	return r
}

func TestTasksAPI_ListAndFilter(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks?method=BMAD", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	var tasks []*types.Task
	if err := json.NewDecoder(w.Body).Decode(&tasks); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(tasks) == 0 {
		t.Fatalf("Expected filtered BMAD tasks, got 0")
	}
	for _, task := range tasks {
		if task.SelectedMethod != "BMAD" {
			t.Errorf("Expected method BMAD, got %s", task.SelectedMethod)
		}
	}
}

func TestTasksAPI_CreateWithStageSlicing(t *testing.T) {
	router := setupTestRouter()

	body := CreateTaskRequest{
		Title:          "Implement Order Checkout Flow with Playwright Video",
		Description:    "Start at stage 7 and halt at stage 8 with video recording",
		WorkflowID:     "general_ai_sdlc",
		AssignedRepos:  []string{"frontend-portal", "backend-core"},
		SelectedMethod: "BMAD",
		ActiveSlice: &types.StageSlice{
			StartStageID: "task_implementation",
			HaltStageID:  "e2e_validation",
			ProduceVideo: true,
		},
		SourceBranch: "feat/checkout-v2",
		Complexity:   "HIGH",
	}

	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created, got %d (body: %s)", w.Code, w.Body.String())
	}

	var created types.Task
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("Failed to decode created task: %v", err)
	}

	if created.CurrentStageID != "task_implementation" {
		t.Errorf("Expected start stage 'task_implementation', got %s", created.CurrentStageID)
	}
	if created.CurrentStageIndex != 4 {
		t.Errorf("Expected stage index 4, got %d", created.CurrentStageIndex)
	}
	if created.ActiveSlice == nil || !created.ActiveSlice.ProduceVideo {
		t.Errorf("Expected ProduceVideo=true in ActiveSlice")
	}
}

func TestTasksAPI_HITL_Inject_Reset_Gate(t *testing.T) {
	router := setupTestRouter()

	// 1. Test Inject Context
	injectBody := InjectContextRequest{Instruction: "Use Stripe API version 2024-06-20 with idempotency keys"}
	payload, _ := json.Marshal(injectBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/TASK-8940/inject", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for inject, got %d", w.Code)
	}

	// 2. Test Reset Workspace
	reqReset := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/TASK-8940/reset", nil)
	wReset := httptest.NewRecorder()
	router.ServeHTTP(wReset, reqReset)

	if wReset.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for reset, got %d", wReset.Code)
	}

	// Verify task unblocked
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/TASK-8940", nil)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)
	var task types.Task
	_ = json.NewDecoder(wGet.Body).Decode(&task)
	if task.State != types.TaskStateRunning && task.State != types.TaskStateWaitingGateApproval {
		t.Errorf("Expected reset to restart the stage, got %s", task.State)
	}

	// 3. Gate approval is only possible once a stage has produced output for review.
	earlyGate := httptest.NewRecorder()
	router.ServeHTTP(earlyGate, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/TASK-8943/gate", bytes.NewBufferString(`{"approved":true}`)))
	if earlyGate.Code != http.StatusConflict {
		t.Fatalf("Expected 409 approving a stage with nothing to review, got %d", earlyGate.Code)
	}
	router.executeTaskWithAI("TASK-8943")
	if st := router.taskSnapshot("TASK-8943"); st.State != types.TaskStateWaitingGateApproval {
		t.Fatalf("Expected stage output awaiting review, got %s", st.State)
	}
	gateBody := GateApprovalRequest{Approved: true, Feedback: "Architecture and schemas approved"}
	gatePayload, _ := json.Marshal(gateBody)
	reqGate := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/TASK-8943/gate", bytes.NewReader(gatePayload))
	reqGate.Header.Set("Content-Type", "application/json")
	wGate := httptest.NewRecorder()
	router.ServeHTTP(wGate, reqGate)

	if wGate.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for gate, got %d", wGate.Code)
	}
	if st := router.taskSnapshot("TASK-8943"); st.CurrentStageID != "techdoc_rfc" {
		t.Fatalf("Expected approval to advance atdd_creation → techdoc_rfc, got %s", st.CurrentStageID)
	}
}

func TestToolsAPI_Lifecycle(t *testing.T) {
	router := setupTestRouter()

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools?category=Methodologies", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	// Rollback
	rbBody := ToolRollbackRequest{ToolID: "bmad", TargetVersion: "v2.3.9"}
	payload, _ := json.Marshal(rbBody)
	reqRb := httptest.NewRequest(http.MethodPost, "/api/v1/tools/rollback", bytes.NewReader(payload))
	reqRb.Header.Set("Content-Type", "application/json")
	wRb := httptest.NewRecorder()
	router.ServeHTTP(wRb, reqRb)

	if wRb.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for rollback, got %d", wRb.Code)
	}

	// Matrix resolve
	reqMat := httptest.NewRequest(http.MethodPost, "/api/v1/tools/resolve-matrix", nil)
	wMat := httptest.NewRecorder()
	router.ServeHTTP(wMat, reqMat)

	if wMat.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for resolve-matrix, got %d", wMat.Code)
	}
}

func TestProvidersAndBenchmarksAPI(t *testing.T) {
	router := setupTestRouter()

	// Providers list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	// Provider test ping
	testBody := ProviderTestRequest{ProviderID: "antigravity"}
	payload, _ := json.Marshal(testBody)
	reqTest := httptest.NewRequest(http.MethodPost, "/api/v1/providers/test", bytes.NewReader(payload))
	reqTest.Header.Set("Content-Type", "application/json")
	wTest := httptest.NewRecorder()
	router.ServeHTTP(wTest, reqTest)
	if wTest.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for ping test, got %d", wTest.Code)
	}

	// Benchmarks: 8 pipeline stages × 4 complexities
	reqBench := httptest.NewRequest(http.MethodGet, "/api/v1/benchmarks", nil)
	wBench := httptest.NewRecorder()
	router.ServeHTTP(wBench, reqBench)
	if wBench.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for benchmarks, got %d", wBench.Code)
	}

	var benchResp struct {
		TotalCells int                `json:"total_cells"`
		Matrix     []shadow.MatrixCell `json:"matrix"`
	}
	_ = json.NewDecoder(wBench.Body).Decode(&benchResp)
	if benchResp.TotalCells != 32 || len(benchResp.Matrix) != 0 {
		t.Errorf("Expected 32 possible cells and no measured ones, got %d / %d", benchResp.TotalCells, len(benchResp.Matrix))
	}
}

func TestWorkflowsAndRegistriesAPI(t *testing.T) {
	router := setupTestRouter()

	// Workflows
	reqWf := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	wWf := httptest.NewRecorder()
	router.ServeHTTP(wWf, reqWf)
	if wWf.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", wWf.Code)
	}

	// Registries
	reqReg := httptest.NewRequest(http.MethodGet, "/api/v1/registries", nil)
	wReg := httptest.NewRecorder()
	router.ServeHTTP(wReg, reqReg)
	if wReg.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", wReg.Code)
	}
}

func TestTasksAPI_ProcessAndTerminalLogs(t *testing.T) {
	router := setupTestRouter()

	// 1. GET /api/v1/tasks/TASK-8942/process
	reqProc := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/TASK-8942/process", nil)
	wProc := httptest.NewRecorder()
	router.ServeHTTP(wProc, reqProc)

	if wProc.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for task process, got %d", wProc.Code)
	}

	var proc types.TaskProcessInfo
	if err := json.NewDecoder(wProc.Body).Decode(&proc); err != nil {
		t.Fatalf("Failed to decode TaskProcessInfo: %v", err)
	}

	if proc.TaskID != "TASK-8942" {
		t.Errorf("Expected task ID TASK-8942, got %s", proc.TaskID)
	}
	if proc.ProcessID <= 0 {
		t.Errorf("Expected valid process ID, got %d", proc.ProcessID)
	}
	// Seeded tasks have no live agent turn, so the orchestrator must not report them as running.
	if proc.Status != "PAUSED" {
		t.Errorf("Expected status PAUSED for a task with no active agent, got %s", proc.Status)
	}
	if len(proc.Logs) == 0 {
		t.Errorf("Expected console logs, got 0")
	}

	// 3. GET /api/v1/tasks/TASK-8942/terminal
	reqTerm := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/TASK-8942/terminal", nil)
	wTerm := httptest.NewRecorder()
	router.ServeHTTP(wTerm, reqTerm)

	if wTerm.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for terminal logs, got %d", wTerm.Code)
	}

	var termResp struct {
		TaskID string   `json:"task_id"`
		Logs   []string `json:"logs"`
	}
	_ = json.NewDecoder(wTerm.Body).Decode(&termResp)
	if len(termResp.Logs) < len(proc.Logs) {
		t.Errorf("Expected logs count to increase after execute, before=%d, after=%d", len(proc.Logs), len(termResp.Logs))
	}
}

func TestTasksAPI_DependencyDAGAndWorktree(t *testing.T) {
	router := setupTestRouter()

	// 1. GET /api/v1/tasks/TASK-8942/dependencies
	reqDep := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/TASK-8942/dependencies", nil)
	wDep := httptest.NewRecorder()
	router.ServeHTTP(wDep, reqDep)

	if wDep.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for dependencies endpoint, got %d", wDep.Code)
	}

	var depInfo struct {
		TaskID        string   `json:"task_id"`
		Dependencies  []string `json:"dependencies"`
		Prerequisites []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"prerequisites"`
		Blocked bool `json:"blocked"`
	}
	if err := json.NewDecoder(wDep.Body).Decode(&depInfo); err != nil {
		t.Fatalf("Failed to decode dependencies response: %v", err)
	}
	if len(depInfo.Dependencies) == 0 {
		t.Errorf("Expected TASK-8942 to have dependencies")
	}

	// 2. GET /api/v1/tasks/TASK-8942/worktree
	reqWt := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/TASK-8942/worktree", nil)
	wWt := httptest.NewRecorder()
	router.ServeHTTP(wWt, reqWt)

	if wWt.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for worktree endpoint, got %d", wWt.Code)
	}

	var wtInfo struct {
		TaskID      string            `json:"task_id"`
		UseWorktree bool              `json:"use_worktree"`
		Branches    map[string]string `json:"branches"`
	}
	if err := json.NewDecoder(wWt.Body).Decode(&wtInfo); err != nil {
		t.Fatalf("Failed to decode worktree response: %v", err)
	}
	if !wtInfo.UseWorktree {
		t.Errorf("Expected UseWorktree to be true")
	}

	// 3. Create a task that depends on TASK-8942 (which is currently RUNNING)
	body := CreateTaskRequest{
		Title:         "Downstream Microservice Worker",
		Description:   "Depends on TASK-8942",
		WorkflowID:    "general_ai_sdlc",
		AssignedRepos: []string{"backend-core"},
		Dependencies:  []string{"TASK-8942"},
		UseWorktree:   true,
	}
	payload, _ := json.Marshal(body)
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(payload))
	reqCreate.Header.Set("Content-Type", "application/json")
	wCreate := httptest.NewRecorder()
	router.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created, got %d", wCreate.Code)
	}
	var createdTask types.Task
	_ = json.NewDecoder(wCreate.Body).Decode(&createdTask)

	if createdTask.State != types.TaskStateWaitingDependency {
		t.Errorf("Expected task state WAITING_DEPENDENCY, got %s", createdTask.State)
	}

	// 4. Attempting to advance createdTask while dependencies are unmet should fail
	advanceBody := map[string]string{"stage": "task_implementation"}
	advPayload, _ := json.Marshal(advanceBody)
	reqAdv := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+createdTask.ID, bytes.NewReader(advPayload))
	reqAdv.Header.Set("Content-Type", "application/json")
	wAdv := httptest.NewRecorder()
	router.ServeHTTP(wAdv, reqAdv)

	if wAdv.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request when advancing blocked task, got %d", wAdv.Code)
	}

	// 5. Complete TASK-8942 -> createdTask should auto-unblock
	completeBody := map[string]string{"state": "COMPLETED"}
	compPayload, _ := json.Marshal(completeBody)
	reqComp := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/TASK-8942", bytes.NewReader(compPayload))
	reqComp.Header.Set("Content-Type", "application/json")
	wComp := httptest.NewRecorder()
	router.ServeHTTP(wComp, reqComp)

	if wComp.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK when completing TASK-8942, got %d", wComp.Code)
	}

	// Verify createdTask is now RUNNING
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+createdTask.ID, nil)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)
	var reloadedTask types.Task
	_ = json.NewDecoder(wGet.Body).Decode(&reloadedTask)

	// Unblocking starts the stage automatically; with a fast stub it may already await review.
	if reloadedTask.State != types.TaskStateRunning && reloadedTask.State != types.TaskStateWaitingGateApproval {
		t.Errorf("Expected unblocked task to start (RUNNING or WAITING_GATE_APPROVAL), got %s", reloadedTask.State)
	}
}

func TestTaskExecutionWith9Router(t *testing.T) {
	router := setupTestRouter()

	// 1. Send execute request for TASK-8942
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/TASK-8942/execute", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from /execute, got %d", w.Code)
	}

	var res map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode execute response: %v", err)
	}
	if res["status"] != "execution_dispatched" {
		t.Errorf("Expected status 'execution_dispatched', got %v", res["status"])
	}

	// 2. Fetch process logs
	reqProc := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/TASK-8942/process", nil)
	wProc := httptest.NewRecorder()
	router.ServeHTTP(wProc, reqProc)

	if wProc.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from /process, got %d", wProc.Code)
	}

	var proc types.TaskProcessInfo
	if err := json.NewDecoder(wProc.Body).Decode(&proc); err != nil {
		t.Fatalf("Failed to decode process info: %v", err)
	}

	if proc.TaskID != "TASK-8942" {
		t.Errorf("Expected task ID TASK-8942, got %s", proc.TaskID)
	}
	if len(proc.Logs) == 0 {
		t.Errorf("Expected 9router logs to be populated, got 0 logs")
	}
}

func TestRouterSettingsEndpoint(t *testing.T) {
	router := setupTestRouter()

	// 1. Test GET /api/v1/router/settings
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/router/settings", nil)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from GET /api/v1/router/settings, got %d", wGet.Code)
	}

	var resGet map[string]interface{}
	if err := json.NewDecoder(wGet.Body).Decode(&resGet); err != nil {
		t.Fatalf("Failed to decode GET router settings response: %v", err)
	}

	if resGet["mode"] == nil {
		t.Errorf("Expected 'mode' in response")
	}
	modes, ok := resGet["available_modes"].([]interface{})
	if !ok || len(modes) < 5 {
		t.Errorf("Expected at least 5 available modes, got %d", len(modes))
	}
	models, ok := resGet["all_models"].([]interface{})
	if !ok || len(models) < 10 {
		t.Errorf("Expected at least 10 all_models, got %d", len(models))
	}

	// 2. Test POST /api/v1/router/settings to update mode and priority chain
	updatePayload := map[string]interface{}{
		"mode": "priority_sequence",
		"priority_chain": []map[string]interface{}{
			{
				"id":          "custom-1",
				"provider":    "antigravity",
				"model":       "gemini-2.5-pro",
				"name":        "Gemini 2.5 Pro",
				"enabled":     true,
				"cost_per_1k": 0.00125,
				"latency_ms":  120,
			},
			{
				"id":          "custom-2",
				"provider":    "claude",
				"model":       "claude-3-7-sonnet-20250219",
				"name":        "Claude 3.7 Sonnet",
				"enabled":     true,
				"cost_per_1k": 0.003,
				"latency_ms":  140,
			},
		},
	}
	payloadBytes, _ := json.Marshal(updatePayload)
	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/router/settings", bytes.NewReader(payloadBytes))
	reqPost.Header.Set("Content-Type", "application/json")
	wPost := httptest.NewRecorder()
	router.ServeHTTP(wPost, reqPost)

	if wPost.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from POST /api/v1/router/settings, got %d: %s", wPost.Code, wPost.Body.String())
	}

	// 3. Verify updated settings via GET
	reqVerify := httptest.NewRequest(http.MethodGet, "/api/v1/router/settings", nil)
	wVerify := httptest.NewRecorder()
	router.ServeHTTP(wVerify, reqVerify)

	var resVerify map[string]interface{}
	_ = json.NewDecoder(wVerify.Body).Decode(&resVerify)
	if resVerify["mode"] != "priority_sequence" {
		t.Errorf("Expected mode 'priority_sequence', got %v", resVerify["mode"])
	}
	chain := resVerify["priority_chain"].([]interface{})
	if len(chain) != 2 {
		t.Errorf("Expected priority_chain length 2, got %d", len(chain))
	}

	// 4. Test POST /api/v1/router/models to register a new frontier custom model
	newModelPayload := map[string]interface{}{
		"provider_id": "claude",
		"model_id":    "claude-opus-5-5-custom",
		"model_name":  "Claude Opus 5.5 (Custom Experimental)",
		"cost_per_1k": 0.015,
		"latency_ms":  240,
	}
	newModelBytes, _ := json.Marshal(newModelPayload)
	reqReg := httptest.NewRequest(http.MethodPost, "/api/v1/router/models", bytes.NewReader(newModelBytes))
	reqReg.Header.Set("Content-Type", "application/json")
	wReg := httptest.NewRecorder()
	router.ServeHTTP(wReg, reqReg)

	if wReg.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from POST /api/v1/router/models, got %d", wReg.Code)
	}

	// Verify it shows up in GET /api/v1/router/settings
	reqCheck := httptest.NewRequest(http.MethodGet, "/api/v1/router/settings", nil)
	wCheck := httptest.NewRecorder()
	router.ServeHTTP(wCheck, reqCheck)

	var resCheck map[string]interface{}
	_ = json.NewDecoder(wCheck.Body).Decode(&resCheck)
	modelsList := resCheck["all_models"].([]interface{})
	foundOpus55 := false
	foundCustom := false
	for _, m := range modelsList {
		mMap := m.(map[string]interface{})
		if mMap["model_id"] == "claude-opus-5-5" {
			foundOpus55 = true
		}
		if mMap["model_id"] == "claude-opus-5-5-custom" {
			foundCustom = true
		}
	}
	if !foundOpus55 {
		t.Errorf("Expected claude-opus-5-5 in all_models")
	}
	if !foundCustom {
		t.Errorf("Expected claude-opus-5-5-custom in all_models after registration")
	}
}


