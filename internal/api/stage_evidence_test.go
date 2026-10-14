package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/evidence"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func init() {
	// Tests never install Playwright: the recorder command is still briefed, without its tool dir.
	ensureEvidencePlaywright = func(*Router, context.Context) (string, error) { return "", nil }
}

// evidenceStub records the stage request and lets the test act as the agent during the turn.
type evidenceStub struct {
	stubProvider
	mu    sync.Mutex
	reqs  []llm.LLMRequest
	onRun func()
}

func (s *evidenceStub) StreamActivity(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) (*llm.LLMResponse, error) {
	s.mu.Lock()
	s.reqs = append(s.reqs, *req)
	s.mu.Unlock()
	if s.onRun != nil {
		s.onRun()
	}
	return &llm.LLMResponse{Content: "Done.\n## Summary\nok", Provider: "anthropic", Model: "claude-x", FinishReason: "stop"}, nil
}

func evidenceRouter(t *testing.T, stub *evidenceStub) (*Router, string) {
	root := t.TempDir()
	r := NewRouter(RouterConfig{RootDir: root})
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, stub)
	}
	return r, root
}

func consoleText(r *Router, taskID string) string {
	r.console.mu.Lock()
	defer r.console.mu.Unlock()
	var log []string
	for _, e := range r.console.get(taskID).entries {
		log = append(log, e.Content)
	}
	return strings.Join(log, "\n")
}

func TestEvidenceSettingIsOnByDefaultForImplementationOnly(t *testing.T) {
	task := &types.Task{ID: "T"}
	if on, skill := evidenceSetting(task, "task_implementation"); !on || skill != "" {
		t.Fatalf("default = %v %q, want on with the built-in skill", on, skill)
	}
	if on, _ := evidenceSetting(task, "e2e_validation"); on {
		t.Fatal("only the implementation stage records evidence")
	}
	task.RoutingPlan = []types.StageRoute{{StageID: "task_implementation", Evidence: &types.StageEvidence{Skill: "record-test-video"}}}
	if on, skill := evidenceSetting(task, "task_implementation"); !on || skill != "record-test-video" {
		t.Fatalf("library skill = %v %q", on, skill)
	}
	task.RoutingPlan[0].Evidence = &types.StageEvidence{Disabled: true, Skill: "record-test-video"}
	if on, _ := evidenceSetting(task, "task_implementation"); on {
		t.Fatal("disabled evidence should be off")
	}
}

func TestPlanValidatesEvidenceSetting(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "record-test-video", "Record a test video", "Record it.")
	r := NewRouter(RouterConfig{RootDir: root})
	model := r.strategyRouter.ModelOptions()[0].Model
	route := func(stage string, e *types.StageEvidence) []types.StageRoute {
		return []types.StageRoute{{StageID: stage, Method: "react", Model: model, Evidence: e}}
	}

	plan, err := r.validatePlan(route("task_implementation", &types.StageEvidence{Skill: " record-test-video "}))
	if err != nil || plan[0].Evidence == nil || plan[0].Evidence.Skill != "record-test-video" {
		t.Fatalf("plan = %+v, err = %v", plan, err)
	}
	if plan, err = r.validatePlan(route("task_implementation", &types.StageEvidence{Skill: evidence.SkillName})); err != nil || plan[0].Evidence != nil {
		t.Fatalf("the built-in default should be stored as no setting, plan = %+v, err = %v", plan, err)
	}
	if _, err := r.validatePlan(route("task_implementation", &types.StageEvidence{Skill: "not-installed"})); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("unknown evidence skill should be rejected, err = %v", err)
	}
	if _, err := r.validatePlan(route("techdoc_rfc", &types.StageEvidence{Disabled: true})); err == nil {
		t.Fatal("evidence on another stage should be rejected")
	}
	if got := planSummary([]types.StageRoute{{StageID: "task_implementation", Method: "react", Model: "m", Evidence: &types.StageEvidence{Disabled: true}}}); !strings.Contains(got, "no test evidence") {
		t.Fatalf("summary = %q", got)
	}
}

func TestEvidenceSuggestionsMatchRecordingSkills(t *testing.T) {
	got := evidenceSuggestions([]skillOption{{Name: "attach-test-evidence"}, {Name: "bmad-prd"}, {Name: "record-test-video"}, {Name: "videography-tips"}})
	if strings.Join(got, ",") != "attach-test-evidence,record-test-video" {
		t.Fatalf("suggestions = %v", got)
	}
}

func TestImplementationStageBriefsBuiltinEvidenceAndReportsIt(t *testing.T) {
	stub := &evidenceStub{}
	r, root := evidenceRouter(t, stub)
	dir := r.evidenceDir("TASK-E1")
	stub.onRun = func() { // the agent records two cases, one failing
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "TC-1.mp4"), []byte("v"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "TC-2.mp4"), []byte("v"), 0o644)
		m := evidence.Manifest{Cases: []evidence.Case{{ID: "TC-1", Status: "passed", Video: "TC-1.mp4"}, {ID: "TC-2", Title: "Refund rejected", Status: "failed", Video: "TC-2.mp4"}}}
		raw, _ := json.Marshal(m)
		_ = os.WriteFile(filepath.Join(dir, evidence.ManifestFile), raw, 0o644)
	}
	r.mu.Lock()
	r.tasks["TASK-E1"] = &types.Task{ID: "TASK-E1", Title: "Refund flow", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"}}
	r.mu.Unlock()

	if w := do(r, "POST", "/api/v1/tasks/TASK-E1/execute", ""); w.Code != 200 {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-E1", types.TaskStateWaitingGateApproval)

	stub.mu.Lock()
	req := stub.reqs[len(stub.reqs)-1]
	stub.mu.Unlock()
	prompt := req.Messages[len(req.Messages)-1].Content
	skillDir := filepath.Join(root, ".sdlc", "tools", evidence.SkillName)
	for _, want := range []string{`<skill name="test-evidence-video"`, "## Test evidence", dir, "record-evidence.mjs", "--case <id>"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("brief is missing %q:\n%s", want, prompt)
		}
	}
	if strings.Join(req.AddDirs, ",") != skillDir+","+dir {
		t.Fatalf("AddDirs = %v, want the skill and evidence dirs", req.AddDirs)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "scripts", "record-evidence.mjs")); err != nil {
		t.Fatalf("built-in skill not installed: %v", err)
	}

	log := consoleText(r, "TASK-E1")
	for _, want := range []string{"Test evidence is on for this stage", "🎬 Test evidence: 2 videos — 1 case passed, 1 failed or blocked", "TC-2 failed — Refund rejected"} {
		if !strings.Contains(log, want) {
			t.Fatalf("console is missing %q:\n%s", want, log)
		}
	}
	if st := r.taskSnapshot("TASK-E1").Metadata["evidence_status"]; st != "failed" {
		t.Fatalf("evidence_status = %q", st)
	}

	w := do(r, "GET", "/api/v1/tasks/TASK-E1/evidence", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"TC-2.mp4"`) || !strings.Contains(w.Body.String(), `"skill":"test-evidence-video"`) {
		t.Fatalf("evidence → %d %s", w.Code, w.Body.String())
	}
	if w := do(r, "GET", "/api/v1/artifacts/TASK-E1/evidence/TC-1.mp4", ""); w.Code != 200 || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("video → %d %s", w.Code, w.Header().Get("Content-Type"))
	}
}

func TestImplementationStageWithoutEvidenceWarnsAndDisabledSkipsIt(t *testing.T) {
	stub := &evidenceStub{}
	r, _ := evidenceRouter(t, stub)
	model := r.strategyRouter.ModelOptions()[0].Model
	r.mu.Lock()
	r.tasks["TASK-E2"] = &types.Task{ID: "TASK-E2", Title: "Refund flow", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"}}
	r.tasks["TASK-E3"] = &types.Task{ID: "TASK-E3", Title: "Refund flow", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"},
		RoutingPlan: []types.StageRoute{{StageID: "task_implementation", Method: "react", Model: model, Evidence: &types.StageEvidence{Disabled: true}}}}
	r.mu.Unlock()

	r.executeTaskWithAI("TASK-E2")
	if log := consoleText(r, "TASK-E2"); !strings.Contains(log, "No test evidence video was recorded") {
		t.Fatalf("missing evidence should be reported:\n%s", log)
	}
	if st := r.taskSnapshot("TASK-E2").Metadata["evidence_status"]; st != "missing" {
		t.Fatalf("evidence_status = %q", st)
	}

	r.executeTaskWithAI("TASK-E3")
	stub.mu.Lock()
	prompt := stub.reqs[len(stub.reqs)-1].Messages[len(stub.reqs[len(stub.reqs)-1].Messages)-1].Content
	stub.mu.Unlock()
	if strings.Contains(prompt, "## Test evidence") || strings.Contains(consoleText(r, "TASK-E3"), "Test evidence") {
		t.Fatal("disabled evidence should not be briefed or reported")
	}
}

func TestLibraryEvidenceSkillIsBriefedInsteadOfBuiltin(t *testing.T) {
	stub := &evidenceStub{}
	r, root := evidenceRouter(t, stub)
	dir := writeSkill(t, root, "record-test-video", "Record a test video", "Drive the spec and record it.")
	r.cfg.SkillResolver.Rescan()
	model := r.strategyRouter.ModelOptions()[0].Model
	r.mu.Lock()
	r.tasks["TASK-E4"] = &types.Task{ID: "TASK-E4", Title: "Refund flow", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"},
		RoutingPlan: []types.StageRoute{{StageID: "task_implementation", Method: "react", Model: model, Evidence: &types.StageEvidence{Skill: "record-test-video"}}}}
	r.mu.Unlock()

	r.executeTaskWithAI("TASK-E4")
	stub.mu.Lock()
	req := stub.reqs[len(stub.reqs)-1]
	stub.mu.Unlock()
	prompt := req.Messages[len(req.Messages)-1].Content
	if !strings.Contains(prompt, `<skill name="record-test-video"`) || strings.Contains(prompt, `<skill name="test-evidence-video"`) ||
		!strings.Contains(prompt, "copy each final video into") {
		t.Fatalf("brief should use the library skill:\n%s", prompt)
	}
	if strings.Join(req.AddDirs, ",") != dir+","+r.evidenceDir("TASK-E4") {
		t.Fatalf("AddDirs = %v", req.AddDirs)
	}
}

func TestRecordEvidenceRunsAnEvidenceTurnAndKeepsTheStageOutput(t *testing.T) {
	stub := &evidenceStub{}
	r, root := evidenceRouter(t, stub)
	writeSkill(t, root, "probe-impl", "Implement it", "Implement carefully.")
	r.cfg.SkillResolver.Rescan()
	model := r.strategyRouter.ModelOptions()[0].Model
	r.mu.Lock()
	r.tasks["TASK-E5"] = &types.Task{ID: "TASK-E5", Title: "Refund flow", CurrentStageID: "task_implementation", State: types.TaskStateWaitingGateApproval,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"},
		RoutingPlan: []types.StageRoute{{StageID: "task_implementation", Method: "react", Model: model, Skills: []string{"probe-impl"}}}}
	r.tasks["TASK-E6"] = &types.Task{ID: "TASK-E6", Title: "Other", CurrentStageID: "techdoc_rfc", State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{}}
	r.mu.Unlock()
	r.saveStageDocument("TASK-E5", "task_implementation", "m", "## Summary\nthe implementation")
	doc, _ := r.resolveArtifactPath("TASK-E5", stageDocName("task_implementation"))

	if w := do(r, "POST", "/api/v1/tasks/TASK-E6/evidence", ""); w.Code != 409 || !strings.Contains(w.Body.String(), "Implementation stage") {
		t.Fatalf("evidence on another stage → %d %s", w.Code, w.Body.String())
	}
	if w := do(r, "POST", "/api/v1/tasks/TASK-E5/evidence", ""); w.Code != 200 {
		t.Fatalf("record evidence → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-E5", types.TaskStateWaitingGateApproval)
	waitIdleTask(t, r, "TASK-E5")

	stub.mu.Lock()
	prompt := stub.reqs[len(stub.reqs)-1].Messages[len(stub.reqs[len(stub.reqs)-1].Messages)-1].Content
	stub.mu.Unlock()
	for _, want := range []string{"already in the working directory", "Do not re-implement", "## Test evidence", `<skill name="test-evidence-video"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("evidence prompt is missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `<skill name="probe-impl"`) {
		t.Fatal("an evidence run should load only the evidence skill")
	}
	if raw, _ := os.ReadFile(doc); !strings.Contains(string(raw), "the implementation") {
		t.Fatalf("the evidence run must keep the implementation output, got:\n%s", raw)
	}
	if !strings.Contains(consoleText(r, "TASK-E5"), "No test evidence video was recorded") {
		t.Fatal("the evidence run should report what it recorded")
	}
}

// waitIdleTask waits until the task's console has no live turn.
func waitIdleTask(t *testing.T, r *Router, taskID string) {
	t.Helper()
	for i := 0; i < 500; i++ {
		r.console.mu.Lock()
		busy := r.console.get(taskID).busy
		r.console.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("turn did not finish")
}
