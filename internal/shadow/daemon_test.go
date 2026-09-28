package shadow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func completedTask(id string) *types.Task {
	return &types.Task{ID: id, Title: "Add CRUD endpoint", Description: "invoice items", State: types.TaskStateCompleted,
		CurrentStageID: "task_implementation", AssignedRepos: []string{"backend-core"},
		Metadata: map[string]string{"complexity": "MEDIUM", "intake_commit_backend-core": "abc1234"}}
}

func TestCaptureReplayClonesInputsCommitsAndAssertions(t *testing.T) {
	dir := t.TempDir()
	artifacts := filepath.Join(dir, "artifacts")
	_ = os.MkdirAll(artifacts, 0o755)
	_ = os.WriteFile(filepath.Join(artifacts, "atdd_acceptance.feature"), []byte("Scenario: creates item"), 0o644)
	_ = os.WriteFile(filepath.Join(artifacts, "notes.txt"), []byte("ignore"), 0o644)

	task := completedTask("TASK-1")
	task.ArtifactDir = artifacts
	task.AssignedRepos = []string{"backend-core", "frontend-portal"}
	run := telemetry.RunRecord{RunMeta: telemetry.RunMeta{TaskID: "TASK-1", Method: "ReAct", Model: "m", Tier: "tier2", Category: "crud"}, FirstPass: true}

	var revCalls []string
	spec, err := CaptureReplay(task, run, map[string]string{"backend-core": "/src/backend", "frontend-portal": "/src/frontend"},
		func(path string) (string, error) { revCalls = append(revCalls, path); return "fff9999", nil })
	if err != nil {
		t.Fatal(err)
	}
	if spec.SourceTaskID != "TASK-1" || spec.Title != task.Title || spec.Complexity != "MEDIUM" || spec.Category != "crud" {
		t.Fatalf("spec = %+v", spec)
	}
	commits := map[string]string{}
	for _, r := range spec.Repos {
		commits[r.Name] = r.CommitSHA
	}
	if commits["backend-core"] != "abc1234" {
		t.Fatalf("intake commit not preserved: %v", commits)
	}
	if commits["frontend-portal"] != "fff9999" || len(revCalls) != 1 || revCalls[0] != "/src/frontend" {
		t.Fatalf("missing intake commit must fall back to git rev-parse: %v %v", commits, revCalls)
	}
	if len(spec.ATDDAssertions) != 1 || !strings.HasSuffix(spec.ATDDAssertions[0].Path, "atdd_acceptance.feature") ||
		spec.ATDDAssertions[0].Content != "Scenario: creates item" {
		t.Fatalf("assertions = %+v", spec.ATDDAssertions)
	}
	// The spec is a deep copy: mutating the task afterwards does not change it.
	task.Title = "changed"
	task.Metadata["complexity"] = "HIGH"
	if spec.Title == "changed" || spec.Complexity == "HIGH" {
		t.Fatal("replay spec aliases the live task")
	}
}

func TestDockerSandboxProvisionsLowestPriorityIsolatedContainer(t *testing.T) {
	var got [][]string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		got = append(got, append([]string{name}, args...))
		if name == "docker" && args[0] == "run" {
			return []byte("container-123\n"), nil
		}
		return nil, nil
	}
	sb := NewDockerSandbox(SandboxConfig{Image: "mo-runner:latest", CPUs: 1.5, MemoryMB: 2048, WorkRoot: t.TempDir()}, runner)
	spec := ReplaySpec{SourceTaskID: "TASK-1", Repos: []RepoSnapshot{{Name: "backend-core", Path: "/src/backend", CommitSHA: "abc1234"}}}
	h, err := sb.Provision(context.Background(), "job-1", spec)
	if err != nil {
		t.Fatal(err)
	}
	if h.ContainerID != "container-123" {
		t.Fatalf("container id = %q", h.ContainerID)
	}

	var worktree, run []string
	for _, c := range got {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "worktree add") {
			worktree = c
		}
		if c[0] == "docker" && c[1] == "run" {
			run = c
		}
	}
	if worktree == nil || !strings.Contains(strings.Join(worktree, " "), "abc1234") {
		t.Fatalf("worktree not checked out at intake commit: %v", got)
	}
	joined := strings.Join(run, " ")
	for _, want := range []string{"--cpu-shares 2", "--cpus 1.50", "--memory 2048m", "--network none", "--label mo.shadow=true", "nice -n 19"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker run missing %q: %s", want, joined)
		}
	}

	if err := sb.Teardown(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	last := strings.Join(got[len(got)-1], " ")
	if !strings.Contains(strings.Join(got[len(got)-2], " ")+last, "rm -f container-123") {
		t.Fatalf("teardown did not remove container: %v", got[len(got)-2:])
	}
}

func TestDaemonQueuesOnlyCompletedProductionTasks(t *testing.T) {
	d := NewDaemon(DaemonConfig{QueueSize: 10}, nil, func(ctx context.Context, job Job) error { return nil })
	if !d.OnTaskCompleted(completedTask("A"), telemetry.RunRecord{Success: true}) {
		t.Fatal("completed task should enqueue")
	}
	running := completedTask("B")
	running.State = types.TaskStateRunning
	if d.OnTaskCompleted(running, telemetry.RunRecord{}) {
		t.Fatal("non-completed task enqueued")
	}
	shadowTask := completedTask("C")
	shadowTask.Metadata["shadow"] = "true"
	if d.OnTaskCompleted(shadowTask, telemetry.RunRecord{}) {
		t.Fatal("shadow replay must not re-enqueue itself")
	}
	if d.OnTaskCompleted(completedTask("A"), telemetry.RunRecord{}) {
		t.Fatal("duplicate task enqueued twice")
	}
	if d.Stats().Queued != 1 {
		t.Fatalf("stats = %+v", d.Stats())
	}
}

func TestDaemonDropsWhenQueueFullWithoutBlocking(t *testing.T) {
	d := NewDaemon(DaemonConfig{QueueSize: 2}, nil, func(ctx context.Context, job Job) error { return nil })
	done := make(chan struct{})
	go func() {
		for i := 0; i < 5; i++ {
			d.OnTaskCompleted(completedTask(fmt.Sprintf("T%d", i)), telemetry.RunRecord{})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("OnTaskCompleted blocked the production path")
	}
	if s := d.Stats(); s.Queued != 2 || s.Dropped != 3 {
		t.Fatalf("stats = %+v", s)
	}
}

func TestDaemonHoldsJobsWhileHostCPUAboveCeiling(t *testing.T) {
	var load atomic.Value
	load.Store(95.0)
	probe := func(ctx context.Context) (float64, error) { return load.Load().(float64), nil }

	var mu sync.Mutex
	var launchedAtLoad []float64
	started := make(chan struct{}, 1)
	d := NewDaemon(DaemonConfig{QueueSize: 4, Workers: 1, CPUCeilingPercent: 70, ThrottleInterval: 5 * time.Millisecond}, probe,
		func(ctx context.Context, job Job) error {
			mu.Lock()
			launchedAtLoad = append(launchedAtLoad, load.Load().(float64))
			mu.Unlock()
			started <- struct{}{}
			return nil
		})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	d.OnTaskCompleted(completedTask("T1"), telemetry.RunRecord{})

	select {
	case <-started:
		t.Fatal("shadow job launched while host CPU above ceiling")
	case <-time.After(60 * time.Millisecond):
	}
	if d.Stats().Throttled == 0 {
		t.Fatalf("throttle not recorded: %+v", d.Stats())
	}

	load.Store(20.0)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("shadow job never launched after load dropped")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(launchedAtLoad) != 1 || launchedAtLoad[0] > 70 {
		t.Fatalf("launched at load %v", launchedAtLoad)
	}
	d.Stop()
	if s := d.Stats(); s.Completed != 1 {
		t.Fatalf("stats = %+v", s)
	}
}

func TestParseLoadAverage(t *testing.T) {
	if v, err := parseLoadAvg("{ 2.50 1.20 0.90 }", 4); err != nil || v != 62.5 {
		t.Fatalf("darwin loadavg → %v %v", v, err)
	}
	if v, err := parseLoadAvg("1.00 0.50 0.25 1/345 12345", 2); err != nil || v != 50 {
		t.Fatalf("linux loadavg → %v %v", v, err)
	}
	if _, err := parseLoadAvg("garbage", 2); err == nil {
		t.Fatal("expected parse error")
	}
}
