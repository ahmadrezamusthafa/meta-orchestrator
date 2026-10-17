package api

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestTaskContainersAttribution(t *testing.T) {
	since := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	list := []telemetry.ContainerInfo{
		{ID: "mine", CreatedAt: since.Add(-time.Hour), Labels: map[string]string{dockerTaskLabel: "TASK-1"}},
		{ID: "other-task", CreatedAt: since.Add(time.Minute), Labels: map[string]string{dockerTaskLabel: "TASK-2"}},
		{ID: "compose-new", CreatedAt: since, Labels: map[string]string{"com.docker.compose.project": "app"}},
		{ID: "old", CreatedAt: since.Add(-time.Second), Labels: map[string]string{}},
	}
	if got := strings.Join(taskContainers(list, "TASK-1", since), ","); got != "mine,compose-new" {
		t.Fatalf("containers = %s", got)
	}
	if got := limitContainers("docker run -d app", "2", "TASK-1"); got != "docker run --cpus=2 --label meta-orchestrator.task=TASK-1 -d app" {
		t.Fatalf("got %q", got)
	}
	if got := limitContainers("docker run app", "0", "bad id;rm"); got != "docker run app" {
		t.Fatalf("an unsafe task id must not be spliced into the command: %q", got)
	}
}

// dockerStub runs a turn in which the agent starts a container through the approval path.
type dockerStub struct {
	stubProvider
	ran chan struct{}
}

func (s *dockerStub) StreamActivity(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) (*llm.LLMResponse, error) {
	d := req.Approver(ctx, llm.ApprovalRequest{ToolName: "Bash", ToolUseID: "tu-1", Input: map[string]interface{}{"command": "docker run -d app"}})
	if !d.Allow {
		return nil, context.Canceled
	}
	select { // keep the turn open until the container was sampled
	case <-s.ran:
	case <-time.After(2 * time.Second):
	}
	return &llm.LLMResponse{Content: "## Summary\nStarted the app.", Provider: "anthropic", Model: "claude-x", FinishReason: "stop"}, nil
}

func dockerStatsRouter(t *testing.T, installed bool) (*Router, *[]string) {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	stub := &dockerStub{ran: make(chan struct{})}
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, stub)
	}
	var mu sync.Mutex
	var calls []string
	var once sync.Once
	stats := 0
	r.docker.hasDocker = func() bool { return installed }
	r.docker.statsInterval = 5 * time.Millisecond
	r.docker.statsRun = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, strings.Join(args, " "))
		switch args[0] {
		case "ps":
			return []byte(`{"ID":"c1","CreatedAt":"2020-01-01 00:00:00 +0000 UTC","Labels":"meta-orchestrator.task=TASK-S1"}` + "\n" +
				`{"ID":"c2","CreatedAt":"2020-01-01 00:00:00 +0000 UTC","Labels":"meta-orchestrator.task=TASK-OTHER"}` + "\n"), nil
		case "stats":
			if args[len(args)-1] != "c1" || len(args) != 5 {
				t.Errorf("sampled other containers: %v", args)
			}
			stats++
			cpu := []string{"20%", "40.5%", "10%"}[min(stats-1, 2)]
			if stats >= 3 {
				once.Do(func() { close(stub.ran) })
			}
			return []byte(`{"ID":"c1","CPUPerc":"` + cpu + `","MemUsage":"256MiB / 2GiB"}`), nil
		}
		return nil, nil
	}
	r.mu.Lock()
	r.tasks["TASK-S1"] = &types.Task{ID: "TASK-S1", Title: "Run the app", CurrentStageID: "e2e_validation", State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW", "approve_all": "true"}}
	r.mu.Unlock()
	if !installed {
		close(stub.ran)
	}
	return r, &calls
}

func TestDockerTurnSamplesTheTaskContainers(t *testing.T) {
	r, _ := dockerStatsRouter(t, true)
	if w := do(r, "POST", "/api/v1/tasks/TASK-S1/execute", ""); w.Code != 200 {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-S1", types.TaskStateWaitingGateApproval)
	waitIdleTask(t, r, "TASK-S1")
	log := consoleText(r, "TASK-S1")
	if !strings.Contains(log, "🐳 Containers this turn: peak CPU 40.5% · peak memory 256 MB") || !strings.Contains(log, "· 1 container") {
		t.Fatalf("console:\n%s", log)
	}
	if got := r.taskSnapshot("TASK-S1").Metadata["docker_stats"]; !strings.HasPrefix(got, "peak CPU 40.5%") {
		t.Fatalf("docker_stats = %q", got)
	}
	r.docker.mu.Lock()
	left := len(r.docker.watches)
	r.docker.mu.Unlock()
	if left != 0 {
		t.Fatal("the turn's monitor must stop when the turn ends")
	}
}

func TestDockerTurnWithoutDockerSamplesNothing(t *testing.T) {
	r, calls := dockerStatsRouter(t, false)
	if w := do(r, "POST", "/api/v1/tasks/TASK-S1/execute", ""); w.Code != 200 {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-S1", types.TaskStateWaitingGateApproval)
	waitIdleTask(t, r, "TASK-S1")
	if len(*calls) != 0 || strings.Contains(consoleText(r, "TASK-S1"), "Containers this turn") {
		t.Fatalf("no docker CLI → no sampling: %v", *calls)
	}
}
