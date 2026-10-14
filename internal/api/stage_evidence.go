package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/evidence"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/uat"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Test evidence: the Implementation stage does not end at "the code compiles". After the change is
// made, the agent runs the tests completely and records a video per test case with an evidence
// skill — the built-in test-evidence-video, or one the operator picks from the skill library. The
// videos and evidence.json land in the task's artifacts (evidence/), the console reports what was
// recorded, and the Evidence tab plays them. It is on by default; the routing plan can turn it off.

// evidenceStage is the stage that records test evidence.
const evidenceStage = "task_implementation"

// evidenceSetting is the task's evidence configuration for stage: enabled, and the skill to use
// ("" is the built-in skill).
func evidenceSetting(t *types.Task, stage string) (enabled bool, skill string) {
	if stage != evidenceStage {
		return false, ""
	}
	if rt := t.RouteFor(stage); rt != nil && rt.Evidence != nil {
		if rt.Evidence.Disabled {
			return false, ""
		}
		skill = rt.Evidence.Skill
	}
	if skill == evidence.SkillName {
		skill = ""
	}
	return true, skill
}

func (r *Router) evidenceDir(taskID string) string {
	return filepath.Join(r.artifactDir(taskID), "evidence")
}

func (r *Router) builtinEvidenceSkillDir() string {
	return filepath.Join(r.cfg.RootDir, ".sdlc", "tools", evidence.SkillName)
}

// builtinEvidenceSkill is the built-in skill as a skill contract, installed at dir.
func builtinEvidenceSkill(dir string) *types.UniversalSkillContract {
	return &types.UniversalSkillContract{Name: evidence.SkillName, Description: evidence.SkillDescription, Enabled: true,
		SourceLocation: dir, Metadata: map[string]interface{}{"instructions": evidence.SkillInstructions()}}
}

// evidencePrep is what the stage needs to record evidence.
type evidencePrep struct {
	Skill      *types.UniversalSkillContract
	Builtin    bool
	Dir        string // evidence directory (absolute)
	Recorder   string // command that records one case, for the built-in skill
	RecorderOK bool   // playwright-core is installed for the built-in recorder
}

// ensureEvidencePlaywright installs playwright-core for the built-in recorder, sharing the UAT
// runner's tool directory (and its lock) so both never install at once. The returned directory
// holds node_modules/playwright-core; "" when no real runner is configured (tests).
var ensureEvidencePlaywright = func(r *Router, ctx context.Context) (string, error) {
	pr, ok := r.screenshotRunner().(*uat.PlaywrightRunner)
	if !ok {
		return "", nil
	}
	return pr.Dir, pr.Ensure(ctx)
}

// prepareEvidence resolves the evidence skill for the task's current stage and readies its
// directory. It returns nil when the stage records no evidence, and notes for the operator.
func (r *Router) prepareEvidence(ctx context.Context, task *types.Task) (p *evidencePrep, notes []string) {
	enabled, name := evidenceSetting(task, task.CurrentStageID)
	if !enabled {
		return nil, nil
	}
	p = &evidencePrep{Dir: r.evidenceDir(task.ID)}
	_ = os.MkdirAll(p.Dir, 0o755)
	if name != "" {
		if s, ok := r.attachableSkills()[name]; ok {
			p.Skill = s
			return p, nil
		}
		notes = append(notes, fmt.Sprintf("⚠ Evidence skill %s is disabled or no longer installed — using the built-in %s instead. "+
			"Pick another in the Routing tab.", name, evidence.SkillName))
	}
	dir := r.builtinEvidenceSkillDir()
	if err := evidence.InstallSkill(dir); err != nil {
		return nil, append(notes, fmt.Sprintf("⚠ Could not install the built-in evidence skill (%v); the stage runs without test evidence.", err))
	}
	p.Skill, p.Builtin = builtinEvidenceSkill(dir), true
	installCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	pwDir, err := ensureEvidencePlaywright(r, installCtx)
	script := filepath.Join(dir, filepath.FromSlash(evidence.RecorderScript))
	p.Recorder = fmt.Sprintf("node %s %s --case <id>", shellQuote(script), shellQuote(filepath.Join(p.Dir, "storyboard.json")))
	if pwDir != "" {
		p.Recorder = "EVIDENCE_PLAYWRIGHT_DIR=" + shellQuote(pwDir) + " " + p.Recorder
	}
	p.RecorderOK = err == nil
	if err != nil {
		notes = append(notes, fmt.Sprintf("⚠ The evidence recorder could not install Playwright (%v). The agent still runs the tests completely "+
			"and records evidence with the repository's own tooling where it can.", err))
	}
	return p, notes
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " '\"$`\\!*?&;|<>(){}[]#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// evidenceRunPrompt is the turn that records evidence for an implementation already in the
// worktree (the operator pressed Record evidence): test and record, do not re-implement.
func evidenceRunPrompt(t *types.Task, sameSession bool) string {
	var b strings.Builder
	if sameSession {
		fmt.Fprintf(&b, "Continue in this conversation on %s: the implementation is done. ", stageLabel(t.CurrentStageID))
	} else {
		fmt.Fprintf(&b, "Stage: %s — the implementation of this task is already in the working directory.\n\nTask: %s\n", stageLabel(t.CurrentStageID), t.Title)
		if t.Description != "" {
			fmt.Fprintf(&b, "Description: %s\n", t.Description)
		}
		b.WriteString("\nRead the changes first (git diff against the base branch). ")
	}
	b.WriteString("Now test the change completely and record the test evidence as described below. Do not re-implement or refactor; " +
		"change code only when a test exposes a bug in this change, and say so in the Summary.")
	b.WriteString("\n\nEnd with the \"## Test evidence\" section and a short \"Summary\" the operator can review.")
	return b.String()
}

// evidenceBrief is the stage-prompt section that asks for complete testing and video evidence.
func (r *Router) evidenceBrief(task *types.Task, p *evidencePrep) string {
	var b strings.Builder
	b.WriteString("\n\n## Test evidence (required before you finish this stage)\n")
	fmt.Fprintf(&b, "When the implementation is done, test it completely and record video evidence with the skill %q (its instructions are in the Skills section).\n", p.Skill.Name)
	fmt.Fprintf(&b, "- Evidence directory (absolute, outside the repository): %s — save every evidence file there and nowhere in the repository. "+
		"It may hold evidence from an earlier run: re-record every case and delete files that no longer apply.\n", p.Dir)
	b.WriteString("- Run the complete test suites of every repository you changed, for real. Fix what your change broke and re-run until they pass; " +
		"a failure that also happens without your change is pre-existing — keep it and say so. Never report a result you did not run.\n")
	b.WriteString("- Make it reviewable step by step by someone who did not write the change: per case the starting state (Given), " +
		"each action with a caption, a check after every action that proves its result, and the proof pointed out. " +
		"A change with any UI gets a walkthrough of the screen it changes in the app running locally; a backend change gets calls to its API " +
		"running locally. Cover the negative and edge cases the acceptance criteria name.\n")
	if doc, err := r.resolveArtifactPath(task.ID, stageDocName("atdd_creation")); err == nil {
		if _, err := os.Stat(doc); err == nil {
			fmt.Fprintf(&b, "- Record one case per acceptance test case in %s (its atdd-cases block), using the case id as the evidence case id, plus one case per full suite run.\n", doc)
		}
	}
	if p.Builtin {
		fmt.Fprintf(&b, "- Record one case per call (timeout 600000 ms): %s\n", p.Recorder)
		if !p.RecorderOK {
			b.WriteString("  The recorder's Playwright is not installed on this machine: if the command fails, record with the repository's own tooling " +
				"(e.g. Playwright or Capybara video) instead, and write evidence.json yourself.\n")
		}
	} else {
		fmt.Fprintf(&b, "- The skill may write its videos elsewhere (e.g. tmp/ in the repository): copy each final video into %s, and remove the copies from the repository.\n", p.Dir)
	}
	fmt.Fprintf(&b, "- %s/%s lists the evidence (the built-in recorder writes it; otherwise write it yourself):\n", p.Dir, evidence.ManifestFile)
	b.WriteString(`  {"cases": [{"id": "ABC-TC-001", "title": "…", "covers": ["ABC-TC-001"], "status": "passed|failed|blocked", "video": "ABC-TC-001.mp4", "notes": "…"}],` + "\n" +
		`   "suites": [{"repository": "…", "command": "…", "status": "passed|failed", "passed": 0, "failed": 0}]}` + "\n")
	b.WriteString("- Use fake test data only, never a production or shared environment, and never put credentials into commands, storyboards or videos.\n")
	b.WriteString("- Add a \"## Test evidence\" section to your report: case id, title, status and video file per case, and the suite results per repository.")
	return b.String()
}

// reportEvidence tells the operator what evidence the stage recorded, and stores its status on the
// task so the board and the Evidence tab show it.
func (r *Router) reportEvidence(taskID, turnID string, p *evidencePrep) {
	rep := evidence.Collect(p.Dir)
	status := rep.Status()
	r.mu.Lock()
	if t, ok := r.tasks[taskID]; ok {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		t.Metadata["evidence_status"] = status
		t.Metadata["evidence_videos"] = fmt.Sprint(len(rep.Videos))
	}
	r.mu.Unlock()

	var msg string
	kind := types.ConsoleKindSystem
	switch status {
	case "missing":
		msg = fmt.Sprintf("⚠ No test evidence video was recorded (expected in %s). Type a message asking the agent to record it, "+
			"or reject the stage with feedback.", p.Dir)
	default:
		msg = fmt.Sprintf("🎬 Test evidence: %d video%s", len(rep.Videos), plural(len(rep.Videos)))
		if rep.Manifest != nil {
			msg += fmt.Sprintf(" — %d case%s passed", rep.Passed, plural(rep.Passed))
			if rep.Failed > 0 {
				msg += fmt.Sprintf(", %d failed or blocked", rep.Failed)
			}
		}
		msg += ". Watch them in the Evidence tab before approving."
		var bad []string
		if rep.Manifest != nil {
			for _, c := range rep.Manifest.Cases {
				switch {
				case c.Status != "passed":
					bad = append(bad, fmt.Sprintf("• %s %s — %s", c.ID, c.Status, c.Title))
				case c.VideoMissing:
					bad = append(bad, fmt.Sprintf("• %s — evidence.json names %s, which is not in the evidence folder", c.ID, c.Video))
				}
			}
			for _, s := range rep.Manifest.Suites {
				if s.Status != "passed" {
					bad = append(bad, fmt.Sprintf("• suite %s %s (%s)", s.Repository, s.Status, s.Command))
				}
			}
		}
		if rep.ManifestError != "" {
			bad = append(bad, "• "+rep.ManifestError)
		} else if rep.Manifest == nil {
			bad = append(bad, "• no evidence.json: the videos are listed without case results")
		}
		if len(bad) > 0 {
			msg += "\n" + strings.Join(bad, "\n")
		}
		if status == "failed" {
			kind = types.ConsoleKindError
		}
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: kind, TurnID: turnID, Content: msg})
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// handleTaskEvidence serves /api/v1/tasks/{id}/evidence. GET: the evidence setting and what the
// evidence directory holds (files come from /api/v1/artifacts/{id}/evidence/{path}). POST: run an
// evidence turn on the Implementation stage — test the change and record the videos.
func (r *Router) handleTaskEvidence(w http.ResponseWriter, req *http.Request, task *types.Task) {
	if req.Method == http.MethodPost {
		if reason := evidenceBlocked(task); reason != "" {
			r.writeJSON(w, http.StatusConflict, map[string]interface{}{"status": "not_started", "task_id": task.ID, "error": reason})
			return
		}
		r.addEntry(task.ID, types.ConsoleEntry{Kind: types.ConsoleKindUser, Content: "Record test evidence for the implementation"})
		r.respondStageStart(w, task.ID, r.startStage(task.ID, "evidence", ""))
		return
	}
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET or POST required for evidence")
		return
	}
	enabled, skill := evidenceSetting(task, evidenceStage)
	if skill == "" {
		skill = evidence.SkillName
	}
	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"task_id": task.ID, "enabled": enabled, "skill": skill, "stage_id": evidenceStage, "record_blocked": evidenceBlocked(task),
		"base_path": "evidence/", "report": evidence.Collect(r.evidenceDir(task.ID)),
	})
}

// evidenceBlocked says why evidence cannot be recorded for the task right now, or "".
func evidenceBlocked(t *types.Task) string {
	switch {
	case t.CurrentStageID != evidenceStage:
		return fmt.Sprintf("test evidence is recorded on the Implementation stage; this task is on %s", t.CurrentStageID)
	case t.State == types.TaskStateRunning:
		return "the agent is already running this task — record evidence when the turn ends"
	case t.State == types.TaskStateCompleted:
		return "the task is completed"
	case len(t.AssignedRepos) == 0:
		return "assign the task's repositories first"
	}
	if on, _ := evidenceSetting(t, evidenceStage); !on {
		return "test evidence is off for this task — turn it on under Evidence settings"
	}
	return ""
}

// evidenceSkillOption describes the built-in evidence skill for the plan editor.
func evidenceSkillOption() skillOption {
	return skillOption{Name: evidence.SkillName, Description: evidence.SkillDescription, Source: "built in"}
}

// evidenceSkillHints mark library skills that record test evidence, for the evidence picker.
var evidenceSkillHints = []string{"test-evidence", "test-video", "record-test", "evidence", "video"}

// evidenceSuggestions proposes library skills for the evidence picker by whole-word name matches.
func evidenceSuggestions(options []skillOption) []string {
	out := []string{}
	for _, o := range options {
		name := "-" + strings.ReplaceAll(strings.ToLower(o.Name), "_", "-") + "-"
		for _, h := range evidenceSkillHints {
			if strings.Contains(name, "-"+h+"-") {
				out = append(out, o.Name)
				break
			}
		}
		if len(out) == maxSkillSuggestions {
			break
		}
	}
	return out
}

// normalizeStageEvidence checks a stage's evidence setting: only the Implementation stage records
// evidence, and a library skill must be attachable.
func normalizeStageEvidence(stage string, e *types.StageEvidence, available map[string]*types.UniversalSkillContract) (*types.StageEvidence, error) {
	if e == nil {
		return nil, nil
	}
	if stage != evidenceStage {
		return nil, fmt.Errorf("test evidence is recorded by %s, not %s", evidenceStage, stage)
	}
	out := &types.StageEvidence{Disabled: e.Disabled, Skill: strings.TrimSpace(e.Skill)}
	if out.Skill == evidence.SkillName {
		out.Skill = ""
	}
	if out.Skill != "" {
		if _, ok := available[out.Skill]; !ok {
			return nil, fmt.Errorf("evidence skill %q is not available: it is disabled, no longer installed, or has no instructions", out.Skill)
		}
	}
	if !out.Disabled && out.Skill == "" {
		return nil, nil // the default
	}
	return out, nil
}
