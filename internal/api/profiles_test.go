package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

const testProfiles = `[
  {"id": "atdd_qa_engineer", "name": "ATDD QA Engineer", "role": "atdd_qa_engineer",
   "description": "Generates acceptance test suites", "allowed_skills": ["read_ast_node"],
   "denied_skills": ["probe-denied"], "system_prompt_template": "prompts/qa.md", "max_token_budget": 5000},
  {"id": "lead_developer", "name": "Lead Developer", "role": "lead_developer",
   "description": "Implements production code", "denied_skills": [], "max_token_budget": 0}
]`

const testQATemplate = "# QA Contract\nYou write failing tests first.\n\n## Inputs\n{{task_spec}}\n\n## Invariants\n1. Never modify production code.\n"

func profileRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for path, body := range map[string]string{"configs/profiles.json": testProfiles, "prompts/qa.md": testQATemplate} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestPlanRejectsSkillTheStageRoleDenies(t *testing.T) {
	root := profileRoot(t)
	writeSkill(t, root, "probe-denied", "A skill QA may not use", "Do it.")
	writeSkill(t, root, "probe-ok", "A skill nobody lists", "Do it.")
	r := NewRouter(RouterConfig{RootDir: root})
	model := r.strategyRouter.ModelOptions()[0].Model

	_, err := r.validatePlan([]types.StageRoute{{StageID: "atdd_creation", Method: "react", Model: model, Skills: []string{"probe-denied"}}})
	if err == nil || !strings.Contains(err.Error(), "ATDD QA Engineer profile denies it") {
		t.Fatalf("a denied skill must be rejected, err = %v", err)
	}
	// Not in allowed_skills is not a denial, and another role may use the denied skill.
	if _, err := r.validatePlan([]types.StageRoute{
		{StageID: "atdd_creation", Method: "react", Model: model, Skills: []string{"probe-ok"}},
		{StageID: "task_implementation", Method: "react", Model: model, Skills: []string{"probe-denied"}},
	}); err != nil {
		t.Fatalf("plan should be accepted: %v", err)
	}
}

func TestStageRunUsesProfilePersonaAndBudget(t *testing.T) {
	stub := &skillProbeStub{}
	r := NewRouter(RouterConfig{RootDir: profileRoot(t)})
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, stub)
	}
	r.mu.Lock()
	r.tasks["TASK-P1"] = &types.Task{ID: "TASK-P1", Title: "Refund flow", CurrentStageID: "atdd_creation", State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"}}
	r.mu.Unlock()

	if w := do(r, "POST", "/api/v1/tasks/TASK-P1/execute", ""); w.Code != 200 {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-P1", types.TaskStateWaitingGateApproval)

	stub.mu.Lock()
	system := stub.reqs[len(stub.reqs)-1].Messages[0].Content
	stub.mu.Unlock()
	for _, want := range []string{"Your role for this stage: ATDD QA Engineer — Generates acceptance test suites.",
		"You write failing tests first.", "1. Never modify production code."} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt is missing %q:\n%s", want, system)
		}
	}
	if strings.Contains(system, "{{") {
		t.Fatalf("unfilled template placeholders reached the prompt:\n%s", system)
	}
	// The routed budget is capped at the profile's 5000 tokens.
	if _, err := r.budgetTracker.RecordUsage("TASK-P1", 5001, 0, 0); err == nil || !strings.Contains(err.Error(), "> 5000") {
		t.Fatalf("budget cap not applied: %v", err)
	}
}

func TestStageTokenBudget(t *testing.T) {
	r := NewRouter(RouterConfig{RootDir: profileRoot(t)})
	for _, c := range []struct {
		stage        string
		routed, want int64
	}{
		{"atdd_creation", 80000, 5000}, {"atdd_creation", 0, 5000}, {"atdd_creation", 1000, 1000},
		{"task_implementation", 80000, 80000}, // max_token_budget 0: no cap
		{"signoff_merge", 80000, 80000},       // no role
	} {
		if got := r.stageTokenBudget(c.stage, c.routed); got != c.want {
			t.Errorf("stageTokenBudget(%s, %d) = %d, want %d", c.stage, c.routed, got, c.want)
		}
	}
}
