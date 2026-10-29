package api

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/evidence"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/testenv"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func init() {
	// Tests never install Playwright: the recorder command is still briefed, without its tool dir.
	ensureEvidencePlaywright = func(*Router, context.Context) (string, error) { return "", nil }
	// Nor start containers: TestEvidenceRunGetsAThrowawayTestDatabase drives a fake docker.
	provisionTestEnv = func(*Router, context.Context, *types.Task, func([]testenv.Kind, map[testenv.Kind]string)) (*testenv.Env, error) {
		return nil, nil
	}
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
	t.Cleanup(r.evidenceJobs.Wait) // background finalization writes into root
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
	for _, want := range []string{"Test evidence is on for this stage", "🎬 Test evidence: 2 videos — 1 case passed, 1 failed or blocked", "TC-2 failed — Refund rejected",
		"• no presentation video (SUMMARY.mp4)"} {
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

func TestTurningEvidenceOffTellsAContinuedSession(t *testing.T) {
	stub := &evidenceStub{}
	r, _ := evidenceRouter(t, stub)
	model := r.strategyRouter.ModelOptions()[0].Model
	r.mu.Lock()
	r.tasks["TASK-E7"] = &types.Task{ID: "TASK-E7", Title: "Refund flow", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"},
		RoutingPlan: []types.StageRoute{{StageID: "task_implementation", Method: "react", Model: model, Evidence: &types.StageEvidence{Disabled: true}}}}
	r.mu.Unlock()
	// The session's first turn was briefed with evidence on; the operator turned it off since.
	r.console.mu.Lock()
	c := r.console.get("TASK-E7")
	c.sessionID, c.sessionStage = "sess-1", "task_implementation"
	r.console.mu.Unlock()

	if w := do(r, "POST", "/api/v1/tasks/TASK-E7/execute", ""); w.Code != 200 {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-E7", types.TaskStateWaitingGateApproval)
	waitIdleTask(t, r, "TASK-E7")

	stub.mu.Lock()
	prompt := stub.reqs[len(stub.reqs)-1].Messages[len(stub.reqs[len(stub.reqs)-1].Messages)-1].Content
	stub.mu.Unlock()
	if !strings.Contains(prompt, "Test evidence is now off") || strings.Contains(prompt, "## Test evidence (required") {
		t.Fatalf("a continued session should be told evidence is off:\n%s", prompt)
	}
	if !strings.Contains(consoleText(r, "TASK-E7"), "Test evidence is off for this stage") {
		t.Fatal("the console should say evidence is off")
	}
}

// resumeImplementation continues the task's session ("sess-1") on the Implementation stage, with
// the session's evidence brief state as given, and returns the turn's prompt.
func resumeImplementation(t *testing.T, r *Router, stub *evidenceStub, taskID string, known bool, briefed string) string {
	t.Helper()
	r.mu.Lock()
	r.tasks[taskID].State = types.TaskStatePending
	r.mu.Unlock()
	r.console.mu.Lock()
	c := r.console.get(taskID)
	c.sessionID, c.sessionStage, c.evidenceSession, c.evidenceSkill = "sess-1", "task_implementation", "", briefed
	if known {
		c.evidenceSession = "sess-1"
	}
	r.console.mu.Unlock()
	if err := r.startStage(taskID, "resume", ""); err != nil {
		t.Fatalf("resume: %v", err)
	}
	waitState(t, r, taskID, types.TaskStateWaitingGateApproval)
	waitIdleTask(t, r, taskID)
	stub.mu.Lock()
	defer stub.mu.Unlock()
	req := stub.reqs[len(stub.reqs)-1]
	return req.Messages[len(req.Messages)-1].Content
}

func TestTurningEvidenceOnBriefsAContinuedSession(t *testing.T) {
	stub := &evidenceStub{}
	r, _ := evidenceRouter(t, stub)
	model := r.strategyRouter.ModelOptions()[0].Model
	r.mu.Lock()
	r.tasks["TASK-E8"] = &types.Task{ID: "TASK-E8", Title: "Refund flow", CurrentStageID: "task_implementation",
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"},
		RoutingPlan: []types.StageRoute{{StageID: "task_implementation", Method: "react", Model: model}}}
	r.mu.Unlock()

	// The session was told evidence is off; the operator turned it on since.
	prompt := resumeImplementation(t, r, stub, "TASK-E8", true, "")
	if !strings.Contains(prompt, "## Test evidence (required") || !strings.Contains(prompt, `<skill name="test-evidence-video"`) {
		t.Fatalf("a continued session should be briefed when evidence is turned on:\n%s", prompt)
	}
	r.console.mu.Lock()
	c := r.console.get("TASK-E8")
	got := c.evidenceSession + "/" + c.evidenceSkill
	r.console.mu.Unlock()
	if got != "sess-1/test-evidence-video" {
		t.Fatalf("evidence brief state = %q", got)
	}

	// Already briefed with the same skill: no second brief.
	if prompt := resumeImplementation(t, r, stub, "TASK-E8", true, "test-evidence-video"); strings.Contains(prompt, "## Test evidence") {
		t.Fatalf("a session briefed with the same skill should not be briefed again:\n%s", prompt)
	}
}

func TestEvidenceOffNoteIsNotRepeated(t *testing.T) {
	stub := &evidenceStub{}
	r, _ := evidenceRouter(t, stub)
	model := r.strategyRouter.ModelOptions()[0].Model
	r.mu.Lock()
	r.tasks["TASK-E9"] = &types.Task{ID: "TASK-E9", Title: "Refund flow", CurrentStageID: "task_implementation",
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"},
		RoutingPlan: []types.StageRoute{{StageID: "task_implementation", Method: "react", Model: model, Evidence: &types.StageEvidence{Disabled: true}}}}
	r.mu.Unlock()

	if prompt := resumeImplementation(t, r, stub, "TASK-E9", true, "test-evidence-video"); !strings.Contains(prompt, "Test evidence is now off") {
		t.Fatalf("a session briefed with evidence should be told it is off:\n%s", prompt)
	}
	if prompt := resumeImplementation(t, r, stub, "TASK-E9", true, ""); strings.Contains(prompt, "Test evidence") {
		t.Fatalf("a session already told evidence is off should not be told again:\n%s", prompt)
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

func TestEvidenceIsPlannedFromTheSpecAndReportsCoverage(t *testing.T) {
	stub := &evidenceStub{}
	r, _ := evidenceRouter(t, stub)
	adir := r.artifactDir("TASK-E5")
	_ = os.MkdirAll(filepath.Join(adir, "stages"), 0o755)
	_ = os.WriteFile(filepath.Join(adir, "PRD.md"), []byte("# Refunds\nFinance can refund a paid invoice."), 0o644)
	_ = os.WriteFile(filepath.Join(adir, filepath.FromSlash(stageDocName("atdd_creation"))), []byte("Cases\n\n```atdd-cases\n"+
		`[{"id":"RF-TC-001","title":"Finance refunds a paid invoice","priority":"P1","platform":"WEB","steps":["Open the invoice","Click Refund"],"expected_results":["The invoice shows Refunded"]},`+
		`{"id":"RF-TC-002","title":"An unpaid invoice cannot be refunded","priority":"P0","platform":"API","expected_results":["422"]}]`+"\n```\n"), 0o644)
	dir := r.evidenceDir("TASK-E5")
	stub.onRun = func() {
		_ = os.MkdirAll(dir, 0o755)
		for _, f := range []string{"FLOW-1.mp4", "SUITE-api.mp4", "SUMMARY.mp4"} {
			_ = os.WriteFile(filepath.Join(dir, f), []byte("v"), 0o644)
		}
		m := evidence.Manifest{Cases: []evidence.Case{
			{ID: "FLOW-1", Title: "Refund journey", Covers: []string{"RF-TC-001"}, Status: "passed", Video: "FLOW-1.mp4"},
			{ID: "SUITE-api", Status: "passed", Video: "SUITE-api.mp4", Covers: []string{"RF-TC-002"}}, // a suite run proves no acceptance case
		}, Summary: &evidence.Summary{Video: "SUMMARY.mp4", DurationS: 96, Chapters: []evidence.Chapter{{ID: "overview", At: 0}, {ID: "FLOW-1", At: 12}}}}
		raw, _ := json.Marshal(m)
		_ = os.WriteFile(filepath.Join(dir, evidence.ManifestFile), raw, 0o644)
	}
	r.mu.Lock()
	r.tasks["TASK-E5"] = &types.Task{ID: "TASK-E5", Title: "Refunds", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"}}
	r.mu.Unlock()

	if w := do(r, "POST", "/api/v1/tasks/TASK-E5/execute", ""); w.Code != 200 {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-E5", types.TaskStateWaitingGateApproval)
	stub.mu.Lock()
	prompt := stub.reqs[len(stub.reqs)-1].Messages[len(stub.reqs[len(stub.reqs)-1].Messages)-1].Content
	stub.mu.Unlock()
	for _, want := range []string{"Prove that every flow this change touches works end to end", "scheduled jobs and schedulers", "`watch`, `expect_log`", filepath.Join(adir, "PRD.md"),
		"#### RF-TC-001 — Finance refunds a paid invoice (P1, WEB)", "1. The invoice shows Refunded", "`flow` — FLOW-1", "set \"blocked\"",
		"record the presentation", "--summary", "signs in once, off camera", "--session <name> --explore <path>"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("brief is missing %q:\n%s", want, prompt)
		}
	}
	log := consoleText(r, "TASK-E5")
	raw, err := os.ReadFile(filepath.Join(dir, evidence.SpecFile))
	if err != nil || !strings.Contains(string(raw), `"RF-TC-002"`) {
		t.Fatalf("spec.json for the presentation = %s, %v", raw, err)
	}
	for _, want := range []string{"Spec coverage: 1 of 2 acceptance cases have evidence.", "• no evidence for RF-TC-002", "▶ Presentation: SUMMARY.mp4 (1m36s, 2 chapters)"} {
		if !strings.Contains(log, want) {
			t.Fatalf("console is missing %q:\n%s", want, log)
		}
	}

	w := do(r, "GET", "/api/v1/tasks/TASK-E5/evidence", "")
	var got struct {
		Spec evidenceSpecView `json:"spec"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	s := got.Spec
	if s.Covered != 1 || len(s.Cases) != 2 || s.Cases[0].Status != "passed" || len(s.Cases[0].Evidence) != 1 || s.Cases[0].Evidence[0].ID != "FLOW-1" ||
		s.Cases[1].Status != "missing" || len(s.Docs) != 2 {
		t.Fatalf("spec = %+v", s)
	}
}

func TestCoverageStatus(t *testing.T) {
	for _, c := range []struct {
		refs []evidenceRef
		want string
	}{
		{nil, "missing"},
		{[]evidenceRef{{Status: "blocked"}}, "blocked"},
		{[]evidenceRef{{Status: "blocked"}, {Status: "passed"}}, "passed"},
		{[]evidenceRef{{Status: "passed"}, {Status: "failed"}}, "failed"},
	} {
		if got := coverageStatus(c.refs); got != c.want {
			t.Errorf("coverageStatus(%v) = %s, want %s", c.refs, got, c.want)
		}
	}
}

// fakeDocker answers the docker CLI calls of a test environment, holding the containers it ran.
func fakeDocker(down bool) testenv.Docker {
	var mu sync.Mutex
	running := map[string]string{}
	return func(_ context.Context, args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		switch args[0] {
		case "info":
			if down {
				return "", errors.New("Cannot connect to the Docker daemon")
			}
		case "inspect":
			if img, ok := running[args[len(args)-1]]; ok {
				return "true|" + img, nil
			}
			return "", errors.New("No such object")
		case "run":
			running[args[3]] = args[len(args)-2] // the image, then the server flag
		case "port":
			return "127.0.0.1:55001\n", nil
		}
		return "", nil
	}
}

func TestEvidenceRunGetsAThrowawayTestDatabase(t *testing.T) {
	stub := &evidenceStub{}
	r, root := evidenceRouter(t, stub)
	repo := t.TempDir() // a Rails app on MySQL whose .env.template has no usable credentials
	_ = os.WriteFile(filepath.Join(repo, "Gemfile"), []byte("gem 'mysql2'\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(repo, "config"), 0o755)
	_ = os.WriteFile(filepath.Join(repo, "config", "database.yml"), []byte("default:\n  adapter: mysql2\n"), 0o644)
	_ = os.WriteFile(filepath.Join(repo, ".env.template"), []byte("DB_HOST=\nDB_USERNAME=\nDB_PASSWORD=\nDB_TEST_NAME=\n"), 0o644)
	var mgr *testenv.Manager
	prev := provisionTestEnv
	t.Cleanup(func() { provisionTestEnv = prev })
	down := false
	provisionTestEnv = func(r *Router, ctx context.Context, task *types.Task, starting func([]testenv.Kind, map[testenv.Kind]string)) (*testenv.Env, error) {
		mgr = &testenv.Manager{Docker: fakeDocker(down), Root: filepath.Join(root, ".sdlc", "testenv")}
		repos := []testenv.Repo{{Name: "billing", Dirs: []string{repo}}}
		_, kinds, images := testenv.Plan(repos)
		starting(kinds, images)
		return mgr.Ensure(ctx, task.ID, repos)
	}
	r.mu.Lock()
	r.tasks["TASK-DB"] = &types.Task{ID: "TASK-DB", Title: "Invoice export", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		AssignedRepos: []string{"billing"}, Metadata: map[string]string{"complexity": "LOW"}}
	r.mu.Unlock()

	if w := do(r, "POST", "/api/v1/tasks/TASK-DB/execute", ""); w.Code != 200 {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-DB", types.TaskStateWaitingGateApproval)
	stub.mu.Lock()
	prompt := stub.reqs[len(stub.reqs)-1].Messages[len(stub.reqs[len(stub.reqs)-1].Messages)-1].Content
	stub.mu.Unlock()
	envFile := filepath.Join(root, ".sdlc", "testenv", "TASK-DB", "env.json")
	for _, want := range []string{"Test environment (provided — do not ask for database credentials)", "MySQL (mysql:8.0) on 127.0.0.1:55001",
		"billing.env.sh", "DB_PASSWORD, DB_TEST_NAME", "- billing: bin/rails db:prepare", "EVIDENCE_TESTENV=" + envFile, "never for setup you can do"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("brief is missing %q:\n%s", want, prompt)
		}
	}
	raw, _ := os.ReadFile(envFile)
	var doc struct{ Redact []string }
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Redact) != 1 || strings.Contains(prompt, doc.Redact[0]) {
		t.Fatalf("the password must reach the env file only (err %v)", err)
	}
	log := consoleText(r, "TASK-DB")
	for _, want := range []string{"🧪 Preparing the test environment: MySQL (mysql:8.0)", "✓ Test environment ready: MySQL on 127.0.0.1:55001; billing gets databases billing_development and billing_test"} {
		if !strings.Contains(log, want) {
			t.Fatalf("console is missing %q:\n%s", want, log)
		}
	}

	// Without Docker the operator is told the one thing to do, and the agent falls back to compose.
	down = true
	r.mu.Lock()
	r.tasks["TASK-DB"].State = types.TaskStatePending
	r.mu.Unlock()
	if w := do(r, "POST", "/api/v1/tasks/TASK-DB/evidence", ""); w.Code != 200 {
		t.Fatalf("record evidence → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-DB", types.TaskStateWaitingGateApproval)
	if log := consoleText(r, "TASK-DB"); !strings.Contains(log, "⚠ Docker is not running") || !strings.Contains(log, "no credentials needed") {
		t.Fatalf("console:\n%s", log)
	}
	stub.mu.Lock()
	last := stub.reqs[len(stub.reqs)-1].Messages
	stub.mu.Unlock()
	if p := last[len(last)-1].Content; !strings.Contains(p, "from the repository's own docker compose file") {
		t.Fatalf("fallback not briefed:\n%s", p)
	}

	teardownTestEnv(r, "TASK-DB") // real docker is never reached: the state dir is checked first
}
