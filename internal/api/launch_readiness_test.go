package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/readiness"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestReadinessWorkflowsAreRunnable(t *testing.T) {
	for _, def := range []*types.WorkflowDefinition{fsm.GetLaunchReadinessWorkflow(), fsm.GetNewServiceWorkflow()} {
		if err := validateRunnable(def); err != nil {
			t.Fatalf("%s: %v", def.ID, err)
		}
		last := def.Stages[len(def.Stages)-1]
		if last.ID != "signoff_merge" || !last.RequiresGate {
			t.Fatalf("%s must end in a gated sign-off, got %s", def.ID, last.ID)
		}
	}
	for _, s := range readiness.Steps {
		if codeStages[s.ID] != s.Code {
			t.Fatalf("%s: code stage = %v, want %v", s.ID, codeStages[s.ID], s.Code)
		}
	}
}

func TestReadinessStageLoadsItsPlaybookAlongsideAttachedSkills(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "probe-extra-security", "Extra checks", "Check more.")
	r := NewRouter(RouterConfig{RootDir: root})

	task := &types.Task{ID: "T-R1", WorkflowID: fsm.LaunchReadinessWorkflowID, CurrentStageID: "security_automation",
		RoutingPlan: []types.StageRoute{{StageID: "security_automation", Skills: []string{"probe-extra-security", "launch-security-scanning"}}}}
	loaded, missing := r.stageSkills(task)
	if len(missing) != 0 || strings.Join(skillNames(loaded), ",") != "launch-security-scanning,probe-extra-security" {
		t.Fatalf("loaded = %v, missing = %v", skillNames(loaded), missing)
	}
	if !strings.Contains(skillsBrief(loaded), "secret scanning") {
		t.Fatal("the playbook's instructions are not in the brief")
	}

	task.CurrentStageID, task.RoutingPlan = "task_implementation", nil
	if loaded, _ := r.stageSkills(task); len(loaded) != 0 {
		t.Fatalf("a stage without a playbook loaded %v", skillNames(loaded))
	}

	pb := stagePlaybooks([]string{"release_audit", "task_implementation"}, r.skillOptions())
	if len(pb) != 1 || pb["release_audit"] != "launch-release-audit" {
		t.Fatalf("playbooks = %v", pb)
	}
}

func TestGateChecklistComesFromTheTaskWorkflow(t *testing.T) {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	task := &types.Task{WorkflowID: fsm.LaunchReadinessWorkflowID}
	got := r.gateChecklist(task, "release_automation")
	if !strings.Contains(got, "☐ Rollback is one documented command") {
		t.Fatalf("checklist = %q", got)
	}
	if r.gateChecklist(&types.Task{}, "task_implementation") != "" {
		t.Fatal("the default workflow has no gate checklist")
	}
}

func TestReadinessStageWaitsForItsRequiredTools(t *testing.T) {
	r := toolchainRouter(t, bareMachine("brew", "tofu", "terragrunt", "gitleaks"))
	r.mu.Lock()
	r.tasks["TASK-SEC"] = &types.Task{ID: "TASK-SEC", Title: "Launch", WorkflowID: fsm.LaunchReadinessWorkflowID,
		CurrentStageID: "security_automation", State: types.TaskStatePending, AssignedRepos: []string{"infra"}, Metadata: map[string]string{}}
	r.mu.Unlock()

	var missing *toolsMissingError
	if err := r.toolchainGate("TASK-SEC"); !errors.As(err, &missing) || strings.Join(missing.Missing, ",") != "trivy,semgrep" {
		t.Fatalf("gate = %v", err)
	}
	// Later readiness stages' tools are offered up front, as recommendations.
	rep := r.taskToolchain(r.taskSnapshot("TASK-SEC"))
	if !strings.Contains(strings.Join(rep.MissingRecommended, ","), "k6") {
		t.Fatalf("k6 (load stage, later) should be recommended: %v", rep.MissingRecommended)
	}
	tools := r.stageTools([]string{"security_automation", "release_audit"})["security_automation"]
	if len(tools) == 0 || tools[0].ID != "gitleaks" || !tools[0].Installed || !tools[0].Required {
		t.Fatalf("stage tools = %+v", tools)
	}
	if brief := r.readinessToolsBrief("security_automation"); !strings.Contains(brief, "Trivy (`trivy`, required)") ||
		!strings.Contains(brief, "never install software yourself") {
		t.Fatalf("brief = %q", brief)
	}
}

func TestSliceHaltStageEndsTheTask(t *testing.T) {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	task := &types.Task{WorkflowID: fsm.LaunchReadinessWorkflowID, CurrentStageID: "security_automation",
		ActiveSlice: &types.StageSlice{StartStageID: "security_automation", HaltStageID: "security_automation"}}
	if next := r.nextStageFor(task, "security_automation"); next != "" {
		t.Fatalf("a task halting at security_automation continued to %q", next)
	}
	for _, req := range r.readinessToolNeeds(task) {
		if req.Tool == "spectral" || req.Tool == "k6" {
			t.Fatalf("tools of stages after the halt stage are offered: %+v", req)
		}
	}
	task.ActiveSlice = nil
	if next := r.nextStageFor(task, "security_automation"); next != "customer_api_setup" {
		t.Fatalf("without a slice the next stage is customer_api_setup, got %q", next)
	}
	task.ActiveSlice = &types.StageSlice{HaltStageID: "not_in_workflow"}
	if next := r.nextStageFor(task, "security_automation"); next != "customer_api_setup" {
		t.Fatalf("a halt stage outside the workflow is ignored, got %q", next)
	}
}
