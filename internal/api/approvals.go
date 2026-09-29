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

// approvalSummary renders the thing being approved in one line.
func approvalSummary(tool string, input map[string]interface{}) string {
	for _, k := range []string{"command", "file_path", "path", "url", "pattern", "query"} {
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
	id      string
	taskID  string
	entryID string
	rule    approvalRule
	ch      chan approvalAnswer
}

type approvalAnswer struct {
	decision string // allowed | always | denied
	message  string
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
// else is shown to the operator in the console and waits for a decision.
func (r *Router) approverFor(taskID, turnID string) llm.Approver {
	return func(ctx context.Context, req llm.ApprovalRequest) llm.ApprovalDecision {
		summary := approvalSummary(req.ToolName, req.Input)
		for _, rule := range r.taskApprovalRules(taskID) {
			if rule.matches(req.ToolName, req.Input) {
				r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindApproval, TurnID: turnID, Status: types.ConsoleDone,
					Content: req.Description, Tool: &types.ConsoleTool{ID: req.ToolUseID, Name: req.ToolName, Input: req.Input},
					Approval: &types.ConsoleApproval{ID: r.console.nextID("apr"), ToolName: req.ToolName, Summary: summary,
						RuleLabel: rule.label(), Decision: types.ApprovalAutomatic, DecidedAt: time.Now()}})
				return llm.ApprovalDecision{Allow: true}
			}
		}

		rule := ruleFor(req.ToolName, req.Input)
		p := &pendingApproval{id: r.console.nextID("apr"), taskID: taskID, rule: rule, ch: make(chan approvalAnswer, 1)}
		expires := time.Now().Add(approvalTimeout)
		e := r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindApproval, TurnID: turnID, Status: types.ConsoleStreaming,
			Content: req.Description, Tool: &types.ConsoleTool{ID: req.ToolUseID, Name: req.ToolName, Input: req.Input},
			Approval: &types.ConsoleApproval{ID: p.id, ToolName: req.ToolName, Summary: summary, RuleLabel: rule.label(),
				BlockedPath: req.BlockedPath, Decision: types.ApprovalPending, ExpiresAt: expires}})
		p.entryID = e.ID
		r.approvals.mu.Lock()
		r.approvals.pending[p.id] = p
		r.approvals.mu.Unlock()
		r.setPendingApprovals(taskID, +1)

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
		if ans.decision == types.ApprovalAlways {
			r.addApprovalRule(taskID, rule)
			r.saveBoardNow()
		}
		status := types.ConsoleDone
		if ans.decision != types.ApprovalAllowed && ans.decision != types.ApprovalAlways {
			status = types.ConsoleFailed
		}
		r.updateEntry(taskID, p.entryID, func(e *types.ConsoleEntry) {
			e.Status = status
			e.Approval.Decision = ans.decision
			e.Approval.Message = ans.message
			e.Approval.DecidedAt = time.Now()
		})
		allow := ans.decision == types.ApprovalAllowed || ans.decision == types.ApprovalAlways
		msg := ans.message
		if !allow && ans.decision == types.ApprovalDenied {
			msg = "The operator denied this action."
			if ans.message != "" {
				msg += " Their note: " + ans.message
			}
		}
		return llm.ApprovalDecision{Allow: allow, Message: msg}
	}
}

// handleTaskApprovals serves GET /api/v1/tasks/{id}/approvals (pending requests) and
// POST /api/v1/tasks/{id}/approvals/{approval_id} {"decision":"allow|always|deny","message":""}.
func (r *Router) handleTaskApprovals(w http.ResponseWriter, req *http.Request, taskID string, parts []string) {
	if len(parts) < 3 || parts[2] == "" {
		if req.Method != http.MethodGet {
			r.writeError(w, http.StatusMethodNotAllowed, "GET required")
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
		r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": taskID, "pending": ids, "rules": r.taskApprovalRules(taskID)})
		return
	}
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Decision string `json:"decision"`
		Message  string `json:"message"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	decision := map[string]string{"allow": types.ApprovalAllowed, "always": types.ApprovalAlways, "deny": types.ApprovalDenied}[body.Decision]
	if decision == "" {
		r.writeError(w, http.StatusBadRequest, `decision must be "allow", "always" or "deny"`)
		return
	}
	r.approvals.mu.Lock()
	p := r.approvals.pending[parts[2]]
	if p != nil && p.taskID == taskID {
		delete(r.approvals.pending, p.id) // first answer wins
	} else {
		p = nil
	}
	r.approvals.mu.Unlock()
	if p == nil {
		r.writeError(w, http.StatusConflict, "This request was already answered, expired, or its turn ended.")
		return
	}
	p.ch <- approvalAnswer{decision: decision, message: strings.TrimSpace(body.Message)}
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"status": decision, "approval_id": p.id, "rule": p.rule.label()})
}
