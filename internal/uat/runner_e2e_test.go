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
	plan := &Plan{Feature: "Prefill", Scenarios: []Scenario{{ID: "S1", Title: "Happy", Steps: []Step{
		{Action: ActionGoto, Target: "/pi/new"},
		{Action: ActionFill, Target: "label=SQ Number", Value: "SQ-0001"},
		{Action: ActionFill, Target: "label=Password", Value: "${UAT_PASSWORD}"},
		{Action: ActionClick, Target: `role=button[name="Save"]`},
		{Action: ActionExpectText, Value: "Package Pro prefilled"},
		{Action: ActionClick, Target: "text=Does not exist"},
		{Action: ActionManual, Description: "Check the email"},
	}}}}
	out := t.TempDir()
	res, err := (&PlaywrightRunner{Dir: dir}).Capture(context.Background(), CaptureJob{Scenarios: []JobScenario{{Scenario: plan.Scenarios[0], BaseURL: srv.URL}}, OutDir: out})
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
	if res[5].OK || res[5].Error == "" || res[5].Screenshot == "" {
		t.Fatalf("failing step should report an error and still screenshot: %+v", res[5])
	}
	if !res[6].Skipped {
		t.Fatalf("manual step should be skipped: %+v", res[6])
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
		job.Scenarios = append(job.Scenarios, JobScenario{Scenario: s, BaseURL: base})
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
	if r := by["B3#1"]; r.OK || r.Reason != ReasonPlaceholder || r.Screenshot != "" {
		t.Fatalf("an unknown placeholder must fail before navigating, without a blank screenshot: %+v", r)
	}
	if r := by["B4#1"]; !r.OK || r.Screenshot == "" {
		t.Fatalf("a scenario starting with a click must open its environment first: %+v", r)
	}
	if r := by["B5#1"]; r.OK || r.Reason != ReasonNotFound || !strings.Contains(r.Error, "HTTP 404") {
		t.Fatalf("an HTTP 404 must fail the step: %+v", r)
	}
	t.Logf("screenshots in %s", out)
}
