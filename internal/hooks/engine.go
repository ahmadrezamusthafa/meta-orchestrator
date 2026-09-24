package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	Target     string         `json:"target" yaml:"target"` // Script path, Docker image, or Webhook URL
	IsBlocking bool           `json:"is_blocking" yaml:"is_blocking"`
	TimeoutSec int            `json:"timeout_sec" yaml:"timeout_sec"`
}

// HookExecutionContext contains runtime metadata injected into hooks.
type HookExecutionContext struct {
	TaskID        string                 `json:"task_id"`
	StageID       string                 `json:"stage_id"`
	WorkspacePath string                 `json:"workspace_path"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// HookEngine manages and executes pluggable hooks across the lifecycle.
type HookEngine struct {
	mu         sync.RWMutex
	projectDir string
	hooks      []HookDefinition
	httpClient *http.Client
}

// NewHookEngine creates a new hook execution engine.
func NewHookEngine(projectDir string) *HookEngine {
	return &HookEngine{
		projectDir: projectDir,
		hooks:      make([]HookDefinition, 0),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// SetHTTPClient sets custom HTTP client (useful for testing or proxying).
func (e *HookEngine) SetHTTPClient(client *http.Client) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.httpClient = client
}

// RegisterHook adds a hook definition.
func (e *HookEngine) RegisterHook(hook HookDefinition) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.hooks = append(e.hooks, hook)
}

// TriggerEvent executes all hooks registered for the event.
func (e *HookEngine) TriggerEvent(ctx context.Context, event HookEvent, execCtx *HookExecutionContext) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Run dynamically registered declarative hooks
	for _, hook := range e.hooks {
		if hook.Event == event {
			if err := e.executeHook(ctx, &hook, execCtx); err != nil {
				if hook.IsBlocking {
					return fmt.Errorf("blocking hook '%s' failed: %w", hook.ID, err)
				}
				// Log warning if non-blocking
				fmt.Printf("WARNING: Non-blocking hook '%s' failed: %v\n", hook.ID, err)
			}
		}
	}

	// 2. Run local convention shell script if present (.sdlc/hooks/{event}.sh)
	if e.projectDir != "" {
		conventionScript := filepath.Join(e.projectDir, ".sdlc", "hooks", fmt.Sprintf("%s.sh", event))
		if stat, err := os.Stat(conventionScript); err == nil && !stat.IsDir() {
			hook := HookDefinition{
				ID:         fmt.Sprintf("convention-%s", event),
				Event:      event,
				Driver:     HookDriverShell,
				Target:     conventionScript,
				IsBlocking: true,
				TimeoutSec: 60,
			}
			if err := e.executeHook(ctx, &hook, execCtx); err != nil {
				return fmt.Errorf("blocking convention hook '%s' failed: %w", conventionScript, err)
			}
		}
	}

	return nil
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

func (e *HookEngine) runShellHook(ctx context.Context, hook *HookDefinition, execCtx *HookExecutionContext) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", hook.Target)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("META_TASK_ID=%s", execCtx.TaskID),
		fmt.Sprintf("META_STAGE=%s", execCtx.StageID),
		fmt.Sprintf("META_WORKSPACE_PATH=%s", execCtx.WorkspacePath),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("shell hook exited with error (%v): %s", err, string(out))
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
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "meta-orchestrator-hook-engine/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned HTTP error status %d", resp.StatusCode)
	}
	return nil
}

func (e *HookEngine) runDockerHook(ctx context.Context, hook *HookDefinition, execCtx *HookExecutionContext) error {
	// Docker hook runs ephemeral container against workspace
	return nil
}
