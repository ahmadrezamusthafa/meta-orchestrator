package uat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPlaywrightRunnerE2E drives a real headless browser. It installs playwright-core on first
// use, so it only runs when UAT_E2E=1 (UAT_RUNNER_DIR reuses an existing install).
func TestPlaywrightRunnerE2E(t *testing.T) {
	if os.Getenv("UAT_E2E") != "1" {
		t.Skip("set UAT_E2E=1 to run the browser test")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch {
		case r.URL.Path == "/secure":
			http.Redirect(w, r, "/login?next=/secure", http.StatusFound)
			return
		case r.URL.Path == "/login":
			_, _ = w.Write([]byte(`<h1>Sign in</h1><label>Email <input></label><label>Password <input type="password"></label>`))
			return
		case r.URL.Path == "/app":
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-session", Path: "/", Expires: time.Now().Add(time.Hour)})
			_, _ = w.Write([]byte(`<h1>Dashboard</h1>`))
			return
		case r.URL.Path == "/missing":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`<h1>gone</h1>`))
			return
		case strings.HasPrefix(r.URL.Path, "/billing/"):
			// An SPA answers 200 for every path and renders its own not-found route.
			if r.URL.Path != "/billing/proforma-invoices/42" {
				_, _ = w.Write([]byte(`<div id="app"></div><script>setTimeout(()=>{app.textContent='404 Halaman yang Anda cari saat ini telah dipindahkan atau telah dihapus.'},300)</script>`))
				return
			}
			_, _ = w.Write([]byte(`<div id="app"></div><script>setTimeout(()=>{app.innerHTML='<h1>Proforma Invoice 42</h1><button>Void</button>'},300)</script>`))
			return
		}
		_, _ = w.Write([]byte(`<!doctype html><title>PI</title><h1>Create PI</h1>
<label>SQ Number <input id="sq"></label><label>Password <input type="password" id="pw"></label>
<button onclick="document.getElementById('out').textContent='Package Pro prefilled'">Save</button><p id="out"></p>`))
	}))
	defer srv.Close()
	dir := os.Getenv("UAT_RUNNER_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "uat-runner-e2e")
	}
	t.Setenv("UAT_PASSWORD", "not-a-real-secret")
	plan := &Plan{Feature: "Prefill", Scenarios: []Scenario{{ID: "S1", Title: "Create a proforma invoice prefilled from the SQ", Sanity: true, Steps: []Step{
		{Action: ActionGoto, Target: "/pi/new", Where: "Subscription Backyard › Proforma Invoices › Create", Description: "Open the Create Proforma Invoice form", Expected: "The form is shown with an empty SQ Number field"},
		{Action: ActionFill, Target: "label=SQ Number", Value: "SQ-0001", Where: "Create Proforma Invoice form", Expected: "SQ-0001 is shown in SQ Number"},
		{Action: ActionFill, Target: "label=Password", Value: "${UAT_PASSWORD}"},
		{Action: ActionClick, Target: `role=button[name="Save"]`, Where: "Create Proforma Invoice form", Description: "Click Save to prefill the package from the SQ", Expected: "The package is prefilled"},
		{Action: ActionExpectText, Value: "Package Pro prefilled", Expected: "“Package Pro prefilled” appears under the form"},
		{Action: ActionClick, Target: "text=Does not exist"},
		{Action: ActionManual, Description: "Check the email"},
	}}}}
	out := t.TempDir()
	if d := os.Getenv("UAT_E2E_OUT"); d != "" {
		out = d
	}
	res, err := (&PlaywrightRunner{Dir: dir}).Capture(context.Background(), CaptureJob{Scenarios: []JobScenario{NewJobScenario(plan.Scenarios[0], srv.URL, "", "Subscription Backyard", "")}, OutDir: out, Label: "TASK-0 · UAT"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 7 {
		t.Fatalf("results = %+v", res)
	}
	for i := 0; i < 5; i++ {
		if !res[i].OK || res[i].Screenshot == "" {
			t.Fatalf("step %d = %+v", i+1, res[i])
		}
		if _, err := os.Stat(filepath.Join(out, res[i].Screenshot)); err != nil {
			t.Fatalf("screenshot missing: %v", err)
		}
	}
	if res[3].Phase != "before" || res[1].Phase != "after" {
		t.Fatalf("a click is captured before it acts, other steps after: %+v / %+v", res[3], res[1])
	}
	if res[5].OK || res[5].Error == "" || res[5].Screenshot == "" {
		t.Fatalf("failing step should report an error and still screenshot: %+v", res[5])
	}
	if !res[6].Skipped || res[6].Screenshot == "" {
		t.Fatalf("a manual step is not driven but still gets an image: %+v", res[6])
	}
	for _, r := range res {
		if r.Screenshot == "" {
			t.Fatalf("every step must have an image: %+v", r)
		}
	}

	// Scenarios that cannot be replayed (API-only, no environment, from the sheet) are drawn as
	// step cards in the same frame.
	cards := &Plan{Scenarios: []Scenario{{ID: "C1", Title: "Self-checkout creates an unpaid PI", ExecutableBy: "Engineer", Steps: []Step{
		{Action: ActionAPI, Target: "POST /api/v4/invoiceables/self-checkout", Value: `{"external_ref_id":"UAT-QSC-0001","token":"${UAT_API_TOKEN}"}`,
			Description: "Create a self-checkout proforma invoice", Expected: "HTTP 200 with invoiceable_id and payment_link"},
		{Action: ActionManual, Description: "Check the invoice email arrives in the UAT inbox", Expected: "One email with the payment link"},
		{Action: ActionClick, Target: `role=button[name="Void"]`, Where: "Billing Dashboard › Proforma Invoices", Description: "Click Void"},
	}}}}
	cardOut := t.TempDir()
	if d := os.Getenv("UAT_E2E_OUT"); d != "" {
		cardOut = d
	}
	cres, err := (&PlaywrightRunner{Dir: dir}).Capture(context.Background(), CaptureJob{OutDir: cardOut, Label: "TASK-0 · UAT",
		Scenarios: []JobScenario{NewJobScenario(cards.Scenarios[0], "", "", "Billing API", "No environment URL is set.")}})
	if err != nil || len(cres) != 3 {
		t.Fatalf("cards = %+v, %v", cres, err)
	}
	for _, r := range cres {
		if r.Screenshot == "" || r.Phase != "card" {
			t.Fatalf("a scenario that is not replayed must get a card per step: %+v", r)
		}
	}

	// Paths resolve inside the app's base path; wrong routes, HTTP errors and missing test data
	// fail with a reason instead of a 404 or blank screenshot.
	t.Setenv("UAT_PI_ID", "42")
	spa := &Plan{Scenarios: []Scenario{
		{ID: "B1", Steps: []Step{{Action: ActionGoto, Target: "/proforma-invoices/${UAT_PI_ID}"}, {Action: ActionExpectText, Value: "Proforma Invoice 42"}}},
		{ID: "B2", Steps: []Step{{Action: ActionGoto, Target: "/proforma-invoices/abc"}}},
		{ID: "B3", Steps: []Step{{Action: ActionGoto, Target: "/proforma-invoices/${PI_ID}"}}},
		{ID: "B4", Steps: []Step{{Action: ActionClick, Target: "text=Void"}}},
		{ID: "B5", Steps: []Step{{Action: ActionGoto, Target: srv.URL + "/missing"}}},
		{ID: "B6", Steps: []Step{{Action: ActionGoto, Target: srv.URL + "/secure"}}},
	}}
	env := func(Scenario) string { return srv.URL + "/billing" }
	if err := spa.ResolveTargets(env); err != nil {
		t.Fatal(err)
	}
	var job CaptureJob
	for _, s := range spa.Scenarios {
		base := env(s)
		if s.ID == "B4" {
			base = srv.URL + "/billing/proforma-invoices/42"
		}
		job.Scenarios = append(job.Scenarios, JobScenario{Scenario: s, BaseURL: base, Replay: true})
	}
	job.OutDir = t.TempDir()
	res, err = (&PlaywrightRunner{Dir: dir}).Capture(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]StepResult{}
	for _, r := range res {
		by[fmt.Sprintf("%s#%d", r.Scenario, r.Step)] = r
	}
	if r := by["B1#1"]; !r.OK || !strings.HasSuffix(r.URL, "/billing/proforma-invoices/42") || !by["B1#2"].OK {
		t.Fatalf("base path + placeholder: %+v / %+v", r, by["B1#2"])
	}
	if r := by["B2#1"]; r.OK || r.Reason != ReasonNotFound {
		t.Fatalf("the SPA not-found screen must fail the step: %+v", r)
	}
	if r := by["B3#1"]; r.OK || r.Reason != ReasonPlaceholder || r.Phase != "card" {
		t.Fatalf("an unknown placeholder must fail before navigating and be drawn as a card, not a blank page: %+v", r)
	}
	if r := by["B4#1"]; !r.OK || r.Screenshot == "" {
		t.Fatalf("a scenario starting with a click must open its environment first: %+v", r)
	}
	if r := by["B6#1"]; r.OK || r.Reason != ReasonLogin {
		t.Fatalf("a redirect to sign-in must be reported, not captured as a pass: %+v", r)
	}
	if r := by["B5#1"]; r.OK || r.Reason != ReasonNotFound || !strings.Contains(r.Error, "HTTP 404") {
		t.Fatalf("an HTTP 404 must fail the step: %+v", r)
	}
	t.Logf("screenshots in %s", out)
}

// TestPlaywrightLoginSavesSession checks the sign-in mode saves the session once the window is on
// the application without a sign-in form. Headless, so nobody has to click.
func TestPlaywrightLoginSavesSession(t *testing.T) {
	if os.Getenv("UAT_E2E") != "1" {
		t.Skip("set UAT_E2E=1 to run the browser test")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-session", Path: "/", Expires: time.Now().Add(time.Hour)})
		_, _ = w.Write([]byte(`<h1>Dashboard</h1>`))
	}))
	defer srv.Close()
	dir := os.Getenv("UAT_RUNNER_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "uat-runner-e2e")
	}
	path := filepath.Join(t.TempDir(), "sessions", "app.json")
	res, err := (&PlaywrightRunner{Dir: dir, HeadlessLogin: true}).Login(context.Background(), srv.URL+"/app", path, false)
	if err != nil || !res.OK || !res.SignedIn || res.Cookies != 1 {
		t.Fatalf("login = %+v, %v", res, err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("session file %v %v", st, err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"name":"sid"`) {
		t.Fatalf("storage state has no session cookie")
	}
}
