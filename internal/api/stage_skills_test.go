package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// skillProbeStub records the stage request and reports reading a file from the skill directory.
type skillProbeStub struct {
	stubProvider
	mu   sync.Mutex
	reqs []llm.LLMRequest
	read string
}

func (s *skillProbeStub) StreamActivity(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) (*llm.LLMResponse, error) {
	s.mu.Lock()
	s.reqs = append(s.reqs, *req)
	s.mu.Unlock()
	return &llm.LLMResponse{Content: "Done.\n## Summary\nok", Provider: "anthropic", Model: "claude-x", FinishReason: "stop",
		ToolCalls: []llm.ToolCall{{ID: "t1", Name: "Read", Arguments: map[string]interface{}{"file_path": s.read}}}}, nil
}

// writeSkill creates a project-local Claude skill under root/.claude/skills/<name>.
func writeSkill(t *testing.T, root, name, description, body string) string {
	dir := filepath.Join(root, ".claude", "skills", name)
	if err := os.MkdirAll(filepath.Join(dir, "steps"), 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: " + name + "\ndescription: " + description + "\n---\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSuggestSkillsMatchesWholeNameWordsAndSkipsDeprecated(t *testing.T) {
	opts := []skillOption{
		{Name: "generate-atdd"}, {Name: "bmad-atdd"}, {Name: "atddish-helper"},
		{Name: "bmad-create-prd", Description: "DEPRECATED — use bmad-prd"}, {Name: "bmad-prd"},
		{Name: "bmad-manual-test-plan"},
	}
	if got := suggestSkills("atdd_creation", opts); strings.Join(got, ",") != "bmad-atdd,generate-atdd" {
		t.Fatalf("atdd suggestions = %v", got)
	}
	if got := suggestSkills("prd_discovery", opts); strings.Join(got, ",") != "bmad-prd" {
		t.Fatalf("prd suggestions = %v", got)
	}
	if got := suggestSkills("uat_verification", opts); strings.Join(got, ",") != "bmad-manual-test-plan" {
		t.Fatalf("uat suggestions = %v", got)
	}
	if got := suggestSkills("unknown_stage", opts); len(got) != 0 {
		t.Fatalf("unknown stage suggestions = %v", got)
	}
}

func TestPlanRejectsUnavailableSkillAndKeepsValidOnes(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "probe-stage-atdd", "Write ATDD cases", "Follow steps/one.md.")
	r := NewRouter(RouterConfig{RootDir: root})
	model := r.strategyRouter.ModelOptions()[0].Model

	plan, err := r.validatePlan([]types.StageRoute{{StageID: "atdd_creation", Method: "react", Model: model,
		Skills: []string{" probe-stage-atdd ", "probe-stage-atdd"}}})
	if err != nil || strings.Join(plan[0].Skills, ",") != "probe-stage-atdd" {
		t.Fatalf("plan = %+v, err = %v", plan, err)
	}
	if _, err := r.validatePlan([]types.StageRoute{{StageID: "atdd_creation", Method: "react", Model: model,
		Skills: []string{"probe-not-installed"}}}); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("unknown skill should be rejected, err = %v", err)
	}
	if err := r.cfg.SkillResolver.ToggleSkill("probe-stage-atdd", false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.validatePlan([]types.StageRoute{{StageID: "atdd_creation", Method: "react", Model: model,
		Skills: []string{"probe-stage-atdd"}}}); err == nil {
		t.Fatal("a disabled skill should be rejected")
	}
}

func TestStageRunLoadsAttachedSkillsAndReportsUsage(t *testing.T) {
	root := t.TempDir()
	dir := writeSkill(t, root, "probe-stage-atdd", "Write ATDD cases", "Follow steps/one.md for every criterion.")
	stub := &skillProbeStub{read: filepath.Join(dir, "steps", "one.md")}
	r := NewRouter(RouterConfig{RootDir: root})
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, stub)
	}
	r.mu.Lock()
	r.tasks["TASK-S1"] = &types.Task{ID: "TASK-S1", Title: "Refund flow", CurrentStageID: "atdd_creation", State: types.TaskStatePending, AssignedRepos: []string{"backend-core"},
		Metadata: map[string]string{"complexity": "LOW"},
		RoutingPlan: []types.StageRoute{{StageID: "atdd_creation", Method: "react",
			Model: r.strategyRouter.ModelOptions()[0].Model, Skills: []string{"probe-stage-atdd", "probe-removed"}}}}
	r.mu.Unlock()

	if w := do(r, "POST", "/api/v1/tasks/TASK-S1/execute", ""); w.Code != 200 {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-S1", types.TaskStateWaitingGateApproval)

	stub.mu.Lock()
	req := stub.reqs[len(stub.reqs)-1]
	stub.mu.Unlock()
	prompt := req.Messages[len(req.Messages)-1].Content
	if !strings.Contains(prompt, `<skill name="probe-stage-atdd"`) || !strings.Contains(prompt, "Follow steps/one.md for every criterion.") {
		t.Fatalf("stage brief is missing the skill instructions:\n%s", prompt)
	}
	if len(req.AddDirs) != 1 || req.AddDirs[0] != dir {
		t.Fatalf("skill dir should be readable to the agent, AddDirs = %v", req.AddDirs)
	}

	r.console.mu.Lock()
	var log []string
	for _, e := range r.console.get("TASK-S1").entries {
		log = append(log, e.Content)
	}
	r.console.mu.Unlock()
	all := strings.Join(log, "\n")
	for _, want := range []string{"Skipping skill probe-removed", "Skills for this stage: probe-stage-atdd",
		"probe-stage-atdd — opened 1 of its files (steps/one.md)"} {
		if !strings.Contains(all, want) {
			t.Fatalf("console is missing %q:\n%s", want, all)
		}
	}
}

func TestSkillUsageReportWhenNothingOpened(t *testing.T) {
	s := &types.UniversalSkillContract{Name: "x", SourceLocation: "/skills/x"}
	got := skillUsageReport([]*types.UniversalSkillContract{s}, []llm.ToolCall{{Name: "Skill", Arguments: map[string]interface{}{"skill": "x"}}})
	if !strings.Contains(got, "invoked it through the Skill tool") {
		t.Fatalf("report = %q", got)
	}
	got = skillUsageReport([]*types.UniversalSkillContract{s}, nil)
	if !strings.Contains(got, "opened none of its files") {
		t.Fatalf("report = %q", got)
	}
}
