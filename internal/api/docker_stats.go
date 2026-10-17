package api

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Container resource sampling: once a turn runs a heavy Docker command, the gate samples
// `docker stats --no-stream` for the task's containers until the turn ends, then reports the peak
// CPU and memory. A container is the task's when it carries the task label (added by the gate to
// docker run/create), or — for compose and other commands the gate does not rewrite — when it is
// unlabelled and was created after the turn's first Docker command (so another task's unlabelled
// container started in the same window is counted too). Nothing runs for turns without Docker,
// or when the docker CLI is not installed.

// dockerTaskLabel marks containers started by a task's agent.
const dockerTaskLabel = "meta-orchestrator.task"

const defaultStatsInterval = 5 * time.Second

func dockerInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

type turnWatch struct {
	taskID string
	cancel context.CancelFunc
	done   chan struct{}
	mon    *telemetry.ContainerMonitor
}

// taskContainers selects the containers attributed to taskID: labelled with it, or unlabelled and
// created at or after since.
func taskContainers(list []telemetry.ContainerInfo, taskID string, since time.Time) []string {
	var ids []string
	for _, c := range list {
		label, labelled := c.Labels[dockerTaskLabel]
		switch {
		case labelled && label == taskID:
			ids = append(ids, c.ID)
		case !labelled && !c.CreatedAt.IsZero() && !c.CreatedAt.Before(since):
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// watchTurn starts sampling the task's containers for the turn, once.
func (g *dockerGate) watchTurn(taskID, turnID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.onStats == nil || g.watches[turnID] != nil || !g.hasDocker() {
		return
	}
	sampler := telemetry.NewDockerStatsSampler(g.statsRun)
	since := time.Now().Truncate(time.Second) // docker ps prints whole seconds
	list := func(ctx context.Context) ([]string, error) {
		cs, err := sampler.ListContainers(ctx)
		if err != nil {
			return nil, err
		}
		return taskContainers(cs, taskID, since), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &turnWatch{taskID: taskID, cancel: cancel, done: make(chan struct{}), mon: telemetry.NewContainerMonitor(sampler, telemetry.SpikeThresholds{})}
	g.watches[turnID] = w
	go func() {
		defer close(w.done)
		w.mon.Run(ctx, list, g.statsInterval)
	}()
}

// stopWatch ends the turn's sampling and hands its summary to onStats.
func (g *dockerGate) stopWatch(turnID string) {
	g.mu.Lock()
	w := g.watches[turnID]
	delete(g.watches, turnID)
	g.mu.Unlock()
	if w == nil {
		return
	}
	w.cancel()
	<-w.done
	g.onStats(w.taskID, turnID, w.mon.Summary())
}

// reportContainerStats attaches a turn's container summary to the task's telemetry run and
// metadata, and tells the operator.
func (r *Router) reportContainerStats(taskID, turnID string, s telemetry.ResourceSummary) {
	if s.Samples == 0 {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: "Docker was used in this turn, " +
			"but no container of this task was running when its resources were sampled."})
		return
	}
	line := fmt.Sprintf("peak CPU %.1f%% · peak memory %.0f MB · %d sample%s · %d container%s", s.PeakCPUPercent, s.PeakMemoryMB,
		s.Samples, plural(s.Samples), s.Containers, plural(s.Containers))
	if r.telemetry != nil {
		r.telemetry.ttr.AttachResources(taskID, s)
	}
	r.mu.Lock()
	if t, ok := r.tasks[taskID]; ok {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		t.Metadata["docker_stats"] = line
	}
	r.mu.Unlock()
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: "🐳 Containers this turn: " + line +
		" (CPU and memory are the total across the task's containers at each sample)."})
}
