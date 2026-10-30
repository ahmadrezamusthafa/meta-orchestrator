package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/evidence"
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
	// Unpushed counts local commits origin does not have yet; OnRemote is false before the first push.
	Unpushed int  `json:"unpushed"`
	OnRemote bool `json:"on_remote"`
	// HeadSHA and RemoteSHA are the local and origin tips; the UI compares them with a push
	// diagnosis to notice git run outside the orchestrator.
	HeadSHA   string `json:"head_sha,omitempty"`
	RemoteSHA string `json:"remote_sha,omitempty"`
	// SameContent is true when HEAD holds exactly the files of origin's branch, even if the
	// commits differ (a rebase or squash done in another clone and force-pushed from there).
	SameContent bool `json:"same_content"`
	// CheckedOut names the branch the worktree is on when it is not SourceBranch (switched or
	// renamed outside the orchestrator); HeldBy is the other work tree that has SourceBranch.
	CheckedOut string `json:"checked_out_branch,omitempty"`
	HeldBy     string `json:"held_by,omitempty"`
	// Sync tells whether an opened pull request shows the worktree: prNotOpened, prInSync or
	// prOutOfSync; SyncNote says what is missing from it.
	// SummarySource says where the description's Summary comes from: "generated" from the diff and
	// the task, or "report" (the implementation report, when no summary could be generated).
	SummarySource string `json:"summary_source"`
	Sync          string `json:"sync"`
	SyncNote      string `json:"sync_note,omitempty"`
	CanCreate     bool   `json:"can_create"`
	Blocker       string `json:"blocker,omitempty"` // why the pull request cannot be opened yet
	// CanCommit is true when the pending edits can be committed on their own, without pushing;
	// CommitBlocker says why not. Remote, provider and stage do not matter for a local commit.
	CanCommit     bool   `json:"can_commit"`
	CommitBlocker string `json:"commit_blocker,omitempty"`

	wt     taskWorktree
	remote gitwt.Remote
	in     pullrequest.Input // the description's content, rendered once every draft is built
	patch  string            // the diff against the target branch, for the generated summary
	cut    bool              // patch was truncated
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

// prInput gathers the description content from the task and its approved stage documents; impl is
// the implementation report, from which each repository's changes are taken.
func (r *Router) prInput(t *types.Task) (in pullrequest.Input, impl string) {
	impl = pullrequest.WithoutEvidence(r.readStageDoc(t.ID, "task_implementation")) // the evidence's results are added from evidence.json
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
		Context:  first(pullrequest.Section(prd, "problem", "background", "context", "goal"), t.Description),
		Summary: first(pullrequest.Section(impl, "summary"), pullrequest.Section(design, "summary", "approach", "overview"),
			t.Description),
		Testing: first(pullrequest.Section(impl, "verif", "how to test", "testing"),
			pullrequest.Section(atdd, "summary", "scenario")),
		AcceptanceCriteria: first(pullrequest.Section(prd, "acceptance"), pullrequest.Section(atdd, "acceptance")),
		Risks:              first(pullrequest.Section(design, "risk"), pullrequest.Section(impl, "risk")),
		Evidence:           r.prEvidence(t),
	}, impl
}

// prEvidence is the outcome of the task's recorded test evidence, or nil when none was recorded.
func (r *Router) prEvidence(t *types.Task) *pullrequest.Evidence {
	rep := evidence.Collect(r.evidenceDir(t.ID))
	if rep.Manifest == nil || len(rep.Manifest.Cases)+len(rep.Manifest.Suites) == 0 {
		return nil
	}
	ev := &pullrequest.Evidence{}
	for _, c := range rep.Manifest.Cases {
		if c.Kind == evidence.KindSuite {
			continue // listed with its suite
		}
		ev.Cases = append(ev.Cases, pullrequest.EvidenceCase{ID: c.ID, Kind: c.Kind, Title: c.Title, Status: c.Status})
	}
	for _, s := range rep.Manifest.Suites {
		ev.Suites = append(ev.Suites, pullrequest.EvidenceSuite{Repository: s.Repository, Command: publicCommand(s.Command),
			Status: s.Status, Passed: s.Passed, Failed: s.Failed})
	}
	spec := r.evidenceSpec(t, rep)
	ev.Covered, ev.Total = spec.Covered, len(spec.Cases)
	return ev
}

// envPrefix matches the variable assignments in front of a command ("DATABASE_URL=… bundle exec rspec").
var envPrefix = regexp.MustCompile(`^(\s*(export\s+)?[A-Za-z_][A-Za-z0-9_]*=('[^']*'|"[^"]*"|\S*)\s+)+`)

// publicCommand is a suite command fit for the pull request: without the environment it ran with
// (the throwaway test environment's credentials) or credentials embedded in URLs.
func publicCommand(cmd string) string {
	return strings.TrimSpace(redactURLCredentials(envPrefix.ReplaceAllString(cmd, "")))
}

// evidenceFor narrows the evidence to the suites of repos; every suite is kept when none matches
// by name.
func evidenceFor(ev *pullrequest.Evidence, repos []string) *pullrequest.Evidence {
	if ev == nil {
		return nil
	}
	var own []pullrequest.EvidenceSuite
	for _, s := range ev.Suites {
		for _, repo := range repos {
			if strings.EqualFold(s.Repository, repo) || strings.HasSuffix(strings.ToLower(s.Repository), "/"+strings.ToLower(repo)) {
				own = append(own, s)
				break
			}
		}
	}
	if len(own) == 0 {
		return ev
	}
	out := *ev
	out.Suites = own
	return &out
}

// buildPRDrafts builds one draft per git checkout of the task. With summarize, a Summary that is
// not cached yet is generated from the diff (the operator is looking at the description);
// otherwise only a cached one is used.
func (r *Router) buildPRDrafts(ctx context.Context, t *types.Task, summarize bool) []*prDraft {
	base, impl := r.prInput(t)
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
		if d.Blocker == "" && wt.Exists && wt.Error == "" {
			if deps, err := gitwt.AddedDependencyFiles(ctx, wt.Checkout, wt.BaseRef); err == nil && len(deps) > 0 {
				d.Blocker = dependencyBlocker(deps)
				d.CommitBlocker = d.Blocker
			}
		}
		if d.Blocker == "" && wt.Exists && wt.Error == "" {
			// Committing "pending edits" with `git add -A` would mark a half-merged file resolved,
			// markers and all, and push it.
			if c, err := gitwt.Conflicts(ctx, wt.Checkout, wt.BaseRef); err == nil && c.Unresolved() {
				d.Blocker = conflictBlocker(c)
				d.CommitBlocker = d.Blocker
			}
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
				d.patch, d.cut = diff.Patch, diff.Truncated
				for _, f := range diff.Files {
					in.Files = append(in.Files, pullrequest.FileChange{Path: f.Path, Status: f.Status, Additions: f.Additions, Deletions: f.Deletions})
				}
			}
			d.Uncommitted, _ = gitwt.IsDirty(ctx, wt.Dir)
			d.Unpushed, d.OnRemote, _ = gitwt.Unpushed(ctx, wt.Checkout, d.SourceBranch)
			d.HeadSHA, d.RemoteSHA = gitwt.BranchTips(ctx, wt.Checkout, d.SourceBranch)
			d.SameContent = d.OnRemote && gitwt.SameTree(ctx, wt.Checkout, "HEAD", d.RemoteSHA)
			if cur := gitwt.CurrentBranch(ctx, wt.Checkout); cur != "" && cur != d.SourceBranch {
				d.CheckedOut, d.HeldBy = cur, gitwt.CheckedOutAt(ctx, wt.Checkout, d.SourceBranch)
				if d.Blocker == "" {
					d.Blocker = branchBlocker(d)
				}
				if d.CommitBlocker == "" {
					d.CommitBlocker = branchBlocker(d)
				}
			}
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
		switch {
		case wt.Error != "":
			d.CommitBlocker = wt.Error
		case !wt.Exists:
			d.CommitBlocker = "no worktree yet — run the task first"
		case d.CommitBlocker == "" && !d.Uncommitted:
			d.CommitBlocker = "the worktree has no uncommitted edits"
		}
		d.Blocker = redactURLCredentials(d.Blocker) // git errors may quote a remote URL with its token
		d.CommitBlocker = redactURLCredentials(d.CommitBlocker)
		d.CanCreate = d.Blocker == ""
		d.CanCommit = d.CommitBlocker == ""
		d.Sync, d.SyncNote = prSync(d)
		d.in = in
	}
	// The description is rendered once every checkout is known: each one names the others.
	for _, d := range drafts {
		d.in.Details = pullrequest.Changes(impl, d.Repo)
		d.in.Evidence = evidenceFor(base.Evidence, d.Repos)
		for _, o := range drafts {
			if o != d {
				d.in.Related = append(d.in.Related, pullrequest.Related{Repo: o.Repo, URL: o.ExistingURL})
			}
		}
	}
	r.summarizeDrafts(ctx, t, drafts, summarize)
	for _, d := range drafts {
		d.Title, d.Body = pullrequest.Title(d.in), pullrequest.Body(d.in)
	}
	return drafts
}

// Pull request sync states (prDraft.Sync).
const (
	prNotOpened = "not_opened"
	prInSync    = "in_sync"
	prOutOfSync = "out_of_sync"
)

// prSync compares an opened pull request with the worktree: changes made after it was opened (by
// the agent or by hand) only reach it when the branch is pushed again.
func prSync(d *prDraft) (state, note string) {
	if d.ExistingURL == "" {
		return prNotOpened, ""
	}
	if d.SameContent && !d.Uncommitted {
		note := "The pull request has every change in the worktree."
		if d.Unpushed > 0 {
			note = fmt.Sprintf("The pull request has the same files as the worktree; only the history differs (%d local "+
				"commit(s) were replaced on origin, e.g. by a rebase pushed from another clone).", d.Unpushed)
		}
		return prInSync, note
	}
	if d.CheckedOut != "" {
		return prOutOfSync, fmt.Sprintf("The worktree is on branch %s, but the pull request follows %s — its changes do not "+
			"reach the pull request. %s", d.CheckedOut, d.SourceBranch, switchHint(d))
	}
	var missing []string
	switch {
	case !d.OnRemote:
		missing = append(missing, "the branch "+d.SourceBranch+" is not on the remote")
	case d.Unpushed > 0:
		missing = append(missing, fmt.Sprintf("%d commit(s) not pushed", d.Unpushed))
	}
	if d.Uncommitted {
		missing = append(missing, "uncommitted edits in the worktree")
	}
	if len(missing) == 0 {
		return prInSync, "The pull request has every change in the worktree."
	}
	return prOutOfSync, "The pull request is missing local changes: " + strings.Join(missing, " and ") +
		". Update the pull request to commit, push and refresh its description."
}

// notePRSync tells the operator, once per change, when work done after a pull request was opened
// has not reached it yet, and keeps the task's pr_out_of_sync flag (the board badge) current.
func (r *Router) notePRSync(taskID, turnID string) {
	t := r.taskSnapshot(taskID)
	if t == nil {
		return
	}
	opened := false
	for k := range t.Metadata {
		if strings.HasPrefix(k, "pr_url.") {
			opened = true
			break
		}
	}
	if !opened {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var stale []string
	var notes []string
	for _, d := range r.buildPRDrafts(ctx, t, false) {
		if d.Sync != prOutOfSync {
			continue
		}
		stale = append(stale, fmt.Sprintf("%s:%d:%t:%t", d.Repo, d.Unpushed, d.OnRemote, d.Uncommitted))
		notes = append(notes, fmt.Sprintf("%s (%s)", d.Repo, d.ExistingURL))
	}
	sig := strings.Join(stale, ",")

	r.mu.Lock()
	task, ok := r.tasks[taskID]
	if !ok || task.Metadata["pr_out_of_sync"] == sig {
		r.mu.Unlock()
		return
	}
	if sig == "" {
		delete(task.Metadata, "pr_out_of_sync")
	} else {
		task.Metadata["pr_out_of_sync"] = sig
	}
	task.UpdatedAt = time.Now()
	snap := cloneTask(task)
	r.mu.Unlock()
	r.broadcastTask(snap)
	if sig != "" {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: fmt.Sprintf(
			"⇡ The pull request for %s does not have the latest changes yet. Open the Pull Request tab and choose "+
				"Update pull request to commit, push and refresh it.", strings.Join(notes, ", "))})
	}
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
	// CommitMessage replaces the generated message for pending edits committed before the push.
	CommitMessage string `json:"commit_message,omitempty"`
	// ForceWithLease is origin's branch tip the operator reviewed in the push-rejected dialog; when
	// set the branch is force-pushed, but only while origin still points there.
	ForceWithLease string `json:"force_with_lease,omitempty"`
}

// pushRejected is returned when origin refuses the push because the branch history diverged;
// the operator decides from Divergence whether to force-push with a lease or cancel.
type pushRejected struct{ Divergence gitwt.Divergence }

func (e *pushRejected) Error() string {
	return fmt.Sprintf("push rejected: %s and origin/%s have diverged. %s", e.Divergence.Branch, e.Divergence.Branch, e.Divergence.Explanation)
}

// handleTaskPullRequest serves GET (drafts) and POST (open) /api/v1/tasks/{id}/pull-request.
func (r *Router) handleTaskPullRequest(w http.ResponseWriter, req *http.Request, t *types.Task, parts []string) {
	if len(parts) > 2 && parts[2] == "commit" {
		r.handleTaskCommit(w, req, t)
		return
	}
	switch req.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(req.Context(), time.Minute)
		defer cancel()
		r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": t.ID, "stage_ready": prAllowed(t), "drafts": r.buildPRDrafts(ctx, t, true)})
		go r.notePRSync(t.ID, "") // the worktree may have changed outside an agent turn; keep the tab badge current
		r.fetchTaskBranches(t.ID) // pushes from another clone show up through the git watcher
	case http.MethodPost:
		var body PullRequestRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid pull request payload")
			return
		}
		res, err := r.openPullRequest(req.Context(), t.ID, body)
		var rejected *pushRejected
		if errors.As(err, &rejected) {
			r.writeJSON(w, http.StatusConflict, map[string]interface{}{"error": err.Error(), "push_rejected": rejected.Divergence})
			return
		}
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

// CommitRequest commits a checkout's pending edits without pushing; an empty message uses the
// draft's generated commit message.
type CommitRequest struct {
	Repo    string `json:"repo"`
	Message string `json:"message"`
}

type commitDone struct {
	Repo    string `json:"repo"`
	Message string `json:"message"`
	SHA     string `json:"sha"`
}

// handleTaskCommit serves POST /api/v1/tasks/{id}/pull-request/commit.
func (r *Router) handleTaskCommit(w http.ResponseWriter, req *http.Request, t *types.Task) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required for commit")
		return
	}
	var body CommitRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid commit payload")
		return
	}
	res, err := r.commitPending(req.Context(), t.ID, body)
	if err != nil {
		r.writeError(w, http.StatusConflict, err.Error())
		return
	}
	r.writeJSON(w, http.StatusOK, res)
}

// commitPending commits the pending edits of one checkout on the task branch. Nothing is pushed:
// an opened pull request turns out of sync until it is updated.
func (r *Router) commitPending(ctx context.Context, taskID string, body CommitRequest) (*commitDone, error) {
	key := "pr:" + taskID // shares the lock with openPullRequest, which commits too
	if _, busy := r.busyOps.LoadOrStore(key, true); busy {
		return nil, fmt.Errorf("a commit or pull request for %s is already in progress", taskID)
	}
	defer r.busyOps.Delete(key)

	t := r.taskSnapshot(taskID)
	if t == nil {
		return nil, errTaskNotFound
	}
	if t.State == types.TaskStateRunning {
		return nil, fmt.Errorf("pause %s or wait for the stage to finish before committing", taskID)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	d := pickDraft(r.buildPRDrafts(ctx, t, false), body.Repo)
	if d == nil {
		return nil, fmt.Errorf("%s has no repository %q with a worktree", taskID, body.Repo)
	}
	if !d.CanCommit {
		return nil, fmt.Errorf("cannot commit %s: %s", d.Repo, d.CommitBlocker)
	}
	msg := strings.TrimSpace(body.Message)
	if msg == "" {
		msg = d.CommitMsg
	}

	if err := r.preCommitHooks(ctx, t, d.wt.Dir); err != nil {
		return nil, err
	}
	committed, err := gitwt.CommitAll(ctx, d.wt.Checkout, d.wt.SubPath, msg)
	if err != nil {
		return nil, fmt.Errorf("commit pending changes: %s", redactURLCredentials(err.Error()))
	}
	if !committed {
		return nil, fmt.Errorf("%s has nothing to commit", d.Repo)
	}
	r.postCommitHooks(t, d.wt.Dir)
	sha, _ := gitwt.BranchTips(ctx, d.wt.Checkout, d.SourceBranch)

	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
		"Committed pending edits in %s on %s as %q (%s). Nothing was pushed.", d.Repo, d.SourceBranch, msg, shortSHA(sha))})
	r.notePRSync(taskID, "") // an opened pull request now lacks this commit
	r.broadcastTask(r.taskSnapshot(taskID))
	return &commitDone{Repo: d.Repo, Message: msg, SHA: sha}, nil
}

// pickDraft returns the draft whose checkout serves repo, or the first one when repo is empty.
func pickDraft(drafts []*prDraft, repo string) *prDraft {
	for _, d := range drafts {
		for _, x := range d.Repos {
			if repo == "" || x == repo {
				return d
			}
		}
	}
	return nil
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

	d := pickDraft(r.buildPRDrafts(ctx, t, true), body.Repo)
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
	commitMsg := strings.TrimSpace(body.CommitMessage)
	if commitMsg == "" {
		commitMsg = d.CommitMsg
	}

	if err := r.preCommitHooks(ctx, t, d.wt.Dir); err != nil {
		return nil, err
	}
	committed, err := gitwt.CommitAll(ctx, d.wt.Checkout, d.wt.SubPath, commitMsg)
	if err != nil {
		return nil, fmt.Errorf("commit pending changes: %s", redactURLCredentials(err.Error()))
	}
	if committed {
		r.postCommitHooks(t, d.wt.Dir)
	}
	forced, err := r.pushBranch(ctx, taskID, d, body.ForceWithLease)
	if err != nil {
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
		note += "\nPending edits were committed as \"" + commitMsg + "\"."
	}
	if forced {
		note += "\nThe rewritten branch was force-pushed with a lease on " + shortSHA(body.ForceWithLease) + "."
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: note})
	r.notePRSync(taskID, "") // clears the out-of-sync flag
	r.broadcastTask(r.taskSnapshot(taskID))
	return &prOpened{Repo: d.Repo, URL: res.URL, Number: res.Number, Updated: res.Updated, Committed: committed}, nil
}

// pushBranch pushes the draft's branch, force-pushing with a lease when the operator confirmed
// one. A non-fast-forward rejection is diagnosed and returned as *pushRejected; nothing is
// forced without that confirmation. forced reports whether the remote history was replaced.
func (r *Router) pushBranch(ctx context.Context, taskID string, d *prDraft, lease string) (forced bool, err error) {
	if lease != "" {
		if err := gitwt.ForcePushWithLease(ctx, d.wt.Checkout, d.SourceBranch, lease); err != nil {
			return false, fmt.Errorf("force push refused — origin/%s changed after you reviewed it, review it again: %s",
				d.SourceBranch, redactURLCredentials(err.Error()))
		}
		return true, nil
	}
	err = gitwt.Push(ctx, d.wt.Checkout, d.SourceBranch)
	if err == nil || !errors.Is(err, gitwt.ErrPushRejected) {
		if err != nil {
			err = fmt.Errorf("%s", redactURLCredentials(err.Error()))
		}
		return false, err
	}
	div, derr := gitwt.Diverge(ctx, d.wt.Checkout, d.SourceBranch)
	if derr != nil {
		return false, fmt.Errorf("%s (diagnosing it failed: %s)", redactURLCredentials(err.Error()), redactURLCredentials(derr.Error()))
	}
	if len(div.RemoteOnly) == 0 { // origin caught up between the push and the fetch
		if err := gitwt.Push(ctx, d.wt.Checkout, d.SourceBranch); err != nil {
			return false, fmt.Errorf("%s", redactURLCredentials(err.Error()))
		}
		return false, nil
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
		"⚠ Push of %s was rejected: origin has %d commit(s) the local branch lacks (%d rebased copies, %d not in the local branch). %s "+
			"Nothing was force-pushed — confirm or cancel in the Pull Request tab.",
		d.SourceBranch, len(div.RemoteOnly), len(div.RemoteOnly)-div.Foreign, div.Foreign, div.Explanation)})
	return false, &pushRejected{Divergence: div}
}

func shortSHA(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

// urlUserinfo matches the credentials part of a URL ("https://user:token@").
var urlUserinfo = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/\s@'"]+@`)

// redactURLCredentials masks credentials embedded in URLs, e.g. in a git remote or its errors.
func redactURLCredentials(s string) string { return urlUserinfo.ReplaceAllString(s, "${1}***@") }

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func safeFileName(s string) string {
	return strings.Trim(unsafeFileChars.ReplaceAllString(s, "-"), "-.")
}

// dependencyBlocker explains why a branch that commits installed dependencies cannot be pushed.
func dependencyBlocker(deps map[string]int) string {
	var dirs []string
	for d, n := range deps {
		dirs = append(dirs, fmt.Sprintf("%s (%d files)", d, n))
	}
	sort.Strings(dirs)
	return "the branch commits installed dependencies: " + strings.Join(limit(dirs, 5), ", ") + ". They are local tool " +
		"output, not part of the change — ask the agent to take them out of the branch's commits (keep the files on disk), " +
		"then open the pull request"
}

// branchBlocker explains why a worktree on another branch cannot update the task's pull request.
func branchBlocker(d *prDraft) string {
	return fmt.Sprintf("the worktree is on branch %s, not the task branch %s (switched or renamed outside the orchestrator). %s",
		d.CheckedOut, d.SourceBranch, switchHint(d))
}

func switchHint(d *prDraft) string {
	hint := fmt.Sprintf("Switch the worktree back with `git -C %s switch %s`", d.wt.Checkout, d.SourceBranch)
	if d.HeldBy != "" {
		hint += fmt.Sprintf(" after moving %s off it (git allows a branch in one checkout at a time)", d.HeldBy)
	}
	return hint + "."
}

// conflictBlocker explains why a pull request cannot be opened while a conflict is unfinished.
func conflictBlocker(c gitwt.ConflictState) string {
	var parts []string
	if c.Operation != "" {
		parts = append(parts, "a "+c.Operation+" is still in progress")
	}
	if len(c.Unmerged) > 0 {
		parts = append(parts, "unresolved files: "+strings.Join(limit(c.Unmerged, 5), ", "))
	}
	if len(c.Markers) > 0 {
		parts = append(parts, "conflict markers at "+strings.Join(limit(c.Markers, 5), ", "))
	}
	return "unfinished merge conflict (" + strings.Join(parts, "; ") + ") — ask the agent to resolve it hunk by hunk, then open the pull request"
}
