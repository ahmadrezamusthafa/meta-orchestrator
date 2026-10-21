package api

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/readiness"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Launch readiness: the steps a new service or initiative goes through before production (see
// internal/readiness). Each is a runnable stage with a built-in playbook skill, installed into the
// project's .sdlc/skills so the skill library lists it like any other skill: the operator can read,
// edit or disable it there. Every step ends at a gate whose checklist the operator confirms.

func init() {
	for _, s := range readiness.Steps {
		stageInstructions[s.ID] = s.Instruction()
		if s.Code {
			codeStages[s.ID] = true
		}
		stageRoles[s.ID] = s.Role
	}
}

// stagePlaybooks maps each stage that loads a built-in playbook to that skill's name, for the
// routing plan editor. Stages whose playbook is disabled are left out.
func stagePlaybooks(stages []string, options []skillOption) map[string]string {
	enabled := map[string]bool{}
	for _, o := range options {
		enabled[o.Name] = true
	}
	out := map[string]string{}
	for _, st := range stages {
		if step, ok := readiness.StepByID(st); ok && enabled[step.Skill] {
			out[st] = step.Skill
		}
	}
	return out
}

// installReadinessSkills writes the built-in readiness playbooks into .sdlc/skills, keeping any
// file the operator has edited.
func (r *Router) installReadinessSkills() {
	if r.cfg.RootDir == "" {
		return
	}
	if err := readiness.InstallSkills(filepath.Join(r.cfg.RootDir, ".sdlc", "skills")); err != nil {
		fmt.Printf("[Readiness] Failed to install the launch-readiness skills: %v\n", err)
		return
	}
	if r.cfg.SkillResolver != nil {
		r.cfg.SkillResolver.Rescan()
	}
}

// playbookSkill is the built-in playbook of a readiness stage, when it is installed and enabled.
// It is loaded whether or not the operator attached other skills to the stage; disabling it in the
// skill library turns it off.
func playbookSkill(stage string, available map[string]*types.UniversalSkillContract) *types.UniversalSkillContract {
	step, ok := readiness.StepByID(stage)
	if !ok {
		return nil
	}
	return available[step.Skill]
}

// gateChecklist is what the operator confirms before approving the task's stage: the gate
// criteria of the stage in the task's workflow, one per line, or "" when it has none.
func (r *Router) gateChecklist(t *types.Task, stage string) string {
	if r.cfg.WorkflowReg == nil {
		return ""
	}
	def, err := r.cfg.WorkflowReg.Get(t.WorkflowID)
	if err != nil {
		return ""
	}
	for _, s := range def.Stages {
		if s.ID != stage || strings.TrimSpace(s.GateCriteria) == "" {
			continue
		}
		var b strings.Builder
		for _, item := range strings.Split(s.GateCriteria, ";") {
			if item = strings.TrimSpace(item); item != "" {
				b.WriteString("\n  ☐ " + item)
			}
		}
		return b.String()
	}
	return ""
}

// readinessToolNeeds are the tools the task's readiness stages run: the current stage's as declared
// (a required one stops the stage until it is installed or acknowledged), and the later readiness
// stages' as recommendations, so the operator can install everything up front.
func (r *Router) readinessToolNeeds(t *types.Task) []toolchain.Requirement {
	var out []toolchain.Requirement
	if step, ok := readiness.StepByID(t.CurrentStageID); ok {
		for _, n := range step.Tools {
			out = append(out, toolchain.Requirement{Tool: n.ID, Required: n.Required, Reasons: []string{step.Name + ": " + n.Why}})
		}
	}
	stages := r.taskStages(t)
	i := indexIn(stages, t.CurrentStageID)
	if i < 0 {
		return out
	}
	for _, id := range stages[i+1:] {
		if step, ok := readiness.StepByID(id); ok {
			for _, n := range step.Tools {
				out = append(out, toolchain.Requirement{Tool: n.ID, Reasons: []string{"Later, " + step.Name + ": " + n.Why}})
			}
		}
	}
	return out
}

// taskToolchain is the toolchain of a task: what its repositories' stacks and the connectors need,
// plus the tools of its readiness stages.
func (r *Router) taskToolchain(t *types.Task) toolchain.Report {
	return r.toolchain.Report(toolchain.Merge(toolchain.Requirements(r.repoStacks(t.AssignedRepos)), r.connectorToolNeeds(),
		r.readinessToolNeeds(t)), false)
}

// readinessToolsBrief tells the agent which of the stage's tools this machine has, so it runs the
// installed ones and, for the missing ones, still writes the pipeline config and lists the commands.
func (r *Router) readinessToolsBrief(stage string) string {
	step, ok := readiness.StepByID(stage)
	if !ok || len(step.Tools) == 0 || r.toolchain == nil {
		return ""
	}
	reqs := make([]toolchain.Requirement, 0, len(step.Tools))
	for _, n := range step.Tools {
		reqs = append(reqs, toolchain.Requirement{Tool: n.ID, Required: n.Required})
	}
	rep := r.toolchain.Report(reqs, false)
	byID := map[string]toolchain.Item{}
	for _, it := range rep.Items {
		byID[it.ID] = it
	}
	var b strings.Builder
	b.WriteString("\n\nTools on this machine for this stage:\n")
	for _, n := range step.Tools {
		it := byID[n.ID]
		state := "not installed"
		if it.Installed {
			state = "installed"
			if v := toolVersion(it.Version); v != "" {
				state += " (" + v + ")"
			}
		}
		kind := "recommended"
		if n.Required {
			kind = "required"
		}
		fmt.Fprintf(&b, "- %s (`%s`, %s) — %s: %s\n", it.Name, it.Binary, kind, n.Why, state)
	}
	b.WriteString("Run the installed tools locally to establish the baseline. For a tool that is not installed, still add it to the " +
		"pipeline, give the exact command to run it, and list it under the report's operator actions — never install software yourself.")
	return b.String()
}

// stageTool is a tool a stage runs and whether this machine has it, for the routing plan editor.
type stageTool struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Why       string `json:"why"`
	Required  bool   `json:"required"`
	Installed bool   `json:"installed"`
}

// stageTools lists, per readiness stage among stages, the tools it runs and whether each is installed.
func (r *Router) stageTools(stages []string) map[string][]stageTool {
	out := map[string][]stageTool{}
	if r.toolchain == nil {
		return out
	}
	var reqs []toolchain.Requirement
	for _, st := range stages {
		if step, ok := readiness.StepByID(st); ok {
			for _, n := range step.Tools {
				reqs = append(reqs, toolchain.Requirement{Tool: n.ID})
			}
		}
	}
	if len(reqs) == 0 {
		return out
	}
	items := map[string]toolchain.Item{}
	for _, it := range r.toolchain.Report(reqs, false).Items {
		items[it.ID] = it
	}
	for _, st := range stages {
		step, ok := readiness.StepByID(st)
		if !ok {
			continue
		}
		for _, n := range step.Tools {
			it := items[n.ID]
			out[st] = append(out[st], stageTool{ID: n.ID, Name: it.Name, Why: n.Why, Required: n.Required, Installed: it.Installed})
		}
	}
	return out
}

var toolVersionRe = regexp.MustCompile(`v?\d+\.\d+(?:\.\d+)?`)

// toolVersion is the first version number in a tool's version output, or "" when it prints none.
func toolVersion(out string) string {
	return toolVersionRe.FindString(out)
}
