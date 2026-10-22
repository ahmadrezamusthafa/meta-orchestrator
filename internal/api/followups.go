package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/readiness"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Follow-up tasks: each launch-readiness stage ends its report with the work it found still missing
// — gaps it did not close, or implementation the product code needs (tracing after the observability
// stage, performance fixes after the load test) — grouped into a few proposed tasks. They are read
// from the stage documents on demand and never created automatically: the operator picks which to
// add to the board as General AI SDLC tasks, and may combine several into one.

const (
	followUpLinksFile   = "followups.json" // proposal id → the task created from it
	maxFollowUps        = 12               // proposals read per stage
	maxFollowUpItems    = 25               // items read per proposal
	maxFollowUpTitle    = 200
	maxFollowUpText     = 2000
	maxFollowUpRepoName = 200
)

// followUpBlock is the last fenced `followups` block of a stage report (a `json followups` info
// string is accepted too).
var followUpBlock = regexp.MustCompile("(?ms)^[ \\t]*```+[ \\t]*(?:json[ \\t]+)?" + readiness.FollowUpFence + "[ \\t]*\\r?\\n(.*?)^[ \\t]*```")

type followUpItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
	Priority string `json:"priority"`
	Evidence string `json:"evidence,omitempty"`
}

type followUpProposal struct {
	ID        string         `json:"id"`
	Stage     string         `json:"stage"`
	StageName string         `json:"stage_name"`
	Title     string         `json:"title"`
	Summary   string         `json:"summary,omitempty"`
	Repos     []string       `json:"repos,omitempty"`
	Items     []followUpItem `json:"items"`
	// CreatedTaskID is the task created from this proposal, while it is still on the board.
	CreatedTaskID    string          `json:"created_task_id,omitempty"`
	CreatedTaskState types.TaskState `json:"created_task_state,omitempty"`
}

// followUpStage is a readiness stage of the task and whether its report lists follow-ups.
type followUpStage struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HasReport bool   `json:"has_report"`
	// Listed is false when the report has no follow-up block (written before the stage asked for
	// one, or the agent left it out); re-running the stage adds it.
	Listed bool `json:"listed"`
}

// parseFollowUps reads the proposed follow-up tasks of a stage report. A report without a block,
// or with a block that is not a JSON array of proposals, has none (ok false).
func parseFollowUps(stage, report string) (out []followUpProposal, ok bool) {
	m := followUpBlock.FindAllStringSubmatch(report, -1)
	if len(m) == 0 {
		return nil, false
	}
	var raw []struct {
		Title   string          `json:"title"`
		Summary string          `json:"summary"`
		Repos   json.RawMessage `json:"repos"`
		Items   []struct {
			Title    string `json:"title"`
			Detail   string `json:"detail"`
			Priority string `json:"priority"`
			Evidence string `json:"evidence"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(m[len(m)-1][1])), &raw); err != nil {
		return nil, false
	}
	name := stage
	if step, found := readiness.StepByID(stage); found {
		name = step.Name
	}
	seen := map[string]bool{}
	for _, p := range raw {
		title := clip(p.Title, maxFollowUpTitle)
		if title == "" {
			continue
		}
		prop := followUpProposal{ID: uniqueID(stage+":"+shortHash(title), seen), Stage: stage, StageName: name, Title: title,
			Summary: clip(p.Summary, maxFollowUpText), Repos: repoNames(p.Repos), Items: []followUpItem{}}
		itemSeen := map[string]bool{}
		for _, it := range p.Items {
			t := clip(it.Title, maxFollowUpTitle)
			if t == "" {
				continue
			}
			prop.Items = append(prop.Items, followUpItem{ID: uniqueID(shortHash(t), itemSeen), Title: t,
				Detail: clip(it.Detail, maxFollowUpText), Priority: priority(it.Priority), Evidence: clip(it.Evidence, maxFollowUpTitle)})
			if len(prop.Items) == maxFollowUpItems {
				break
			}
		}
		out = append(out, prop)
		if len(out) == maxFollowUps {
			break
		}
	}
	return out, true
}

// repoNames accepts the repos field as a list of names or a single name; anything else is ignored.
func repoNames(raw json.RawMessage) []string {
	var list []string
	if json.Unmarshal(raw, &list) != nil {
		var one string
		if json.Unmarshal(raw, &one) != nil {
			return nil
		}
		list = []string{one}
	}
	var out []string
	for _, r := range list {
		if r = clip(r, maxFollowUpRepoName); r != "" && indexIn(out, r) < 0 {
			out = append(out, r)
		}
	}
	return out
}

func priority(p string) string {
	switch p = strings.ToLower(strings.TrimSpace(p)); p {
	case "high", "medium", "low":
		return p
	case "critical", "p0", "p1":
		return "high"
	case "p3", "p4":
		return "low"
	}
	return "medium"
}

// clip trims s, collapses it to at most n runes, and drops control characters other than newlines.
func clip(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if rs := []rune(s); len(rs) > n {
		s = strings.TrimSpace(string(rs[:n])) + "…"
	}
	return s
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.Join(strings.Fields(s), " "))))
	return hex.EncodeToString(sum[:5])
}

// uniqueID returns id, suffixed when an earlier entry of the same list already took it.
func uniqueID(id string, seen map[string]bool) string {
	out := id
	for n := 2; seen[out]; n++ {
		out = fmt.Sprintf("%s-%d", id, n)
	}
	seen[out] = true
	return out
}

// readinessStagesOf lists the launch-readiness stages the task runs, in order.
func (r *Router) readinessStagesOf(t *types.Task) []string {
	var out []string
	for _, st := range r.taskStages(t) {
		if _, ok := readiness.StepByID(st); ok {
			out = append(out, st)
		}
	}
	return out
}

// taskFollowUps reads the follow-up proposals of every readiness stage of the task, each marked with
// the task created from it while that task is still on the board.
func (r *Router) taskFollowUps(t *types.Task) ([]followUpProposal, []followUpStage) {
	links := r.followUpLinks(t.ID)
	proposals := []followUpProposal{}
	stages := []followUpStage{}
	for _, st := range r.readinessStagesOf(t) {
		step, _ := readiness.StepByID(st)
		info := followUpStage{ID: st, Name: step.Name}
		if path, err := r.resolveArtifactPath(t.ID, stageDocName(st)); err == nil {
			if raw, err := os.ReadFile(path); err == nil {
				info.HasReport = true
				var list []followUpProposal
				list, info.Listed = parseFollowUps(st, string(raw))
				proposals = append(proposals, list...)
			}
		}
		stages = append(stages, info)
	}
	r.mu.RLock()
	for i := range proposals {
		if id := links[proposals[i].ID]; id != "" {
			if created, ok := r.tasks[id]; ok {
				proposals[i].CreatedTaskID, proposals[i].CreatedTaskState = id, created.State
			}
		}
	}
	r.mu.RUnlock()
	return proposals, stages
}

// followUpLinks reads which task was created from which proposal of the task.
func (r *Router) followUpLinks(taskID string) map[string]string {
	links := map[string]string{}
	if path, err := r.resolveArtifactPath(taskID, followUpLinksFile); err == nil {
		if raw, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(raw, &links)
		}
	}
	return links
}

func (r *Router) saveFollowUpLinks(taskID string, links map[string]string) error {
	path, err := r.resolveArtifactPath(taskID, followUpLinksFile)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// noteFollowUps tells the operator, after a readiness stage, how many follow-up tasks it proposed.
func (r *Router) noteFollowUps(taskID, stage, report string) {
	if _, ok := readiness.StepByID(stage); !ok {
		return
	}
	list, ok := parseFollowUps(stage, report)
	if !ok || len(list) == 0 {
		return
	}
	items := 0
	for _, p := range list {
		items += len(p.Items)
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
		"%s proposed %d follow-up task%s (%d item%s) for what is still missing. Review them in the Follow-ups tab and add the ones you want to the board.",
		stage, len(list), plural(len(list)), items, plural(items))})
}

type followUpSelection struct {
	ProposalID string `json:"proposal_id"`
	// ItemIDs are the items to include; empty includes all of the proposal's items.
	ItemIDs []string `json:"item_ids,omitempty"`
}

type createFollowUpRequest struct {
	Selections []followUpSelection `json:"selections"`
	// Title names the new task; empty uses the proposal's title (or a combined one).
	Title string `json:"title,omitempty"`
}

var errFollowUpCreated = errors.New("already added to the board")

// handleTaskFollowUps serves /api/v1/tasks/{id}/followups: GET lists the proposals of the task's
// readiness stages; POST creates one General AI SDLC task from the selected proposals and items.
func (r *Router) handleTaskFollowUps(w http.ResponseWriter, req *http.Request, t *types.Task) {
	switch req.Method {
	case http.MethodGet:
		proposals, stages := r.taskFollowUps(t)
		r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": t.ID, "proposals": proposals, "stages": stages})
	case http.MethodPost:
		var body createFollowUpRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid follow-up request payload")
			return
		}
		created, status, err := r.createFollowUpTask(req, t, body)
		if err != nil {
			r.writeError(w, status, err.Error())
			return
		}
		r.writeJSON(w, status, created)
	default:
		r.writeError(w, http.StatusMethodNotAllowed, "GET or POST required for followups")
	}
}

// createFollowUpTask adds one task to the board covering the selected proposals. Proposals are
// checked and linked under followUpMu, so a double click cannot add the same proposal twice.
func (r *Router) createFollowUpTask(req *http.Request, src *types.Task, body createFollowUpRequest) (*types.Task, int, error) {
	if len(body.Selections) == 0 {
		return nil, http.StatusBadRequest, errors.New("select at least one follow-up")
	}
	r.followUpMu.Lock()
	defer r.followUpMu.Unlock()

	proposals, _ := r.taskFollowUps(src)
	byID := map[string]followUpProposal{}
	for _, p := range proposals {
		byID[p.ID] = p
	}
	var chosen []followUpProposal
	picked := map[string]bool{}
	for _, sel := range body.Selections {
		p, ok := byID[sel.ProposalID]
		if !ok {
			return nil, http.StatusNotFound, fmt.Errorf("follow-up %q is no longer in the stage reports — reload the list", sel.ProposalID)
		}
		if picked[p.ID] {
			continue
		}
		picked[p.ID] = true
		if p.CreatedTaskID != "" {
			return nil, http.StatusConflict, fmt.Errorf("%q was %s as %s", p.Title, errFollowUpCreated, p.CreatedTaskID)
		}
		if len(sel.ItemIDs) > 0 && len(p.Items) > 0 {
			want := map[string]bool{}
			for _, id := range sel.ItemIDs {
				want[id] = true
			}
			var items []followUpItem
			for _, it := range p.Items {
				if want[it.ID] {
					items = append(items, it)
				}
			}
			if len(items) == 0 {
				return nil, http.StatusBadRequest, fmt.Errorf("none of the selected items are in %q any more — reload the list", p.Title)
			}
			p.Items = items
		}
		chosen = append(chosen, p)
	}

	title := clip(body.Title, maxFollowUpTitle)
	if title == "" {
		title = chosen[0].Title
		if len(chosen) > 1 {
			title = fmt.Sprintf("%s (+%d related follow-up%s)", title, len(chosen)-1, plural(len(chosen)-1))
		}
	}
	var stageIDs []string
	for _, p := range chosen {
		if indexIn(stageIDs, p.Stage) < 0 {
			stageIDs = append(stageIDs, p.Stage)
		}
	}
	created, status, err := r.createTask(req.Context(), CreateTaskRequest{
		Title:         title,
		Description:   followUpDescription(src, chosen),
		WorkflowID:    fsm.DefaultWorkflowID,
		AssignedRepos: followUpRepos(src.AssignedRepos, chosen),
	}, map[string]string{"followup_of": src.ID, "followup_stages": strings.Join(stageIDs, ",")})
	if err != nil {
		return nil, status, err
	}

	links := r.followUpLinks(src.ID)
	for _, p := range chosen {
		links[p.ID] = created.ID
	}
	if err := r.saveFollowUpLinks(src.ID, links); err != nil {
		fmt.Printf("[FollowUps] %s: failed to record follow-ups created as %s: %v\n", src.ID, created.ID, err)
	}
	titles := make([]string, len(chosen))
	for i, p := range chosen {
		titles[i] = p.Title
	}
	r.addEntry(src.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
		"Added follow-up task %s to the board: %s.", created.ID, strings.Join(titles, "; "))})
	r.addEntry(created.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
		"Follow-up of %s (%s): covers what its launch-readiness stage%s found still missing. The full list is in the description.",
		src.ID, src.Title, plural(len(stageIDs)))})
	return created, status, nil
}

// followUpRepos keeps the source task's repositories the proposals name; when they name none of
// them (or none at all), the follow-up works on all of the source task's repositories.
func followUpRepos(source []string, chosen []followUpProposal) []string {
	var out []string
	for _, p := range chosen {
		for _, repo := range p.Repos {
			if indexIn(source, repo) >= 0 && indexIn(out, repo) < 0 {
				out = append(out, repo)
			}
		}
	}
	if len(out) == 0 {
		return append([]string(nil), source...)
	}
	return out
}

// followUpDescription is the new task's description: where it comes from and every selected item.
func followUpDescription(src *types.Task, chosen []followUpProposal) string {
	var b strings.Builder
	artifactDir := src.ArtifactDir
	if artifactDir == "" {
		artifactDir = ".sdlc/artifacts/" + src.ID
	}
	fmt.Fprintf(&b, "Follow-up of %s (%s): work its launch-readiness review found still missing.\n", src.ID, src.Title)
	for _, p := range chosen {
		fmt.Fprintf(&b, "\n## %s\nFrom stage: %s (report: %s/%s)\n", p.Title, p.StageName, artifactDir, stageDocName(p.Stage))
		if p.Summary != "" {
			b.WriteString("\n" + p.Summary + "\n")
		}
		if len(p.Items) > 0 {
			b.WriteString("\n")
		}
		for _, it := range p.Items {
			fmt.Fprintf(&b, "- [%s] %s", it.Priority, it.Title)
			if it.Detail != "" {
				b.WriteString(" — " + strings.Join(strings.Fields(it.Detail), " "))
			}
			if it.Evidence != "" {
				b.WriteString(" (evidence: " + it.Evidence + ")")
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
