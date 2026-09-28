package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func TestParseDockerStatsLine(t *testing.T) {
	line := `{"ID":"abc123","Name":"atdd-runner","CPUPerc":"187.25%","MemUsage":"512.5MiB / 1.944GiB","MemPerc":"25.74%"}`
	s, err := ParseDockerStatsLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if s.ContainerID != "abc123" || s.CPUPercent != 187.25 {
		t.Fatalf("sample = %+v", s)
	}
	if s.MemoryMB < 512.49 || s.MemoryMB > 512.51 {
		t.Fatalf("memory = %v", s.MemoryMB)
	}
	for in, want := range map[string]float64{"1.5GiB / 4GiB": 1536, "800kB / 1GB": 0.8 * 1000 / 1024 / 1024 * 1000, "2GB / 4GB": 2e9 / (1024 * 1024)} {
		got, err := parseMemUsageMB(in)
		if err != nil {
			t.Fatal(err)
		}
		if diff := got - want; diff > 0.01 || diff < -0.01 {
			t.Fatalf("parseMemUsageMB(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseDockerStatsLine("not json"); err == nil {
		t.Fatal("expected error for malformed line")
	}
}

func TestContainerMonitorTracksPeaksAndSpikes(t *testing.T) {
	cpu := []string{"10.00%", "95.50%", "40.00%", "12.00%"}
	mem := []string{"100MiB / 2GiB", "300MiB / 2GiB", "1.5GiB / 2GiB", "200MiB / 2GiB"}
	var calls int
	var mu sync.Mutex
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		if name != "docker" || args[0] != "stats" || args[1] != "--no-stream" {
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
		i := calls
		if i >= len(cpu) {
			i = len(cpu) - 1
		}
		calls++
		return []byte(fmt.Sprintf(`{"ID":"c1","CPUPerc":%q,"MemUsage":%q}`+"\n", cpu[i], mem[i])), nil
	}
	sampler := NewDockerStatsSampler(runner)
	mon := NewContainerMonitor(sampler, SpikeThresholds{CPUPercent: 80, MemoryMB: 1024})

	for i := 0; i < 4; i++ {
		if err := mon.SampleOnce(context.Background(), "c1"); err != nil {
			t.Fatal(err)
		}
	}
	sum := mon.Summary()
	if sum.Samples != 4 || sum.PeakCPUPercent != 95.5 || sum.PeakMemoryMB != 1536 {
		t.Fatalf("summary = %+v", sum)
	}
	if len(sum.Spikes) != 2 {
		t.Fatalf("spikes = %+v, want cpu spike + memory spike", sum.Spikes)
	}
	if sum.Spikes[0].Kind != "cpu" || sum.Spikes[1].Kind != "memory" {
		t.Fatalf("spike kinds = %+v", sum.Spikes)
	}
}

func TestContainerMonitorRunLoopStopsOnCancel(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{"ID":"c1","CPUPerc":"5%","MemUsage":"10MiB / 1GiB"}`), nil
	}
	mon := NewContainerMonitor(NewDockerStatsSampler(runner), SpikeThresholds{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { mon.Run(ctx, "c1", 5*time.Millisecond); close(done) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor did not stop on cancel")
	}
	if mon.Summary().Samples < 2 {
		t.Fatalf("expected multiple samples, got %d", mon.Summary().Samples)
	}
}

func TestTTRCollectorPhaseDurationsAndTaskMetadata(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)}
	c := NewTTRCollector()
	c.now = clk.Now

	task := &types.Task{ID: "TASK-7", CurrentStageID: "prd_discovery", AssignedRepos: []string{"backend-core"}}
	c.StartRun(RunMeta{TaskID: "TASK-7", Model: "claude/claude-3-5-sonnet", Tier: "tier1", Method: "BMAD",
		Repo: "backend-core", Category: "crud", Complexity: "MEDIUM", RouterStrategy: "best_practice"})

	hook := c.TransitionHook()
	clk.Advance(1500 * time.Microsecond)
	if err := hook(task, "prd_discovery", "atdd_creation"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(2 * time.Second)
	c.RecordTestIteration("TASK-7", false)
	c.RecordTestIteration("TASK-7", false)
	c.RecordTestIteration("TASK-7", true)
	_ = hook(task, "atdd_creation", "task_implementation")
	clk.Advance(3 * time.Second)

	c.AttachResources("TASK-7", ResourceSummary{Samples: 3, PeakCPUPercent: 180, PeakMemoryMB: 900,
		Spikes: []ResourceSpike{{Kind: "cpu", Value: 180}}})

	rec, ok := c.Finish("TASK-7", true)
	if !ok {
		t.Fatal("finish returned !ok")
	}
	if rec.PhaseDurationsUS["prd_discovery"] != 1500 {
		t.Fatalf("prd phase = %dµs, want 1500", rec.PhaseDurationsUS["prd_discovery"])
	}
	if rec.PhaseDurationsUS["atdd_creation"] != 2_000_000 || rec.PhaseDurationsUS["task_implementation"] != 3_000_000 {
		t.Fatalf("phase durations = %+v", rec.PhaseDurationsUS)
	}
	wantTotal := int64(1500 + 2_000_000 + 3_000_000)
	if rec.DurationUS != wantTotal {
		t.Fatalf("duration = %d, want %d", rec.DurationUS, wantTotal)
	}
	if rec.TestIterations != 3 || rec.FailureLoops != 2 || rec.FirstPass {
		t.Fatalf("test cycle stats = iter %d loops %d firstPass %v", rec.TestIterations, rec.FailureLoops, rec.FirstPass)
	}
	if !rec.Success || rec.PeakCPUPercent != 180 || rec.PeakMemoryMB != 900 || rec.ResourceSpikes != 1 {
		t.Fatalf("record = %+v", rec)
	}

	rec.ApplyToTask(task)
	if task.Metadata["ttr_seconds"] != "5.0015" {
		t.Fatalf("ttr_seconds = %q", task.Metadata["ttr_seconds"])
	}
	if task.Metadata["peak_cpu_percent"] != "180.00" || task.Metadata["peak_memory_mb"] != "900.00" {
		t.Fatalf("resource metadata = %+v", task.Metadata)
	}
	if task.Metadata["test_iterations"] != "3" || task.Metadata["first_pass"] != "false" {
		t.Fatalf("test metadata = %+v", task.Metadata)
	}
	var phases map[string]int64
	if err := json.Unmarshal([]byte(task.Metadata["phase_durations_us"]), &phases); err != nil || phases["atdd_creation"] != 2_000_000 {
		t.Fatalf("phase_durations_us = %q (%v)", task.Metadata["phase_durations_us"], err)
	}

	if _, ok := c.Finish("TASK-7", true); ok {
		t.Fatal("second finish of the same run should report !ok")
	}
}

func TestTTRCollectorFirstPassWhenFirstIterationPasses(t *testing.T) {
	c := NewTTRCollector()
	c.StartRun(RunMeta{TaskID: "T"})
	c.RecordTestIteration("T", true)
	rec, _ := c.Finish("T", true)
	if !rec.FirstPass || rec.FailureLoops != 0 {
		t.Fatalf("record = %+v", rec)
	}
	// A run with no test iterations that succeeds counts as first pass.
	c.StartRun(RunMeta{TaskID: "U"})
	rec, _ = c.Finish("U", true)
	if !rec.FirstPass {
		t.Fatal("successful run with no test cycles should be first pass")
	}
	// Unknown task transitions are ignored, not errors.
	if err := c.TransitionHook()(&types.Task{ID: "nope"}, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(rec.StartedAt), "20") {
		t.Fatal("StartedAt not set")
	}
}
