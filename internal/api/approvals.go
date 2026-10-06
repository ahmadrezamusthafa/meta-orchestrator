package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// approvalTimeout is how long an agent waits for the operator before its request is denied.
var approvalTimeout = 30 * time.Minute

// approvalRule auto-approves later requests from the same task. Bash rules match a command
// prefix; other tools match by name.
type approvalRule struct {
	Tool   string `json:"tool"`
	Prefix string `json:"prefix,omitempty"`
}

func (r approvalRule) label() string {
	if r.Prefix != "" {
		return fmt.Sprintf("%s commands starting with `%s`", r.Tool, r.Prefix)
	}
	return "every " + r.Tool + " call"
}

// shellChaining marks commands that could run something beyond the approved prefix.
var shellChaining = []string{"&&", "||", ";", "|", "`", "$(", ">", "<", "\n"}

func (r approvalRule) matches(tool string, input map[string]interface{}) bool {
	if !strings.EqualFold(r.Tool, tool) {
		return false
	}
	if r.Prefix == "" {
		return true
	}
	cmd, _ := input["command"].(string)
	cmd = strings.TrimSpace(cmd)
	for _, c := range shellChaining {
		if strings.Contains(cmd, c) {
			return false
		}
	}
	return cmd == r.Prefix || strings.HasPrefix(cmd, r.Prefix+" ")
}

// ruleFor proposes the narrowest useful "always allow" rule for a request.
func ruleFor(tool string, input map[string]interface{}) approvalRule {
	if tool != "Bash" {
		return approvalRule{Tool: tool}
	}
	cmd, _ := input["command"].(string)
	var words []string
	for _, w := range strings.Fields(cmd) {
		if strings.HasPrefix(w, "-") || strings.ContainsAny(w, "/=&|;<>$`'\"") || len(words) == 2 {
			break
		}
		words = append(words, w)
	}
	if len(words) == 0 {
		words = strings.Fields(cmd)[:min(1, len(strings.Fields(cmd)))]
	}
	return approvalRule{Tool: tool, Prefix: strings.Join(words, " ")}
}

// approvalKind tells a gated tool call apart from the operator interactions the CLI routes
// through the same permission prompt.
func approvalKind(tool string) string {
	switch tool {
	case llm.ToolExitPlanMode:
		return types.ApprovalKindPlan
	case llm.ToolAskUserQuestion:
		return types.ApprovalKindQuestion
	}
	return types.ApprovalKindTool
}

// approvalQuestions reads AskUserQuestion's input.
func approvalQuestions(input map[string]interface{}) []types.ConsoleQuestion {
	var out struct {
		Questions []struct {
			Question    string                        `json:"question"`
			Header      string                        `json:"header"`
			Options     []types.ConsoleQuestionOption `json:"options"`
			MultiSelect bool                          `json:"multiSelect"`
		} `json:"questions"`
	}
	data, _ := json.Marshal(input)
	_ = json.Unmarshal(data, &out)
	qs := make([]types.ConsoleQuestion, 0, len(out.Questions))
	for _, q := range out.Questions {
		if strings.TrimSpace(q.Question) != "" {
			qs = append(qs, types.ConsoleQuestion{Question: q.Question, Header: q.Header, Options: q.Options, MultiSelect: q.MultiSelect})
		}
	}
	return qs
}

// approvalSummary renders the thing being approved in one line (the whole plan for a plan).
func approvalSummary(tool string, input map[string]interface{}) string {
	if tool == llm.ToolAskUserQuestion {
		var qs []string
		for _, q := range approvalQuestions(input) {
			qs = append(qs, q.Question)
		}
		if len(qs) > 0 {
			return strings.Join(qs, "\n")
		}
	}
	for _, k := range []string{"command", "file_path", "path", "url", "pattern", "query", "plan"} {
		if v, ok := input[k].(string); ok && v != "" {
			return v
		}
	}
	if data, err := json.Marshal(input); err == nil && len(data) > 2 {
		return string(data)
	}
	return tool
}

type pendingApproval struct {
	id        string
	taskID    string
	entryID   string
	kind      string
	rule      approvalRule
	questions []types.ConsoleQuestion
	ch        chan approvalAnswer
}

type approvalAnswer struct {
	decision string // allowed | always | all | denied | expired | cancelled
	message  string
	answers  map[string]string // question kind: question → chosen answer
}

type approvalHub struct {
	mu      sync.Mutex
	pending map[string]*pendingApproval
}

func newApprovalHub() *approvalHub { return &approvalHub{pending: map[string]*pendingApproval{}} }

func (r *Router) taskApprovalRules(taskID string) []approvalRule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.tasks[taskID]
	if t == nil || t.Metadata["approval_rules"] == "" {
		return nil
	}
	var rules []approvalRule
	_ = json.Unmarshal([]byte(t.Metadata["approval_rules"]), &rules)
	return rules
}

func (r *Router) addApprovalRule(taskID string, rule approvalRule) {
	rules := r.taskApprovalRules(taskID)
	for _, existing := range rules {
		if existing == rule {
			return
		}
	}
	rules = append(rules, rule)
	data, _ := json.Marshal(rules)
	r.mu.Lock()
	if t := r.tasks[taskID]; t != nil {
		t.Metadata["approval_rules"] = string(data)
	}
	r.mu.Unlock()
}

// allowAllLabel is the rule shown on requests the "allow all" bypass answered.
const allowAllLabel = "everything in this task (allow all)"

func (r *Router) taskAllowsAll(taskID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.tasks[taskID]
	return t != nil && t.Metadata["approve_all"] == "true"
}

// setAllowAll turns the task's "allow all" bypass on or off. Turning it on also releases every
// request of the task that is still waiting for the operator.
func (r *Router) setAllowAll(taskID string, on bool) bool {
	r.mu.Lock()
	t := r.tasks[taskID]
	if t == nil {
		r.mu.Unlock()
		return false
	}
	if on {
		t.Metadata["approve_all"] = "true"
	} else {
		delete(t.Metadata, "approve_all")
	}
	t.UpdatedAt = time.Now()
	snap := cloneTask(t)
	r.mu.Unlock()
	r.broadcastTask(snap)
	r.saveBoardNow()
	if on {
		r.approvals.mu.Lock()
		var waiting []*pendingApproval
		for id, p := range r.approvals.pending {
			if p.taskID == taskID && p.kind != types.ApprovalKindQuestion { // only the operator can answer a question
				waiting = append(waiting, p)
				delete(r.approvals.pending, id)
			}
		}
		r.approvals.mu.Unlock()
		for _, p := range waiting {
			p.ch <- approvalAnswer{decision: types.ApprovalAll}
		}
	}
	return true
}

// setPendingApprovals updates the count the board shows as "Needs approval".
func (r *Router) setPendingApprovals(taskID string, delta int) {
	r.mu.Lock()
	t := r.tasks[taskID]
	if t == nil {
		r.mu.Unlock()
		return
	}
	n, _ := strconv.Atoi(t.Metadata["pending_approvals"])
	if n += delta; n > 0 {
		t.Metadata["pending_approvals"] = strconv.Itoa(n)
	} else {
		delete(t.Metadata, "pending_approvals")
	}
	t.UpdatedAt = time.Now()
	snap := cloneTask(t)
	r.mu.Unlock()
	r.broadcastTask(snap)
}

// approverFor returns the Approver for one agent turn: saved rules answer immediately, anything
// else is shown to the operator in the console and waits for a decision. planExit is the
// permission mode the agent gets when the operator approves its plan.
func (r *Router) approverFor(taskID, turnID, planExit string) llm.Approver {
	return func(ctx context.Context, req llm.ApprovalRequest) llm.ApprovalDecision {
		kind := approvalKind(req.ToolName)
		info := types.ConsoleApproval{Kind: kind, ToolName: req.ToolName, Summary: approvalSummary(req.ToolName, req.Input),
			BlockedPath: req.BlockedPath}
		switch kind {
		case types.ApprovalKindPlan:
			info.Mode = planExit
		case types.ApprovalKindQuestion:
			info.Questions = approvalQuestions(req.Input)
		}
		entry := func(status string, a types.ConsoleApproval) types.ConsoleEntry {
			return types.ConsoleEntry{Kind: types.ConsoleKindApproval, TurnID: turnID, Status: status, Content: req.Description,
				Tool: &types.ConsoleTool{ID: req.ToolUseID, Name: req.ToolName, Input: req.Input}, Approval: &a}
		}
		auto := func(label string) llm.ApprovalDecision {
			a := info
			a.ID, a.RuleLabel, a.Decision, a.DecidedAt = r.console.nextID("apr"), label, types.ApprovalAutomatic, time.Now()
			r.addEntry(taskID, entry(types.ConsoleDone, a))
			return llm.ApprovalDecision{Allow: true, Mode: planExit}
		}

		// A question needs an answer only the operator can give: rules and "allow all" never answer it.
		if kind != types.ApprovalKindQuestion {
			if r.taskAllowsAll(taskID) {
				return auto(allowAllLabel)
			}
			if kind == types.ApprovalKindTool {
				for _, rule := range r.taskApprovalRules(taskID) {
					if rule.matches(req.ToolName, req.Input) {
						return auto(rule.label())
					}
				}
			}
		}

		p := &pendingApproval{id: r.console.nextID("apr"), taskID: taskID, kind: kind, questions: info.Questions,
			ch: make(chan approvalAnswer, 1)}
		if kind == types.ApprovalKindTool {
			p.rule = ruleFor(req.ToolName, req.Input)
			info.RuleLabel = p.rule.label()
		}
		pending := info
		pending.ID, pending.Decision, pending.ExpiresAt = p.id, types.ApprovalPending, time.Now().Add(approvalTimeout)
		p.entryID = r.addEntry(taskID, entry(types.ConsoleStreaming, pending)).ID
		r.approvals.mu.Lock()
		r.approvals.pending[p.id] = p
		r.approvals.mu.Unlock()
		r.setPendingApprovals(taskID, +1)
		if kind != types.ApprovalKindQuestion && r.taskAllowsAll(taskID) { // "allow all" was switched on while this request was being filed
			r.approvals.mu.Lock()
			if r.approvals.pending[p.id] == p {
				delete(r.approvals.pending, p.id)
				p.ch <- approvalAnswer{decision: types.ApprovalAll}
			}
			r.approvals.mu.Unlock()
		}

		var ans approvalAnswer
		timer := time.NewTimer(approvalTimeout)
		select {
		case ans = <-p.ch:
		case <-ctx.Done():
			ans = approvalAnswer{decision: types.ApprovalCancelled, message: "The turn was stopped before the operator answered."}
		case <-timer.C:
			ans = approvalAnswer{decision: types.ApprovalExpired, message: fmt.Sprintf(
				"No operator response within %s. Continue without this action, or explain what you need.", approvalTimeout)}
		}
		timer.Stop()

		r.approvals.mu.Lock()
		delete(r.approvals.pending, p.id)
		r.approvals.mu.Unlock()
		r.setPendingApprovals(taskID, -1)
		if ans.decision == types.ApprovalAlways && kind == types.ApprovalKindTool {
			r.addApprovalRule(taskID, p.rule)
			r.saveBoardNow()
		}
		allow := ans.decision == types.ApprovalAllowed || ans.decision == types.ApprovalAlways || ans.decision == types.ApprovalAll
		status := types.ConsoleDone
		if !allow {
			status = types.ConsoleFailed
		}
		r.updateEntry(taskID, p.entryID, func(e *types.ConsoleEntry) {
			e.Status = status
			e.Approval.Decision = ans.decision
			e.Approval.Message = ans.message
			e.Approval.Answers = ans.answers
			e.Approval.DecidedAt = time.Now()
		})
		d := llm.ApprovalDecision{Allow: allow, Message: ans.message, Mode: planExit}
		if allow && kind == types.ApprovalKindQuestion {
			d.UpdatedInput = map[string]interface{}{}
			for k, v := range req.Input {
				d.UpdatedInput[k] = v
			}
			d.UpdatedInput["answers"] = ans.answers
		}
		if !allow && ans.decision == types.ApprovalDenied {
			d.Message = map[string]string{
				types.ApprovalKindPlan:     "The operator did not approve the plan yet. Stay in plan mode and revise it.",
				types.ApprovalKindQuestion: "The operator declined to answer. Continue with your best judgement and state your assumptions.",
			}[kind]
			if d.Message == "" {
				d.Message = "The operator denied this action."
			}
			if ans.message != "" {
				d.Message += " Their note: " + ans.message
			}
		}
		return d
	}
}

// handleTaskApprovals serves GET /api/v1/tasks/{id}/approvals (pending requests),
// POST /api/v1/tasks/{id}/approvals {"allow_all":true|false} (the bypass that answers every request) and
// POST /api/v1/tasks/{id}/approvals/{approval_id} {"decision":"allow|always|all|deny","message":"","answers":{}}
// (answers, keyed by question, are required to allow a question request).
func (r *Router) handleTaskApprovals(w http.ResponseWriter, req *http.Request, taskID string, parts []string) {
	if len(parts) < 3 || parts[2] == "" {
		if req.Method == http.MethodPost {
			var body struct {
				AllowAll *bool `json:"allow_all"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.AllowAll == nil {
				r.writeError(w, http.StatusBadRequest, `body must be {"allow_all": true|false}`)
				return
			}
			if !r.setAllowAll(taskID, *body.AllowAll) {
				r.writeError(w, http.StatusNotFound, "Task not found")
				return
			}
			r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": taskID, "allow_all": *body.AllowAll})
			return
		}
		if req.Method != http.MethodGet {
			r.writeError(w, http.StatusMethodNotAllowed, "GET or POST required")
			return
		}
		r.approvals.mu.Lock()
		ids := []string{}
		for id, p := range r.approvals.pending {
			if p.taskID == taskID {
				ids = append(ids, id)
			}
		}
		r.approvals.mu.Unlock()
		r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": taskID, "pending": ids, "rules": r.taskApprovalRules(taskID),
			"allow_all": r.taskAllowsAll(taskID)})
		return
	}
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Decision string            `json:"decision"`
		Message  string            `json:"message"`
		Answers  map[string]string `json:"answers"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	decision := map[string]string{"allow": types.ApprovalAllowed, "always": types.ApprovalAlways, "all": types.ApprovalAll,
		"deny": types.ApprovalDenied}[body.Decision]
	if decision == "" {
		r.writeError(w, http.StatusBadRequest, `decision must be "allow", "always", "all" or "deny"`)
		return
	}
	r.approvals.mu.Lock()
	p := r.approvals.pending[parts[2]]
	if p == nil || p.taskID != taskID {
		r.approvals.mu.Unlock()
		r.writeError(w, http.StatusConflict, "This request was already answered, expired, or its turn ended.")
		return
	}
	var answers map[string]string
	if p.kind == types.ApprovalKindQuestion && decision != types.ApprovalDenied {
		if decision != types.ApprovalAllowed {
			r.approvals.mu.Unlock()
			r.writeError(w, http.StatusBadRequest, `a question is answered with {"decision":"allow","answers":{...}} or skipped with "deny"`)
			return
		}
		answers = map[string]string{}
		for _, q := range p.questions {
			a := strings.TrimSpace(body.Answers[q.Question])
			if a == "" {
				r.approvals.mu.Unlock()
				r.writeError(w, http.StatusBadRequest, fmt.Sprintf("answer every question (missing: %q)", q.Question))
				return
			}
			answers[q.Question] = a
		}
	}
	delete(r.approvals.pending, p.id) // first answer wins
	r.approvals.mu.Unlock()
	p.ch <- approvalAnswer{decision: decision, message: strings.TrimSpace(body.Message), answers: answers}
	if decision == types.ApprovalAll {
		r.setAllowAll(taskID, true)
	}
	out := map[string]interface{}{"status": decision, "approval_id": p.id}
	if p.rule.Tool != "" {
		out["rule"] = p.rule.label()
	}
	r.writeJSON(w, http.StatusOK, out)
}

// Claude Code permission modes the orchestrator uses.
const (
	modePlan        = "plan"        // read-only; the agent asks to leave it through ExitPlanMode
	modeDefault     = "default"     // every gated action asks the operator
	modeAcceptEdits = "acceptEdits" // file edits run without asking; other gated actions ask
)

// editMode is how far an agent may go once the operator lets it act: file edits run without asking
// only inside the task's own worktree; anywhere else (an operator checkout) every change asks first.
func (r *Router) editMode(taskID, workDir string) string {
	if r.isTaskWorktree(taskID, workDir) {
		return modeAcceptEdits
	}
	return modeDefault
}

// permissionBrief tells the agent how permissions work in this console, so it asks through the
// tool call instead of stopping to report that it is blocked.
func permissionBrief(mode string) string {
	switch mode {
	case modePlan:
		return "Permissions: you are in read-only plan mode. If the work needs changes (file edits, commands with side effects, " +
			"git push), call ExitPlanMode with a concise plan; the operator approves it in the console and you can then act. " +
			"Do not end the turn just to report that you lack permission."
	case modeAcceptEdits:
		return "Permissions: file edits in the working directory are pre-approved. Other gated tool calls (commands, network, " +
			"git push) are shown to the operator in the console and wait for their decision: make the call directly instead of " +
			"asking for permission in chat or stopping. If a call is denied, adapt or explain what you need."
	default:
		return "Permissions: gated tool calls (file edits, commands, network, git push) are shown to the operator in the console " +
			"and wait for their decision: make the call directly instead of asking for permission in chat or stopping. " +
			"If a call is denied, adapt or explain what you need."
	}
}

// denialSummary describes the tool calls a turn was not permitted to make, or "" when none were.
func denialSummary(denials []llm.PermissionDenial) string {
	if len(denials) == 0 {
		return ""
	}
	var items []string
	for i, d := range denials {
		if i == 5 {
			items = append(items, fmt.Sprintf("and %d more", len(denials)-i))
			break
		}
		switch d.ToolName {
		case llm.ToolExitPlanMode:
			items = append(items, "leaving plan mode (plan not approved)")
		case llm.ToolAskUserQuestion:
			items = append(items, "a question to you (not answered)")
		default:
			s := approvalSummary(d.ToolName, d.Input)
			if len(s) > 80 {
				s = s[:77] + "…"
			}
			items = append(items, fmt.Sprintf("%s `%s`", d.ToolName, s))
		}
	}
	return fmt.Sprintf("⚠ %d action(s) in this turn were not permitted: %s. The agent continued without them. "+
		"To retry, send a message such as \"retry\" and approve the requests when they appear here, or switch approvals to allow all in the footer.",
		len(denials), strings.Join(items, "; "))
}
