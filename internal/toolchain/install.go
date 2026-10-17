package toolchain

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Job states.
const (
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
)

// Job is one install the operator started.
type Job struct {
	ID       string    `json:"id"`
	Tool     string    `json:"tool"`
	Command  string    `json:"command"`
	State    string    `json:"state"`
	Log      []string  `json:"log"`
	Message  string    `json:"message,omitempty"` // the outcome in one sentence
	Status   *Status   `json:"status,omitempty"`  // the tool after the install
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitempty"`
}

const (
	maxLogLines    = 400
	installTimeout = 20 * time.Minute
)

// Errors returned by Start.
var (
	ErrUnknownTool = errors.New("unknown tool")
	ErrNotAuto     = errors.New("this install method needs your terminal (sudo, a password or a GUI); run the command shown instead")
	ErrBusy        = errors.New("this tool is already being installed")
)

// Installer runs install methods in the background, one at a time per package manager (Homebrew
// and npm lock their own state), and verifies the tool afterwards.
type Installer struct {
	checker  *Checker
	mu       sync.Mutex
	jobs     map[string]*Job
	latest   map[string]string // tool id → its latest job id
	seq      int
	managers map[string]*sync.Mutex
	// exec starts the command; tests replace it.
	exec func(ctx context.Context, argv []string) (io.ReadCloser, func() error, error)
}

// NewInstaller installs with checker's view of this machine.
func NewInstaller(checker *Checker) *Installer {
	return &Installer{checker: checker, jobs: map[string]*Job{}, latest: map[string]string{}, managers: map[string]*sync.Mutex{},
		exec: startCommand}
}

func startCommand(ctx context.Context, argv []string) (io.ReadCloser, func() error, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	// No terminal and no input: anything that would prompt fails fast instead of hanging.
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(), "NONINTERACTIVE=1", "HOMEBREW_NO_ENV_HINTS=1")
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return nil, nil, err
	}
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		pw.Close() // the reader sees EOF once the command and its output are done
		done <- err
	}()
	return pr, func() error { return <-done }, nil
}

// Start installs tool with its method at index, unless that tool is already installing.
func (in *Installer) Start(toolID string, method int) (*Job, error) {
	t, ok := Lookup(toolID)
	if !ok {
		return nil, ErrUnknownTool
	}
	if method < 0 || method >= len(t.Methods) {
		return nil, fmt.Errorf("tool %s has no install method %d", toolID, method)
	}
	m := t.Methods[method]
	if !m.Auto {
		return nil, ErrNotAuto
	}
	if ok, why := in.checker.managerAvailable(m); !ok {
		return nil, fmt.Errorf("cannot use %s here: %s", m.Manager, why)
	}

	in.mu.Lock()
	if id := in.latest[toolID]; id != "" && in.jobs[id].State == JobRunning {
		in.mu.Unlock()
		return nil, ErrBusy
	}
	in.seq++
	job := &Job{ID: fmt.Sprintf("install-%d", in.seq), Tool: toolID, Command: strings.Join(m.Command, " "), State: JobRunning, Started: time.Now()}
	in.jobs[job.ID] = job
	in.latest[toolID] = job.ID
	lock := in.managers[m.Manager]
	if lock == nil {
		lock = &sync.Mutex{}
		in.managers[m.Manager] = lock
	}
	in.mu.Unlock()

	go in.run(job, t, m, lock)
	return in.Job(job.ID), nil
}

func (in *Installer) run(job *Job, t Tool, m Method, lock *sync.Mutex) {
	if !lock.TryLock() {
		in.appendLog(job, "Waiting for another "+m.Manager+" install to finish…")
		lock.Lock()
	}
	defer lock.Unlock()
	in.appendLog(job, "$ "+job.Command)

	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	out, wait, err := in.exec(ctx, m.Command)
	if err != nil {
		in.finish(job, t, fmt.Errorf("could not start: %w", err))
		return
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		in.appendLog(job, sc.Text())
	}
	_, _ = io.Copy(io.Discard, out) // drain an over-long line so the command never blocks
	err = wait()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("timed out after %s", installTimeout)
	}
	in.finish(job, t, err)
}

func (in *Installer) finish(job *Job, t Tool, runErr error) {
	EnsurePath()
	in.checker.Invalidate()
	st := in.checker.Check(t)

	in.mu.Lock()
	defer in.mu.Unlock()
	job.Finished = time.Now()
	job.Status = &st
	switch {
	case runErr != nil:
		job.State = JobFailed
		job.Message = fmt.Sprintf("The install failed (%v). Check the log, or install %s another way.", runErr, t.Name)
	case !st.Installed:
		job.State = JobFailed
		job.Message = fmt.Sprintf("The command finished, but %s still is not usable", t.Name)
		if st.Problem != "" {
			job.Message += ": " + st.Problem
		} else {
			job.Message += " — it is not on the orchestrator's PATH. Open a new terminal, restart the orchestrator from it and re-check."
		}
	default:
		job.State = JobSucceeded
		job.Message = fmt.Sprintf("%s is installed (%s).", t.Name, st.Version)
		if st.Version == "" {
			job.Message = t.Name + " is installed."
		}
	}
}

func (in *Installer) appendLog(job *Job, line string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	job.Log = append(job.Log, line)
	if over := len(job.Log) - maxLogLines; over > 0 {
		job.Log = append([]string{"…"}, job.Log[over+1:]...)
	}
}

// Job returns a copy of the job with id, or nil.
func (in *Installer) Job(id string) *Job {
	in.mu.Lock()
	defer in.mu.Unlock()
	j := in.jobs[id]
	if j == nil {
		return nil
	}
	cp := *j
	cp.Log = append([]string(nil), j.Log...)
	return &cp
}

// Latest returns a copy of the tool's most recent job, or nil.
func (in *Installer) Latest(toolID string) *Job {
	in.mu.Lock()
	id := in.latest[toolID]
	in.mu.Unlock()
	if id == "" {
		return nil
	}
	return in.Job(id)
}
