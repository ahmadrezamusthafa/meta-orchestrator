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
