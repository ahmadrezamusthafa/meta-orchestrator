// Package shadow replays completed production tasks in isolated, low-priority sandboxes
// across alternative models and methods to find cheaper or better routing.
package shadow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// RepoSnapshot pins a repository to the commit the original ticket was taken at.
type RepoSnapshot struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	CommitSHA string `json:"commit_sha"`
}

// Assertion is one ATDD acceptance artifact captured from the original task.
type Assertion struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ReplaySpec is an immutable copy of everything needed to re-run a ticket.
type ReplaySpec struct {
	SourceTaskID   string              `json:"source_task_id"`
	Title          string              `json:"title"`
	Description    string              `json:"description"`
	WorkflowID     string              `json:"workflow_id"`
	StageID        string              `json:"stage_id"`
	Complexity     string              `json:"complexity"`
	Category       string              `json:"category"`
	Repos          []RepoSnapshot      `json:"repos"`
	ATDDAssertions []Assertion         `json:"atdd_assertions"`
	Baseline       telemetry.RunRecord `json:"baseline"` // the production run being challenged
}

// RevResolver returns the current HEAD commit of a repository path.
type RevResolver func(repoPath string) (string, error)

// GitHeadResolver resolves HEAD with `git rev-parse`.
func GitHeadResolver(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", repoPath, err)
	}
	return strings.TrimSpace(string(out)), nil
}

var assertionFile = regexp.MustCompile(`(?i)(atdd|acceptance|\.feature$|_test\.|\.spec\.|\.test\.)`)

const maxAssertionBytes = 256 * 1024

// CaptureReplay snapshots a completed task. Intake commits come from task metadata
// "intake_commit_<repo>" when present, otherwise from the repository's current HEAD.
func CaptureReplay(task *types.Task, baseline telemetry.RunRecord, repoPaths map[string]string, rev RevResolver) (ReplaySpec, error) {
	if task == nil {
		return ReplaySpec{}, fmt.Errorf("task is nil")
	}
	if rev == nil {
		rev = GitHeadResolver
	}
	meta := task.Metadata
	spec := ReplaySpec{
		SourceTaskID: task.ID,
		Title:        task.Title,
		Description:  task.Description,
		WorkflowID:   task.WorkflowID,
		StageID:      task.CurrentStageID,
		Complexity:   strings.ToUpper(meta["complexity"]),
		Category:     baseline.Category,
		Baseline:     baseline,
	}
	if spec.Complexity == "" {
		spec.Complexity = strings.ToUpper(baseline.Complexity)
	}
	if spec.Category == "" {
		spec.Category = meta["task_type"]
	}

	for _, name := range task.AssignedRepos {
		snap := RepoSnapshot{Name: name, Path: repoPaths[name], CommitSHA: meta["intake_commit_"+name]}
		if snap.CommitSHA == "" && snap.Path != "" {
			sha, err := rev(snap.Path)
			if err != nil {
				return ReplaySpec{}, err
			}
			snap.CommitSHA = sha
		}
		spec.Repos = append(spec.Repos, snap)
	}

	if task.ArtifactDir != "" {
		_ = filepath.WalkDir(task.ArtifactDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !assertionFile.MatchString(d.Name()) {
				return nil
			}
			info, err := d.Info()
			if err != nil || info.Size() > maxAssertionBytes {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			spec.ATDDAssertions = append(spec.ATDDAssertions, Assertion{Path: path, Content: string(data)})
			return nil
		})
		sort.Slice(spec.ATDDAssertions, func(i, j int) bool { return spec.ATDDAssertions[i].Path < spec.ATDDAssertions[j].Path })
	}
	return spec, nil
}

// SandboxConfig bounds shadow sandbox resources.
type SandboxConfig struct {
	Image    string  `json:"image"`
	CPUs     float64 `json:"cpus"`
	MemoryMB int     `json:"memory_mb"`
	WorkRoot string  `json:"work_root"` // host directory for per-job worktrees
}

// SandboxHandle identifies a provisioned sandbox.
type SandboxHandle struct {
	JobID       string            `json:"job_id"`
	ContainerID string            `json:"container_id"`
	Worktrees   map[string]string `json:"worktrees"` // repo name → worktree dir
	repoPaths   map[string]string
}

// Sandbox provisions isolated replay environments.
type Sandbox interface {
	Provision(ctx context.Context, jobID string, spec ReplaySpec) (SandboxHandle, error)
	Teardown(ctx context.Context, h SandboxHandle) error
}

// DockerSandbox runs replays in a detached container at the lowest CPU priority:
// --cpu-shares 2 (the minimum scheduler weight) plus `nice -n 19` inside the container,
// no network, and repositories mounted from detached git worktrees at the intake commit.
type DockerSandbox struct {
	cfg SandboxConfig
	run telemetry.CommandRunner
}

// NewDockerSandbox creates a sandbox provisioner.
func NewDockerSandbox(cfg SandboxConfig, runner telemetry.CommandRunner) *DockerSandbox {
	if runner == nil {
		runner = telemetry.ExecRunner
	}
	if cfg.Image == "" {
		cfg.Image = "alpine:3"
	}
	if cfg.CPUs <= 0 {
		cfg.CPUs = 1
	}
	if cfg.MemoryMB <= 0 {
		cfg.MemoryMB = 1024
	}
	if cfg.WorkRoot == "" {
		cfg.WorkRoot = filepath.Join(os.TempDir(), "mo-shadow")
	}
	return &DockerSandbox{cfg: cfg, run: runner}
}

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// Provision checks out each repo at its intake commit and starts the sandbox container.
func (s *DockerSandbox) Provision(ctx context.Context, jobID string, spec ReplaySpec) (SandboxHandle, error) {
	safeID := unsafeName.ReplaceAllString(jobID, "-")
	h := SandboxHandle{JobID: jobID, Worktrees: map[string]string{}, repoPaths: map[string]string{}}
	jobDir := filepath.Join(s.cfg.WorkRoot, safeID)

	args := []string{"run", "-d", "--rm",
		"--name", "mo-shadow-" + safeID,
		"--label", "mo.shadow=true",
		"--label", "mo.shadow.source=" + unsafeName.ReplaceAllString(spec.SourceTaskID, "-"),
		"--cpu-shares", "2",
		"--cpus", fmt.Sprintf("%.2f", s.cfg.CPUs),
		"--memory", fmt.Sprintf("%dm", s.cfg.MemoryMB),
		"--network", "none",
	}
	for _, repo := range spec.Repos {
		if repo.Path == "" || repo.CommitSHA == "" {
			continue
		}
		wt := filepath.Join(jobDir, unsafeName.ReplaceAllString(repo.Name, "-"))
		if _, err := s.run(ctx, "git", "-C", repo.Path, "worktree", "add", "--detach", wt, repo.CommitSHA); err != nil {
			_ = s.Teardown(ctx, h)
			return SandboxHandle{}, fmt.Errorf("checkout %s@%s: %w", repo.Name, repo.CommitSHA, err)
		}
		h.Worktrees[repo.Name] = wt
		h.repoPaths[repo.Name] = repo.Path
		args = append(args, "-v", wt+":/workspace/"+filepath.Base(wt))
	}
	args = append(args, "-w", "/workspace", s.cfg.Image, "nice", "-n", "19", "sleep", "infinity")

	out, err := s.run(ctx, "docker", args...)
	if err != nil {
		_ = s.Teardown(ctx, h)
		return SandboxHandle{}, fmt.Errorf("start shadow sandbox: %w", err)
	}
	h.ContainerID = strings.TrimSpace(string(out))
	return h, nil
}

// Teardown removes the container and worktrees. It is safe on a partial handle.
func (s *DockerSandbox) Teardown(ctx context.Context, h SandboxHandle) error {
	var firstErr error
	if h.ContainerID != "" {
		if _, err := s.run(ctx, "docker", "rm", "-f", h.ContainerID); err != nil {
			firstErr = err
		}
	}
	for name, wt := range h.Worktrees {
		if _, err := s.run(ctx, "git", "-C", h.repoPaths[name], "worktree", "remove", "--force", wt); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
