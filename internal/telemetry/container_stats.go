package telemetry

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CommandRunner executes an external command and returns stdout (injectable for tests).
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// ExecRunner is the default CommandRunner backed by os/exec.
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// ContainerSample is one point-in-time resource reading of a container.
type ContainerSample struct {
	ContainerID string    `json:"container_id"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryMB    float64   `json:"memory_mb"`
	SampledAt   time.Time `json:"sampled_at"`
}

// ResourceSpike marks a sample that crossed a configured threshold.
type ResourceSpike struct {
	Kind      string    `json:"kind"` // "cpu" or "memory"
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	At        time.Time `json:"at"`
}

// ResourceSummary aggregates samples collected during a build or test run.
type ResourceSummary struct {
	Samples        int             `json:"samples"`
	PeakCPUPercent float64         `json:"peak_cpu_percent"`
	PeakMemoryMB   float64         `json:"peak_memory_mb"`
	Spikes         []ResourceSpike `json:"spikes,omitempty"`
}

// SpikeThresholds configures spike detection. Zero disables a dimension.
type SpikeThresholds struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
}

// DockerStatsSampler reads resource usage via `docker stats --no-stream`.
type DockerStatsSampler struct {
	run CommandRunner
}

// NewDockerStatsSampler creates a sampler; runner defaults to ExecRunner.
func NewDockerStatsSampler(runner CommandRunner) *DockerStatsSampler {
	if runner == nil {
		runner = ExecRunner
	}
	return &DockerStatsSampler{run: runner}
}

// Sample returns the current usage of one container.
func (s *DockerStatsSampler) Sample(ctx context.Context, containerID string) (ContainerSample, error) {
	out, err := s.run(ctx, "docker", "stats", "--no-stream", "--format", "{{json .}}", containerID)
	if err != nil {
		return ContainerSample{}, fmt.Errorf("docker stats %s: %w", containerID, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		sample, err := ParseDockerStatsLine(line)
		if err != nil {
			return ContainerSample{}, err
		}
		sample.SampledAt = time.Now()
		return sample, nil
	}
	return ContainerSample{}, fmt.Errorf("docker stats %s: empty output", containerID)
}

type dockerStatsJSON struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
}

// ParseDockerStatsLine parses one `docker stats --format '{{json .}}'` line.
func ParseDockerStatsLine(line string) (ContainerSample, error) {
	var raw dockerStatsJSON
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return ContainerSample{}, fmt.Errorf("invalid docker stats line: %w", err)
	}
	cpu, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(raw.CPUPerc), "%"), 64)
	if err != nil {
		return ContainerSample{}, fmt.Errorf("invalid CPUPerc %q: %w", raw.CPUPerc, err)
	}
	mem, err := parseMemUsageMB(raw.MemUsage)
	if err != nil {
		return ContainerSample{}, err
	}
	id := raw.ID
	if id == "" {
		id = raw.Name
	}
	return ContainerSample{ContainerID: id, CPUPercent: cpu, MemoryMB: mem}, nil
}

var memUnits = []struct {
	suffix string
	bytes  float64
}{
	{"KiB", 1024}, {"MiB", 1024 * 1024}, {"GiB", 1024 * 1024 * 1024}, {"TiB", 1024 * 1024 * 1024 * 1024},
	{"kB", 1e3}, {"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12},
	{"B", 1},
}

// parseMemUsageMB converts the used side of "512MiB / 2GiB" to MiB.
func parseMemUsageMB(s string) (float64, error) {
	used := strings.TrimSpace(strings.SplitN(s, "/", 2)[0])
	for _, u := range memUnits {
		if strings.HasSuffix(used, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(used, u.suffix)), 64)
			if err != nil {
				return 0, fmt.Errorf("invalid MemUsage %q: %w", s, err)
			}
			return v * u.bytes / (1024 * 1024), nil
		}
	}
	return 0, fmt.Errorf("invalid MemUsage %q: unknown unit", s)
}

// Sampler abstracts container resource sampling.
type Sampler interface {
	Sample(ctx context.Context, containerID string) (ContainerSample, error)
}

// ContainerMonitor samples a container periodically, tracking peaks and spikes.
type ContainerMonitor struct {
	mu         sync.Mutex
	sampler    Sampler
	thresholds SpikeThresholds
	summary    ResourceSummary
	inCPUSpike bool
	inMemSpike bool
}

// NewContainerMonitor creates a monitor.
func NewContainerMonitor(sampler Sampler, thresholds SpikeThresholds) *ContainerMonitor {
	return &ContainerMonitor{sampler: sampler, thresholds: thresholds}
}

// SampleOnce takes one sample and folds it into the summary.
func (m *ContainerMonitor) SampleOnce(ctx context.Context, containerID string) error {
	s, err := m.sampler.Sample(ctx, containerID)
	if err != nil {
		return err
	}
	if s.SampledAt.IsZero() {
		s.SampledAt = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.summary.Samples++
	if s.CPUPercent > m.summary.PeakCPUPercent {
		m.summary.PeakCPUPercent = s.CPUPercent
	}
	if s.MemoryMB > m.summary.PeakMemoryMB {
		m.summary.PeakMemoryMB = s.MemoryMB
	}
	// A spike is recorded once when a threshold is first crossed, not per sample above it.
	cpuOver := m.thresholds.CPUPercent > 0 && s.CPUPercent >= m.thresholds.CPUPercent
	if cpuOver && !m.inCPUSpike {
		m.summary.Spikes = append(m.summary.Spikes, ResourceSpike{Kind: "cpu", Value: s.CPUPercent, Threshold: m.thresholds.CPUPercent, At: s.SampledAt})
	}
	m.inCPUSpike = cpuOver
	memOver := m.thresholds.MemoryMB > 0 && s.MemoryMB >= m.thresholds.MemoryMB
	if memOver && !m.inMemSpike {
		m.summary.Spikes = append(m.summary.Spikes, ResourceSpike{Kind: "memory", Value: s.MemoryMB, Threshold: m.thresholds.MemoryMB, At: s.SampledAt})
	}
	m.inMemSpike = memOver
	return nil
}

// Run samples every interval until ctx is cancelled. Sampling errors are skipped
// (the container may not be up yet or may have exited).
func (m *ContainerMonitor) Run(ctx context.Context, containerID string, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	_ = m.SampleOnce(ctx, containerID)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = m.SampleOnce(ctx, containerID)
		}
	}
}

// Summary returns a copy of the aggregated readings.
func (m *ContainerMonitor) Summary() ResourceSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.summary
	out.Spikes = append([]ResourceSpike(nil), m.summary.Spikes...)
	return out
}
