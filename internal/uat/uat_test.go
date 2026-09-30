package uat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/atdd"
)

const stageOutput = "## UAT checklist\n...\n\n```uat-plan\n" + `{"feature":"Prefill package","objective":"Package is prefilled from the SQ",
 "preconditions":["Product bundling flag is on"],
 "scenarios":[{"id":"S1","title":"Prefill from SQ","app":"subscription_backyard","covers":["T-TC-001"],"acceptance_criterion":"Given a SQ\nThen prefilled",
   "steps":[{"action":"goto","target":"/pi/new","where":"Subscription Backyard › Proforma Invoices › Create","description":"Open the PI create form","expected":"Form is shown"},
            {"action":"fill","target":"label=Password","value":"${UAT_PASSWORD}"},
            {"action":"click","target":"role=button[name=\"Save\"]","expected":"Saved"},
            {"action":"manual","description":"Check the email"}]},
   {"id":"S1","title":"dup id","covers":["NOT-IN-SHEET"],"steps":[{"action":"GOTO","target":"/x"}]}]}` + "\n```\n\n## Summary\nok"

var testApps = AppsForRepos(DefaultApps, []RepoRef{{Name: "billing-frontend", Path: "/src/billing-internal-fe"}, {Name: "billing-dashboard", Path: "/src/billing"}})

var scope = []atdd.Case{
	{ID: "T-TC-001", Title: "Prefill package", Platform: "WEB", UAT: true, Sanity: true, Priority: "P1"},
	{ID: "T-TC-002", Title: "Dashboard shows the prefilled package", Platform: "WEB", UAT: true, Priority: "P2",
		Preconditions: []string{"Signed in to the Billing Dashboard."}, Steps: []string{"Open the PI", "Look at Package"},
		ExpectedResults: []string{"PI opens", "Package is Pro"}},
	{ID: "T-TC-003", Title: "Health check", Platform: "API", Sanity: true, Steps: []string{"GET /health"}, ExpectedResults: []string{"200 OK"}},
}

func TestExtractPlan(t *testing.T) {
	p, err := ExtractPlan(stageOutput)
	if err != nil {
		t.Fatal(err)
	}
	if p.Feature != "Prefill package" || len(p.Scenarios) != 2 || p.Scenarios[1].ID != "S2" || p.Scenarios[1].Steps[0].Action != "goto" {
		t.Fatalf("plan = %+v", p)
	}
	if _, err := ExtractPlan("no plan here"); err == nil {
		t.Fatal("missing block should fail")
	}
	if _, err := ExtractPlan("```uat-plan\n{\"scenarios\":[{\"steps\":[{\"action\":\"hack\"}]}]}\n```"); err == nil {
		t.Fatal("unknown action should fail")
	}
}

func TestValidateKeepsBrowserOnEnvironment(t *testing.T) {
	p, _ := ExtractPlan(stageOutput)
	env := func(Scenario) string { return "https://staging.example.com" }
	if err := p.Validate(env); err != nil {
		t.Fatal(err)
	}
	p.Scenarios[0].Steps[0].Target = "https://evil.example.net/steal"
	if err := p.Validate(env); err == nil {
		t.Fatal("navigation to another host must be rejected")
	}
	if err := p.Validate(func(Scenario) string { return "" }); err != nil {
		t.Fatal("scenarios without an environment are not replayed, so not validated")
	}
}

func TestAppsFromReposAndCaseRouting(t *testing.T) {
	ids := []string{}
	for _, a := range testApps {
		ids = append(ids, a.ID)
	}
	if strings.Join(ids, ",") != "subscription_backyard,billing_dashboard,billing_api" {
		t.Fatalf("apps = %v", ids)
	}
	cases := map[string]atdd.Case{
		"billing_api":           {Platform: "API", Title: "x"},
		"subscription_backyard": {Platform: "WEB", Title: "Open Subscription Backyard dashboard"}, // longest keyword wins
		"billing_dashboard":     {Platform: "WEB", Repository: "billing-dashboard"},
	}
	for want, c := range cases {
		if got := AppFor(c, testApps); got != want {
			t.Errorf("AppFor(%+v) = %s, want %s", c, got, want)
		}
	}
	custom := AppsForRepos(DefaultApps, []RepoRef{{Name: "qontak-web", Path: "/src/qontak"}, {Name: "artifacts", Path: "/src/a"}})
	if len(custom) != 1 || custom[0].ID != "qontak_web" || !custom[0].IsWeb() {
		t.Fatalf("unknown repo should become its own web app: %+v", custom)
	}
}

func TestCoverageFillsEveryUATAndSanityCase(t *testing.T) {
	p, _ := ExtractPlan(stageOutput)
	cov := ApplyCoverage(p, scope, testApps)
	if len(cov.Cases) != 3 || cov.Planned != 1 || cov.Sanity != 2 || cov.UAT != 2 {
		t.Fatalf("coverage = %+v", cov)
	}
	if strings.Join(cov.Missing, ",") != "T-TC-002,T-TC-003" || strings.Join(cov.Unknown, ",") != "NOT-IN-SHEET" {
		t.Fatalf("missing=%v unknown=%v", cov.Missing, cov.Unknown)
	}
	if !p.Scenarios[0].Sanity {
		t.Fatal("a scenario covering a sanity case is a sanity scenario")
	}
	if len(p.Scenarios) != 4 {
		t.Fatalf("two scenarios should be added from the sheet: %d", len(p.Scenarios))
	}
	dash, api := p.Scenarios[2], p.Scenarios[3]
	if dash.App != "billing_dashboard" || dash.Steps[1].Expected != "Package is Pro" || dash.Steps[0].Action != ActionManual {
		t.Fatalf("dashboard walkthrough = %+v", dash)
	}
	if api.App != "billing_api" || api.Steps[0].Action != ActionAPI || api.ExecutableBy != "Engineer" || !api.Sanity {
		t.Fatalf("api walkthrough = %+v", api)
	}
}

func TestRenderGuides(t *testing.T) {
	p, _ := ExtractPlan(stageOutput)
	cov := ApplyCoverage(p, scope, testApps)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "S1-01.png"), []byte("\x89PNG fake"), 0o644)
	results := []StepResult{
		{Scenario: "S1", Step: 1, OK: true, Screenshot: "S1-01.png"},
		{Scenario: "S1", Step: 2, OK: true},
		{Scenario: "S1", Step: 3, OK: false, Error: "timeout waiting for Save"},
		{Scenario: "S1", Step: 4, OK: true, Skipped: true},
	}
	apps := append([]App(nil), testApps...)
	apps[0].BaseURL = "https://backyard.staging.example.com"
	scenarios, sum := Assemble(p, results, apps, cov)
	if !scenarios[0].Sanity || !scenarios[1].Sanity || scenarios[0].ID != "S1" {
		t.Fatalf("sanity scenarios must come first: %s %s", scenarios[0].ID, scenarios[1].ID)
	}
	if sum.Captured != 1 || sum.Failed != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	meta := GuideMeta{TaskID: "TASK-2", JiraKey: "MIB-1", JiraURL: "https://jira.example/browse/MIB-1", ATDDSource: "sheet.csv", GeneratedAt: time.Now(),
		ImageURL: func(f string) string { return "/api/v1/artifacts/TASK-2/uat/screenshots/" + f }}
	md := RenderMarkdown(p, scenarios, sum, cov, apps, meta)
	for _, want := range []string{
		"# UAT Guide: Prefill package", "Classification:** INTERNAL", "## 1. How to use this guide",
		"covers **all 3** ATDD case(s) marked for UAT or Sanity (2 Sanity, 2 UAT)",
		"| T-TC-002 | UAT | P2 | Dashboard shows the prefilled package | Billing Dashboard | T-TC-002 |",
		"### Subscription Backyard", "- **Environment:** https://backyard.staging.example.com", "From the Backyard navigation, open **Subscription Backyard**.",
		"Sanity checks — run these first", "#### S1 · Prefill from SQ", "##### Step 1 of 4 — Open the PI create form",
		"- **Where:** Subscription Backyard › Proforma Invoices › Create", "- **You should see:** Form is shown",
		"- **Where:** Same screen as the previous step", "Enter your UAT password in **Password**",
		"![S1 step 1 — what you should see](/api/v1/artifacts/TASK-2/uat/screenshots/S1-01.png)",
		"could not complete this step (timeout waiting for Save)", "Written from the ATDD sheet",
		"### In Billing Dashboard", "| Billing API (backend) | Engineer | T-TC-003 | 1 |",
		"## 7. Reporting a problem", "[MIB-1](https://jira.example/browse/MIB-1)", "| Subscription Backyard | | | ☐ Accepted ☐ Rejected | |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
	if strings.Contains(md, "${UAT_PASSWORD}") {
		t.Error("secret placeholders must not appear in the guide")
	}
	html, err := RenderHTML(md, p.Feature, dir)
	if err != nil {
		t.Fatal(err)
	}
	h := string(html)
	if !strings.Contains(h, `src="data:image/png;base64,`) || !strings.Contains(h, "<table>") || strings.Contains(h, "<script") ||
		!strings.Contains(h, `<input checked="" type="checkbox"`) && !strings.Contains(h, `type="checkbox"`) {
		t.Fatalf("html guide should embed screenshots and render safely")
	}
	if strings.Contains(h, ` disabled="">`) {
		t.Fatal("checkboxes should stay tickable")
	}
}

func TestResolveURLKeepsTheAppBasePath(t *testing.T) {
	cases := []struct{ env, target, want string }{
		{"https://backyard.test/billing", "/proforma-invoices/12", "https://backyard.test/billing/proforma-invoices/12"},
		{"https://backyard.test/billing/", "proforma-invoices", "https://backyard.test/billing/proforma-invoices"},
		{"https://backyard.test/billing", "/billing/proforma-invoices", "https://backyard.test/billing/proforma-invoices"},
		{"https://backyard.test/billing", "/billing", "https://backyard.test/billing"},
		{"https://backyard.test/billing", "/billingx", "https://backyard.test/billing/billingx"},
		{"https://backyard.test", "/billing/proforma-invoices?tab=log#top", "https://backyard.test/billing/proforma-invoices?tab=log#top"},
		{"https://backyard.test/billing", "/proforma-invoices/${UAT_PI_ID}", "https://backyard.test/billing/proforma-invoices/${UAT_PI_ID}"},
		{"https://backyard.test/billing", "https://backyard.test/other", "https://backyard.test/other"},
		{"https://backyard.test", "/", "https://backyard.test/"},
	}
	for _, c := range cases {
		got, err := ResolveURL(c.env, c.target)
		if err != nil || got != c.want {
			t.Errorf("ResolveURL(%q, %q) = %q, %v; want %q", c.env, c.target, got, err, c.want)
		}
	}
	if _, err := ResolveURL("backyard.test", "/x"); err == nil {
		t.Error("a relative environment URL must be rejected")
	}
}

func TestResolveTargetsRewritesOnlyScenariosWithAnEnvironment(t *testing.T) {
	p := &Plan{Scenarios: []Scenario{
		{ID: "S1", App: "web", Steps: []Step{{Action: ActionGoto, Target: "/proforma-invoices"}, {Action: ActionClick, Target: "text=/x"}}},
		{ID: "S2", App: "api", Steps: []Step{{Action: ActionGoto, Target: "/health"}}},
	}}
	env := func(s Scenario) string {
		if s.App == "web" {
			return "https://backyard.test/billing"
		}
		return ""
	}
	if err := p.Validate(env); err != nil {
		t.Fatal(err)
	}
	if err := p.ResolveTargets(env); err != nil {
		t.Fatal(err)
	}
	if got := p.Scenarios[0].Steps[0].Target; got != "https://backyard.test/billing/proforma-invoices" {
		t.Fatalf("goto target = %q", got)
	}
	if got := p.Scenarios[0].Steps[1].Target; got != "text=/x" {
		t.Fatalf("selectors must not be touched, got %q", got)
	}
	if got := p.Scenarios[1].Steps[0].Target; got != "/health" {
		t.Fatalf("a scenario without an environment must keep its target, got %q", got)
	}
}
