package uat

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// GuideMeta identifies the task and environment a guide was produced for.
type GuideMeta struct {
	TaskID      string
	JiraKey     string
	JiraURL     string
	ATDDSource  string // where the in-scope cases came from
	GeneratedAt time.Time
	ImageURL    func(file string) string // screenshot link used in the markdown guide
	Note        string                   // why screenshots are missing, if they are
}

// GuideStep is one rendered step: the plan step plus what the runner captured.
type GuideStep struct {
	N          int
	Step       Step
	Do         string
	Where      string
	Result     *StepResult
	Screenshot string // file name, empty when none was captured
}

// GuideScenario groups the rendered steps of one scenario.
type GuideScenario struct {
	Scenario
	App      *App
	FromATDD bool // built from the ATDD sheet because the plan did not cover the case
	Steps    []GuideStep
	Captured int
	Failed   int
}

// Summary counts the screenshot run's outcome.
type Summary struct {
	Steps, Captured, Failed, Manual int
}

// Assemble joins the plan with the runner's results and the apps the scenarios run in.
func Assemble(p *Plan, results []StepResult, apps []App, cov Coverage) ([]GuideScenario, Summary) {
	byKey := map[string]*StepResult{}
	for i := range results {
		byKey[fmt.Sprintf("%s#%d", results[i].Scenario, results[i].Step)] = &results[i]
	}
	fromATDD := map[string]bool{}
	for _, c := range cov.Cases {
		if c.FromATDD {
			fromATDD[c.Scenarios[0]] = true
		}
	}
	var out []GuideScenario
	var sum Summary
	for _, s := range p.Scenarios {
		gs := GuideScenario{Scenario: s, FromATDD: fromATDD[s.ID]}
		for i := range apps {
			if apps[i].ID == s.App {
				gs.App = &apps[i]
			}
		}
		where := ""
		for i, st := range s.Steps {
			g := GuideStep{N: i + 1, Step: st, Do: Instruction(st), Result: byKey[fmt.Sprintf("%s#%d", s.ID, i+1)]}
			switch {
			case strings.TrimSpace(st.Where) != "":
				where = st.Where
				g.Where = st.Where
			case st.Action == ActionGoto:
				where = appName(gs.App) + " — `" + st.Target + "`"
				g.Where = where
			case where != "":
				g.Where = "Same screen as the previous step"
			}
			sum.Steps++
			if !st.Automated() {
				sum.Manual++
			}
			if g.Result != nil && g.Result.Screenshot != "" {
				g.Screenshot = g.Result.Screenshot
				gs.Captured++
				sum.Captured++
			}
			if g.Result != nil && !g.Result.OK {
				gs.Failed++
				sum.Failed++
			}
			gs.Steps = append(gs.Steps, g)
		}
		out = append(out, gs)
	}
	// Sanity checks first, then scenarios grouped by app in app order, plan order within.
	appRank := map[string]int{}
	for i, a := range apps {
		appRank[a.ID] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sanity != out[j].Sanity {
			return out[i].Sanity
		}
		return appRank[out[i].App.idOrEmpty()] < appRank[out[j].App.idOrEmpty()]
	})
	return out, sum
}

func (a *App) idOrEmpty() string {
	if a == nil {
		return ""
	}
	return a.ID
}

func appName(a *App) string {
	if a == nil {
		return "the application"
	}
	return a.Name
}

// fieldName makes a selector readable ("label=SQ Number" → "SQ Number").
func fieldName(target string) string {
	t := strings.TrimSpace(target)
	for _, p := range []string{"label=", "placeholder=", "testid=", "text="} {
		if strings.HasPrefix(t, p) {
			return strings.Trim(strings.TrimPrefix(t, p), `"'`)
		}
	}
	if strings.HasPrefix(t, "role=") {
		if i := strings.Index(t, `name="`); i >= 0 {
			rest := t[i+6:]
			if j := strings.Index(rest, `"`); j >= 0 {
				return rest[:j]
			}
		}
	}
	return t
}

// displayValue never shows a ${UAT_…} secret — the tester uses their own.
func displayValue(v string) string {
	if m := secretRef.FindStringSubmatch(v); m != nil {
		name := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(m[1], "UAT_"), "_", " "))
		return "your UAT " + name
	}
	return "`" + v + "`"
}

// Instruction is the tester-facing sentence for a step.
func Instruction(st Step) string {
	if d := strings.TrimSpace(st.Description); d != "" {
		return secretRef.ReplaceAllStringFunc(d, displayValue)
	}
	f := fieldName(st.Target)
	switch st.Action {
	case ActionGoto:
		return fmt.Sprintf("Open `%s`", st.Target)
	case ActionClick:
		return fmt.Sprintf("Click **%s**", f)
	case ActionFill:
		return fmt.Sprintf("Enter %s in **%s**", displayValue(st.Value), f)
	case ActionSelect:
		return fmt.Sprintf("Choose %s in **%s**", displayValue(st.Value), f)
	case ActionCheck:
		return fmt.Sprintf("Tick **%s**", f)
	case ActionPress:
		return fmt.Sprintf("Press **%s**", st.Value)
	case ActionWait:
		return "Wait for the page to finish loading"
	case ActionExpectText:
		return fmt.Sprintf("Check that the page shows “%s”", st.Value)
	case ActionAPI:
		return fmt.Sprintf("Send the request `%s`", st.Target)
	}
	return "Perform the step described"
}

// short is the step heading: the first sentence of the instruction, without markup.
func short(s string) string {
	s = strings.NewReplacer("**", "", "`", "").Replace(s)
	if i := strings.IndexAny(s, ".;\n"); i > 0 && i < len(s)-1 {
		s = s[:i]
	}
	if len(s) > 90 {
		s = strings.TrimSpace(s[:87]) + "…"
	}
	return s
}

func cell(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "|", "\\|"))
	if s == "" {
		return "—"
	}
	return s
}

// RenderMarkdown writes the step-by-step guide testers follow. It is shown in Mission Control and
// is the source of the downloadable HTML guide.
func RenderMarkdown(p *Plan, scenarios []GuideScenario, sum Summary, cov Coverage, apps []App, meta GuideMeta) string {
	var b strings.Builder
	w := func(format string, a ...interface{}) { fmt.Fprintf(&b, format, a...) }

	w("# UAT Guide: %s\n\n", p.Feature)
	b.WriteString("> **Classification:** INTERNAL — use test data only; never enter or record real customer data.  \n")
	w("> **Task:** %s", meta.TaskID)
	if meta.JiraKey != "" {
		if meta.JiraURL != "" {
			w(" · **Ticket:** [%s](%s)", meta.JiraKey, meta.JiraURL)
		} else {
			w(" · **Ticket:** %s", meta.JiraKey)
		}
	}
	w("  \n> **Generated:** %s · %d scenario(s) · %d step(s) · %d screenshot(s)\n", meta.GeneratedAt.Format("2006-01-02 15:04 MST"),
		len(scenarios), sum.Steps, sum.Captured)
	if meta.ATDDSource != "" {
		w("> **Test cases from:** %s\n", meta.ATDDSource)
	}
	b.WriteString("\n")
	if meta.Note != "" {
		w("> ⚠ %s\n\n", meta.Note)
	}
	if sum.Failed > 0 {
		w("> ⚠ %d step(s) could not be reproduced automatically. They are marked below — follow the written instructions and verify them by hand.\n\n", sum.Failed)
	}

	sec := 0
	next := func(title string) { sec++; w("## %d. %s\n\n", sec, title) }

	next("How to use this guide")
	b.WriteString("1. Read **Before you start** and make sure every item is ready. Ask the engineer on the ticket for anything marked *engineer*.\n")
	b.WriteString("2. Run the **Sanity checks** first. If any sanity check fails, stop and report it — the build is not ready for UAT.\n")
	b.WriteString("3. Run each **UAT scenario** in order. Every step says **Where** you should be, what to **Do**, and what **You should see**. Compare your screen with the screenshot.\n")
	b.WriteString("4. Tick **Pass** or **Fail** on every step. On a fail, write down what you saw, take your own screenshot, and report it (see *Reporting a problem*).\n")
	b.WriteString("5. When every scenario is done, fill in the **Sign-off** table.\n\n")
	if p.Objective != "" {
		w("**What this release should do:** %s\n\n", p.Objective)
	}

	next("Test case coverage")
	switch {
	case len(cov.Cases) == 0:
		b.WriteString("_No ATDD sheet was found for this task, so the scenarios below come from the UAT plan only._\n\n")
	default:
		w("This guide covers **all %d** ATDD case(s) marked for UAT or Sanity (%d Sanity, %d UAT).", len(cov.Cases), cov.Sanity, cov.UAT)
		if len(cov.Missing) > 0 {
			w(" %d of them are written from the ATDD sheet's own steps and shown as step cards.", len(cov.Missing))
		}
		b.WriteString("\n\n| Case ID | Type | Priority | Test case | Application | Walkthrough |\n|---|---|---|---|---|---|\n")
		for _, c := range cov.Cases {
			w("| %s | %s | %s | %s | %s | %s |\n", cell(c.Case.ID), c.Case.Kind(), cell(c.Case.Priority), cell(c.Case.Title),
				cell(appLabel(apps, c.App)), strings.Join(c.Scenarios, ", "))
		}
		b.WriteString("\n")
	}

	next("Applications under test")
	used := map[string]bool{}
	for _, s := range scenarios {
		used[s.App.idOrEmpty()] = true
	}
	for _, a := range apps {
		if !used[a.ID] {
			continue
		}
		w("### %s\n\n", a.Name)
		if a.Summary != "" {
			w("%s\n\n", a.Summary)
		}
		if a.Audience != "" {
			w("- **Used by:** %s\n", a.Audience)
		}
		if a.BaseURL != "" {
			w("- **Environment:** %s\n", a.BaseURL)
		} else if a.IsWeb() {
			b.WriteString("- **Environment:** _not set — ask the engineer on the ticket for the UAT URL_\n")
		}
		if len(a.Repos) > 0 {
			w("- **Changed in:** %s\n", strings.Join(a.Repos, ", "))
		}
		if len(a.SignIn) > 0 {
			b.WriteString("- **How to get in:**\n")
			for i, s := range a.SignIn {
				w("  %d. %s\n", i+1, s)
			}
		}
		if a.Flags != "" {
			w("- **Feature flags:** %s\n", a.Flags)
		}
		b.WriteString("\n")
	}

	next("Before you start")
	b.WriteString("- [ ] You can sign in to every application listed above with a **test** account.\n")
	for _, pre := range p.Preconditions {
		w("- [ ] %s\n", pre)
	}
	if len(p.TestData) > 0 {
		b.WriteString("\n**Test data to use**\n\n")
		for _, d := range p.TestData {
			w("- %s\n", d)
		}
	}
	b.WriteString("\n")

	writeScenarios := func(list []GuideScenario) {
		lastApp := "\x00"
		for _, s := range list {
			if !s.Sanity && s.App.idOrEmpty() != lastApp {
				lastApp = s.App.idOrEmpty()
				w("### In %s\n\n", appName(s.App))
			}
			w("#### %s · %s\n\n", s.ID, s.Title)
			b.WriteString("| Application | Who runs it | Covers | Steps |\n|---|---|---|---|\n")
			who := s.ExecutableBy
			if who == "" {
				who = "Finance/Ops"
				if s.App != nil && !s.App.IsWeb() {
					who = "Engineer"
				}
			}
			w("| %s | %s | %s | %d |\n\n", appName(s.App), who, cell(strings.Join(s.Covers, ", ")), len(s.Steps))
			if s.FromATDD {
				b.WriteString("> ℹ Written from the ATDD sheet — there is no automated walkthrough or screenshot for this case. Follow the steps exactly as written.\n\n")
			}
			if len(s.Preconditions) > 0 {
				b.WriteString("**Before this scenario**\n\n")
				for _, pre := range s.Preconditions {
					w("- [ ] %s\n", pre)
				}
				b.WriteString("\n")
			}
			if ac := strings.TrimSpace(s.AcceptanceCriterion); ac != "" {
				b.WriteString("**Acceptance criterion**\n\n")
				for _, l := range strings.Split(ac, "\n") {
					w("> %s  \n", strings.TrimSpace(l))
				}
				b.WriteString("\n")
			}
			for _, g := range s.Steps {
				w("##### Step %d of %d — %s\n\n", g.N, len(s.Steps), short(g.Do))
				if g.Where != "" {
					w("- **Where:** %s\n", g.Where)
				}
				w("- **Do:** %s\n", g.Do)
				if g.Step.Action == ActionAPI && g.Step.Value != "" {
					w("- **Request details:** `%s`\n", g.Step.Value)
				}
				if g.Step.Expected != "" {
					w("- **You should see:** %s\n", g.Step.Expected)
				}
				b.WriteString("\n")
				switch {
				case g.Step.Action == ActionManual && !s.FromATDD && g.Screenshot == "":
					b.WriteString("_Manual check — no screenshot._\n\n")
				case g.Result != nil && !g.Result.OK:
					w("> ⚠ The automated walkthrough could not complete this step (%s). Follow the instructions above and verify it by hand.\n\n", g.Result.Error)
				}
				if g.Screenshot != "" {
					caption := "what you should see"
					switch {
					case g.Result != nil && g.Result.Phase == "before":
						caption = "where to act (highlighted)"
					case g.Result != nil && g.Result.Phase == "card":
						caption = "step card"
					}
					w("![%s step %d — %s](%s)\n\n", s.ID, g.N, caption, meta.ImageURL(g.Screenshot))
				}
				b.WriteString("- [ ] Pass\n- [ ] Fail — what I saw: \n\n")
			}
			if s.ExpectedResult != "" {
				w("**Scenario passes when:** %s\n\n", s.ExpectedResult)
			}
			w("**Scenario %s result:** ☐ Pass ☐ Fail\n\n---\n\n", s.ID)
		}
	}
	var sanity, rest []GuideScenario
	for _, s := range scenarios {
		if s.Sanity {
			sanity = append(sanity, s)
		} else {
			rest = append(rest, s)
		}
	}
	if len(sanity) > 0 {
		next("Sanity checks — run these first")
		b.WriteString("These prove the release is healthy enough to test. If any fails, stop and report it before continuing.\n\n")
		writeScenarios(sanity)
	}
	if len(rest) > 0 {
		next("UAT scenarios")
		writeScenarios(rest)
	}

	next("Reporting a problem")
	b.WriteString("When a step fails:\n\n")
	b.WriteString("1. Note the **scenario ID** and **step number** (for example *S2, step 4*).\n")
	b.WriteString("2. Write what you expected and what you actually saw, including any message shown on screen.\n")
	b.WriteString("3. Take a screenshot. Blur or crop anything that is not test data.\n")
	if meta.JiraURL != "" {
		w("4. Add it as a comment on [%s](%s), or raise a bug linked to it.\n\n", meta.JiraKey, meta.JiraURL)
	} else {
		b.WriteString("4. Add it to the ticket for this release, or raise a bug linked to it.\n\n")
	}

	next("Sign-off")
	b.WriteString("| Application | Tester | Date | Result | Notes |\n|---|---|---|---|---|\n")
	for _, a := range apps {
		if used[a.ID] {
			w("| %s | | | ☐ Accepted ☐ Rejected | |\n", a.Name)
		}
	}
	b.WriteString("\n---\n*Generated by Meta-Orchestrator from the ATDD sheet and the UAT stage plan. Screenshots are captured from the environments above and may differ if the data changes.*\n")
	return b.String()
}

func appLabel(apps []App, id string) string {
	for _, a := range apps {
		if a.ID == id {
			return a.Name
		}
	}
	return id
}

var md = goldmark.New(goldmark.WithExtensions(extension.GFM)) // raw HTML is not rendered (safe default)

var imgSrc = regexp.MustCompile(`src="([^"]+)"`)

var htmlShell = template.Must(template.New("guide").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>UAT Guide — {{.Title}}</title>
<style>
:root{--bg:#fff;--fg:#1f2937;--muted:#6b7280;--line:#e5e7eb;--card:#f9fafb;--accent:#047857}
@media (prefers-color-scheme:dark){:root{--bg:#0f172a;--fg:#e2e8f0;--muted:#94a3b8;--line:#334155;--card:#1e293b;--accent:#34d399}}
body{background:var(--bg);color:var(--fg);font:15px/1.6 system-ui,-apple-system,Segoe UI,sans-serif;margin:0}
main{max-width:1040px;margin:0 auto;padding:24px 16px 64px}
h1{font-size:26px}h2{font-size:21px;margin-top:40px;padding-top:14px;border-top:2px solid var(--line)}
h3{font-size:18px;color:var(--accent)}h4{font-size:17px;margin-top:28px}h5{font-size:15px;margin:22px 0 6px}
blockquote{margin:12px 0;padding:8px 14px;border-left:4px solid var(--line);background:var(--card);color:var(--fg)}
table{border-collapse:collapse;width:100%;font-size:14px;margin:8px 0}td,th{border:1px solid var(--line);padding:6px 8px;text-align:left;vertical-align:top}
img{display:block;max-width:100%;height:auto;border:1px solid var(--line);border-radius:6px;margin:8px 0 12px}
code{font-size:13px;background:var(--card);padding:1px 4px;border-radius:4px}ul{padding-left:22px}
li input[type=checkbox]{margin-right:6px}hr{border:0;border-top:1px dashed var(--line);margin:28px 0}
@media print{h4{break-before:auto}img{break-inside:avoid}}
</style></head><body><main>{{.Body}}</main></body></html>
`))

// RenderHTML turns the markdown guide into a self-contained page (screenshots embedded) testers
// can open offline, print, or attach to a ticket. Checkboxes are clickable while the page is open.
func RenderHTML(markdown, title, shotDir string) ([]byte, error) {
	var body bytes.Buffer
	if err := md.Convert([]byte(markdown), &body); err != nil {
		return nil, err
	}
	html := body.String()
	html = strings.ReplaceAll(html, `<!-- raw HTML omitted -->`, "")
	html = strings.ReplaceAll(html, ` disabled="">`, `>`) // task-list boxes stay tickable
	html = imgSrc.ReplaceAllStringFunc(html, func(attr string) string {
		src := imgSrc.FindStringSubmatch(attr)[1]
		raw, err := os.ReadFile(filepath.Join(shotDir, filepath.Base(src)))
		if err != nil {
			return `src=""`
		}
		return `src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(raw) + `"`
	})
	var out bytes.Buffer
	err := htmlShell.Execute(&out, map[string]interface{}{"Title": title, "Body": template.HTML(html)})
	return out.Bytes(), err
}
