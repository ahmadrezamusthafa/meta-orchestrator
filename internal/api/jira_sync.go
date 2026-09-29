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

// writeJiraPRD seeds the task's PRD.md from the issue so the first stage starts from real requirements.
func (r *Router) writeJiraPRD(taskID string, issue types.JiraIssueDTO) {
	dir := filepath.Join(r.cfg.RootDir, ".sdlc", "artifacts", taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	doc := fmt.Sprintf("# Product Requirements Document: [%s] %s\n\n"+
		"**Source:** [JIRA %s](%s)\n"+
		"**Type:** %s | **Status:** %s | **Priority:** %s\n"+
		"**Imported At:** %s\n\n"+
		"## Overview & Business Context\n%s\n",
		issue.Key, issue.Summary, issue.Key, issue.URL, issue.IssueType, issue.Status, issue.Priority,
		time.Now().Format(time.RFC3339), issue.Description)
	_ = os.WriteFile(filepath.Join(dir, "PRD.md"), []byte(doc), 0o644)
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
	now := time.Now().Format(time.RFC3339)

	r.mu.Lock()
	linked := r.jiraTaskIndexLocked()
	for _, issue := range issues {
		key := strings.ToUpper(issue.Key)
		if existing, ok := linked[key]; ok {
			if cfg.UpdateExisting && applyJiraFields(existing, issue) {
				existing.Metadata["jira_synced_at"] = now
				existing.UpdatedAt = time.Now()
				result.Updated++
				changed = append(changed, cloneTask(existing))
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
			AssignedRepos:  cfg.AssignedRepos,
			StartStageID:   stage,
			Source:         "sync",
		})
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
