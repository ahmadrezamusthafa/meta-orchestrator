package uat

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed runner.mjs
var runnerScript []byte

// JobScenario is a scenario replayed against its app's environment.
type JobScenario struct {
	Scenario
	BaseURL      string `json:"base_url"`
	StorageState string `json:"storage_state,omitempty"` // Playwright storage-state file with a logged-in session
}

// CaptureJob is one screenshot run: every scenario that has a web environment to run against.
type CaptureJob struct {
	Scenarios   []JobScenario
	OutDir      string // screenshots land here
	IgnoreHTTPS bool
}

// StepResult is what happened when the runner drove one step.
type StepResult struct {
	Scenario   string `json:"scenario"`
	Step       int    `json:"step"`
	OK         bool   `json:"ok"`
	Skipped    bool   `json:"skipped,omitempty"` // manual step, not automated
	Error      string `json:"error,omitempty"`
	Reason     string `json:"reason,omitempty"`     // ReasonNotFound or ReasonPlaceholder when the runner knows why
	Screenshot string `json:"screenshot,omitempty"` // file name inside OutDir
	URL        string `json:"url,omitempty"`
}

// Why a step failed, when the runner can tell.
const (
	ReasonNotFound    = "not_found"      // the page is an HTTP error or the app's own not-found screen
	ReasonPlaceholder = "placeholder"    // a ${…} test-data value the runner has no value for
	ReasonLogin       = "login_required" // the app redirected to sign-in: no saved session, or it expired
)

// PlaywrightRunner captures screenshots with playwright-core, installed on first use into Dir.
type PlaywrightRunner struct {
	Dir string // tool directory holding runner.mjs and node_modules
	// HeadlessLogin runs the sign-in window headless (tests only: nobody can sign in to it).
	HeadlessLogin bool
	Timeout       time.Duration // whole run; default 10 minutes
	mu            sync.Mutex
}

// captureTimeout bounds one run so a hung page never stalls the daemon.
const captureTimeout = 10 * time.Minute

func command(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stderr, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 600 {
			msg = "…" + msg[len(msg)-600:]
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), msg)
	}
	return nil
}

// ensure installs playwright-core and a Chromium build into the tool directory once.
func (p *PlaywrightRunner) ensure(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("node is not installed — install Node.js to capture UAT screenshots")
	}
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(p.Dir, "runner.mjs"), runnerScript, 0o644); err != nil {
		return err
	}
	pkg := filepath.Join(p.Dir, "node_modules", "playwright-core", "package.json")
	if _, err := os.Stat(pkg); err == nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(p.Dir, "package.json")); os.IsNotExist(err) {
		_ = os.WriteFile(filepath.Join(p.Dir, "package.json"), []byte(`{"name":"uat-runner","private":true,"type":"module"}`+"\n"), 0o644)
	}
	if err := command(ctx, p.Dir, "npm", "install", "--no-audit", "--no-fund", "--silent", "playwright-core"); err != nil {
		return fmt.Errorf("install playwright-core: %w", err)
	}
	// Downloads Chromium only when this playwright-core build has none cached yet.
	if err := command(ctx, p.Dir, "node", filepath.Join("node_modules", "playwright-core", "cli.js"), "install", "chromium"); err != nil {
		return fmt.Errorf("install chromium: %w", err)
	}
	return nil
}

// Capture drives every scenario in a headless browser and returns one result per step.
func (p *PlaywrightRunner) Capture(ctx context.Context, job CaptureJob) ([]StepResult, error) {
	timeout := p.Timeout
	if timeout == 0 {
		timeout = captureTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := p.ensure(ctx); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(job.OutDir, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "uat-job-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	spec := map[string]interface{}{
		"out_dir":             job.OutDir,
		"ignore_https_errors": job.IgnoreHTTPS,
		"scenarios":           job.Scenarios,
		"results_path":        filepath.Join(tmp, "results.json"),
	}
	raw, _ := json.Marshal(spec)
	jobPath := filepath.Join(tmp, "job.json")
	if err := os.WriteFile(jobPath, raw, 0o600); err != nil {
		return nil, err
	}
	runErr := command(ctx, p.Dir, "node", "runner.mjs", jobPath)
	var results []StepResult
	if out, err := os.ReadFile(filepath.Join(tmp, "results.json")); err == nil {
		_ = json.Unmarshal(out, &results)
	}
	if runErr != nil && len(results) == 0 {
		return nil, runErr
	}
	return results, nil
}

// loginTimeout bounds how long the sign-in window stays open.
const loginTimeout = 5 * time.Minute

// LoginResult is what the sign-in window produced.
type LoginResult struct {
	OK       bool   `json:"ok"`
	Cookies  int    `json:"cookies,omitempty"`
	SignedIn bool   `json:"signed_in,omitempty"` // the window returned to the app without a sign-in form
	Error    string `json:"error,omitempty"`
}

// Login opens a visible browser window at url so a tester signs in once (SSO included), then saves
// the session as a Playwright storage state at storagePath (0600). The file holds live session
// cookies: it is never returned by the API or written into a guide.
func (p *PlaywrightRunner) Login(ctx context.Context, url, storagePath string, ignoreHTTPS bool) (LoginResult, error) {
	ctx, cancel := context.WithTimeout(ctx, loginTimeout+time.Minute)
	defer cancel()
	if err := p.ensure(ctx); err != nil {
		return LoginResult{}, err
	}
	tmp, err := os.MkdirTemp("", "uat-login-")
	if err != nil {
		return LoginResult{}, err
	}
	defer os.RemoveAll(tmp)
	spec := map[string]interface{}{
		"mode": "login", "url": url, "storage_path": storagePath, "ignore_https_errors": ignoreHTTPS,
		"timeout_ms": loginTimeout.Milliseconds(), "results_path": filepath.Join(tmp, "results.json"), "headless": p.HeadlessLogin,
	}
	raw, _ := json.Marshal(spec)
	jobPath := filepath.Join(tmp, "job.json")
	if err := os.WriteFile(jobPath, raw, 0o600); err != nil {
		return LoginResult{}, err
	}
	runErr := command(ctx, p.Dir, "node", "runner.mjs", jobPath)
	var res LoginResult
	out, err := os.ReadFile(filepath.Join(tmp, "results.json"))
	if err != nil || json.Unmarshal(out, &res) != nil {
		if runErr != nil {
			return LoginResult{}, runErr
		}
		return LoginResult{}, fmt.Errorf("the sign-in window returned no result")
	}
	return res, nil
}
