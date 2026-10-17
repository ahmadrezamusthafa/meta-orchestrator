package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Agent profiles (configs/profiles.json) give each pipeline stage a role: its persona goes into the
// stage's system prompt, its max_token_budget caps the stage's token budget, and its denied_skills
// cannot be attached to the stage. allowed_skills is not enforced: it names abstract capabilities
// (read_ast_node, write_source_code, …) rather than installed skills, so treating it as an allow
// list would make every real skill unattachable.

// stageRoles maps a pipeline stage to the profile role that performs it. signoff_merge has no
// matching profile and runs without one.
var stageRoles = map[string]string{
	"prd_discovery":       "product_manager",
	"techdoc_rfc":         "system_architect",
	"task_breakdown":      "system_architect",
	"atdd_creation":       "atdd_qa_engineer",
	"e2e_validation":      "atdd_qa_engineer",
	"uat_verification":    "atdd_qa_engineer",
	"task_implementation": "lead_developer",
}

// profileRegistry returns the configured profiles, or loads RootDir/configs/profiles.json once.
func (r *Router) profileRegistry() *registry.ProfileRegistry {
	r.profilesOnce.Do(func() {
		r.profiles = r.cfg.ProfileRegistry
		if r.profiles == nil {
			r.profiles = registry.NewProfileRegistry()
			_ = r.profiles.LoadFromFile(filepath.Join(r.cfg.RootDir, "configs", "profiles.json"))
		}
	})
	return r.profiles
}

// stageProfile is the profile of the stage's role, or nil.
func (r *Router) stageProfile(stage string) *registry.AgentProfile {
	role, ok := stageRoles[stage]
	if !ok {
		return nil
	}
	p, err := r.profileRegistry().GetProfile(role)
	if err != nil {
		return nil
	}
	return p
}

// stageRole is the role name of a stage ("" when it has none).
func (r *Router) stageRole(stage string) string {
	if p := r.stageProfile(stage); p != nil {
		return p.Role
	}
	return ""
}

// checkStageSkillAllowed rejects a skill the stage's role explicitly denies.
func (r *Router) checkStageSkillAllowed(stage, skill string) error {
	if p := r.stageProfile(stage); p != nil && r.profileRegistry().IsDenied(p.Role, skill) {
		return fmt.Errorf("skill %q cannot be attached to %s: the %s profile denies it (configs/profiles.json)", skill, stage, p.Name)
	}
	return nil
}

// stageTokenBudget caps the routed token budget with the role's max_token_budget.
func (r *Router) stageTokenBudget(stage string, routed int64) int64 {
	p := r.stageProfile(stage)
	if p == nil || p.MaxTokenBudget <= 0 {
		return routed
	}
	if routed <= 0 || routed > p.MaxTokenBudget {
		return p.MaxTokenBudget
	}
	return routed
}

// stagePersona is the system-prompt section describing the stage's role, with the profile's
// prompt template when that file exists. Template sections that need values the orchestrator does
// not fill ({{placeholders}}) are left out.
func (r *Router) stagePersona(task *types.Task) string {
	p := r.stageProfile(task.CurrentStageID)
	if p == nil {
		return ""
	}
	out := fmt.Sprintf("\n\nYour role for this stage: %s — %s.", p.Name, strings.TrimSuffix(p.Description, "."))
	if p.SystemPromptTemplate == "" {
		return out
	}
	path := p.SystemPromptTemplate
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.cfg.RootDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	if tmpl := withoutPlaceholderSections(string(data)); tmpl != "" {
		out += "\n" + tmpl
	}
	return out
}

// withoutPlaceholderSections drops every "## " section that contains a {{placeholder}}.
func withoutPlaceholderSections(md string) string {
	var keep []string
	for i, sec := range strings.Split(md, "\n## ") {
		if strings.Contains(sec, "{{") {
			continue
		}
		if i > 0 {
			sec = "## " + sec
		}
		keep = append(keep, strings.TrimSpace(sec))
	}
	return strings.TrimSpace(strings.Join(keep, "\n\n"))
}
