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
	Screenshot string `json:"screenshot,omitempty"` // file name inside OutDir
	URL        string `json:"url,omitempty"`
}

// PlaywrightRunner captures screenshots with playwright-core, installed on first use into Dir.
type PlaywrightRunner struct {
	Dir     string        // tool directory holding runner.mjs and node_modules
	Timeout time.Duration // whole run; default 10 minutes
	mu      sync.Mutex
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
