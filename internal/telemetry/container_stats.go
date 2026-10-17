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

// ResourceSummary aggregates samples collected during a build or test run. With several
// containers, each sample is their total CPU and memory at that moment.
type ResourceSummary struct {
	Samples        int             `json:"samples"`
	Containers     int             `json:"containers"` // distinct containers seen
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

// Sample returns the current usage of the given containers, one `docker stats` call for all.
func (s *DockerStatsSampler) Sample(ctx context.Context, containerIDs ...string) ([]ContainerSample, error) {
	if len(containerIDs) == 0 {
		return nil, nil
	}
	args := append([]string{"stats", "--no-stream", "--format", "{{json .}}"}, containerIDs...)
	out, err := s.run(ctx, "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("docker stats: %w", err)
	}
	var samples []ContainerSample
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		sample, err := ParseDockerStatsLine(line)
		if err != nil {
			return nil, err
		}
		sample.SampledAt = time.Now()
		samples = append(samples, sample)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("docker stats: empty output")
	}
	return samples, nil
}

// ContainerInfo is one running container as `docker ps` lists it.
type ContainerInfo struct {
	ID        string
	CreatedAt time.Time
	Labels    map[string]string
}

type dockerPSJSON struct {
	ID        string `json:"ID"`
	CreatedAt string `json:"CreatedAt"`
	Labels    string `json:"Labels"`
}

// dockerTimeLayout is how `docker ps` prints CreatedAt.
const dockerTimeLayout = "2006-01-02 15:04:05 -0700 MST"

// ListContainers returns the running containers.
func (s *DockerStatsSampler) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	out, err := s.run(ctx, "docker", "ps", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	var list []ContainerInfo
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var raw dockerPSJSON
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("invalid docker ps line: %w", err)
		}
		c := ContainerInfo{ID: raw.ID, Labels: map[string]string{}}
		c.CreatedAt, _ = time.Parse(dockerTimeLayout, raw.CreatedAt)
		for _, kv := range strings.Split(raw.Labels, ",") {
			if k, v, ok := strings.Cut(kv, "="); ok {
				c.Labels[strings.TrimSpace(k)] = v
			}
		}
		list = append(list, c)
	}
	return list, nil
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
	Sample(ctx context.Context, containerIDs ...string) ([]ContainerSample, error)
}

// ContainerLister returns the containers to sample now.
type ContainerLister func(ctx context.Context) ([]string, error)

// ContainerMonitor samples a container periodically, tracking peaks and spikes.
type ContainerMonitor struct {
	mu         sync.Mutex
	sampler    Sampler
	thresholds SpikeThresholds
	summary    ResourceSummary
	seen       map[string]bool
	inCPUSpike bool
	inMemSpike bool
}

// NewContainerMonitor creates a monitor.
func NewContainerMonitor(sampler Sampler, thresholds SpikeThresholds) *ContainerMonitor {
	return &ContainerMonitor{sampler: sampler, thresholds: thresholds, seen: map[string]bool{}}
}

// SampleOnce samples the containers together and folds their total into the summary. No
// containers is not a sample.
func (m *ContainerMonitor) SampleOnce(ctx context.Context, containerIDs ...string) error {
	if len(containerIDs) == 0 {
		return nil
	}
	samples, err := m.sampler.Sample(ctx, containerIDs...)
	if err != nil {
		return err
	}
	s := ContainerSample{SampledAt: time.Now()}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range samples {
		s.CPUPercent += c.CPUPercent
		s.MemoryMB += c.MemoryMB
		m.seen[c.ContainerID] = true
	}
	m.summary.Containers = len(m.seen)
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

// Run samples the listed containers every interval until ctx is cancelled. Listing and sampling
// errors are skipped (a container may not be up yet or may have exited).
func (m *ContainerMonitor) Run(ctx context.Context, list ContainerLister, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	tick := func() {
		if ids, err := list(ctx); err == nil {
			_ = m.SampleOnce(ctx, ids...)
		}
	}
	tick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
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
