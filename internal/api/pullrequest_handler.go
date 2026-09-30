package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	gitwt "github.com/ahmadrezamusthafa/meta-orchestrator/internal/git"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/pullrequest"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Pull requests are opened from the task_implementation stage onward: the task's worktree branch
// is committed (if it has pending edits), pushed, and filed with the standard description built by
// package pullrequest. Each git checkout of the task gets one pull request.

// prDraft is the pull request a task would open for one git checkout.
type prDraft struct {
	Repo         string   `json:"repo"`
	Repos        []string `json:"repos"` // assigned repositories sharing this checkout
	Provider     string   `json:"provider"`
	RepoURL      string   `json:"repo_url,omitempty"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	CommitMsg    string   `json:"commit_message"`
	Uncommitted  bool     `json:"uncommitted"` // pending edits are committed before the push
	Commits      int      `json:"commits"`
	Files        int      `json:"files"`
	ExistingURL  string   `json:"existing_url,omitempty"`
	CanCreate    bool     `json:"can_create"`
	Blocker      string   `json:"blocker,omitempty"` // why the pull request cannot be opened yet

	wt     taskWorktree
	remote gitwt.Remote
}

// prStages are the stages from which a pull request may be opened.
func prAllowed(t *types.Task) bool {
	return t.State == types.TaskStateCompleted || stageIndexOf(t.CurrentStageID) >= stageIndexOf("task_implementation")
}

// readStageDoc returns a stage's saved output without its provenance header.
func (r *Router) readStageDoc(taskID, stage string) string {
	path, err := r.resolveArtifactPath(taskID, stageDocName(stage))
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return leadingComment.ReplaceAllString(string(raw), "")
}

var leadingComment = regexp.MustCompile(`^\s*(<!--[\s\S]*?-->\s*)+`)

// prInput gathers the description content from the task and its approved stage documents.
func (r *Router) prInput(t *types.Task) pullrequest.Input {
	impl := r.readStageDoc(t.ID, "task_implementation")
	design := r.readStageDoc(t.ID, "techdoc_rfc")
	prd := r.readStageDoc(t.ID, "prd_discovery")
	atdd := r.readStageDoc(t.ID, "atdd_creation")

	first := func(vals ...string) string {
		for _, v := range vals {
			if strings.TrimSpace(v) != "" {
				return v
			}
		}
		return ""
	}
	taskType := t.Metadata["task_type"]
	if taskType == "" {
		taskType = router.ClassifyTaskType(t.Title, t.Description)
	}
	return pullrequest.Input{
		TaskID:   t.ID,
		JiraKey:  t.Metadata["jira_key"],
		JiraURL:  t.Metadata["jira_url"],
		Title:    t.Title,
		TaskType: taskType,
		Summary: first(pullrequest.Section(impl, "summary"), pullrequest.Section(design, "summary", "approach", "overview"),
			t.Description),
		Testing: first(pullrequest.Section(impl, "verif", "how to test", "testing"),
			pullrequest.Section(atdd, "summary", "scenario")),
		AcceptanceCriteria: first(pullrequest.Section(prd, "acceptance"), pullrequest.Section(atdd, "acceptance")),
		Risks:              first(pullrequest.Section(design, "risk"), pullrequest.Section(impl, "risk")),
	}
}

// buildPRDrafts builds one draft per git checkout of the task.
func (r *Router) buildPRDrafts(ctx context.Context, t *types.Task) []*prDraft {
	base := r.prInput(t)
	var drafts []*prDraft
	byCheckout := map[string]*prDraft{}
	for _, wt := range r.describeWorktrees(ctx, t) {
		if d := byCheckout[wt.Checkout]; d != nil && wt.Checkout != "" {
			d.Repos = append(d.Repos, wt.Repo)
			continue
		}
		d := &prDraft{Repo: wt.Repo, Repos: []string{wt.Repo}, SourceBranch: wt.Branch, TargetBranch: gitwt.BranchName(wt.BaseRef),
			ExistingURL: t.Metadata["pr_url."+wt.Repo], wt: wt}
		byCheckout[wt.Checkout] = d
		drafts = append(drafts, d)

		in := base
		in.Repo, in.SourceBranch, in.TargetBranch = wt.Repo, d.SourceBranch, d.TargetBranch
		d.CommitMsg = pullrequest.CommitMessage(in)
		switch {
		case wt.Error != "":
			d.Blocker = wt.Error
		case !wt.Exists:
			d.Blocker = "no worktree yet — run the task first"
		case d.TargetBranch == "" || d.TargetBranch == "HEAD":
			d.Blocker = "the worktree has no base branch to target"
		}
		if d.Blocker == "" {
			if rem, err := gitwt.OriginRemote(ctx, wt.Checkout); err != nil {
				d.Blocker = err.Error()
			} else {
				d.remote, d.Provider, d.RepoURL = rem, rem.Provider(), rem.WebURL()
			}
		}
		if wt.Exists && wt.Error == "" {
			if diff, err := gitwt.Diff(ctx, wt.Checkout, wt.BaseRef, gitwt.DiffAgainstBase, wt.SubPath); err == nil {
				d.Commits, d.Files = len(diff.Commits), len(diff.Files)
				for _, f := range diff.Files {
					in.Files = append(in.Files, pullrequest.FileChange{Path: f.Path, Status: f.Status, Additions: f.Additions, Deletions: f.Deletions})
				}
			}
			d.Uncommitted, _ = gitwt.IsDirty(ctx, wt.Dir)
			in.Commits, _ = gitwt.CommitSubjects(ctx, wt.Checkout, wt.BaseRef, wt.SubPath)
			if d.Uncommitted {
				in.Commits = append(in.Commits, d.CommitMsg+" (pending — committed when the pull request is opened)")
			}
		}
		if d.Blocker == "" && d.Files == 0 {
			d.Blocker = "the branch has no changes against " + d.TargetBranch
		}
		if d.Blocker == "" && d.Provider != "bitbucket" && d.Provider != "github" {
			d.Blocker = fmt.Sprintf("%s is not supported for automatic pull requests — copy the description and open it manually", d.remote.Host)
		}
		if d.Blocker == "" && !prAllowed(t) {
			d.Blocker = "pull requests open from the task_implementation stage onward"
		}
		d.CanCreate = d.Blocker == ""
		d.Title, d.Body = pullrequest.Title(in), pullrequest.Body(in)
	}
	return drafts
}

// prCredentials resolves the connector credentials for a provider without exposing them.
func (r *Router) prCredentials(provider string) (pullrequest.Credentials, error) {
	m := r.cfg.ConnectorsManager
	if m == nil {
		return pullrequest.Credentials{}, fmt.Errorf("connectors are not configured")
	}
	item, err := m.GetConnector(provider)
	if err != nil || item == nil || !item.Enabled {
		return pullrequest.Credentials{}, fmt.Errorf("enable the %s connector in Connectors to open pull requests", provider)
	}
	token, _ := m.ResolveToken(provider, "")
	user := ""
	if provider == "bitbucket" {
		user, _ = m.ResolveUsername(provider, "")
	}
	if token == "" {
		return pullrequest.Credentials{}, fmt.Errorf("the %s connector has no API token", provider)
	}
	return pullrequest.Credentials{Username: user, Token: token}, nil
}

// PullRequestRequest opens a pull request; empty fields use the generated draft.
type PullRequestRequest struct {
	Repo  string `json:"repo"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// handleTaskPullRequest serves GET (drafts) and POST (open) /api/v1/tasks/{id}/pull-request.
func (r *Router) handleTaskPullRequest(w http.ResponseWriter, req *http.Request, t *types.Task) {
	switch req.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(req.Context(), time.Minute)
		defer cancel()
		r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": t.ID, "stage_ready": prAllowed(t), "drafts": r.buildPRDrafts(ctx, t)})
	case http.MethodPost:
		var body PullRequestRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid pull request payload")
			return
		}
		res, err := r.openPullRequest(req.Context(), t.ID, body)
		if err != nil {
			r.writeError(w, http.StatusConflict, err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, res)
	default:
		r.writeError(w, http.StatusMethodNotAllowed, "GET or POST required for pull-request")
	}
}

type prOpened struct {
	Repo      string `json:"repo"`
	URL       string `json:"url"`
	Number    int    `json:"number"`
	Updated   bool   `json:"updated"`
	Committed bool   `json:"committed"`
}

// openPullRequest commits pending work, pushes the branch and files the pull request.
func (r *Router) openPullRequest(ctx context.Context, taskID string, body PullRequestRequest) (*prOpened, error) {
	key := "pr:" + taskID
	if _, busy := r.busyOps.LoadOrStore(key, true); busy {
		return nil, fmt.Errorf("a pull request for %s is already being opened", taskID)
	}
	defer r.busyOps.Delete(key)

	t := r.taskSnapshot(taskID)
	if t == nil {
		return nil, errTaskNotFound
	}
	if t.State == types.TaskStateRunning {
		return nil, fmt.Errorf("pause %s or wait for the stage to finish before opening a pull request", taskID)
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()

	var d *prDraft
	for _, x := range r.buildPRDrafts(ctx, t) {
		for _, repo := range x.Repos {
			if body.Repo == "" || repo == body.Repo {
				d = x
			}
		}
		if d != nil {
			break
		}
	}
	if d == nil {
		return nil, fmt.Errorf("%s has no repository %q with a worktree", taskID, body.Repo)
	}
	if !d.CanCreate {
		return nil, fmt.Errorf("cannot open a pull request for %s: %s", d.Repo, d.Blocker)
	}
	creds, err := r.prCredentials(d.Provider)
	if err != nil {
		return nil, err
	}
	title, desc := strings.TrimSpace(body.Title), strings.TrimSpace(body.Body)
	if title == "" {
		title = d.Title
	}
	if desc == "" {
		desc = d.Body
	}

	committed, err := gitwt.CommitAll(ctx, d.wt.Checkout, d.wt.SubPath, d.CommitMsg)
	if err != nil {
		return nil, fmt.Errorf("commit pending changes: %w", err)
	}
	if err := gitwt.Push(ctx, d.wt.Checkout, d.SourceBranch); err != nil {
		return nil, err
	}
	res, err := r.prClient.Open(ctx, d.Provider, creds, pullrequest.Request{Owner: d.remote.Owner, Repo: d.remote.Repo,
		SourceBranch: d.SourceBranch, TargetBranch: d.TargetBranch, Title: title, Body: desc})
	if err != nil {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindError, Content: fmt.Sprintf(
			"Pushed %s but could not open the pull request: %v", d.SourceBranch, err)})
		return nil, err
	}

	r.mu.Lock()
	if task, ok := r.tasks[taskID]; ok {
		if task.Metadata == nil {
			task.Metadata = map[string]string{}
		}
		for _, repo := range d.Repos {
			task.Metadata["pr_url."+repo] = res.URL
			task.Metadata["pr_number."+repo] = fmt.Sprint(res.Number)
		}
		task.UpdatedAt = time.Now()
	}
	r.mu.Unlock()
	r.saveBoardNow()
	if path, err := r.resolveArtifactPath(taskID, "PULL_REQUEST_"+safeFileName(d.Repo)+".md"); err == nil {
		_ = os.WriteFile(path, []byte(fmt.Sprintf("<!-- %s · pull request · %s -->\n\n# %s\n\n%s\n", taskID, res.URL, title, desc)), 0o644)
	}

	verb := "Opened"
	if res.Updated {
		verb = "Updated"
	}
	note := fmt.Sprintf("%s pull request #%d for %s (%s → %s): %s", verb, res.Number, d.Repo, d.SourceBranch, d.TargetBranch, res.URL)
	if committed {
		note += "\nPending edits were committed as \"" + d.CommitMsg + "\"."
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: note})
	r.broadcastTask(r.taskSnapshot(taskID))
	return &prOpened{Repo: d.Repo, URL: res.URL, Number: res.Number, Updated: res.Updated, Committed: committed}, nil
}

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func safeFileName(s string) string {
	return strings.Trim(unsafeFileChars.ReplaceAllString(s, "-"), "-.")
}
