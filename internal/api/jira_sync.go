package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// jiraSyncStartDelay lets the daemon finish booting before the first sync hits JIRA.
const jiraSyncStartDelay = 3 * time.Second

var errJiraSyncBusy = errors.New("a JIRA sync is already running")

// jiraSyncState tracks the board sync loop and the outcome of its last run.
type jiraSyncState struct {
	mu      sync.Mutex
	status  types.JiraSyncStatus
	running bool
	kick    chan struct{} // wakes the loop after the rules change
}

func newJiraSyncState() *jiraSyncState {
	return &jiraSyncState{kick: make(chan struct{}, 1)}
}

// jiraTaskOptions controls how an issue becomes a task.
type jiraTaskOptions struct {
	WorkflowID     string
	SelectedMethod string
	AssignedRepos  []string
	StartStageID   string
	Source         string // "import" or "sync"
}

// newJiraTaskLocked builds a PENDING task for issue and adds it to the board. Caller holds r.mu.
func (r *Router) newJiraTaskLocked(issue types.JiraIssueDTO, opts jiraTaskOptions) *types.Task {
	taskID := r.nextTaskIDLocked()
	stage := opts.StartStageID
	idx := stageIndexOf(stage)
	if idx < 0 {
		stage, idx = "prd_discovery", 0
	}
	workflowID := opts.WorkflowID
	if workflowID == "" {
		workflowID = "general_ai_sdlc"
	}
	method := opts.SelectedMethod
	if method == "" || method == "Auto" {
		method = "BMAD"
	}
	repos := append([]string{}, opts.AssignedRepos...)

	title := fmt.Sprintf("[%s] %s", issue.Key, issue.Summary)
	if strings.Contains(issue.Summary, issue.Key) {
		title = issue.Summary
	}
	now := time.Now()
	task := &types.Task{
		ID:                taskID,
		WorkflowID:        workflowID,
		Title:             title,
		Description:       issue.Description,
		CurrentStageID:    stage,
		CurrentStageIndex: idx,
		State:             types.TaskStatePending,
		AssignedRepos:     repos,
		ProfileName:       "orchestrator_agent",
		SelectedMethod:    method,
		MaxTokenBudget:    50000,
		ArtifactDir:       fmt.Sprintf(".sdlc/artifacts/%s", taskID),
		Metadata: map[string]string{
			"router_strategy":  "BEST_PRACTICE",
			"router_source":    "JIRA_CONNECTOR",
			"router_rationale": fmt.Sprintf("From JIRA %s (%s). Routed to %s.", issue.Key, issue.Status, method),
			"jira_source":      opts.Source,
			"jira_synced_at":   now.Format(time.RFC3339),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	applyJiraFields(task, issue)
	task.Metadata["worktree_enabled"] = "true"
	task.Metadata["worktree_branch"] = plannedBranch(task)
	r.tasks[taskID] = task
	return task
}

// applyJiraFields copies issue fields onto the task and reports whether anything changed.
func applyJiraFields(t *types.Task, issue types.JiraIssueDTO) bool {
	changed := false
	set := func(k, v string) {
		if t.Metadata[k] != v {
			t.Metadata[k] = v
			changed = true
		}
	}
	set("jira_key", issue.Key)
	set("jira_url", issue.URL)
	set("jira_status", issue.Status)
	set("jira_priority", issue.Priority)
	set("jira_assignee", issue.Assignee)
	set("jira_issue_type", issue.IssueType)
	set("jira_parent_key", issue.ParentKey)
	set("jira_project", issue.ProjectKey)
	set("jira_components", strings.Join(issue.Components, ", "))
	set("jira_labels", strings.Join(issue.Labels, ", "))
	set("jira_epic_key", issue.EpicKey)
	if issue.EpicSummary != "" || issue.EpicKey == "" { // Epic Link fallback carries a key without a summary
		set("jira_epic_name", issue.EpicSummary)
	}
	if t.Description != issue.Description {
		t.Description = issue.Description
		changed = true
	}
	title := fmt.Sprintf("[%s] %s", issue.Key, issue.Summary)
	if strings.Contains(issue.Summary, issue.Key) {
		title = issue.Summary
	}
	if t.Title != title {
		t.Title = title
		changed = true
	}
	return changed
}

// writeJiraPRD seeds the task's PRD.md from the issue so the first stage starts from real
// requirements. It is rewritten whenever a sync brings changes to the issue.
func (r *Router) writeJiraPRD(taskID string, issue types.JiraIssueDTO) {
	dir := filepath.Join(r.cfg.RootDir, ".sdlc", "artifacts", taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	cell := func(v string) string {
		if strings.TrimSpace(v) == "" {
			return "—"
		}
		return strings.ReplaceAll(v, "|", `\|`)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s\n\n", issue.Key, issue.Summary)
	b.WriteString("| Field | Value |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| Source | [%s](%s) |\n", issue.Key, issue.URL)
	fmt.Fprintf(&b, "| Type | %s |\n| Status | %s |\n| Priority | %s |\n", cell(issue.IssueType), cell(issue.Status), cell(issue.Priority))
	fmt.Fprintf(&b, "| Assignee | %s |\n| Reporter | %s |\n", cell(issue.Assignee), cell(issue.Reporter))
	if issue.EpicKey != "" && issue.EpicKey != issue.Key {
		epic := issue.EpicKey
		if issue.EpicSummary != "" {
			epic += " — " + issue.EpicSummary
		}
		fmt.Fprintf(&b, "| Epic | %s |\n", cell(epic))
	}
	fmt.Fprintf(&b, "| Synced | %s |\n\n", time.Now().Format("2006-01-02 15:04 MST"))
	b.WriteString("## Requirements\n\n")
	if strings.TrimSpace(issue.Description) == "" {
		b.WriteString("_The JIRA issue has no description. The PRD Discovery stage will draft the requirements._\n")
	} else {
		b.WriteString(demoteHeadings(issue.Description) + "\n")
	}
	_ = os.WriteFile(filepath.Join(dir, "PRD.md"), []byte(b.String()), 0o644)
}

// demoteHeadings nests the issue's own headings under "## Requirements" (# → ###).
func demoteHeadings(md string) string {
	lines := strings.Split(md, "\n")
	inFence := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(l, "#") {
			if n := len(l) - len(strings.TrimLeft(l, "#")); n < 6 && strings.HasPrefix(l[n:], " ") {
				lines[i] = strings.Repeat("#", min(n+2, 6)) + l[n:]
			}
		}
	}
	return strings.Join(lines, "\n")
}

// jiraTaskIndexLocked maps upper-cased JIRA keys to their tasks. Caller holds r.mu.
func (r *Router) jiraTaskIndexLocked() map[string]*types.Task {
	idx := make(map[string]*types.Task)
	for _, t := range r.tasks {
		if k := t.Metadata["jira_key"]; k != "" {
			idx[strings.ToUpper(k)] = t
		}
	}
	return idx
}

// syncJiraBoard pulls the issues matched by the sync rules onto the board: new issues become
// PENDING tasks, linked tasks get their JIRA fields refreshed, and dismissed issues are skipped.
// Stages are never changed by a sync — the orchestrator owns task progress.
func (r *Router) syncJiraBoard(ctx context.Context) (types.JiraSyncStatus, error) {
	s := r.jira
	s.mu.Lock()
	if s.running {
		st := s.status
		s.mu.Unlock()
		return st, errJiraSyncBusy
	}
	s.running = true
	s.status.Running = true
	s.mu.Unlock()

	result := types.JiraSyncStatus{LastRunAt: time.Now()}
	finish := func(err error) (types.JiraSyncStatus, error) {
		if err != nil {
			result.LastError = err.Error()
		}
		s.mu.Lock()
		result.NextRunAt = s.status.NextRunAt
		s.status = result
		s.running = false
		s.mu.Unlock()
		if r.cfg.WSHub != nil {
			r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{Type: types.EventJiraSync, Timestamp: time.Now(), Payload: r.jiraSyncStatus()})
		}
		return r.jiraSyncStatus(), err
	}

	mgr := r.cfg.ConnectorsManager
	result.Connected = mgr.JiraConnected()
	if !result.Connected {
		return finish(fmt.Errorf("JIRA is not connected: add base URL, username and API token in Connectors"))
	}
	cfg := mgr.GetJiraSyncConfig()
	issues, err := mgr.QueryOpenJiraIssues(ctx, cfg.JQL, cfg.MaxIssues, cfg.ExcludeStatuses)
	if err != nil {
		return finish(err)
	}
	result.Fetched = len(issues)

	type created struct {
		task  *types.Task
		issue types.JiraIssueDTO
	}
	var newTasks []created
	var changed []*types.Task
	var refreshed []created
	now := time.Now().Format(time.RFC3339)

	inferred := map[string][]string{}
	inferredFrom := map[string]string{}
	for _, issue := range issues {
		key := strings.ToUpper(issue.Key)
		inferred[key], inferredFrom[key] = r.inferRepos(issue, cfg)
	}

	r.mu.Lock()
	linked := r.jiraTaskIndexLocked()
	for _, issue := range issues {
		key := strings.ToUpper(issue.Key)
		if existing, ok := linked[key]; ok {
			// Tasks that still have no repositories get them as soon as a rule matches.
			if len(existing.AssignedRepos) == 0 && existing.State != types.TaskStateRunning && len(inferred[key]) > 0 {
				existing.AssignedRepos = inferred[key]
				existing.Metadata["repos_assigned_by"] = inferredFrom[key]
				existing.UpdatedAt = time.Now()
				result.Updated++
				changed = append(changed, cloneTask(existing))
			}
			if cfg.UpdateExisting && applyJiraFields(existing, issue) {
				existing.Metadata["jira_synced_at"] = now
				existing.UpdatedAt = time.Now()
				result.Updated++
				changed = append(changed, cloneTask(existing))
				refreshed = append(refreshed, created{task: existing, issue: issue})
			}
			continue
		}
		if r.dismissedJira[key] {
			result.Skipped++
			continue
		}
		stage := cfg.StatusStageMap[strings.ToLower(strings.TrimSpace(issue.Status))]
		if stage == "" {
			stage = cfg.DefaultStageID
		}
		t := r.newJiraTaskLocked(issue, jiraTaskOptions{
			WorkflowID:     cfg.WorkflowID,
			SelectedMethod: cfg.SelectedMethod,
			AssignedRepos:  inferred[key],
			StartStageID:   stage,
			Source:         "sync",
		})
		if src := inferredFrom[key]; src != "" {
			t.Metadata["repos_assigned_by"] = src
		}
		linked[key] = t
		newTasks = append(newTasks, created{task: cloneTask(t), issue: issue})
		result.Created++
	}
	r.mu.Unlock()

	for _, c := range newTasks {
		r.writeJiraPRD(c.task.ID, c.issue)
		r.addEntry(c.task.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
			"Synced from JIRA %s (%s). Run the stage to start the agent, or ask it a question below.", c.issue.Key, c.issue.Status)})
		r.broadcastTask(c.task)
	}
	for _, c := range refreshed {
		r.writeJiraPRD(c.task.ID, c.issue)
	}
	for _, t := range changed {
		r.broadcastTask(t)
	}
	if len(newTasks) > 0 || len(changed) > 0 {
		r.saveBoardNow()
	}
	return finish(nil)
}

// jiraSyncStatus returns the last sync outcome with live board counts.
func (r *Router) jiraSyncStatus() types.JiraSyncStatus {
	r.jira.mu.Lock()
	st := r.jira.status
	st.Running = r.jira.running
	r.jira.mu.Unlock()
	st.Connected = r.cfg.ConnectorsManager.JiraConnected()
	r.mu.RLock()
	st.LinkedTasks = len(r.jiraTaskIndexLocked())
	st.Dismissed = len(r.dismissedJira)
	r.mu.RUnlock()
	return st
}

// startJiraSync runs the board sync on the configured interval. Changing the rules wakes it early.
func (r *Router) startJiraSync() {
	go func() {
		wait := jiraSyncStartDelay
		for {
			r.jira.mu.Lock()
			r.jira.status.NextRunAt = time.Now().Add(wait)
			r.jira.mu.Unlock()
			timer := time.NewTimer(wait)
			select {
			case <-r.stop:
				timer.Stop()
				return
			case <-r.jira.kick:
				timer.Stop()
			case <-timer.C:
			}
			cfg := r.cfg.ConnectorsManager.GetJiraSyncConfig()
			wait = time.Duration(cfg.IntervalSeconds) * time.Second
			if wait < time.Minute {
				wait = time.Minute
			}
			if !cfg.Enabled || !r.cfg.ConnectorsManager.JiraConnected() {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			if _, err := r.syncJiraBoard(ctx); err != nil && !errors.Is(err, errJiraSyncBusy) {
				fmt.Printf("[JIRA Sync] %v\n", err)
			}
			cancel()
		}
	}()
}

// kickJiraSync schedules an immediate sync on the background loop.
func (r *Router) kickJiraSync() {
	select {
	case r.jira.kick <- struct{}{}:
	default:
	}
}

// registeredRepoNames lists every repository registered in a project.
func (r *Router) registeredRepoNames() map[string]string {
	names := map[string]string{} // lower-case → registered name
	if r.cfg.ProjectManager == nil {
		return names
	}
	for _, p := range r.cfg.ProjectManager.List() {
		for _, repo := range p.Repos {
			if repo.Name != "" && repo.Path != "" {
				names[strings.ToLower(repo.Name)] = repo.Name
			}
		}
	}
	return names
}

// inferRepos picks the repositories for an issue: explicit rules on its project key, components
// and labels first, then components/labels named exactly like a registered repository, then the
// sync default. Only registered repositories are returned. source explains the choice.
func (r *Router) inferRepos(issue types.JiraIssueDTO, cfg types.JiraSyncConfig) (repos []string, source string) {
	registered := r.registeredRepoNames()
	seen := map[string]bool{}
	add := func(name string) {
		if canon, ok := registered[strings.ToLower(strings.TrimSpace(name))]; ok && !seen[canon] {
			seen[canon] = true
			repos = append(repos, canon)
		}
	}
	keys := []string{issue.ProjectKey}
	keys = append(keys, issue.Components...)
	keys = append(keys, issue.Labels...)
	var matched []string
	for _, k := range keys {
		if rule := cfg.RepoRules[strings.ToLower(strings.TrimSpace(k))]; len(rule) > 0 {
			for _, name := range rule {
				add(name)
			}
			matched = append(matched, k)
		}
	}
	if len(repos) > 0 {
		return repos, "JIRA rule: " + strings.Join(matched, ", ")
	}
	for _, k := range append(append([]string{}, issue.Components...), issue.Labels...) {
		add(k)
	}
	if len(repos) > 0 {
		return repos, "JIRA component/label matches repository name"
	}
	for _, name := range cfg.AssignedRepos {
		add(name)
	}
	if len(repos) > 0 {
		return repos, "sync default repositories"
	}
	return nil, ""
}
