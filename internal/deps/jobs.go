package deps

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultTimeout bounds one install.
	DefaultTimeout = 20 * time.Minute
	maxLogLines    = 1000
	maxKeptJobs    = 100
	// updateEvery throttles progress updates while a job is logging.
	updateEvery = 500 * time.Millisecond
)

// Service runs installs, one per repository at a time, and keeps their jobs.
type Service struct {
	// Timeout bounds one install; zero uses DefaultTimeout.
	Timeout time.Duration
	// OnUpdate is called with a snapshot on every state change and at most every 500ms while
	// the job is logging. It must not block for long.
	OnUpdate func(Job)

	mu    sync.Mutex
	jobs  map[string]*Job
	order []string          // job ids, oldest first
	busy  map[string]string // key → running job id
	last  map[string]string // key → most recent job id
	wg    sync.WaitGroup
}

// NewService returns an empty service.
func NewService() *Service {
	return &Service{jobs: map[string]*Job{}, busy: map[string]string{}, last: map[string]string{}}
}

// ErrBusy is returned by Start when the key already has a job running; the job is returned too.
var ErrBusy = errors.New("an install is already running for this repository")

// Start runs the ecosystem's install command in dir (absolute) as a new job under key (e.g.
// "repo:web"). Only one job per key runs at a time.
func (s *Service) Start(key, dir string, e Ecosystem) (Job, error) {
	if !e.Runnable() {
		return Job{}, fmt.Errorf("%s cannot be run automatically", strings.Join(e.Install.Command, " "))
	}
	s.mu.Lock()
	if id, ok := s.busy[key]; ok {
		j := s.jobs[id].clone()
		s.mu.Unlock()
		return j, ErrBusy
	}
	j := &Job{ID: newJobID(), Package: key, Action: "install", State: StateQueued, Log: []string{}, Started: time.Now()}
	s.jobs[j.ID] = j
	s.order = append(s.order, j.ID)
	s.busy[key] = j.ID
	s.last[key] = j.ID
	s.evictLocked()
	snap := j.clone()
	s.mu.Unlock()
	s.notify(snap)

	s.wg.Add(1)
	go s.run(j, key, dir, e)
	return snap, nil
}

// Get returns a job by id.
func (s *Service) Get(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return Job{}, false
	}
	return j.clone(), true
}

// Last returns the most recent job for key, if any.
func (s *Service) Last(key string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[s.last[key]]
	if !ok {
		return Job{}, false
	}
	return j.clone(), true
}

// Wait blocks until every started job has finished (tests and shutdown).
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) evictLocked() {
	for len(s.order) > maxKeptJobs {
		evicted := false
		for i, id := range s.order {
			if s.jobs[id].Done() {
				delete(s.jobs, id)
				s.order = append(s.order[:i], s.order[i+1:]...)
				evicted = true
				break
			}
		}
		if !evicted {
			return
		}
	}
}

func (s *Service) notify(j Job) {
	if s.OnUpdate != nil {
		s.OnUpdate(j)
	}
}

// update applies fn to the job under the lock and returns a snapshot.
func (s *Service) update(j *Job, fn func(*Job)) Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(j)
	return j.clone()
}

func (s *Service) run(j *Job, key, dir string, e Ecosystem) {
	defer s.wg.Done()
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	command := strings.Join(e.Install.Command, " ")
	s.notify(s.update(j, func(j *Job) {
		j.State = StateRunning
		j.Log = append(j.Log, "$ "+command)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.bin, e.Install.Command[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CI=true")
	ownProcessGroup(cmd)
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = 5 * time.Second
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw

	// Lines are applied as they come; watchers get a snapshot at most every updateEvery.
	var dirty atomic.Bool
	readDone := make(chan struct{})
	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		tick := time.NewTicker(updateEvery)
		defer tick.Stop()
		for {
			select {
			case <-readDone:
				return
			case <-tick.C:
				if dirty.Swap(false) {
					s.notify(s.update(j, func(*Job) {}))
				}
			}
		}
	}()
	go func() {
		defer close(readDone)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			line := Redact(strings.TrimRight(sc.Text(), "\r"))
			s.update(j, func(j *Job) {
				j.Log = append(j.Log, line)
				if len(j.Log) > maxLogLines {
					j.Log = append([]string{"… earlier output trimmed"}, j.Log[len(j.Log)-maxLogLines+1:]...)
				}
			})
			dirty.Store(true)
		}
		_, _ = io.Copy(io.Discard, pr)
	}()

	started := time.Now()
	err := cmd.Start()
	if err == nil {
		err = cmd.Wait()
	}
	_ = pw.Close()
	<-readDone
	<-flushDone

	state, msg := StateSucceeded, fmt.Sprintf("%s finished in %s.", command, time.Since(started).Round(time.Second))
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		state, msg = StateFailed, fmt.Sprintf("%s timed out after %s and was stopped.", command, timeout)
	case err != nil:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			msg = fmt.Sprintf("%s exited with code %d.", command, exit.ExitCode())
		} else {
			msg = fmt.Sprintf("%s could not run: %s", command, Redact(err.Error()))
		}
		state = StateFailed
	}
	snap := s.update(j, func(j *Job) {
		j.State, j.Message, j.Finished = state, msg, time.Now()
	})
	s.mu.Lock()
	if s.busy[key] == j.ID {
		delete(s.busy, key)
	}
	s.mu.Unlock()
	s.notify(snap)
}

func newJobID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "dep-" + hex.EncodeToString(b)
}

// secretPatterns hide credentials an install log may echo: URL user-info, auth tokens and
// well-known token formats.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(://)[^/\s:@]+:[^/\s@]+@`),
	regexp.MustCompile(`(?i)(_authToken|_auth|_password|token|password|secret|apikey|api_key)(["']?\s*[=:]\s*["']?)[^\s"']+`),
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr|github_pat|npm|glpat|xox[abpr])_[A-Za-z0-9_]{10,}`),
}

// Redact masks credentials in one line of output.
func Redact(s string) string {
	s = secretPatterns[0].ReplaceAllString(s, "${1}***@")
	s = secretPatterns[1].ReplaceAllString(s, "${1}${2}***")
	s = secretPatterns[2].ReplaceAllString(s, "${1}***")
	return secretPatterns[3].ReplaceAllString(s, "***")
}
