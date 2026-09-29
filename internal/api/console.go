package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

const (
	maxConsoleEntries   = 1000
	maxRecordedChars    = 8000 // per request message recorded in the transcript
	maxHistoryTurns     = 20   // conversation turns replayed to stateless providers
	chatTurnTimeout     = 10 * time.Minute
	defaultChatMaxToken = 4096
)

// taskConsole is the live transcript and turn state of one task.
type taskConsole struct {
	entries   []types.ConsoleEntry
	busy      bool
	turnID    string
	cancel    context.CancelFunc
	sessionID string // provider session (Claude Code CLI) reused across chat turns
	model     string // last model that answered
}

// consoleHub holds all task consoles behind its own lock (independent of Router.mu).
type consoleHub struct {
	mu       sync.Mutex
	consoles map[string]*taskConsole
	seq      atomic.Int64
}

func newConsoleHub() *consoleHub { return &consoleHub{consoles: map[string]*taskConsole{}} }

func (h *consoleHub) get(taskID string) *taskConsole {
	c, ok := h.consoles[taskID]
	if !ok {
		c = &taskConsole{}
		h.consoles[taskID] = c
	}
	return c
}

func (h *consoleHub) nextID(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixMilli(), h.seq.Add(1))
}

func (r *Router) broadcast(taskID string, typ types.EventType, payload interface{}) {
	if r.cfg.WSHub == nil {
		return
	}
	r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{Type: typ, TaskID: taskID, Timestamp: time.Now(), Payload: payload})
}

// addEntry appends an entry (assigning id and timestamps) and publishes it.
func (r *Router) addEntry(taskID string, e types.ConsoleEntry) types.ConsoleEntry {
	now := time.Now()
	h := r.console
	h.mu.Lock()
	if e.ID == "" {
		e.ID = h.nextID("act")
	}
	e.TaskID = taskID
	e.CreatedAt, e.UpdatedAt = now, now
	if e.Status == "" {
		e.Status = types.ConsoleDone
	}
	c := h.get(taskID)
	c.entries = append(c.entries, e)
	if over := len(c.entries) - maxConsoleEntries; over > 0 {
		c.entries = append([]types.ConsoleEntry(nil), c.entries[over:]...)
	}
	h.mu.Unlock()
	r.broadcast(taskID, types.EventAgentActivity, e)
	return e
}

// updateEntry mutates an entry in place and publishes the authoritative copy.
func (r *Router) updateEntry(taskID, id string, fn func(e *types.ConsoleEntry)) {
	h := r.console
	h.mu.Lock()
	c := h.get(taskID)
	var out *types.ConsoleEntry
	for i := len(c.entries) - 1; i >= 0; i-- {
		if c.entries[i].ID == id {
			fn(&c.entries[i])
			c.entries[i].UpdatedAt = time.Now()
			cp := c.entries[i]
			out = &cp
			break
		}
	}
	h.mu.Unlock()
	if out != nil {
		r.broadcast(taskID, types.EventAgentActivity, *out)
	}
}

// appendDelta streams text into a streaming entry.
func (r *Router) appendDelta(taskID, id, kind, text string) {
	h := r.console
	h.mu.Lock()
	c := h.get(taskID)
	for i := len(c.entries) - 1; i >= 0; i-- {
		if c.entries[i].ID == id {
			c.entries[i].Content += text
			c.entries[i].UpdatedAt = time.Now()
			break
		}
	}
	h.mu.Unlock()
	r.broadcast(taskID, types.EventAgentActivityDelta, map[string]string{"id": id, "delta": text, "kind": kind})
}

// recordStateChange writes a state entry when a task's state or stage changes.
func (r *Router) recordStateChange(taskID, from, to, stage string) {
	if from == to {
		return
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindState, State: &types.ConsoleState{From: from, To: to, Stage: stage}})
}

// setTaskState changes a task's state and records the transition in its console.
func (r *Router) setTaskState(task *types.Task, state types.TaskState) {
	if task == nil {
		return
	}
	prev := task.State
	task.State = state
	r.recordStateChange(task.ID, string(prev), string(state), task.CurrentStageID)
}

func truncateRecorded(s string) string {
	if len(s) <= maxRecordedChars {
		return s
	}
	return s[:maxRecordedChars] + fmt.Sprintf("\n… [%d more characters not recorded]", len(s)-maxRecordedChars)
}

// agentTurn is one request→response exchange routed through the 9router chain.
type agentTurn struct {
	Source    string // "chat" or "execute"
	TurnID    string
	Task      *types.Task
	Decision  *router.RoutingDecision
	TaskType  string
	Messages  []llm.Message
	SessionID string
	WorkDir   string
	MaxTokens int
	// PermissionMode for agentic providers; empty keeps them read-only.
	PermissionMode string
	// Approver answers the agent's permission prompts; nil denies them.
	Approver llm.Approver
}

// beginTurn marks the task console busy. It fails if a turn is already running.
func (r *Router) beginTurn(taskID, turnID string, cancel context.CancelFunc) error {
	h := r.console
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.get(taskID)
	if c.busy {
		return fmt.Errorf("a turn is already running for %s (turn %s)", taskID, c.turnID)
	}
	c.busy, c.turnID, c.cancel = true, turnID, cancel
	return nil
}

func (r *Router) endTurn(taskID, turnID string, resp *llm.LLMResponse, model string) {
	h := r.console
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.get(taskID)
	if c.turnID != turnID {
		return
	}
	c.busy, c.turnID, c.cancel = false, "", nil
	if resp != nil {
		if resp.SessionID != "" {
			c.sessionID = resp.SessionID
		}
		c.model = model
	}
}

// runAgentTurn streams one turn into the console transcript: request entry, live assistant /
// thinking / tool entries, failovers, and a response (usage) or error entry.
func (r *Router) runAgentTurn(ctx context.Context, t agentTurn) (*llm.LLMResponse, string, error) {
	taskID := t.Task.ID
	recorded := make([]types.ConsoleMessage, 0, len(t.Messages))
	for _, m := range t.Messages {
		recorded = append(recorded, types.ConsoleMessage{Role: string(m.Role), Content: truncateRecorded(m.Content)})
	}
	chain := t.Decision.FallbackChain
	if len(chain) == 0 {
		chain = []string{t.Decision.Model}
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindRequest, TurnID: t.TurnID, Request: &types.ConsoleRequest{
		Model: chain[0], Method: t.Decision.Method, Strategy: t.Decision.Strategy, FallbackChain: chain,
		Messages: recorded, SessionID: t.SessionID, Source: t.Source,
	}})

	var textID, thinkID string
	var streaming []string
	closeStreams := func(status string) {
		for _, id := range streaming {
			r.updateEntry(taskID, id, func(e *types.ConsoleEntry) { e.Status = status })
		}
		streaming = nil
		textID, thinkID = "", ""
	}
	emit := func(ev llm.StreamEvent) {
		switch ev.Type {
		case llm.StreamTextDelta:
			if ev.Text == "" {
				return
			}
			if textID == "" {
				e := r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindAssistant, TurnID: t.TurnID, Status: types.ConsoleStreaming})
				textID = e.ID
				streaming = append(streaming, e.ID)
			}
			r.appendDelta(taskID, textID, types.ConsoleKindAssistant, ev.Text)
		case llm.StreamThinkingDelta:
			if thinkID == "" {
				e := r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindThinking, TurnID: t.TurnID, Status: types.ConsoleStreaming})
				thinkID = e.ID
				streaming = append(streaming, e.ID)
			}
			r.appendDelta(taskID, thinkID, types.ConsoleKindThinking, ev.Text)
		case llm.StreamToolUse:
			closeStreams(types.ConsoleDone) // text after a tool call is a new assistant block
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindToolUse, TurnID: t.TurnID,
				Tool: &types.ConsoleTool{ID: ev.ToolID, Name: ev.ToolName, Input: ev.ToolInput}})
		case llm.StreamToolResult:
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindToolResult, TurnID: t.TurnID, Content: ev.Text,
				Tool: &types.ConsoleTool{ID: ev.ToolID, IsError: ev.IsError}})
		case llm.StreamSession:
			if ev.Model != "" || ev.SessionID != "" {
				r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: t.TurnID,
					Content: strings.TrimSpace(fmt.Sprintf("Session %s · model %s", shortID(ev.SessionID), ev.Model))})
			}
		}
	}
	onFailover := func(failed, next string, err error) {
		closeStreams(types.ConsoleFailed)
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: t.TurnID,
			Content: fmt.Sprintf("Failover: %s failed (%v) → trying %s", failed, err, next)})
	}

	req := &llm.LLMRequest{Messages: t.Messages, MaxTokens: t.MaxTokens, SessionID: t.SessionID, WorkDir: t.WorkDir, PermissionMode: t.PermissionMode, Approver: t.Approver}
	start := time.Now()
	resp, used, err := r.clientFactory.StreamWithFallbackChain(ctx, chain, req, emit, onFailover)

	switch {
	case ctx.Err() == context.Canceled:
		closeStreams(types.ConsoleCancelled)
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: t.TurnID, Content: "Interrupted by operator"})
		return nil, used, ctx.Err()
	case err != nil:
		closeStreams(types.ConsoleFailed)
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindError, TurnID: t.TurnID, Content: err.Error(), Status: types.ConsoleFailed})
		return nil, used, err
	}
	closeStreams(types.ConsoleDone)

	cost := resp.TokenUsage.EstimatedCostUSD
	if r.telemetry != nil {
		repo := ""
		if len(t.Task.AssignedRepos) > 0 {
			repo = t.Task.AssignedRepos[0]
		}
		ev := r.telemetry.tracker.Record(telemetry.CallMeta{TaskID: taskID, StageID: t.Task.CurrentStageID, Model: used,
			Provider: resp.Provider, Tier: t.Decision.Tier, Method: t.Decision.Method, Repo: repo, Category: t.TaskType}, resp.TokenUsage)
		cost = ev.CostUSD
		resp.TokenUsage.EstimatedCostUSD = cost
	}
	dur := resp.DurationMS
	if dur == 0 {
		dur = time.Since(start).Milliseconds()
	}
	model := used
	if resp.Model != "" && !strings.HasSuffix(used, "/"+resp.Model) {
		model = fmt.Sprintf("%s (%s)", used, resp.Model)
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindResponse, TurnID: t.TurnID, Usage: &types.ConsoleUsage{
		Model: model, Provider: resp.Provider, PromptTokens: resp.TokenUsage.PromptTokens,
		CompletionTokens: resp.TokenUsage.CompletionTokens, CachedTokens: resp.TokenUsage.CachedTokens,
		CostUSD: cost, DurationMS: dur, FinishReason: resp.FinishReason, SessionID: resp.SessionID,
	}})
	return resp, used, nil
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// resolveWorkDir picks a real directory for agentic providers: the task's git worktree (the
// worktree root when several repos are assigned), else the task's process dir if it exists, else
// the first assigned repo's registered path, else the orchestrator root.
func (r *Router) resolveWorkDir(task *types.Task) string {
	var ready []string
	checkouts := map[string]bool{}
	for _, wt := range r.ensureTaskWorktrees(context.Background(), task.ID) {
		if wt.Exists && wt.Error == "" {
			ready = append(ready, wt.Dir)
			checkouts[wt.Checkout] = true
		}
	}
	switch {
	case len(ready) == 1:
		return ready[0]
	case len(checkouts) == 1: // several repos inside one git repository
		for c := range checkouts {
			return c
		}
	case len(ready) > 1:
		return r.taskWorktreeRoot(task.ID)
	}
	r.mu.RLock()
	proc := r.taskProcesses[task.ID]
	r.mu.RUnlock()
	if proc != nil && proc.WorkingDir != "" && dirExists(proc.WorkingDir) {
		return proc.WorkingDir
	}
	for _, repo := range task.AssignedRepos {
		if p := r.repoPaths([]string{repo})[repo]; p != "" && dirExists(p) {
			return p
		}
	}
	// No repository: work in the task's own artifact folder, never the orchestrator's checkout.
	dir := r.artifactDir(task.ID)
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// isTaskWorktree reports whether dir lies inside the task's worktree root.
func (r *Router) isTaskWorktree(taskID, dir string) bool {
	rel, err := filepath.Rel(r.taskWorktreeRoot(taskID), dir)
	return err == nil && !strings.HasPrefix(rel, "..") && dir != ""
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func taskSystemPrompt(t *types.Task, method string) string {
	return fmt.Sprintf("You are the AI engineering agent attached to task %s (%q) in the Meta-Orchestrator.\n"+
		"Current stage: %s. State: %s. Execution method: %s. Assigned repositories: %s.\n"+
		"Task description: %s\n"+
		"Answer the operator directly and concisely. Do not claim to have run tools, tests or commands unless you actually did.",
		t.ID, t.Title, t.CurrentStageID, t.State, method, strings.Join(t.AssignedRepos, ", "), t.Description)
}

// chatHistory rebuilds user/assistant turns from the transcript for stateless providers.
func chatHistory(entries []types.ConsoleEntry) []llm.Message {
	var msgs []llm.Message
	for _, e := range entries {
		switch {
		case e.Kind == types.ConsoleKindUser:
			msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: e.Content})
		case e.Kind == types.ConsoleKindAssistant && e.Status == types.ConsoleDone && e.Content != "":
			if n := len(msgs); n > 0 && msgs[n-1].Role == llm.RoleAssistant {
				msgs[n-1].Content += "\n\n" + e.Content
			} else {
				msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: e.Content})
			}
		}
	}
	if len(msgs) > maxHistoryTurns*2 {
		msgs = msgs[len(msgs)-maxHistoryTurns*2:]
	}
	return msgs
}

type chatRequest struct {
	Message string `json:"message"`
	Model   string `json:"model,omitempty"`
}

// handleTaskConsole serves /tasks/{id}/activity and /tasks/{id}/chat[/cancel].
func (r *Router) handleTaskConsole(w http.ResponseWriter, req *http.Request, task *types.Task, parts []string) {
	taskID := task.ID
	switch parts[1] {
	case "activity":
		switch req.Method {
		case http.MethodGet:
			after := req.URL.Query().Get("after")
			h := r.console
			h.mu.Lock()
			c := h.get(taskID)
			entries := c.entries
			if after != "" {
				for i, e := range entries {
					if e.ID == after {
						entries = entries[i+1:]
						break
					}
				}
			}
			out := append([]types.ConsoleEntry{}, entries...)
			resp := map[string]interface{}{"entries": out, "busy": c.busy, "active_turn_id": c.turnID,
				"session_id": c.sessionID, "model": c.model}
			h.mu.Unlock()
			r.writeJSON(w, http.StatusOK, resp)
		case http.MethodDelete:
			h := r.console
			h.mu.Lock()
			c := h.get(taskID)
			c.entries = nil
			c.sessionID = "" // /clear starts a fresh conversation, like the CLI
			h.mu.Unlock()
			r.broadcast(taskID, types.EventAgentActivityClear, map[string]string{"task_id": taskID})
			r.writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
		default:
			r.writeError(w, http.StatusMethodNotAllowed, "GET or DELETE required")
		}
	case "chat":
		if len(parts) > 2 && parts[2] == "cancel" {
			if req.Method != http.MethodPost {
				r.writeError(w, http.StatusMethodNotAllowed, "POST required")
				return
			}
			h := r.console
			h.mu.Lock()
			cancel := h.get(taskID).cancel
			h.mu.Unlock()
			if cancel == nil {
				r.writeJSON(w, http.StatusOK, map[string]string{"status": "idle"})
				return
			}
			cancel()
			r.writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
			return
		}
		if req.Method != http.MethodPost {
			r.writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		var body chatRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.Message) == "" {
			r.writeError(w, http.StatusBadRequest, "message is required")
			return
		}
		turnID, userID, err := r.startChatTurn(task, strings.TrimSpace(body.Message), strings.TrimSpace(body.Model))
		if err != nil {
			r.writeError(w, http.StatusConflict, err.Error())
			return
		}
		r.writeJSON(w, http.StatusAccepted, map[string]string{"turn_id": turnID, "entry_id": userID})
	}
}

// startChatTurn records the operator message and runs the model turn in the background.
func (r *Router) startChatTurn(task *types.Task, message, model string) (string, string, error) {
	turnID := r.console.nextID("turn")
	ctx, cancel := context.WithTimeout(context.Background(), chatTurnTimeout)
	if err := r.beginTurn(task.ID, turnID, cancel); err != nil {
		cancel()
		return "", "", err
	}

	h := r.console
	h.mu.Lock()
	c := h.get(task.ID)
	history := chatHistory(c.entries)
	sessionID := c.sessionID
	h.mu.Unlock()

	user := r.addEntry(task.ID, types.ConsoleEntry{Kind: types.ConsoleKindUser, TurnID: turnID, Content: message})

	complexity := task.Metadata["complexity"]
	if complexity == "" {
		complexity = "MEDIUM"
	}
	taskType := task.Metadata["task_type"]
	if taskType == "" {
		taskType = router.ClassifyTaskType(task.Title, task.Description)
	}
	decision := r.strategyRouter.RouteForTask(task.CurrentStageID, complexity, taskType, task.AssignedRepos)
	if model != "" {
		decision.Model = model
		decision.FallbackChain = []string{model}
		decision.Reasoning = "operator-selected model"
	}

	msgs := append([]llm.Message{{Role: llm.RoleSystem, Content: taskSystemPrompt(task, decision.Method)}}, history...)
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: message})

	go func() {
		defer cancel()
		resp, used, _ := r.runAgentTurn(ctx, agentTurn{Source: "chat", TurnID: turnID, Task: task, Decision: decision,
			TaskType: taskType, Messages: msgs, SessionID: sessionID, WorkDir: r.resolveWorkDir(task), MaxTokens: defaultChatMaxToken})
		r.endTurn(task.ID, turnID, resp, used)
	}()
	return turnID, user.ID, nil
}
