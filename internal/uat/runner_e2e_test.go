package uat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	t.Logf("screenshots in %s", out)
}
