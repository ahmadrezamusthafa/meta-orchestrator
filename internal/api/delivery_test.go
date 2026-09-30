package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/pullrequest"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/uat"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestPullRequestCommitsPushesAndOpensStandardPR(t *testing.T) {
	r, src := worktreeRouter(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "-q", "--bare")
	// Fetch URL names the hosted repo; pushes go to the local bare repository.
	runGit(t, src, "remote", "add", "origin", "https://bitbucket.org/ws/app.git")
	runGit(t, src, "config", "remote.origin.pushurl", bare)
	runGit(t, src, "config", "user.name", "t")
	runGit(t, src, "config", "user.email", "t@t")

	var got map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"values":[]}`))
		case http.MethodPost:
			_ = json.NewDecoder(req.Body).Decode(&got)
			_, _ = w.Write([]byte(`{"id":11,"links":{"html":{"href":"https://bitbucket.org/ws/app/pull-requests/11"}}}`))
		}
	}))
	defer srv.Close()
	r.prClient = &pullrequest.Client{BaseURL: srv.URL}
	if _, err := r.cfg.ConnectorsManager.UpdateConnector(types.ConnectorItem{ID: "bitbucket", Enabled: true, Username: "me@example.com", APIToken: "tok"}); err != nil {
		t.Fatal(err)
	}

	r.mu.Lock()
	r.tasks["TASK-1"] = &types.Task{ID: "TASK-1", Title: "[PAY-42] Refund webhook retries", CurrentStageID: "task_implementation",
		State: types.TaskStateWaitingGateApproval, AssignedRepos: []string{"app"},
		Metadata: map[string]string{"jira_key": "PAY-42", "jira_url": "https://jira.example/browse/PAY-42", "task_type": "bugfix"}}
	r.mu.Unlock()
	r.saveStageDocument("TASK-1", "task_implementation", "m", "Did it.\n\n## Summary\nRetries refund webhooks.\n\n## How to test\n1. Replay a webhook\n")

	dir := r.resolveWorkDir(r.taskSnapshot("TASK-1"))
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc retry() {}\n"), 0o644)

	w := do(r, http.MethodGet, "/api/v1/tasks/TASK-1/pull-request", "")
	var drafts struct {
		Drafts []prDraft `json:"drafts"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &drafts)
	if w.Code != 200 || len(drafts.Drafts) != 1 {
		t.Fatalf("drafts → %d %s", w.Code, w.Body.String())
	}
	d := drafts.Drafts[0]
	if !d.CanCreate || d.Provider != "bitbucket" || d.TargetBranch != "master" || !d.Uncommitted ||
		d.Title != "[PAY-42] fix: Refund webhook retries" || !strings.Contains(d.Body, "## Summary\nRetries refund webhooks.") ||
		!strings.Contains(d.Body, "## How to test\n1. Replay a webhook") {
		t.Fatalf("draft = %+v", d)
	}

	w = do(r, http.MethodPost, "/api/v1/tasks/TASK-1/pull-request", `{"repo":"app"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"committed":true`) {
		t.Fatalf("open → %d %s", w.Code, w.Body.String())
	}
	if got["title"] != d.Title || got["source"].(map[string]interface{})["branch"].(map[string]interface{})["name"] != "feat/PAY-42-refund-webhook-retries" {
		t.Fatalf("provider payload = %v", got)
	}
	runGit(t, bare, "rev-parse", "--verify", "refs/heads/feat/PAY-42-refund-webhook-retries")
	if u := r.taskSnapshot("TASK-1").Metadata["pr_url.app"]; u != "https://bitbucket.org/ws/app/pull-requests/11" {
		t.Fatalf("pr_url = %q", u)
	}

	r.mu.Lock()
	r.tasks["TASK-1"].State = types.TaskStateRunning
	r.mu.Unlock()
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-1/pull-request", `{}`); w.Code != http.StatusConflict {
		t.Fatalf("opening while the agent runs must be refused: %d", w.Code)
	}
}

func TestPullRequestBlockedBeforeImplementation(t *testing.T) {
	r, _ := worktreeRouter(t)
	r.mu.Lock()
	r.tasks["TASK-1"] = &types.Task{ID: "TASK-1", Title: "x", CurrentStageID: "techdoc_rfc", State: types.TaskStatePending,
		AssignedRepos: []string{"app"}, Metadata: map[string]string{}}
	r.mu.Unlock()
	w := do(r, http.MethodGet, "/api/v1/tasks/TASK-1/pull-request", "")
	if !strings.Contains(w.Body.String(), `"can_create":false`) || !strings.Contains(w.Body.String(), `"stage_ready":false`) {
		t.Fatalf("drafts before implementation → %s", w.Body.String())
	}
}

type stubRunner struct{ job uat.CaptureJob }

func (s *stubRunner) Capture(_ context.Context, job uat.CaptureJob) ([]uat.StepResult, error) {
	s.job = job
	_ = os.MkdirAll(job.OutDir, 0o755)
	_ = os.WriteFile(filepath.Join(job.OutDir, "S1-01.png"), []byte("png"), 0o644)
	return []uat.StepResult{{Scenario: "S1", Step: 1, OK: true, Screenshot: "S1-01.png"}}, nil
}

func TestUATGuideGeneratedFromStagePlan(t *testing.T) {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	stub := &stubRunner{}
	r.uatRunner = stub
	r.mu.Lock()
	r.tasks["TASK-3"] = &types.Task{ID: "TASK-3", Title: "Prefill", CurrentStageID: "uat_verification", State: types.TaskStateWaitingGateApproval,
		Metadata: map[string]string{}}
	r.mu.Unlock()

	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-3/uat-guide", `{}`); w.Code != http.StatusConflict {
		t.Fatalf("no stage output yet → %d", w.Code)
	}
	r.saveStageDocument("TASK-3", "uat_verification", "m", "Checklist\n\n```uat-plan\n"+
		`{"feature":"Prefill","scenarios":[{"id":"S1","title":"Happy","steps":[{"action":"goto","target":"/pi/new","description":"Open form"}]}]}`+"\n```\n")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-3/uat-guide", `{"base_url":"javascript:alert(1)"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad URL → %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-3/uat-guide", `{"base_url":"https://staging.example.com/"}`); w.Code != 200 {
		t.Fatalf("generate → %d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.taskSnapshot("TASK-3").Metadata["uat_guide_status"] != "READY" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	w := do(r, http.MethodGet, "/api/v1/tasks/TASK-3/uat-guide", "")
	if !strings.Contains(w.Body.String(), `"status":"READY"`) || !strings.Contains(w.Body.String(), `"html_path":"uat/UAT_GUIDE.html"`) {
		t.Fatalf("status → %s", w.Body.String())
	}
	if len(stub.job.Scenarios) != 1 || stub.job.Scenarios[0].BaseURL != "https://staging.example.com" {
		t.Fatalf("runner job = %+v", stub.job)
	}
	w = do(r, http.MethodGet, "/api/v1/artifacts/TASK-3/uat/UAT_GUIDE.md", "")
	if !strings.Contains(w.Body.String(), "/api/v1/artifacts/TASK-3/uat/screenshots/S1-01.png") {
		t.Fatalf("guide should link the screenshot: %s", w.Body.String())
	}
	w = do(r, http.MethodGet, "/api/v1/artifacts/TASK-3/uat/UAT_GUIDE.html", "")
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("html guide must be sandboxed and downloaded: %v", w.Header())
	}
}

func TestUATGuideCoversEveryATDDUATAndSanityCase(t *testing.T) {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	stub := &stubRunner{}
	r.uatRunner = stub
	r.mu.Lock()
	r.tasks["TASK-4"] = &types.Task{ID: "TASK-4", Title: "Tags", CurrentStageID: "uat_verification", State: types.TaskStateWaitingGateApproval,
		Metadata: map[string]string{}}
	r.mu.Unlock()
	sheet := "Case ID,Title,Steps,Expected Results,Platform,UAT,Sanity\n" +
		"C-1,Tag shown in Backyard,1. Open Subscription Backyard,1. Tag shown,WEB,Yes,Yes\n" +
		"C-2,Tag filter,1. Filter by tag,1. Filtered,WEB,Yes,\n" +
		"C-3,Tags API healthy,1. GET /tags,1. 200,API,,Yes\n" +
		"C-4,Internal only,1. x,1. y,API,,\n"
	if err := r.writeArtifact("TASK-4", "atdd/tags-atdd.csv", []byte(sheet)); err != nil {
		t.Fatal(err)
	}

	ctx := r.uatStageContext(r.taskSnapshot("TASK-4"))
	for _, want := range []string{"subscription_backyard", "MUST be covered (3, from task document atdd/tags-atdd.csv)", "### C-1 [Sanity + UAT]", "### C-3 [Sanity]"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("stage context missing %q:\n%s", want, ctx)
		}
	}
	if strings.Contains(ctx, "C-4") {
		t.Error("cases not marked UAT or Sanity are out of scope")
	}

	r.saveStageDocument("TASK-4", "uat_verification", "m", "```uat-plan\n"+
		`{"feature":"Tags","scenarios":[{"id":"S1","title":"Tag shown","app":"subscription_backyard","covers":["C-1"],"steps":[{"action":"goto","target":"/tags","description":"Open tags"}]}]}`+"\n```\n")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-4/uat-guide", `{"apps":{"subscription_backyard":{"url":"https://backyard.example.com"}}}`); w.Code != 200 {
		t.Fatalf("generate → %d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.taskSnapshot("TASK-4").Metadata["uat_guide_status"] != "READY" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	var st struct {
		Coverage struct {
			Planned string   `json:"planned"`
			Missing []string `json:"missing"`
		} `json:"coverage"`
		ATDD struct {
			InScope int `json:"in_scope"`
			Sanity  int `json:"sanity"`
		} `json:"atdd"`
		CanRequestChanges bool `json:"can_request_changes"`
	}
	w := do(r, http.MethodGet, "/api/v1/tasks/TASK-4/uat-guide", "")
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	if st.Coverage.Planned != "1/3" || strings.Join(st.Coverage.Missing, ",") != "C-2,C-3" || st.ATDD.InScope != 3 || st.ATDD.Sanity != 2 || !st.CanRequestChanges {
		t.Fatalf("status = %s", w.Body.String())
	}
	// Every scenario gets images: the planned web one is replayed, the ones written from the sheet
	// are drawn as step cards that say why.
	if len(stub.job.Scenarios) != 3 || !stub.job.Scenarios[0].Replay || stub.job.Scenarios[0].BaseURL != "https://backyard.example.com" {
		t.Fatalf("job = %+v", stub.job.Scenarios)
	}
	for _, js := range stub.job.Scenarios[1:] {
		if js.Replay || js.BaseURL != "" || !strings.Contains(js.CardNote, "ATDD sheet") {
			t.Fatalf("sheet-only scenario %s must be a card with a note: %+v", js.ID, js)
		}
	}
	guide := do(r, http.MethodGet, "/api/v1/artifacts/TASK-4/uat/UAT_GUIDE.md", "").Body.String()
	for _, want := range []string{"covers **all 3** ATDD case(s)", "#### C-2 · Tag filter", "#### C-3 · Tags API healthy", "Filter by tag"} {
		if !strings.Contains(guide, want) {
			t.Errorf("guide missing %q", want)
		}
	}
}

func TestUATTestDataFillsPlanPlaceholders(t *testing.T) {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	stub := &stubRunner{}
	r.uatRunner = stub
	r.mu.Lock()
	r.tasks["TASK-5"] = &types.Task{ID: "TASK-5", Title: "Void PI", CurrentStageID: "uat_verification", State: types.TaskStateWaitingGateApproval,
		Metadata: map[string]string{}}
	r.mu.Unlock()
	r.saveStageDocument("TASK-5", "uat_verification", "m", "```uat-plan\n"+
		`{"feature":"Void","scenarios":[{"id":"S1","title":"Void","steps":[`+
		`{"action":"goto","target":"/proforma-invoices/${PI_ID_PAID}","description":"Open PI ${PI_ID_PAID}"},`+
		`{"action":"fill","target":"label=Password","value":"${UAT_PASSWORD}"}]}]}`+"\n```\n\n"+
		"```uat-seed ruby\n# run: bundle exec rails runner tmp/uat_seed.rb\nputs \"PI_ID_PAID=#{pi.id}\"\n```\n")
	if w := do(r, http.MethodGet, "/api/v1/tasks/TASK-5/uat-guide", ""); !strings.Contains(w.Body.String(), `"seed":{"language":"ruby","run":"bundle exec rails runner tmp/uat_seed.rb"`) {
		t.Fatalf("status must offer the seed script: %s", w.Body.String())
	}
	if w := do(r, http.MethodGet, "/api/v1/tasks/TASK-5/uat-guide", ""); !strings.Contains(w.Body.String(), `"seed_check":{"issues":[{"severity":"error","message":"no production guard`) {
		t.Fatalf("a seed without a production guard must be flagged: %s", w.Body.String())
	}

	w := do(r, http.MethodGet, "/api/v1/tasks/TASK-5/uat-guide", "")
	if !strings.Contains(w.Body.String(), `{"name":"PI_ID_PAID","source":"missing","secret":false,"used":true,"browser":true}`) ||
		!strings.Contains(w.Body.String(), `"name":"UAT_PASSWORD"`) || !strings.Contains(w.Body.String(), `"secret":true`) {
		t.Fatalf("status must list the plan's placeholders: %s", w.Body.String())
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-5/uat-guide", `{"save_only":true,"variables":{"UAT_PASSWORD":"hunter2"}}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a credential must not be stored with the task → %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-5/uat-guide", `{"save_only":true,"variables":{"bad name":"1"}}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid name → %d", w.Code)
	}
	body := `{"apps":{"subscription_backyard":{"url":"https://backyard.example.com/billing"}},"variables":{"PI_ID_PAID":"4242","OLD":""}}`
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-5/uat-guide", body); w.Code != 200 {
		t.Fatalf("generate → %d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.taskSnapshot("TASK-5").Metadata["uat_guide_status"] != "READY" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(stub.job.Scenarios) != 1 {
		t.Fatalf("runner job = %+v", stub.job)
	}
	steps := stub.job.Scenarios[0].Steps
	if steps[0].Target != "https://backyard.example.com/billing/proforma-invoices/4242" || steps[0].Description != "Open PI 4242" {
		t.Fatalf("test data must be filled into the plan: %+v", steps[0])
	}
	if steps[1].Value != "${UAT_PASSWORD}" {
		t.Fatalf("credentials stay placeholders for the runner's environment, got %q", steps[1].Value)
	}
	w = do(r, http.MethodGet, "/api/v1/tasks/TASK-5/uat-guide", "")
	if !strings.Contains(w.Body.String(), `{"name":"PI_ID_PAID","value":"4242","source":"task","secret":false,"used":true,"browser":true}`) {
		t.Fatalf("saved value must be reported: %s", w.Body.String())
	}
}

type loginStubRunner struct {
	stubRunner
	url string
}

func (s *loginStubRunner) Login(_ context.Context, url, path string, _ bool) (uat.LoginResult, error) {
	s.url = url
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte(`{"cookies":[{"name":"sid","value":"x"}],"origins":[]}`), 0o600)
	return uat.LoginResult{OK: true, Cookies: 1, SignedIn: true}, nil
}

func TestUATSignInSavesASharedSession(t *testing.T) {
	root := t.TempDir()
	r := NewRouter(RouterConfig{RootDir: root})
	stub := &loginStubRunner{}
	r.uatRunner = stub
	r.mu.Lock()
	r.tasks["TASK-6"] = &types.Task{ID: "TASK-6", Title: "Void", CurrentStageID: "uat_verification", State: types.TaskStateWaitingGateApproval,
		Metadata: map[string]string{"uat_app.subscription_backyard.url": "https://backyard.example.com/billing"}}
	r.tasks["TASK-7"] = &types.Task{ID: "TASK-7", Title: "Other", Metadata: map[string]string{}}
	r.mu.Unlock()

	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-6/uat-guide/login", `{"app":"billing_dashboard"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("an app without an environment URL cannot sign in → %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-6/uat-guide/login", `{"app":"../etc"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid app id → %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-6/uat-guide/login", `{"app":"subscription_backyard"}`); w.Code != http.StatusAccepted {
		t.Fatalf("sign in → %d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	for r.taskSnapshot("TASK-6").Metadata["uat_login.subscription_backyard"] != "ok" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if stub.url != "https://backyard.example.com/billing" {
		t.Fatalf("sign-in window opened %q", stub.url)
	}
	path := filepath.Join(root, ".sdlc", "uat", "sessions", "subscription_backyard.json")
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("session file %v %v", st, err)
	}
	w := do(r, http.MethodGet, "/api/v1/tasks/TASK-6/uat-guide", "")
	if !strings.Contains(w.Body.String(), `{"app":"subscription_backyard","source":"shared"`) || strings.Contains(w.Body.String(), `"sid"`) {
		t.Fatalf("status must report the session without its contents: %s", w.Body.String())
	}
	// Every task testing the application reuses it.
	for _, a := range r.taskApps(r.taskSnapshot("TASK-7")) {
		if a.ID == "subscription_backyard" && a.StorageState != path {
			t.Fatalf("other tasks must reuse the shared session, got %q", a.StorageState)
		}
	}
	if w := do(r, http.MethodDelete, "/api/v1/tasks/TASK-6/uat-guide/login", `{"app":"subscription_backyard"}`); w.Code != 200 {
		t.Fatalf("forget session → %d", w.Code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("session file must be removed: %v", err)
	}
}
