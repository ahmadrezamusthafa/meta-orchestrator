package hooks

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// HookEvent defines valid lifecycle interception points.
type HookEvent string

const (
	HookPreStage   HookEvent = "pre-stage"
	HookPostStage  HookEvent = "post-stage"
	HookOnFailure  HookEvent = "on-failure"
	HookOnGate     HookEvent = "on-gate"
	HookPreCommit  HookEvent = "pre-commit"
	HookPostCommit HookEvent = "post-commit"
)

// Events lists every lifecycle event in the order a stage meets them.
var Events = []HookEvent{HookPreStage, HookPostStage, HookOnFailure, HookOnGate, HookPreCommit, HookPostCommit}

// HookDriverType represents the execution runner type.
type HookDriverType string

const (
	HookDriverShell   HookDriverType = "shell"
	HookDriverDocker  HookDriverType = "docker"
	HookDriverWebhook HookDriverType = "webhook"
)

// HookDefinition configures an individual lifecycle hook.
type HookDefinition struct {
	ID         string         `json:"id" yaml:"id"`
	Event      HookEvent      `json:"event" yaml:"event"`
	Driver     HookDriverType `json:"driver" yaml:"driver"`
	Target     string         `json:"target" yaml:"target"` // Shell command, Docker image, or Webhook URL
	IsBlocking bool           `json:"is_blocking" yaml:"is_blocking"`
	TimeoutSec int            `json:"timeout_sec" yaml:"timeout_sec"`
	// Source is where the hook was defined: "config" (hooks.json) or "convention" (.sdlc/hooks/<event>.sh).
	Source string `json:"source,omitempty" yaml:"-"`
}

// HookExecutionContext contains runtime metadata injected into hooks.
type HookExecutionContext struct {
	Event         HookEvent              `json:"event"`
	TaskID        string                 `json:"task_id"`
	StageID       string                 `json:"stage_id"`
	WorkspacePath string                 `json:"workspace_path"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// maxHookOutput bounds the hook output kept in an error message.
const maxHookOutput = 4096

// HookEngine manages and executes pluggable hooks across the lifecycle.
type HookEngine struct {
	mu         sync.RWMutex
	projectDir string
	hooks      []HookDefinition
	httpClient *http.Client
	dockerBin  string
}

// NewHookEngine creates a new hook execution engine. Convention scripts are looked up under
// projectDir/.sdlc/hooks; an empty projectDir disables them.
func NewHookEngine(projectDir string) *HookEngine {
	return &HookEngine{
		projectDir: projectDir,
		hooks:      make([]HookDefinition, 0),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		dockerBin:  "docker",
	}
}

// SetHTTPClient sets custom HTTP client (useful for testing or proxying).
func (e *HookEngine) SetHTTPClient(client *http.Client) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.httpClient = client
}

// SetDockerBinary sets the docker CLI the Docker driver runs (tests point it at a fake).
func (e *HookEngine) SetDockerBinary(path string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dockerBin = path
}

// RegisterHook adds a hook definition.
func (e *HookEngine) RegisterHook(hook HookDefinition) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if hook.Source == "" {
		hook.Source = "config"
	}
	e.hooks = append(e.hooks, hook)
}

// LoadDefinitions reads a JSON list of hook definitions (the operator's .sdlc/hooks.json). A missing
// file is no hooks.
func LoadDefinitions(path string) ([]HookDefinition, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read hooks file: %w", err)
	}
	var defs []HookDefinition
	if err := json.Unmarshal(data, &defs); err != nil {
		return nil, fmt.Errorf("parse hooks file %s: %w", path, err)
	}
	seen := map[string]bool{}
	for i, d := range defs {
		if err := d.Validate(); err != nil {
			return nil, fmt.Errorf("hook %d in %s: %w", i, path, err)
		}
		if seen[d.ID] {
			return nil, fmt.Errorf("hook ID %q appears twice in %s", d.ID, path)
		}
		seen[d.ID] = true
	}
	return defs, nil
}

// Validate checks that a definition names a known event and driver and has an ID and target.
func (h HookDefinition) Validate() error {
	if strings.TrimSpace(h.ID) == "" {
		return fmt.Errorf("hook is missing its id")
	}
	known := false
	for _, ev := range Events {
		known = known || ev == h.Event
	}
	if !known {
		return fmt.Errorf("hook %q has unknown event %q", h.ID, h.Event)
	}
	switch h.Driver {
	case HookDriverShell, HookDriverDocker, HookDriverWebhook:
	default:
		return fmt.Errorf("hook %q has unknown driver %q", h.ID, h.Driver)
	}
	if strings.TrimSpace(h.Target) == "" {
		return fmt.Errorf("hook %q has no target", h.ID)
	}
	return nil
}

// List returns the configured hooks plus the convention scripts present, for every event.
func (e *HookEngine) List() []HookDefinition {
	var out []HookDefinition
	for _, ev := range Events {
		out = append(out, e.hooksFor(ev)...)
	}
	return out
}

// HasHooks reports whether anything would run for event.
func (e *HookEngine) HasHooks(event HookEvent) bool {
	return len(e.hooksFor(event)) > 0
}

// hooksFor snapshots the hooks for event, so none run while the engine's lock is held.
func (e *HookEngine) hooksFor(event HookEvent) []HookDefinition {
	e.mu.RLock()
	var out []HookDefinition
	for _, h := range e.hooks {
		if h.Event == event {
			out = append(out, h)
		}
	}
	e.mu.RUnlock()
	if e.projectDir != "" {
		script := filepath.Join(e.projectDir, ".sdlc", "hooks", string(event)+".sh")
		if stat, err := os.Stat(script); err == nil && !stat.IsDir() {
			out = append(out, HookDefinition{
				ID:         "convention-" + string(event),
				Event:      event,
				Driver:     HookDriverShell,
				Target:     "sh " + shellQuote(script),
				IsBlocking: true,
				TimeoutSec: 60,
				Source:     "convention",
			})
		}
	}
	return out
}

// TriggerEvent runs every hook for the event in order. A blocking hook's failure stops the run and
// is returned as err; non-blocking failures are returned as warnings for the caller to report.
func (e *HookEngine) TriggerEvent(ctx context.Context, event HookEvent, execCtx *HookExecutionContext) (warnings []error, err error) {
	if execCtx == nil {
		execCtx = &HookExecutionContext{}
	}
	execCtx.Event = event
	for _, hook := range e.hooksFor(event) {
		hook := hook
		if herr := e.executeHook(ctx, &hook, execCtx); herr != nil {
			if hook.IsBlocking {
				return warnings, fmt.Errorf("blocking hook '%s' failed: %w", hook.ID, herr)
			}
			warnings = append(warnings, fmt.Errorf("non-blocking hook '%s' failed: %w", hook.ID, herr))
		}
	}
	return warnings, nil
}

func (e *HookEngine) executeHook(ctx context.Context, hook *HookDefinition, execCtx *HookExecutionContext) error {
	timeout := time.Duration(hook.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	execCtxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch hook.Driver {
	case HookDriverShell:
		return e.runShellHook(execCtxTimeout, hook, execCtx)
	case HookDriverWebhook:
		return e.runWebhookHook(execCtxTimeout, hook, execCtx)
	case HookDriverDocker:
		return e.runDockerHook(execCtxTimeout, hook, execCtx)
	default:
		return fmt.Errorf("unsupported hook driver: %s", hook.Driver)
	}
}

func metaEnv(execCtx *HookExecutionContext, workspace string) []string {
	return []string{
		"META_EVENT=" + string(execCtx.Event),
		"META_TASK_ID=" + execCtx.TaskID,
		"META_STAGE=" + execCtx.StageID,
		"META_WORKSPACE_PATH=" + workspace,
	}
}

// runShellHook runs the operator's command with sh -c in the task workspace. The environment is the
// daemon's own plus META_* variables; nothing else is added.
func (e *HookEngine) runShellHook(ctx context.Context, hook *HookDefinition, execCtx *HookExecutionContext) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", hook.Target)
	cmd.Env = append(os.Environ(), metaEnv(execCtx, execCtx.WorkspacePath)...)
	if dirExists(execCtx.WorkspacePath) {
		cmd.Dir = execCtx.WorkspacePath
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return fmt.Errorf("shell hook exited with error (%v): %s", err, capOutput(out))
	}
	return nil
}

func (e *HookEngine) runWebhookHook(ctx context.Context, hook *HookDefinition, execCtx *HookExecutionContext) error {
	payloadBytes, err := json.Marshal(execCtx)
	if err != nil {
		return fmt.Errorf("failed to serialize webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", hook.Target, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: invalid target URL")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "meta-orchestrator-hook-engine/1.0")

	e.mu.RLock()
	client := e.httpClient
	e.mu.RUnlock()
	resp, err := client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err // the URL itself may carry a token: never echo it
		}
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned HTTP error status %d", resp.StatusCode)
	}
	return nil
}

// runDockerHook runs the image named by Target in an ephemeral container with the task workspace
// mounted read-write at /workspace. A container left behind by a timeout is removed.
func (e *HookEngine) runDockerHook(ctx context.Context, hook *HookDefinition, execCtx *HookExecutionContext) error {
	ws := execCtx.WorkspacePath
	if ws == "" || !filepath.IsAbs(ws) || !dirExists(ws) {
		return fmt.Errorf("docker hook needs an existing absolute workspace directory, got %q", ws)
	}
	e.mu.RLock()
	bin := e.dockerBin
	e.mu.RUnlock()

	name := "meta-hook-" + randomSuffix()
	args := []string{"run", "--rm", "--name", name, "-v", ws + ":/workspace", "-w", "/workspace"}
	for _, kv := range metaEnv(execCtx, "/workspace") {
		args = append(args, "-e", kv)
	}
	args = append(args, hook.Target)
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = exec.CommandContext(cleanup, bin, "rm", "-f", name).Run()
		cancel()
		err = ctx.Err()
	}
	return fmt.Errorf("docker hook (%s) failed (%v): %s", hook.Target, err, capOutput(out))
}

// Redacted returns the definition safe to display: a webhook target keeps only its scheme and host,
// since the path or query of a webhook URL is often its secret.
func (h HookDefinition) Redacted() HookDefinition {
	if h.Driver == HookDriverWebhook {
		if u, err := url.Parse(h.Target); err == nil && u.Host != "" {
			h.Target = u.Scheme + "://" + u.Hostname() + "/…"
		} else {
			h.Target = "(redacted)"
		}
	}
	return h
}

// capOutput keeps the tail of a hook's output, where errors usually are.
func capOutput(out []byte) string {
	s := strings.TrimSpace(string(out))
	if len(s) > maxHookOutput {
		s = "…" + s[len(s)-maxHookOutput:]
	}
	return s
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func dirExists(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
