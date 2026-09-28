package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ws"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func setupTestRouter() *Router {
	hub := ws.NewHub()
	go hub.Run()
	return NewRouter(RouterConfig{
		WSHub:   hub,
		RootDir: "/tmp",
	})
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
	if task.State != types.TaskStateRunning {
		t.Errorf("Expected task state RUNNING after reset, got %s", task.State)
	}

	// 3. Test Gate Approval
	gateBody := GateApprovalRequest{Approved: true, Feedback: "Architecture and schemas approved"}
	gatePayload, _ := json.Marshal(gateBody)
	reqGate := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/TASK-8943/gate", bytes.NewReader(gatePayload))
	reqGate.Header.Set("Content-Type", "application/json")
	wGate := httptest.NewRecorder()
	router.ServeHTTP(wGate, reqGate)

	if wGate.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for gate, got %d", wGate.Code)
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

	// Benchmarks 36-cell matrix
	reqBench := httptest.NewRequest(http.MethodGet, "/api/v1/benchmarks", nil)
	wBench := httptest.NewRecorder()
	router.ServeHTTP(wBench, reqBench)
	if wBench.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for benchmarks, got %d", wBench.Code)
	}

	var benchResp struct {
		TotalCells int                `json:"total_cells"`
		Matrix     []BenchmarkCellDTO `json:"matrix"`
	}
	_ = json.NewDecoder(wBench.Body).Decode(&benchResp)
	if benchResp.TotalCells != 36 {
		t.Errorf("Expected 36 benchmark matrix cells, got %d", benchResp.TotalCells)
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
	if proc.Status != "RUNNING" {
		t.Errorf("Expected status RUNNING, got %s", proc.Status)
	}
	if len(proc.Logs) == 0 {
		t.Errorf("Expected console logs, got 0")
	}

	// 2. POST /api/v1/tasks/TASK-8942/process/execute
	cmdPayload := []byte(`{"command":"go test -v ./... -run TestBilling"}`)
	reqExec := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/TASK-8942/process/execute", bytes.NewReader(cmdPayload))
	reqExec.Header.Set("Content-Type", "application/json")
	wExec := httptest.NewRecorder()
	router.ServeHTTP(wExec, reqExec)

	if wExec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for command execute, got %d", wExec.Code)
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

	if reloadedTask.State != types.TaskStateRunning {
		t.Errorf("Expected unblocked task state RUNNING, got %s", reloadedTask.State)
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


