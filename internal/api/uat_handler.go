package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/atdd"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/uat"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// The UAT guide is the manual-testing contract for a release. Its scope is every ATDD case
// marked UAT or Sanity; its structure follows the applications the task changes (Subscription
// Backyard, Billing Dashboard, the billing API, …). When the uat_verification stage finishes,
// its ```uat-plan block is replayed in a headless browser per application to capture a
// screenshot after every step, and any in-scope case the plan missed is written into the guide
// from the sheet's own steps — so no UAT or Sanity case is ever silently dropped.
//
// Outputs: uat/UAT_GUIDE.md (Mission Control), uat/UAT_GUIDE.html (self-contained, for
// testers), uat/plan.json and uat/coverage.json.

const (
	uatGuideDoc  = "uat/UAT_GUIDE.md"
	uatGuideHTML = "uat/UAT_GUIDE.html"
	uatPlanDoc   = "uat/plan.json"
	uatCoverage  = "uat/coverage.json"
	uatShotsDir  = "uat/screenshots"
)

// uatScreenshotRunner captures one screenshot per plan step.
type uatScreenshotRunner interface {
	Capture(ctx context.Context, job uat.CaptureJob) ([]uat.StepResult, error)
}

func (r *Router) screenshotRunner() uatScreenshotRunner {
	if r.uatRunner == nil {
		r.uatRunner = &uat.PlaywrightRunner{Dir: filepath.Join(r.cfg.RootDir, ".sdlc", "tools", "uat-runner")}
	}
	return r.uatRunner
}

// taskApps lists the applications the task touches, with the operator's per-task environment.
func (r *Router) taskApps(t *types.Task) []uat.App {
	all := uat.LoadApps(r.cfg.RootDir)
	paths := r.repoPaths(t.AssignedRepos)
	var refs []uat.RepoRef
	for _, repo := range t.AssignedRepos {
		refs = append(refs, uat.RepoRef{Name: repo, Path: paths[repo]})
	}
	apps := uat.AppsForRepos(all, refs)
	if len(apps) == 0 {
		apps = all // no repositories yet: offer every known application
	}
	for i := range apps {
		a := &apps[i]
		a.BaseURL = t.Metadata["uat_app."+a.ID+".url"]
		a.StorageState = t.Metadata["uat_app."+a.ID+".storage_state"]
		if a.BaseURL == "" && a.IsWeb() {
			a.BaseURL = t.Metadata["uat_base_url"] // task-wide default from earlier versions
		}
		if a.StorageState == "" && a.IsWeb() {
			a.StorageState = t.Metadata["uat_storage_state"]
		}
		// A session saved with "Sign in" is shared by every task that tests this application.
		if a.StorageState == "" && a.IsWeb() {
			if p := r.uatSessionPath(a.ID); fileExists(p) {
				a.StorageState = p
			}
		}
	}
	return apps
}

// loadATDD finds the task's ATDD cases. In order: a sheet path the operator set, the newest ATDD
// CSV in the task's documents (uploads), one written by the bmad-atdd skill into a worktree
// (_bmad-output/), then the ```atdd-cases block of the atdd_creation stage.
func (r *Router) loadATDD(t *types.Task) ([]atdd.Case, string, error) {
	readCSV := func(path string) ([]atdd.Case, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return atdd.ParseCSV(f)
	}
	if p := strings.TrimSpace(t.Metadata["uat_atdd_path"]); p != "" {
		cases, err := readCSV(p)
		if err != nil {
			return nil, "", fmt.Errorf("ATDD sheet %s: %w", p, err)
		}
		return cases, filepath.Base(p), nil
	}

	type candidate struct {
		path  string
		label string
		mod   time.Time
	}
	var found []candidate
	collect := func(root, label string, match func(rel string) bool) {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "uat") {
				return filepath.SkipDir
			}
			rel, _ := filepath.Rel(root, path)
			if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".csv") && match(filepath.ToSlash(rel)) {
				if info, err := d.Info(); err == nil {
					found = append(found, candidate{path, label + filepath.ToSlash(rel), info.ModTime()})
				}
			}
			return nil
		})
	}
	collect(r.artifactDir(t.ID), "task document ", func(string) bool { return true })
	for _, wt := range r.describeWorktrees(context.Background(), t) {
		if wt.Exists {
			dir := filepath.Join(wt.Checkout, "_bmad-output")
			collect(dir, wt.Repo+" ", func(rel string) bool { return strings.Contains(strings.ToLower(rel), "atdd") })
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].mod.After(found[j].mod) })
	for _, c := range found {
		if cases, err := readCSV(c.path); err == nil && len(cases) > 0 {
			return cases, c.label, nil
		}
	}
	if cases, err := atdd.ParseBlock(r.readStageDoc(t.ID, "atdd_creation")); err == nil && len(cases) > 0 {
		return cases, "atdd_creation stage output", nil
	}
	return nil, "", nil
}

// uatStageContext is appended to the uat_verification prompt: the applications and every ATDD
// case the plan must cover.
func (r *Router) uatStageContext(t *types.Task) string {
	var b strings.Builder
	apps := r.taskApps(t)
	b.WriteString("Applications under test (use these ids in each scenario's \"app\"):\n")
	for _, a := range apps {
		fmt.Fprintf(&b, "- %s — %s (%s)", a.ID, a.Name, a.Kind)
		if len(a.Repos) > 0 {
			fmt.Fprintf(&b, "; repositories: %s", strings.Join(a.Repos, ", "))
		}
		if a.IsWeb() {
			if a.BaseURL != "" {
				fmt.Fprintf(&b, "; environment URL: %s", a.BaseURL)
			} else {
				b.WriteString("; environment URL: not set yet")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\nEach goto target is appended to its application's environment URL. Write it as the path the browser address bar shows " +
		"after the host, including the frontend router's base path (vue-router `base`, React Router `basename`, Vite `base`, Nginx location) — " +
		"read it from the repository, e.g. /billing/proforma-invoices, not /proforma-invoices. A prefix the environment URL already contains is not repeated.\n")
	cases, source, err := r.loadATDD(t)
	scope := atdd.UATScope(cases)
	switch {
	case err != nil:
		fmt.Fprintf(&b, "\nThe ATDD sheet could not be read (%v). Cover every acceptance criterion instead.\n", err)
	case len(scope) == 0:
		b.WriteString("\nNo ATDD case is marked UAT or Sanity for this task. Cover every acceptance criterion from the earlier stages.\n")
	default:
		fmt.Fprintf(&b, "\nATDD cases that MUST be covered (%d, from %s). Every id below must appear in the \"covers\" list of at least one scenario; "+
			"mark sanity cases with \"sanity\": true and keep them first. Keep the case's intent, preconditions and expected results; "+
			"turn its steps into concrete screen actions:\n", len(scope), source)
		for _, c := range scope {
			fmt.Fprintf(&b, "\n### %s [%s] %s — %s (platform %s, app %s)\n", c.ID, c.Kind(), c.Priority, c.Title, orDash(c.Platform), uat.AppFor(c, apps))
			writeList(&b, "Preconditions", c.Preconditions)
			writeList(&b, "Steps", c.Steps)
			writeList(&b, "Expected", c.ExpectedResults)
		}
	}
	return b.String()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func writeList(b *strings.Builder, label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", label)
	for i, it := range items {
		fmt.Fprintf(b, "  %d. %s\n", i+1, it)
	}
}

func (r *Router) setUATStatus(taskID, status, errMsg string, extra map[string]string) {
	r.mu.Lock()
	if t, ok := r.tasks[taskID]; ok {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		t.Metadata["uat_guide_status"] = status
		if errMsg != "" {
			t.Metadata["uat_guide_error"] = errMsg
		} else {
			delete(t.Metadata, "uat_guide_error")
		}
		if status == "READY" {
			t.Metadata["uat_guide_at"] = time.Now().Format(time.RFC3339)
		}
		for k, v := range extra {
			t.Metadata[k] = v
		}
		t.UpdatedAt = time.Now()
	}
	r.mu.Unlock()
	r.saveBoardNow()
	r.broadcastTask(r.taskSnapshot(taskID))
}

// startUATGuide generates the guide in the background unless a run is already in progress.
func (r *Router) startUATGuide(taskID string) bool {
	key := "uat:" + taskID
	if _, busy := r.busyOps.LoadOrStore(key, true); busy {
		return false
	}
	r.setUATStatus(taskID, "GENERATING", "", nil)
	go func() {
		defer r.busyOps.Delete(key)
		r.generateUATGuide(context.Background(), taskID)
	}()
	return true
}

func (r *Router) writeArtifact(taskID, name string, data []byte) error {
	p, err := r.resolveArtifactPath(taskID, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// generateUATGuide builds the guide from the latest uat_verification output and the ATDD sheet.
func (r *Router) generateUATGuide(ctx context.Context, taskID string) {
	fail := func(msg string) {
		r.setUATStatus(taskID, "FAILED", msg, nil)
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindError, Content: "UAT guide not generated: " + msg})
	}
	t := r.taskSnapshot(taskID)
	if t == nil {
		return
	}
	plan, err := uat.ExtractPlan(r.readStageDoc(taskID, "uat_verification"))
	if err != nil {
		fail(err.Error() + ". Re-run uat_verification so the agent emits the plan.")
		return
	}
	apps := r.taskApps(t)
	cases, source, atddErr := r.loadATDD(t)
	cov := uat.ApplyCoverage(plan, atdd.UATScope(cases), apps)
	plan.Fill(uatVars(t)) // test-data ids from the UAT Guide tab; secrets stay in the environment

	appOf := map[string]uat.App{}
	for _, a := range apps {
		appOf[a.ID] = a
	}
	envFor := func(s uat.Scenario) string {
		if a, ok := appOf[s.App]; ok && a.IsWeb() {
			return strings.TrimRight(a.BaseURL, "/")
		}
		return ""
	}
	if err := plan.Validate(envFor); err != nil {
		fail(err.Error())
		return
	}
	// Paths are inside each app: https://host/billing + /proforma-invoices opens
	// https://host/billing/proforma-invoices, and the guide shows testers that same address.
	if err := plan.ResolveTargets(envFor); err != nil {
		fail(err.Error())
		return
	}
	if raw, err := json.MarshalIndent(plan, "", "  "); err == nil {
		_ = r.writeArtifact(taskID, uatPlanDoc, raw)
	}
	if raw, err := json.MarshalIndent(cov, "", "  "); err == nil {
		_ = r.writeArtifact(taskID, uatCoverage, raw)
	}
	shotDir, err := r.resolveArtifactPath(taskID, uatShotsDir)
	if err != nil {
		fail(err.Error())
		return
	}
	_ = os.RemoveAll(shotDir) // screenshots always match the current plan

	meta := uat.GuideMeta{TaskID: taskID, JiraKey: t.Metadata["jira_key"], JiraURL: t.Metadata["jira_url"], ATDDSource: source,
		GeneratedAt: time.Now(), ImageURL: func(file string) string {
			return fmt.Sprintf("/api/v1/artifacts/%s/%s/%s", url.PathEscape(taskID), uatShotsDir, url.PathEscape(file))
		}}
	var notes []string
	if atddErr != nil {
		notes = append(notes, "The ATDD sheet could not be read ("+atddErr.Error()+"), so coverage could not be checked.")
	}

	// Every scenario gets an image for every step: replayed in its application when it can be,
	// otherwise drawn as step cards in the same frame, so the guide has no gaps.
	fromSheet := map[string]bool{}
	for _, c := range cov.Cases {
		if c.FromATDD && len(c.Scenarios) > 0 {
			fromSheet[c.Scenarios[0]] = true
		}
	}
	var job uat.CaptureJob
	missingEnv := map[string]bool{}
	for _, s := range plan.Scenarios {
		a, known := appOf[s.App]
		hasBrowserStep := false
		for _, st := range s.Steps {
			hasBrowserStep = hasBrowserStep || st.Automated()
		}
		env, note := envFor(s), ""
		switch {
		case fromSheet[s.ID]:
			note = "Written from the ATDD sheet: the agent's UAT plan did not walk through this case, so its steps are shown as written."
		case hasBrowserStep && env == "" && known && a.IsWeb():
			missingEnv[a.Name] = true
			note = "No environment URL is set for " + a.Name + ", so this screen was not captured. Set it in the UAT Guide tab and regenerate."
		case known && !a.IsWeb():
			note = a.Name + " has no screen: an engineer runs this step with the team's API client."
		}
		job.Scenarios = append(job.Scenarios, uat.NewJobScenario(s, env, a.StorageState, a.Name, note))
	}
	if len(missingEnv) > 0 {
		var names []string
		for n := range missingEnv {
			names = append(names, n)
		}
		sort.Strings(names)
		notes = append(notes, "No environment URL is set for "+strings.Join(names, ", ")+", so those scenarios are shown as step cards instead of real screens. Set it in the UAT Guide tab and regenerate.")
	}
	var results []uat.StepResult
	if len(job.Scenarios) > 0 {
		job.OutDir, job.IgnoreHTTPS = shotDir, t.Metadata["uat_ignore_https"] == "true"
		job.Label = strings.Join(nonEmpty(taskID, t.Metadata["jira_key"], "UAT"), " · ")
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
			"Capturing UAT screenshots for %d scenario(s)…", len(job.Scenarios))})
		results, err = r.screenshotRunner().Capture(ctx, job)
		if err != nil {
			notes = append(notes, "Screenshots could not be captured ("+err.Error()+"). The written steps are complete; verify them by hand.")
		}
	}
	notes = append(notes, uatResultNotes(results)...)
	meta.Note = strings.Join(notes, " ")

	scenarios, sum := uat.Assemble(plan, results, apps, cov)
	md := uat.RenderMarkdown(plan, scenarios, sum, cov, apps, meta)
	header := fmt.Sprintf("<!-- %s · uat guide · generated %s -->\n\n", taskID, meta.GeneratedAt.Format(time.RFC3339))
	if err := r.writeArtifact(taskID, uatGuideDoc, []byte(header+md)); err != nil {
		fail(err.Error())
		return
	}
	if html, err := uat.RenderHTML(md, plan.Feature, shotDir); err == nil {
		_ = r.writeArtifact(taskID, uatGuideHTML, html)
	}

	status := "READY"
	extra := map[string]string{"uat_coverage": fmt.Sprintf("%d/%d", cov.Planned, len(cov.Cases)),
		"uat_coverage_missing": strings.Join(cov.Missing, ",")}
	r.setUATStatus(taskID, status, meta.Note, extra)
	note := fmt.Sprintf("UAT guide ready: %d scenario(s), %d step(s), %d screenshot(s).", len(scenarios), sum.Steps, sum.Captured)
	if len(cov.Cases) > 0 {
		note += fmt.Sprintf(" Covers all %d ATDD UAT/Sanity case(s) from %s", len(cov.Cases), source)
		if len(cov.Missing) > 0 {
			note += fmt.Sprintf("; %d were missing from the agent's plan (%s) and are written from the sheet without screenshots — "+
				"reject this stage with feedback to get full walkthroughs", len(cov.Missing), strings.Join(cov.Missing, ", "))
		}
		note += "."
	}
	if sum.Failed > 0 {
		note += fmt.Sprintf(" %d step(s) need to be verified by hand.", sum.Failed)
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: note + " Open the UAT Guide tab to review or download it."})
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

// uatVarPrefix keys the per-task test-data values (${NAME} → value) in task metadata.
const uatVarPrefix = "uat_var."

func uatVars(t *types.Task) map[string]string {
	out := map[string]string{}
	for k, v := range t.Metadata {
		if name, ok := strings.CutPrefix(k, uatVarPrefix); ok {
			out[name] = v
		}
	}
	return out
}

// UATVariable is one ${NAME} placeholder of the plan and where its value comes from.
type UATVariable struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"` // only for task test data; environment values are never returned
	Source string `json:"source"`          // "task", "environment" or "missing"
	Secret bool   `json:"secret"`          // read from the orchestrator's environment only
	Used   bool   `json:"used"`            // referenced by the current plan
	// Browser is true when a replayed step needs the value; api/manual-only names are for the
	// engineer and do not block the screenshot run.
	Browser bool `json:"browser"`
}

// uatVariables lists the placeholders the current plan needs plus any value saved for the task.
func (r *Router) uatVariables(t *types.Task) []UATVariable {
	var names []string
	used, browser := map[string]bool{}, map[string]bool{}
	if plan, err := uat.ExtractPlan(r.readStageDoc(t.ID, "uat_verification")); err == nil {
		names, browser = plan.Placeholders()
		for _, n := range names {
			used[n] = true
		}
	}
	vars := uatVars(t)
	var extra []string
	for n := range vars {
		if !used[n] {
			extra = append(extra, n)
		}
	}
	sort.Strings(extra)
	out := make([]UATVariable, 0, len(names)+len(extra))
	for _, n := range append(names, extra...) {
		v := UATVariable{Name: n, Secret: uat.IsSecretName(n), Used: used[n], Browser: browser[n], Source: "missing"}
		if val, ok := vars[n]; ok && val != "" && !v.Secret {
			v.Value, v.Source = val, "task"
		} else if _, ok := os.LookupEnv(n); ok && strings.HasPrefix(n, "UAT_") {
			v.Source = "environment"
		}
		out = append(out, v)
	}
	return out
}

// uatResultNotes explains runs that reached the wrong page or lacked test data, which otherwise
// show up only as a 404 or blank screenshot inside one step.
func uatResultNotes(results []uat.StepResult) []string {
	var notFound, placeholder, login []string
	for _, res := range results {
		ref := fmt.Sprintf("%s step %d", res.Scenario, res.Step)
		switch res.Reason {
		case uat.ReasonNotFound:
			notFound = append(notFound, ref)
		case uat.ReasonPlaceholder:
			placeholder = append(placeholder, ref)
		case uat.ReasonLogin:
			login = append(login, ref)
		}
	}
	var notes []string
	if len(notFound) > 0 {
		notes = append(notes, fmt.Sprintf("%s opened a page that does not exist. Check that each application's environment URL "+
			"includes its base path (for example https://<backyard-host>/billing for Subscription Backyard) and that the plan uses real routes, then regenerate.",
			strings.Join(notFound, ", ")))
	}
	if len(login) > 0 {
		notes = append(notes, fmt.Sprintf("%s landed on a sign-in page, so those screenshots show the login screen. "+
			"Use \"Sign in\" for the application in the UAT Guide tab (Environments) to save a session, then regenerate.", strings.Join(login, ", ")))
	}
	if len(placeholder) > 0 {
		notes = append(notes, fmt.Sprintf("%s need a test-data value the orchestrator was not given; "+
			"fill it under Environments › Test data in the UAT Guide tab (credentials go in the orchestrator's environment) and regenerate.",
			strings.Join(placeholder, ", ")))
	}
	return notes
}

// uatSessionPath is where "Sign in" saves an application's session: one per application, shared by
// every task, inside .sdlc (0700 directory, 0600 file — it holds live session cookies).
func (r *Router) uatSessionPath(appID string) string {
	return filepath.Join(r.cfg.RootDir, ".sdlc", "uat", "sessions", appID+".json")
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// UATSession describes an application's saved sign-in, never its contents.
type UATSession struct {
	App     string `json:"app"`
	Source  string `json:"source"`             // "task" (a file set for this task), "shared" (saved with Sign in) or "none"
	SavedAt string `json:"saved_at,omitempty"` // when the file was written
	Login   string `json:"login,omitempty"`    // sign-in window state: "waiting", "ok" or "failed"
	Error   string `json:"error,omitempty"`
}

func (r *Router) uatSessions(t *types.Task, apps []uat.App) []UATSession {
	var out []UATSession
	for _, a := range apps {
		if !a.IsWeb() {
			continue
		}
		s := UATSession{App: a.ID, Source: "none", Login: t.Metadata["uat_login."+a.ID], Error: t.Metadata["uat_login_error."+a.ID]}
		switch {
		case a.StorageState != "" && a.StorageState == r.uatSessionPath(a.ID):
			s.Source = "shared"
		case a.StorageState != "":
			s.Source = "task"
		}
		if st, err := os.Stat(a.StorageState); a.StorageState != "" && err == nil {
			s.SavedAt = st.ModTime().Format(time.RFC3339)
		}
		out = append(out, s)
	}
	return out
}

// uatLoginRunner opens the sign-in window; PlaywrightRunner implements it.
type uatLoginRunner interface {
	Login(ctx context.Context, url, storagePath string, ignoreHTTPS bool) (uat.LoginResult, error)
}

func (r *Router) setUATLogin(taskID, appID, state, errMsg string) {
	r.mu.Lock()
	if t, ok := r.tasks[taskID]; ok {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		t.Metadata["uat_login."+appID] = state
		if errMsg != "" {
			t.Metadata["uat_login_error."+appID] = errMsg
		} else {
			delete(t.Metadata, "uat_login_error."+appID)
		}
		t.UpdatedAt = time.Now()
	}
	r.mu.Unlock()
	r.saveBoardNow()
	r.broadcastTask(r.taskSnapshot(taskID))
}

// handleUATLogin serves POST /api/v1/tasks/{id}/uat-guide/login {"app": "<id>"}: it opens a browser
// window on the orchestrator host at the application's environment URL, waits for the tester to sign
// in, and saves the session for every task testing that application. DELETE forgets the session.
func (r *Router) handleUATLogin(w http.ResponseWriter, req *http.Request, t *types.Task) {
	var body struct {
		App string `json:"app"`
	}
	if req.Method != http.MethodPost && req.Method != http.MethodDelete {
		r.writeError(w, http.StatusMethodNotAllowed, "POST or DELETE required for uat-guide/login")
		return
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || !safeTaskID.MatchString(body.App) {
		r.writeError(w, http.StatusBadRequest, "expected {\"app\": \"<application id>\"}")
		return
	}
	var app *uat.App
	for _, a := range r.taskApps(t) {
		if a.ID == body.App && a.IsWeb() {
			a := a
			app = &a
		}
	}
	if app == nil {
		r.writeError(w, http.StatusNotFound, "no web application "+body.App+" in this task")
		return
	}
	path := r.uatSessionPath(app.ID)
	if req.Method == http.MethodDelete {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			r.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		r.setUATLogin(t.ID, app.ID, "", "")
		r.writeJSON(w, http.StatusOK, r.uatGuideStatus(r.taskSnapshot(t.ID)))
		return
	}
	if !uat.ValidURL(app.BaseURL) {
		r.writeError(w, http.StatusBadRequest, "set the environment URL for "+app.Name+" first")
		return
	}
	runner, ok := r.screenshotRunner().(uatLoginRunner)
	if !ok {
		r.writeError(w, http.StatusNotImplemented, "this screenshot runner cannot open a sign-in window")
		return
	}
	key := "uat-login:" + app.ID
	if _, busy := r.busyOps.LoadOrStore(key, true); busy {
		r.writeError(w, http.StatusConflict, "a sign-in window for "+app.Name+" is already open")
		return
	}
	r.setUATLogin(t.ID, app.ID, "waiting", "")
	ignoreHTTPS := t.Metadata["uat_ignore_https"] == "true"
	go func() {
		defer r.busyOps.Delete(key)
		res, err := runner.Login(context.Background(), app.BaseURL, path, ignoreHTTPS)
		switch {
		case err != nil:
			r.setUATLogin(t.ID, app.ID, "failed", err.Error())
		case !res.OK:
			r.setUATLogin(t.ID, app.ID, "failed", res.Error)
		case !res.SignedIn:
			r.setUATLogin(t.ID, app.ID, "ok", "The window closed before it was back on "+app.Name+"; if screenshots still show a sign-in page, sign in again.")
		default:
			r.setUATLogin(t.ID, app.ID, "ok", "")
		}
		if err == nil && res.OK {
			r.addEntry(t.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: "UAT session saved for " + app.Name + ". Regenerate the UAT guide to capture signed-in screenshots."})
		}
	}()
	r.writeJSON(w, http.StatusAccepted, r.uatGuideStatus(r.taskSnapshot(t.ID)))
}

// UATAppSettings is one application's environment for this task.
type UATAppSettings struct {
	URL          *string `json:"url,omitempty"`
	StorageState *string `json:"storage_state,omitempty"`
}

// UATGuideRequest updates the guide settings and regenerates it.
type UATGuideRequest struct {
	Apps     map[string]UATAppSettings `json:"apps,omitempty"`
	ATDDPath *string                   `json:"atdd_path,omitempty"`
	// Test-data values for ${NAME} placeholders (record ids, codes). An empty value removes one.
	// Credentials are refused: they are read from the orchestrator's environment only.
	Variables   map[string]string `json:"variables,omitempty"`
	IgnoreHTTPS *bool             `json:"ignore_https_errors,omitempty"`
	// Deprecated single-environment fields (applied to every web app without its own URL).
	BaseURL      *string `json:"base_url,omitempty"`
	StorageState *string `json:"storage_state,omitempty"`
	// Only save the settings; do not regenerate.
	SaveOnly bool `json:"save_only,omitempty"`
}

// handleTaskUATGuide serves GET (status) and POST (settings + regenerate) /api/v1/tasks/{id}/uat-guide.
func (r *Router) handleTaskUATGuide(w http.ResponseWriter, req *http.Request, t *types.Task) {
	switch req.Method {
	case http.MethodGet:
	case http.MethodPost:
		var body UATGuideRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid UAT guide payload")
			return
		}
		if err := r.applyUATSettings(t.ID, body); err != nil {
			r.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !body.SaveOnly {
			if r.readStageDoc(t.ID, "uat_verification") == "" {
				r.writeError(w, http.StatusConflict, "run the uat_verification stage first — the guide is built from its plan")
				return
			}
			if !r.startUATGuide(t.ID) {
				r.writeError(w, http.StatusConflict, "the UAT guide is already being generated")
				return
			}
		}
		t = r.taskSnapshot(t.ID)
	default:
		r.writeError(w, http.StatusMethodNotAllowed, "GET or POST required for uat-guide")
		return
	}
	r.writeJSON(w, http.StatusOK, r.uatGuideStatus(t))
}

func (r *Router) uatGuideStatus(t *types.Task) map[string]interface{} {
	exists := func(name string) string {
		if p, err := r.resolveArtifactPath(t.ID, name); err == nil {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return name
			}
		}
		return ""
	}
	status := t.Metadata["uat_guide_status"]
	if status == "" {
		status = "NOT_STARTED"
	}
	var seedCheck *uat.SeedCheck
	seed, _ := uat.ExtractSeed(r.readStageDoc(t.ID, "uat_verification"))
	if seed != nil {
		c := seed.Check(context.Background()) // lint + parse only; the script is never executed here
		seedCheck = &c
	}
	apps := r.taskApps(t)
	cases, source, atddErr := r.loadATDD(t)
	scope := atdd.UATScope(cases)
	type scopeCase struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Kind     string `json:"kind"`
		Priority string `json:"priority"`
		Platform string `json:"platform"`
		App      string `json:"app"`
	}
	list := make([]scopeCase, 0, len(scope))
	sanity := 0
	for _, c := range scope {
		if c.Sanity {
			sanity++
		}
		list = append(list, scopeCase{c.ID, c.Title, c.Kind(), c.Priority, c.Platform, uat.AppFor(c, apps)})
	}
	atddError := ""
	if atddErr != nil {
		atddError = atddErr.Error()
	}
	var missing []string
	if m := t.Metadata["uat_coverage_missing"]; m != "" {
		missing = strings.Split(m, ",")
	}
	return map[string]interface{}{
		"task_id":             t.ID,
		"status":              status,
		"error":               t.Metadata["uat_guide_error"],
		"generated_at":        t.Metadata["uat_guide_at"],
		"ignore_https_errors": t.Metadata["uat_ignore_https"] == "true",
		"stage_ready":         r.readStageDoc(t.ID, "uat_verification") != "",
		"can_request_changes": t.State == types.TaskStateWaitingGateApproval && t.CurrentStageID == "uat_verification",
		"guide_path":          exists(uatGuideDoc),
		"html_path":           exists(uatGuideHTML),
		"apps":                apps,
		"variables":           r.uatVariables(t),
		"sessions":            r.uatSessions(t, apps),
		"seed":                seed,
		"seed_check":          seedCheck,
		"atdd": map[string]interface{}{
			"source": source, "path": t.Metadata["uat_atdd_path"], "error": atddError,
			"total": len(cases), "in_scope": len(scope), "sanity": sanity, "cases": list,
		},
		"coverage": map[string]interface{}{"planned": t.Metadata["uat_coverage"], "missing": missing},
	}
}

func (r *Router) applyUATSettings(taskID string, body UATGuideRequest) error {
	checkURL := func(v *string) error {
		if v != nil && strings.TrimSpace(*v) != "" && !uat.ValidURL(strings.TrimSpace(*v)) {
			return fmt.Errorf("environment URLs must be absolute http(s) URLs")
		}
		return nil
	}
	checkFile := func(v *string, what string) error {
		if v != nil && strings.TrimSpace(*v) != "" {
			if st, err := os.Stat(strings.TrimSpace(*v)); err != nil || st.IsDir() {
				return fmt.Errorf("%s %s does not exist", what, strings.TrimSpace(*v))
			}
		}
		return nil
	}
	if err := checkURL(body.BaseURL); err != nil {
		return err
	}
	if err := checkFile(body.StorageState, "session file"); err != nil {
		return err
	}
	if err := checkFile(body.ATDDPath, "ATDD sheet"); err != nil {
		return err
	}
	if body.ATDDPath != nil && strings.TrimSpace(*body.ATDDPath) != "" {
		f, err := os.Open(strings.TrimSpace(*body.ATDDPath))
		if err != nil {
			return err
		}
		_, perr := atdd.ParseCSV(f)
		f.Close()
		if perr != nil {
			return perr
		}
	}
	for name, v := range body.Variables {
		if !uat.ValidVarName(name) {
			return fmt.Errorf("invalid test data name %q: use letters, digits and _ only", name)
		}
		if uat.IsSecretName(name) && strings.TrimSpace(v) != "" {
			return fmt.Errorf("%s looks like a credential; set it in the orchestrator's environment instead of the task", name)
		}
		if len(v) > 512 || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("the value of %s must be a single line of at most 512 characters", name)
		}
	}
	for id, a := range body.Apps {
		if !safeTaskID.MatchString(id) {
			return fmt.Errorf("invalid application id %q", id)
		}
		if err := checkURL(a.URL); err != nil {
			return err
		}
		if err := checkFile(a.StorageState, "session file"); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[taskID]
	if !ok {
		return errTaskNotFound
	}
	if t.Metadata == nil {
		t.Metadata = map[string]string{}
	}
	set := func(key string, v *string) {
		if v == nil {
			return
		}
		if s := strings.TrimRight(strings.TrimSpace(*v), "/"); s == "" {
			delete(t.Metadata, key)
		} else {
			t.Metadata[key] = s
		}
	}
	set("uat_base_url", body.BaseURL)
	set("uat_storage_state", body.StorageState)
	set("uat_atdd_path", body.ATDDPath)
	for id, a := range body.Apps {
		set("uat_app."+id+".url", a.URL)
		set("uat_app."+id+".storage_state", a.StorageState)
	}
	for name, v := range body.Variables {
		if v = strings.TrimSpace(v); v == "" {
			delete(t.Metadata, uatVarPrefix+name)
		} else {
			t.Metadata[uatVarPrefix+name] = v
		}
	}
	if body.IgnoreHTTPS != nil {
		t.Metadata["uat_ignore_https"] = fmt.Sprint(*body.IgnoreHTTPS)
	}
	t.UpdatedAt = time.Now()
	return nil
}
