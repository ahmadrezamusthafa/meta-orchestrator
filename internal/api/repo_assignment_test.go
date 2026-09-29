package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// recordingProvider captures what the orchestrator asked the agent to do.
type recordingProvider struct {
	stubProvider
	mu   sync.Mutex
	reqs []llm.LLMRequest
}

func (p *recordingProvider) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, *req)
	p.mu.Unlock()
	return p.stubProvider.Complete(ctx, req)
}
func (p *recordingProvider) Stream(ctx context.Context, req *llm.LLMRequest, ch chan<- types.ThoughtChunk) (*llm.LLMResponse, error) {
	return p.Complete(ctx, req)
}
func (p *recordingProvider) last() llm.LLMRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reqs[len(p.reqs)-1]
}

func TestInferReposFromJiraFields(t *testing.T) {
	r, _ := worktreeRouter(t) // registers repo "app"
	cfg := types.DefaultJiraSyncConfig()

	if repos, _ := r.inferRepos(types.JiraIssueDTO{ProjectKey: "MIB"}, cfg); len(repos) != 0 {
		t.Fatalf("nothing should match without rules: %v", repos)
	}
	if repos, src := r.inferRepos(types.JiraIssueDTO{ProjectKey: "MIB", Components: []string{"App"}}, cfg); len(repos) != 1 || !strings.Contains(src, "component") {
		t.Fatalf("component named like a repo should match: %v %q", repos, src)
	}
	cfg.RepoRules = map[string][]string{"mib": {"app", "not-registered"}}
	if repos, src := r.inferRepos(types.JiraIssueDTO{ProjectKey: "MIB"}, cfg); len(repos) != 1 || repos[0] != "app" || !strings.Contains(src, "rule") {
		t.Fatalf("project rule should assign only registered repos: %v %q", repos, src)
	}
	cfg.RepoRules, cfg.AssignedRepos = nil, []string{"app"}
	if repos, src := r.inferRepos(types.JiraIssueDTO{ProjectKey: "X"}, cfg); len(repos) != 1 || src != "sync default repositories" {
		t.Fatalf("sync default should be the fallback: %v %q", repos, src)
	}
}

func TestCodeStageNeedsReposAndWritesOnlyInWorktree(t *testing.T) {
	r, _ := worktreeRouter(t)
	rec := &recordingProvider{}
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, rec)
	}
	r.mu.Lock()
	r.tasks["TASK-9"] = &types.Task{ID: "TASK-9", Title: "Fix", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		Metadata: map[string]string{}}
	r.mu.Unlock()

	w := do(r, http.MethodPost, "/api/v1/tasks/TASK-9/execute", "")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "needs_repositories") {
		t.Fatalf("code stage without repos must be refused: %d %s", w.Code, w.Body.String())
	}
	if len(rec.reqs) != 0 || r.taskSnapshot("TASK-9").State != types.TaskStatePending {
		t.Fatal("no agent turn may start without repositories")
	}

	do(r, http.MethodPatch, "/api/v1/tasks/TASK-9", `{"assigned_repos":["app"]}`)
	r.executeTaskWithAI("TASK-9")
	got := rec.last()
	if got.PermissionMode != "acceptEdits" || !r.isTaskWorktree("TASK-9", got.WorkDir) {
		t.Fatalf("implementation should edit inside the worktree: mode=%q dir=%q", got.PermissionMode, got.WorkDir)
	}

	// Analysis stages stay read-only even inside the worktree.
	r.mu.Lock()
	r.tasks["TASK-9"].CurrentStageID, r.tasks["TASK-9"].State = "techdoc_rfc", types.TaskStatePending
	r.mu.Unlock()
	r.executeTaskWithAI("TASK-9")
	if m := rec.last().PermissionMode; m != "" {
		t.Fatalf("non-code stage must stay read-only, got %q", m)
	}
}

func TestTaskWithoutReposNeverRunsInOrchestratorRoot(t *testing.T) {
	r := hermeticRouter(t)
	task := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Clarify requirement"}`).Body.Bytes())
	if dir := r.resolveWorkDir(r.taskSnapshot(task.ID)); dir == r.cfg.RootDir || !strings.HasPrefix(dir, r.artifactDir(task.ID)) {
		t.Fatalf("repo-less task must work in its artifact folder, got %s", dir)
	}
}
