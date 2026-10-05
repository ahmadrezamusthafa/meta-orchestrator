package api

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Stage skills: the operator attaches skills (SKILL.md instruction bundles) to stages in the
// routing plan. When a stage runs, the instructions of its skills go into the stage brief and
// each skill's directory is made readable to the agent, so it can open the files a skill refers
// to. After the turn the console reports which skill files the agent actually opened.

const (
	maxSkillsPerStage = 5
	// maxSkillInstructionBytes caps one skill's instructions in the brief; the full file stays readable.
	maxSkillInstructionBytes = 24 * 1024
)

// skillOption is a skill the operator can attach to a stage.
type skillOption struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// skillInstructions is the instruction body of a skill, or "" when it has none (MCP servers,
// built-in tools and script-only bundles cannot be attached to a stage).
func skillInstructions(s *types.UniversalSkillContract) string {
	if s == nil || s.Metadata == nil {
		return ""
	}
	inst, _ := s.Metadata["instructions"].(string)
	return strings.TrimSpace(inst)
}

// attachableSkills lists the enabled skills that carry instructions, keyed by name.
func (r *Router) attachableSkills() map[string]*types.UniversalSkillContract {
	out := map[string]*types.UniversalSkillContract{}
	if r.cfg.SkillResolver == nil {
		return out
	}
	list, err := r.cfg.SkillResolver.ListSkills()
	if err != nil {
		return out
	}
	for _, s := range list {
		if s.Enabled && skillInstructions(s) != "" {
			out[s.Name] = s
		}
	}
	return out
}

// skillOptions lists the attachable skills, sorted by name, for the plan editor.
func (r *Router) skillOptions() []skillOption {
	skills := r.attachableSkills()
	out := make([]skillOption, 0, len(skills))
	for _, s := range skills {
		out = append(out, skillOption{Name: s.Name, Description: s.Description, Source: skillSourceLabel(s.SourceLocation)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// skillSourceLabel names where a skill lives: "~/.claude/skills" or "billing/.claude/skills".
func skillSourceLabel(dir string) string {
	if dir == "" {
		return ""
	}
	parent := filepath.Dir(dir)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, parent); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	parts := strings.Split(filepath.ToSlash(parent), "/")
	if len(parts) > 3 {
		parts = parts[len(parts)-3:]
	}
	return strings.Join(parts, "/")
}

// stageSkillHints are name phrases that mark a skill as relevant to a stage, with their weight.
var stageSkillHints = map[string]map[string]int{
	"prd_discovery":       {"prd": 3, "product-brief": 3, "requirements": 2, "impact-analysis": 2},
	"atdd_creation":       {"atdd": 3, "acceptance-test": 3, "test-case": 2},
	"techdoc_rfc":         {"tech-doc": 3, "techdoc": 3, "rfc": 3, "architecture": 2},
	"task_breakdown":      {"breakdown": 3, "task-plan": 3, "epics-and-stories": 2, "task-detailing": 2},
	"task_implementation": {"implement": 3, "dev-story": 2, "quick-dev": 2},
	"e2e_validation":      {"e2e": 3, "rspec": 2, "test-evidence": 2, "test-video": 2},
	"uat_verification":    {"uat": 3, "manual-test": 3},
	"signoff_merge":       {"release-doc": 3, "pull-request": 3, "delivery": 2},
}

const maxSkillSuggestions = 3

// suggestSkills proposes skills for a stage by whole-word phrase matches on the skill name.
// Suggestions are only offered to the operator; nothing is attached without their choice.
func suggestSkills(stage string, options []skillOption) []string {
	hints := stageSkillHints[stage]
	if len(hints) == 0 {
		return nil
	}
	type scored struct {
		name  string
		score int
	}
	var hits []scored
	for _, o := range options {
		if strings.Contains(strings.ToUpper(o.Description), "DEPRECATED") {
			continue
		}
		name := "-" + strings.ReplaceAll(strings.ToLower(o.Name), "_", "-") + "-"
		score := 0
		for phrase, w := range hints {
			if strings.Contains(name, "-"+phrase+"-") {
				score += w
			}
		}
		if score > 0 {
			hits = append(hits, scored{o.Name, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].name < hits[j].name
	})
	out := []string{}
	for _, h := range hits {
		if len(out) == maxSkillSuggestions {
			break
		}
		out = append(out, h.name)
	}
	return out
}

// skillSuggestions maps each stage to its suggested skills.
func skillSuggestions(stages []string, options []skillOption) map[string][]string {
	out := map[string][]string{}
	for _, st := range stages {
		if s := suggestSkills(st, options); len(s) > 0 {
			out[st] = s
		}
	}
	return out
}

// normalizeStageSkills trims, de-duplicates and checks a stage's attached skills.
func normalizeStageSkills(stage string, names []string, available map[string]*types.UniversalSkillContract) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		if _, ok := available[n]; !ok {
			return nil, fmt.Errorf("skill %q for %s is not available: it is disabled, no longer installed, or has no instructions", n, stage)
		}
		out = append(out, n)
	}
	if len(out) > maxSkillsPerStage {
		return nil, fmt.Errorf("%s has %d skills; attach at most %d so the brief stays focused", stage, len(out), maxSkillsPerStage)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// stageSkills resolves the skills attached to the task's current stage. Missing lists the
// attached names that can no longer be used (disabled or removed since the plan was saved).
func (r *Router) stageSkills(task *types.Task) (loaded []*types.UniversalSkillContract, missing []string) {
	rt := task.RouteFor(task.CurrentStageID)
	if rt == nil || len(rt.Skills) == 0 {
		return nil, nil
	}
	available := r.attachableSkills()
	for _, n := range rt.Skills {
		if s, ok := available[n]; ok {
			loaded = append(loaded, s)
		} else {
			missing = append(missing, n)
		}
	}
	return loaded, missing
}

// skillsBrief is the stage-prompt section carrying the attached skills' instructions.
func skillsBrief(skills []*types.UniversalSkillContract) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nSkills for this stage — the operator attached these; apply them while doing the stage. ")
	b.WriteString("Their instructions are included below, so you do not need to load them again. ")
	b.WriteString("Paths a skill mentions are relative to its directory; open those files from there when the skill needs them. ")
	b.WriteString("This stage runs unattended: where a skill says to ask the user or wait for input, make a reasonable assumption and list it in the Summary. ")
	b.WriteString("Where a skill conflicts with this stage's required output (for example a fenced JSON block), the stage's requirements win.\n")
	for _, s := range skills {
		inst := skillInstructions(s)
		truncated := false
		if len(inst) > maxSkillInstructionBytes {
			inst, truncated = inst[:maxSkillInstructionBytes], true
		}
		fmt.Fprintf(&b, "\n<skill name=%q directory=%q>\n", s.Name, s.SourceLocation)
		if s.Description != "" {
			fmt.Fprintf(&b, "Purpose: %s\n\n", s.Description)
		}
		b.WriteString(inst)
		if truncated {
			fmt.Fprintf(&b, "\n[… truncated; read the rest from %s]", filepath.Join(s.SourceLocation, "SKILL.md"))
		}
		b.WriteString("\n</skill>\n")
	}
	return b.String()
}

// skillDirs are the skill directories the agent must be able to read.
func skillDirs(skills []*types.UniversalSkillContract) []string {
	var out []string
	for _, s := range skills {
		if s.SourceLocation != "" {
			out = append(out, s.SourceLocation)
		}
	}
	return out
}

func skillNames(skills []*types.UniversalSkillContract) []string {
	out := make([]string, 0, len(skills))
	for _, s := range skills {
		out = append(out, s.Name)
	}
	return out
}

// skillUsageReport tells the operator, per attached skill, what the agent did with it during
// the turn: invoked it through the Skill tool and/or opened files from its directory.
func skillUsageReport(skills []*types.UniversalSkillContract, calls []llm.ToolCall) string {
	if len(skills) == 0 {
		return ""
	}
	lines := make([]string, 0, len(skills))
	for _, s := range skills {
		invoked := false
		files := []string{}
		seen := map[string]bool{}
		for _, c := range calls {
			if c.Name == "Skill" {
				for _, k := range []string{"skill", "command", "name"} {
					if v, _ := c.Arguments[k].(string); strings.TrimPrefix(v, "/") == s.Name {
						invoked = true
					}
				}
				continue
			}
			for _, v := range c.Arguments {
				str, ok := v.(string)
				if !ok || s.SourceLocation == "" || !strings.Contains(str, s.SourceLocation) {
					continue
				}
				f := str
				if fp, _ := c.Arguments["file_path"].(string); fp != "" {
					f = fp
				}
				if rel, err := filepath.Rel(s.SourceLocation, f); err == nil && !strings.HasPrefix(rel, "..") {
					f = rel
				}
				if !seen[f] {
					seen[f] = true
					files = append(files, f)
				}
				break
			}
		}
		var what []string
		if invoked {
			what = append(what, "invoked it through the Skill tool")
		}
		if len(files) > 0 {
			shown := files
			if len(shown) > 4 {
				shown = append(append([]string(nil), shown[:4]...), fmt.Sprintf("+%d more", len(files)-4))
			}
			what = append(what, fmt.Sprintf("opened %d of its files (%s)", len(files), strings.Join(shown, ", ")))
		}
		if len(what) == 0 {
			what = append(what, "worked from the instructions in its brief and opened none of its files")
		}
		lines = append(lines, fmt.Sprintf("• %s — %s", s.Name, strings.Join(what, "; ")))
	}
	return "Skill usage this run:\n" + strings.Join(lines, "\n")
}
