package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/atdd"
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
	Presenter  string // command that records the presentation video, for the built-in skill
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
	r.writeEvidenceSpec(task, p.Dir)
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
	base := fmt.Sprintf("node %s %s", shellQuote(script), shellQuote(filepath.Join(p.Dir, "storyboard.json")))
	if pwDir != "" {
		base = "EVIDENCE_PLAYWRIGHT_DIR=" + shellQuote(pwDir) + " " + base
	}
	p.Recorder, p.Presenter = base+" --case <id>", base+" --summary"
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
	fmt.Fprintf(&b, "When the implementation is done, prove the feature works as specified — end to end, in the running system — and record the evidence with the skill %q (its instructions are in the Skills section).\n", p.Skill.Name)
	fmt.Fprintf(&b, "- Evidence directory (absolute, outside the repository): %s — save every evidence file there and nowhere in the repository. "+
		"It may hold evidence from an earlier run: re-record every case and delete files that no longer apply.\n", p.Dir)
	b.WriteString("- Run the complete test suites of every repository you changed, for real. Fix what your change broke and re-run until they pass; " +
		"a failure that also happens without your change is pre-existing — keep it and say so. Never report a result you did not run.\n")
	if p.Builtin {
		fmt.Fprintf(&b, "- Record one case per call (timeout 600000 ms): %s\n", p.Recorder)
		fmt.Fprintf(&b, "- When every case is recorded, record the presentation — one video that presents the whole evidence to the reviewer "+
			"(verdict, spec coverage, a chapter per case with its key steps, results) — and again after any re-take (timeout 600000 ms): %s\n", p.Presenter)
		if !p.RecorderOK {
			b.WriteString("  The recorder's Playwright is not installed on this machine: if the command fails, record with the repository's own tooling " +
				"(e.g. Playwright or Capybara video) instead, and write evidence.json yourself.\n")
		}
	} else {
		fmt.Fprintf(&b, "- The skill may write its videos elsewhere (e.g. tmp/ in the repository): copy each final video into %s, and remove the copies from the repository.\n", p.Dir)
	}
	fmt.Fprintf(&b, "- %s/%s lists the evidence (the built-in recorder writes it; otherwise write it yourself):\n", p.Dir, evidence.ManifestFile)
	b.WriteString(`  {"criteria": [{"id": "AC-1", "text": "…"}],` + "\n" +
		`   "cases": [{"id": "ABC-TC-001", "kind": "flow|acceptance|suite", "title": "…", "covers": ["ABC-TC-001"], "status": "passed|failed|blocked", "video": "ABC-TC-001.mp4", "notes": "…"}],` + "\n" +
		`   "suites": [{"repository": "…", "command": "…", "status": "passed|failed", "passed": 0, "failed": 0}]}` + "\n")
	b.WriteString("- Use fake test data only, never a production or shared environment, and never put credentials into commands, storyboards or videos.\n")
	b.WriteString("- Add a \"## Test evidence\" section to your report: per case its id, kind, title, status and video file; the acceptance cases " +
		"without evidence and why; and the suite results per repository.")
	b.WriteString(r.specBrief(task))
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
		if t := r.taskSnapshot(taskID); t != nil {
			spec := r.evidenceSpec(t, rep)
			var missing []string
			for _, c := range spec.Cases {
				if c.Status == "missing" {
					missing = append(missing, c.ID)
				}
			}
			if len(spec.Cases) > 0 {
				msg += fmt.Sprintf("\nSpec coverage: %d of %d acceptance case%s have evidence.", spec.Covered, len(spec.Cases), plural(len(spec.Cases)))
			}
			if len(missing) > 0 {
				bad = append(bad, "• no evidence for "+strings.Join(missing, ", "))
			}
		}
		if sm := manifestSummary(rep); sm != nil && sm.Video != "" && !sm.VideoMissing {
			msg += fmt.Sprintf("\n▶ Presentation: %s (%s, %d chapters) — the whole evidence in one video.", sm.Video, (time.Duration(sm.DurationS) * time.Second).String(), len(sm.Chapters))
		} else if p.Builtin {
			bad = append(bad, "• no presentation video (SUMMARY.mp4): ask the agent to record it with --summary")
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
	r.finalizeEvidenceAsync(taskID, turnID, p.Dir)
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
	rep := evidence.Collect(r.evidenceDir(task.ID))
	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"task_id": task.ID, "enabled": enabled, "skill": skill, "stage_id": evidenceStage, "record_blocked": evidenceBlocked(task),
		"base_path": "evidence/", "report": rep, "spec": r.evidenceSpec(task, rep),
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

// ---------------------------------------------------------------------------------------------
// The spec the evidence must prove. The evidence is planned from the spec — the acceptance test
// cases and the documents they come from — not from the diff: it shows the feature working as
// specified, end to end, in the running system.
// ---------------------------------------------------------------------------------------------

// specCase is one acceptance test case the evidence must prove, with the evidence that covers it.
type specCase struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Priority string        `json:"priority,omitempty"`
	Platform string        `json:"platform,omitempty"`
	Kind     string        `json:"kind,omitempty"` // UAT, Sanity
	Status   string        `json:"status"`         // passed, failed, blocked, missing (no evidence covers it)
	Evidence []evidenceRef `json:"evidence"`
	atdd     *atdd.Case
}

type evidenceRef struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// evidenceSpecView is the spec and how far the recorded evidence covers it.
type evidenceSpecView struct {
	Source  string     `json:"source,omitempty"` // where the cases come from
	Docs    []string   `json:"docs"`             // spec documents in the task's artifacts
	Cases   []specCase `json:"cases"`
	Covered int        `json:"covered"`
}

func manifestSummary(rep evidence.Report) *evidence.Summary {
	if rep.Manifest == nil {
		return nil
	}
	return rep.Manifest.Summary
}

// writeEvidenceSpec writes the acceptance cases the evidence must prove into the evidence
// directory, for the recorder's presentation (its coverage slide and verdict).
func (r *Router) writeEvidenceSpec(t *types.Task, dir string) {
	path := filepath.Join(dir, evidence.SpecFile)
	cases, source, err := r.loadATDD(t)
	if err != nil || len(cases) == 0 {
		_ = os.Remove(path)
		return
	}
	type item struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Priority string `json:"priority,omitempty"`
		Platform string `json:"platform,omitempty"`
	}
	out := struct {
		Source string `json:"source"`
		Cases  []item `json:"cases"`
	}{Source: source}
	for _, c := range cases {
		out.Cases = append(out.Cases, item{c.ID, c.Title, c.Priority, c.Platform})
	}
	if raw, err := json.MarshalIndent(out, "", "  "); err == nil {
		_ = os.WriteFile(path, raw, 0o644)
	}
}

// specDocs are the task documents that specify the feature, in reading order.
func (r *Router) specDocs(taskID string) []string {
	var out []string
	for _, name := range []string{"PRD.md", stageDocName("prd_discovery"), stageDocName("atdd_creation"), stageDocName("techdoc_rfc"),
		stageDocName("task_breakdown"), stageDocName("contract_spec"), stageDocName("hotfix_reproduction")} {
		if p, err := r.resolveArtifactPath(taskID, name); err == nil && fileExists(p) {
			out = append(out, name)
		}
	}
	return out
}

// evidenceSpec lists what the evidence must prove: the task's ATDD cases, or else the acceptance
// criteria the agent listed in evidence.json, each with the evidence cases that cover it.
func (r *Router) evidenceSpec(t *types.Task, rep evidence.Report) evidenceSpecView {
	v := evidenceSpecView{Docs: r.specDocs(t.ID), Cases: []specCase{}}
	if cases, source, err := r.loadATDD(t); err == nil && len(cases) > 0 {
		v.Source = source
		for i := range cases {
			c := &cases[i]
			v.Cases = append(v.Cases, specCase{ID: c.ID, Title: c.Title, Priority: c.Priority, Platform: c.Platform, Kind: c.Kind(), atdd: c})
		}
	} else if rep.Manifest != nil && len(rep.Manifest.Criteria) > 0 {
		v.Source = "acceptance criteria listed in evidence.json"
		for _, c := range rep.Manifest.Criteria {
			v.Cases = append(v.Cases, specCase{ID: c.ID, Title: c.Text})
		}
	}
	for i := range v.Cases {
		sc := &v.Cases[i]
		sc.Evidence, sc.Status = []evidenceRef{}, "missing"
		if rep.Manifest == nil {
			continue
		}
		for _, c := range rep.Manifest.Cases {
			if c.Kind != evidence.KindSuite && c.Proves(sc.ID) {
				sc.Evidence = append(sc.Evidence, evidenceRef{ID: c.ID, Status: c.Status})
			}
		}
		sc.Status = coverageStatus(sc.Evidence)
		if sc.Status != "missing" {
			v.Covered++
		}
	}
	return v
}

// coverageStatus: failed when any covering case failed, passed when one passed, else blocked.
func coverageStatus(refs []evidenceRef) string {
	if len(refs) == 0 {
		return "missing"
	}
	status := "blocked"
	for _, e := range refs {
		switch e.Status {
		case "failed":
			return "failed"
		case "passed":
			status = "passed"
		}
	}
	return status
}

// maxBriefedCases bounds how many acceptance cases the brief spells out in full; the rest are
// listed by id and title (the agent reads them from the source).
const maxBriefedCases = 30

// specBrief is the part of the evidence brief that says what to prove and against what.
func (r *Router) specBrief(t *types.Task) string {
	var b strings.Builder
	b.WriteString("\n### What to prove: the changed flows, end to end\n")
	b.WriteString("Prove that every flow this change touches works end to end, as specified, in the running system — the way it runs for its users. " +
		"Read the diff to find each entry point the change affects: screens, API endpoints, scheduled jobs and schedulers, workers and queue " +
		"consumers, CLI commands, notifications and alerts. Each gets end-to-end evidence through that entry point, and the spec says what it must do. " +
		"The evidence must show the system working: the UI, the API's responses, the terminal of a command, the live log of a job or worker " +
		"(start it, trigger it, watch its log for what must — and must not — happen). Unit and integration test runs are regression only " +
		"(the SUITE cases): the recorder refuses a flow or acceptance case made only of them.\n")
	if docs := r.specDocs(t.ID); len(docs) > 0 {
		b.WriteString("Spec documents (read them first; take the user journeys, screens, rules, API contracts and acceptance criteria from them):\n")
		for _, d := range docs {
			if p, err := r.resolveArtifactPath(t.ID, d); err == nil {
				fmt.Fprintf(&b, "- %s\n", p)
			}
		}
	}

	if apps := r.taskApps(t); len(apps) > 0 {
		b.WriteString("Applications of this task (run the changed ones locally from the task worktrees):\n")
		for _, a := range apps {
			fmt.Fprintf(&b, "- %s — %s (%s)", a.ID, a.Name, a.Kind)
			if len(a.Repos) > 0 {
				fmt.Fprintf(&b, "; repositories: %s", strings.Join(a.Repos, ", "))
			}
			if len(a.SignIn) > 0 {
				fmt.Fprintf(&b, "; sign in: %s", strings.Join(a.SignIn, " → "))
			}
			b.WriteString("\n")
		}
	}

	cases, source, err := r.loadATDD(t)
	switch {
	case err != nil:
		fmt.Fprintf(&b, "The ATDD sheet could not be read (%v): derive the acceptance criteria from the spec documents instead, as below.\n", err)
		fallthrough
	case len(cases) == 0:
		b.WriteString("There are no ATDD cases for this task. Derive the acceptance criteria from the spec documents, number them AC-1, AC-2, … " +
			"and list them in the storyboard's \"criteria\" ([{\"id\": \"AC-1\", \"text\": \"…\"}]); record one acceptance case per criterion " +
			"with that id. The Evidence tab checks every criterion is covered.\n")
	default:
		fmt.Fprintf(&b, "Acceptance cases to prove (%d, from %s) — record each as its own case with the same id, its preconditions as `given`, "+
			"its steps driven through the running system, and a check for every expected result. The Evidence tab shows which cases have no "+
			"evidence:\n", len(cases), source)
		for i, c := range cases {
			if i >= maxBriefedCases {
				fmt.Fprintf(&b, "- %s — %s\n", c.ID, c.Title)
				continue
			}
			fmt.Fprintf(&b, "\n#### %s — %s (%s)\n", c.ID, c.Title, strings.Join(nonEmpty(c.Priority, c.Platform, c.TestLevel, c.Kind()), ", "))
			writeList(&b, "Preconditions", c.Preconditions)
			writeList(&b, "Steps", c.Steps)
			writeList(&b, "Expected", c.ExpectedResults)
		}
		b.WriteString("\n")
	}

	b.WriteString("Record three kinds of case (\"kind\" in the storyboard):\n" +
		"1. `flow` — FLOW-1, FLOW-2, …: each main user journey of the feature in one continuous video, from where the user starts " +
		"(sign in, the menu) through every screen of the journey to the final outcome, including what other applications show of it " +
		"(the record in the admin screen, the API's response). This is the demo that the feature works completely; most features have 1–3.\n" +
		"2. `acceptance` — one case per acceptance case or criterion, as above, including the negative and edge cases: validation " +
		"errors, empty states, permissions. A WEB case is a browser walkthrough; an API case is `api` steps against the running service " +
		"(and the screen that shows the result, when there is one); a job, scheduler, worker or consumer case starts it (`start`), " +
		"triggers it, and checks its live log (`watch`, `expect_log`) and the outcome (the API, the database through a `run`, the alert " +
		"a mock receiver got).\n" +
		"3. `suite` — SUITE-<repo>: the full test suite of each repository you changed, as regression.\n" +
		"An existing end-to-end or system test that drives the real app (Playwright, Cypress, a Capybara feature spec, an API contract test) " +
		"may be run inside a flow or acceptance case in addition to the walkthrough — never instead of it.\n" +
		"When part of a case cannot run locally (a third-party service, data you cannot create), record what can run, add a `card` saying " +
		"exactly what is missing and why, and set \"blocked\" on the case to that reason — never let such a case pass.\n")
	return b.String()
}
