package packages

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
)

// Job states and actions.
const (
	JobQueued    = "queued"
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"

	ActionInstall   = "install"
	ActionUpdate    = "update"
	ActionRollback  = "rollback"
	ActionUninstall = "uninstall"
	ActionSetup     = "setup"
)

const (
	defaultJobTimeout = 15 * time.Minute
	queryTimeout      = 60 * time.Second // npm view, go list, brew info, claude plugin list
	checkTimeout      = 20 * time.Second // version commands
	latestTTL         = time.Hour
	pluginListTTL     = time.Minute
	maxLogLines       = 500
	maxJobs           = 200
	emitInterval      = 500 * time.Millisecond
)

// Errors returned by the actions; the API maps them to status codes.
var (
	ErrUnknownPackage       = errors.New("unknown package")
	ErrBusy                 = errors.New("a job for this package is already queued or running")
	ErrNothingToRollback    = errors.New("there is no earlier version or snapshot to roll back to")
	ErrRollbackUnsupported  = errors.New("rollback is not supported for this package")
	ErrVersionNotPinnable   = errors.New("this package cannot be installed at a chosen version; it installs the current release")
	ErrNotInstalled         = errors.New("this package is not installed")
	ErrNothingManaged       = errors.New("the orchestrator manages no install of this package; a system copy is left to its own package manager")
	errVersionNotDetectable = errors.New("could not read the installed version")
)

// Manual tells the operator how to do what the orchestrator cannot do unattended.
type Manual struct {
	Commands []string `json:"commands"`
	Note     string   `json:"note"`
}

// ManualError means the action needs the operator's terminal; Manual says how.
type ManualError struct {
	Reason string
	Manual Manual
}

func (e *ManualError) Error() string { return e.Reason }

// Job is one action on one package.
type Job struct {
	ID          string   `json:"id"`
	Package     string   `json:"package"`
	Action      string   `json:"action"`
	Version     string   `json:"version"`
	FromVersion string   `json:"from_version"`
	State       string   `json:"state"`
	Log         []string `json:"log"`
	Message     string   `json:"message"`
	Started     string   `json:"started"`
	Finished    string   `json:"finished"`
}

// Options configure a Manager.
type Options struct {
	// Home holds managed installs, shims, snapshots and state; default DefaultHome().
	Home string
	// UserHome is the operator's home folder, where npx installers and plugins write (~);
	// default os.UserHomeDir().
	UserHome string
	// ClaudeDir is Claude Code's configuration folder; default $CLAUDE_CONFIG_DIR or ~/.claude.
	ClaudeDir string
	// Toolchain installs system packages through its catalog; nil uses toolchain.New().
	Toolchain *toolchain.Toolchain
	// OnJob receives a copy of a job when its state changes and, while it logs, at most every
	// 500 ms.
	OnJob func(Job)
	// JobTimeout bounds one job; default 15 minutes.
	JobTimeout time.Duration
}

// DefaultHome is $MO_HOME, else ~/.meta-orchestrator.
func DefaultHome() string {
	if h := os.Getenv("MO_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".meta-orchestrator"
	}
	return filepath.Join(home, ".meta-orchestrator")
}

// Manager installs and tracks the manifest's packages.
type Manager struct {
	opts     Options
	manifest *Manifest
	store    *store
	tc       *toolchain.Toolchain

	mu        sync.Mutex
	jobs      map[string]*Job
	order     []string          // job ids, oldest first
	latestJob map[string]string // package id → most recent job id
	busy      map[string]string // package id → queued/running job id
	locks     map[string]chan struct{}
	seq       int
	dirty     map[string]bool // job ids with log lines not yet emitted

	latestMu sync.Mutex
	latest   map[string]latestEntry

	pluginMu   sync.Mutex
	pluginList *pluginListing

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type latestEntry struct {
	version  string
	at       time.Time
	fetching bool
}

// New loads state from opts.Home. A manifest error leaves a manager with no packages.
func New(manifest *Manifest, opts Options) (*Manager, error) {
	if opts.Home == "" {
		opts.Home = DefaultHome()
	}
	if abs, err := filepath.Abs(opts.Home); err == nil {
		opts.Home = abs
	}
	if opts.UserHome == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("home folder: %w", err)
		}
		opts.UserHome = h
	}
	if opts.ClaudeDir == "" {
		opts.ClaudeDir = os.Getenv("CLAUDE_CONFIG_DIR")
		if opts.ClaudeDir == "" {
			opts.ClaudeDir = filepath.Join(opts.UserHome, ".claude")
		}
	}
	if opts.JobTimeout == 0 {
		opts.JobTimeout = defaultJobTimeout
	}
	if manifest == nil {
		manifest = &Manifest{byID: map[string]*Spec{}}
	}
	tc := opts.Toolchain
	if tc == nil {
		tc = toolchain.New()
	}
	st, err := openStore(filepath.Join(opts.Home, "packages", "state.json"))
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{opts: opts, manifest: manifest, store: st, tc: tc,
		jobs: map[string]*Job{}, latestJob: map[string]string{}, busy: map[string]string{},
		locks: map[string]chan struct{}{}, dirty: map[string]bool{}, latest: map[string]latestEntry{},
		ctx: ctx, cancel: cancel}
	if err != nil {
		return m, fmt.Errorf("package state %s unreadable, starting empty: %w", st.path, err)
	}
	return m, nil
}

// Load reads the manifest at path and creates a manager.
func Load(path string, opts Options) (*Manager, error) {
	man, err := LoadManifest(path)
	if err != nil {
		m, _ := New(nil, opts)
		return m, err
	}
	return New(man, opts)
}

// Close cancels running jobs (killing their process groups) and waits for background work.
func (m *Manager) Close() {
	m.cancel()
	m.wg.Wait()
}

// Home is the managed folder; BinDir holds the shims.
func (m *Manager) Home() string   { return m.opts.Home }
func (m *Manager) BinDir() string { return filepath.Join(m.opts.Home, "bin") }

// Manifest returns the loaded manifest.
func (m *Manager) Manifest() *Manifest { return m.manifest }

// Count is the number of manifest packages.
func (m *Manager) Count() int { return len(m.manifest.Packages) }

// EnsurePath creates <home>/bin and puts it first on this process's PATH, so the daemon and the
// agents it starts run the managed versions.
func EnsurePath(home string) string {
	bin := filepath.Join(home, "bin")
	_ = os.MkdirAll(bin, 0o755)
	parts := filepath.SplitList(os.Getenv("PATH"))
	out := []string{bin}
	for _, p := range parts {
		if p != "" && filepath.Clean(p) != filepath.Clean(bin) {
			out = append(out, p)
		}
	}
	_ = os.Setenv("PATH", strings.Join(out, string(os.PathListSeparator)))
	return bin
}

// OnPath reports whether the shim folder is on this process's PATH.
func (m *Manager) OnPath() bool {
	bin := filepath.Clean(m.BinDir())
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(p) == bin {
			return true
		}
	}
	return false
}

func (m *Manager) packageDir(id string) string {
	return filepath.Join(m.opts.Home, "packages", id)
}

func (m *Manager) snapshotDir(id string) string {
	return filepath.Join(m.packageDir(id), "snapshots")
}

// ---- jobs ----

// Job returns a copy of the job with id, or nil.
func (m *Manager) Job(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.copyJobLocked(m.jobs[id])
}

// LatestJob returns a copy of the package's most recent job, or nil.
func (m *Manager) LatestJob(pkg string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.copyJobLocked(m.jobs[m.latestJob[pkg]])
}

func (m *Manager) copyJobLocked(j *Job) *Job {
	if j == nil {
		return nil
	}
	cp := *j
	cp.Log = append([]string{}, j.Log...)
	return &cp
}

// newJobLocked registers a queued job and marks its package busy. Caller holds m.mu and has
// checked that the package is not busy.
func (m *Manager) newJobLocked(pkg, action, version, from string) *Job {
	m.seq++
	j := &Job{ID: fmt.Sprintf("pkg-%d-%d", time.Now().Unix(), m.seq), Package: pkg, Action: action, Version: version,
		FromVersion: from, State: JobQueued, Log: []string{}, Started: stamp(time.Now())}
	m.jobs[j.ID] = j
	m.order = append(m.order, j.ID)
	m.latestJob[pkg] = j.ID
	m.busy[pkg] = j.ID
	for len(m.order) > maxJobs {
		old := m.order[0]
		m.order = m.order[1:]
		if m.jobs[old] != nil && m.jobs[old].State != JobQueued && m.jobs[old].State != JobRunning {
			delete(m.jobs, old)
		}
	}
	return j
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// SetOnJob replaces the job listener (see Options.OnJob).
func (m *Manager) SetOnJob(fn func(Job)) {
	m.mu.Lock()
	m.opts.OnJob = fn
	m.mu.Unlock()
}

func (m *Manager) onJob() func(Job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.opts.OnJob
}

func (m *Manager) emit(id string) {
	fn := m.onJob()
	if fn == nil {
		return
	}
	if j := m.Job(id); j != nil {
		fn(*j)
	}
}

func (m *Manager) logLine(id, line string) {
	m.mu.Lock()
	if j := m.jobs[id]; j != nil {
		j.Log = append(j.Log, line)
		if over := len(j.Log) - maxLogLines; over > 0 {
			j.Log = append([]string{"…"}, j.Log[over+1:]...)
		}
		m.dirty[id] = true
	}
	m.mu.Unlock()
}

func (m *Manager) setJob(id string, fn func(*Job)) {
	m.mu.Lock()
	if j := m.jobs[id]; j != nil {
		fn(j)
	}
	m.mu.Unlock()
	m.emit(id)
}

// lock waits for the package manager lock name (npm, go, brew, claude). It reports false when the
// manager is closing.
func (m *Manager) lock(ctx context.Context, name string, waiting func()) bool {
	m.mu.Lock()
	ch := m.locks[name]
	if ch == nil {
		ch = make(chan struct{}, 1)
		m.locks[name] = ch
	}
	m.mu.Unlock()
	select {
	case ch <- struct{}{}:
		return true
	default:
	}
	waiting()
	select {
	case ch <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (m *Manager) unlock(name string) {
	m.mu.Lock()
	ch := m.locks[name]
	m.mu.Unlock()
	<-ch
}

// jobRun is a job while it executes.
type jobRun struct {
	m       *Manager
	job     string
	ctx     context.Context
	spec    *Spec
	message string // the outcome, set by the action
}

func (r *jobRun) log(format string, args ...interface{}) {
	r.m.logLine(r.job, fmt.Sprintf(format, args...))
}

func (r *jobRun) setVersion(v string) {
	r.m.setJob(r.job, func(j *Job) { j.Version = v })
}

// run executes argv (never through a shell), streaming its output into the job log.
func (r *jobRun) run(argv []string, env []string, dir string) error {
	if len(argv) == 0 {
		return errors.New("empty command")
	}
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("%s is not on PATH", argv[0])
	}
	r.log("$ %s", strings.Join(argv, " "))
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	cmd := exec.CommandContext(r.ctx, bin, argv[1:]...)
	ownProcessGroup(cmd)
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = dir
	cmd.Stdin = nil // no terminal: anything that prompts fails instead of hanging
	cmd.Env = append(os.Environ(), "NONINTERACTIVE=1", "HOMEBREW_NO_ENV_HINTS=1", "npm_config_yes=true",
		"npm_config_fund=false", "npm_config_update_notifier=false")
	cmd.Env = append(cmd.Env, env...)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return err
	}
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		pw.Close()
		done <- err
	}()
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		r.log("%s", stripANSI(sc.Text()))
	}
	_, _ = io.Copy(io.Discard, pr)
	err = <-done
	if r.ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timed out after %s", r.m.opts.JobTimeout)
	}
	if r.ctx.Err() != nil {
		return errors.New("cancelled")
	}
	if err != nil {
		return fmt.Errorf("`%s` failed: %v", strings.Join(argv, " "), err)
	}
	return nil
}

// stripANSI drops terminal colour codes installers print.
func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// output runs a read-only query and returns its standard output.
func (m *Manager) output(ctx context.Context, argv []string, dir string) (string, error) {
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return "", fmt.Errorf("%s is not on PATH", argv[0])
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, argv[1:]...)
	ownProcessGroup(cmd)
	cmd.WaitDelay = 2 * time.Second
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "npm_config_update_notifier=false", "HOMEBREW_NO_AUTO_UPDATE=1")
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		msg := firstLine(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("`%s`: %s", strings.Join(argv, " "), msg)
	}
	return out.String(), nil
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(stripANSI(l)); l != "" {
			if len(l) > 200 {
				l = l[:197] + "…"
			}
			return l
		}
	}
	return ""
}

// lockName is the package manager an action of spec uses.
func (m *Manager) lockName(s *Spec) string {
	switch s.Kind {
	case KindNPMCLI, KindNPXInstaller:
		return "npm"
	case KindGoCLI:
		return "go"
	case KindClaudePlugin:
		return "claude"
	}
	return "brew"
}

type action struct {
	spec    *Spec
	name    string // ActionInstall …
	version string // requested version ("" = latest / default)
	from    string
	fn      func(*jobRun) error
}

// start validates nothing more: it registers the job and runs it in the background.
func (m *Manager) start(a action) (*Job, error) {
	m.mu.Lock()
	if id := m.busy[a.spec.ID]; id != "" {
		m.mu.Unlock()
		return m.Job(id), ErrBusy
	}
	j := m.newJobLocked(a.spec.ID, a.name, a.version, a.from)
	m.mu.Unlock()
	m.emit(j.ID)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.execute(j.ID, a)
	}()
	return m.Job(j.ID), nil
}

// execute runs one job to completion: manager lock, the action, a re-check, the history entry.
func (m *Manager) execute(jobID string, a action) {
	defer func() {
		m.mu.Lock()
		if m.busy[a.spec.ID] == jobID {
			delete(m.busy, a.spec.ID)
		}
		delete(m.dirty, jobID)
		m.mu.Unlock()
		m.emit(jobID)
	}()
	lockName := m.lockName(a.spec)
	if a.spec.Kind == KindSystem {
		if ln := m.systemLock(a.spec); ln != "" {
			lockName = ln
		}
	}
	if !m.lock(m.ctx, lockName, func() { m.logLine(jobID, "Waiting for another "+lockName+" job to finish…"); m.emit(jobID) }) {
		m.finishJob(jobID, a, errors.New("cancelled"), "")
		return
	}
	defer m.unlock(lockName)

	m.setJob(jobID, func(j *Job) { j.State = JobRunning })
	ctx, cancel := context.WithTimeout(m.ctx, m.opts.JobTimeout)
	defer cancel()
	stopEmit := m.emitWhileRunning(jobID)
	r := &jobRun{m: m, job: jobID, ctx: ctx, spec: a.spec}
	err := a.fn(r)
	stopEmit()
	m.finishJob(jobID, a, err, r.message)
}

// emitWhileRunning publishes a job whose log grew, at most every emitInterval.
func (m *Manager) emitWhileRunning(id string) func() {
	if m.onJob() == nil {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(emitInterval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				m.mu.Lock()
				d := m.dirty[id]
				m.dirty[id] = false
				m.mu.Unlock()
				if d {
					m.emit(id)
				}
			}
		}
	}()
	return func() { close(stop); <-done }
}

func (m *Manager) finishJob(jobID string, a action, runErr error, message string) {
	toolchain.EnsurePath()
	m.Invalidate()
	d := m.detect(a.spec)
	ok := runErr == nil
	if ok {
		switch a.name {
		case ActionUninstall:
			if a.spec.Kind != KindNPMCLI && a.spec.Kind != KindGoCLI && d.status == StatusInstalled {
				ok, runErr = false, fmt.Errorf("the command finished, but %s is still installed", a.spec.Name)
			}
		default:
			if d.status != StatusInstalled {
				why := d.problem
				if why == "" {
					why = "it is not found on the orchestrator's PATH"
				}
				ok, runErr = false, fmt.Errorf("the command finished, but %s is not usable: %s", a.spec.Name, why)
			}
		}
	}
	if ok && message == "" {
		switch a.name {
		case ActionUninstall:
			message = a.spec.Name + " is uninstalled."
		default:
			message = fmt.Sprintf("%s %s is installed.", a.spec.Name, displayVersion(d.version))
		}
	}
	if !ok {
		message = fmt.Sprintf("%s failed: %v", strings.ToUpper(a.name[:1])+a.name[1:], runErr)
	}
	to := d.version
	if a.name == ActionUninstall && ok {
		to = ""
	}
	j := m.Job(jobID)
	from := ""
	if j != nil {
		from = j.FromVersion
	}
	_ = m.store.update(a.spec.ID, func(p *PackageState) {
		p.History = append(p.History, HistoryEntry{At: time.Now().UTC(), Action: a.name, From: from, To: to, OK: ok, Message: message})
	})
	m.setJob(jobID, func(j *Job) {
		j.Finished = stamp(time.Now())
		j.Message = message
		if ok {
			j.State = JobSucceeded
			if j.Version == "" {
				j.Version = to
			}
		} else {
			j.State = JobFailed
		}
	})
}

func displayVersion(v string) string {
	if v == "" {
		return "(version unknown)"
	}
	return v
}

// Invalidate forgets cached version checks and the plugin listing.
func (m *Manager) Invalidate() {
	m.tc.Checker.Invalidate()
	m.pluginMu.Lock()
	m.pluginList = nil
	m.pluginMu.Unlock()
}

// ---- public actions ----

func (m *Manager) spec(id string) (*Spec, error) {
	if !ValidID(id) {
		return nil, ErrUnknownPackage
	}
	s := m.manifest.Get(id)
	if s == nil {
		return nil, ErrUnknownPackage
	}
	return s, nil
}

func (m *Manager) busyJob(id string) *Job {
	m.mu.Lock()
	jid := m.busy[id]
	m.mu.Unlock()
	if jid == "" {
		return nil
	}
	return m.Job(jid)
}

// Install installs version ("" = the latest release) of package id.
func (m *Manager) Install(id, version string) (*Job, error) {
	s, err := m.spec(id)
	if err != nil {
		return nil, err
	}
	if err := ValidateVersion(version); err != nil {
		return nil, err
	}
	if j := m.busyJob(id); j != nil {
		return j, ErrBusy
	}
	fn, err := m.planInstall(s, version, nil)
	if err != nil {
		return nil, err
	}
	return m.start(action{spec: s, name: ActionInstall, version: version, from: m.detect(s).version, fn: fn})
}

// Update installs the latest release.
func (m *Manager) Update(id string) (*Job, error) {
	s, err := m.spec(id)
	if err != nil {
		return nil, err
	}
	if j := m.busyJob(id); j != nil {
		return j, ErrBusy
	}
	fn, err := m.planUpdate(s)
	if err != nil {
		return nil, err
	}
	return m.start(action{spec: s, name: ActionUpdate, from: m.detect(s).version, fn: fn})
}

// Rollback returns to version, or with "" to the previous kept version or snapshot.
func (m *Manager) Rollback(id, version string) (*Job, error) {
	s, err := m.spec(id)
	if err != nil {
		return nil, err
	}
	if err := ValidateVersion(version); err != nil {
		return nil, err
	}
	if j := m.busyJob(id); j != nil {
		return j, ErrBusy
	}
	target, fn, err := m.planRollback(s, version)
	if err != nil {
		return nil, err
	}
	return m.start(action{spec: s, name: ActionRollback, version: target, from: m.detect(s).version, fn: fn})
}

// Uninstall removes the package (only what the orchestrator manages, for npm-cli and go-cli).
func (m *Manager) Uninstall(id string) (*Job, error) {
	s, err := m.spec(id)
	if err != nil {
		return nil, err
	}
	if j := m.busyJob(id); j != nil {
		return j, ErrBusy
	}
	fn, err := m.planUninstall(s)
	if err != nil {
		return nil, err
	}
	return m.start(action{spec: s, name: ActionUninstall, from: m.detect(s).version, fn: fn})
}

// Skipped is a package setup did not install.
type Skipped struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// SetupResult is what setup started.
type SetupResult struct {
	Jobs    []Job     `json:"jobs"`
	Skipped []Skipped `json:"skipped"`
}

// Setup installs every required package that is missing, dependencies first, one job per package
// run one after another; a failure stops the chain.
func (m *Manager) Setup() (SetupResult, error) {
	return m.SetupCategory("")
}

// SetupCategory installs every missing package of a category (its name, or its slug such as
// "launch-readiness"), dependencies first, one after another. Empty means the required packages.
// In the required chain a failure stops the rest; a category's packages are independent, so one
// failing does not stop the others (a package whose dependency failed fails on its own check).
func (m *Manager) SetupCategory(category string) (SetupResult, error) {
	res := SetupResult{Jobs: []Job{}, Skipped: []Skipped{}}
	var wanted []string
	for _, s := range m.manifest.Packages {
		if (category == "" && s.Required) || (category != "" && categoryMatches(s.Category, category)) {
			wanted = append(wanted, s.ID)
		}
	}
	if category != "" && len(wanted) == 0 {
		return res, fmt.Errorf("unknown package category %q: use one of %s", category, strings.Join(Categories, ", "))
	}
	stopOnFailure := category == ""
	ids, err := m.manifest.order(wanted)
	if err != nil {
		return res, err
	}
	inChain := map[string]bool{}
	var acts []action
	for _, id := range ids {
		s := m.manifest.Get(id)
		if j := m.busyJob(id); j != nil {
			res.Skipped = append(res.Skipped, Skipped{ID: id, Reason: "a job is already running (" + j.ID + ")"})
			continue
		}
		d := m.detect(s)
		if d.status == StatusInstalled {
			res.Skipped = append(res.Skipped, Skipped{ID: id, Reason: "already installed (" + displayVersion(d.version) + ")"})
			continue
		}
		fn, err := m.planInstall(s, "", inChain)
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{ID: id, Reason: err.Error()})
			continue
		}
		inChain[id] = true
		acts = append(acts, action{spec: s, name: ActionSetup, from: d.version, fn: fn})
	}
	var jobIDs []string
	var queued []action // acts[i] runs as jobIDs[i]
	m.mu.Lock()
	for _, a := range acts {
		if m.busy[a.spec.ID] != "" {
			continue
		}
		j := m.newJobLocked(a.spec.ID, a.name, "", a.from)
		jobIDs = append(jobIDs, j.ID)
		queued = append(queued, a)
		res.Jobs = append(res.Jobs, *m.copyJobLocked(j))
	}
	m.mu.Unlock()
	for _, id := range jobIDs {
		m.emit(id)
	}
	if len(jobIDs) == 0 {
		return res, nil
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		failed := ""
		for i, id := range jobIDs {
			a := queued[i]
			if failed != "" {
				m.abandon(id, a, fmt.Sprintf("Not run: setup stopped because %s failed.", failed))
				continue
			}
			m.execute(id, a)
			if j := m.Job(id); stopOnFailure && (j == nil || j.State != JobSucceeded) {
				failed = a.spec.Name
			}
		}
	}()
	return res, nil
}

// abandon fails a queued job without running it.
func (m *Manager) abandon(id string, a action, message string) {
	m.setJob(id, func(j *Job) {
		j.State, j.Message, j.Finished = JobFailed, message, stamp(time.Now())
	})
	m.mu.Lock()
	if m.busy[a.spec.ID] == id {
		delete(m.busy, a.spec.ID)
	}
	m.mu.Unlock()
	m.emit(id)
}

// ---- versions ----

// Versions lists the versions the package can be installed at, newest first (at most 50), and
// refreshes the cached latest version.
func (m *Manager) Versions(ctx context.Context, id string, prerelease bool) ([]string, string, error) {
	s, err := m.spec(id)
	if err != nil {
		return nil, "", err
	}
	all, err := m.fetchVersions(ctx, s)
	if err != nil {
		return []string{}, m.cachedLatest(s.ID), err
	}
	latest := ""
	out := []string{}
	for _, v := range all {
		if isPrerelease(v) {
			if !prerelease {
				continue
			}
		} else if latest == "" {
			latest = v
		}
		if len(out) < 50 {
			out = append(out, v)
		}
	}
	if latest != "" {
		m.latestMu.Lock()
		m.latest[s.ID] = latestEntry{version: latest, at: time.Now()}
		m.latestMu.Unlock()
	}
	return out, latest, nil
}

// fetchVersions asks the package's registry, newest first.
func (m *Manager) fetchVersions(ctx context.Context, s *Spec) ([]string, error) {
	switch s.Kind {
	case KindNPMCLI, KindNPXInstaller:
		out, err := m.output(ctx, []string{"npm", "view", s.Package, "versions", "--json"}, m.opts.Home)
		if err != nil {
			return nil, err
		}
		list, err := parseNPMVersions(out)
		if err != nil {
			return nil, err
		}
		sortVersionsDesc(list)
		return list, nil
	case KindGoCLI:
		out, err := m.output(ctx, []string{"go", "list", "-m", "-versions", s.Module}, m.opts.Home)
		if err != nil {
			return nil, err
		}
		fields := strings.Fields(out)
		var list []string
		for _, f := range fields[min(1, len(fields)):] {
			v := strings.TrimPrefix(f, "v")
			if ValidateVersion(v) == nil {
				list = append(list, v)
			}
		}
		sortVersionsDesc(list)
		return list, nil
	case KindSystem:
		if f := m.brewFormula(s); f != "" {
			if _, err := exec.LookPath("brew"); err == nil {
				out, err := m.output(ctx, []string{"brew", "info", "--json=v2", f}, m.opts.Home)
				if err != nil {
					return nil, err
				}
				if v := parseBrewStable(out); v != "" {
					return []string{v}, nil
				}
			}
		}
	}
	return []string{}, nil
}

// RefreshLatest looks up every package's latest version now, in parallel (the CLI's list waits
// for it; the API never does).
func (m *Manager) RefreshLatest(ctx context.Context) {
	var wg sync.WaitGroup
	for _, s := range m.manifest.Packages {
		if s.Kind == KindClaudePlugin {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, _, _ = m.Versions(ctx, id, false)
		}(s.ID)
	}
	wg.Wait()
}

func (m *Manager) cachedLatest(id string) string {
	m.latestMu.Lock()
	defer m.latestMu.Unlock()
	return m.latest[id].version
}

// latestVersion returns the cached latest version and refreshes it in the background when stale;
// it never waits for the network.
func (m *Manager) latestVersion(s *Spec) string {
	m.latestMu.Lock()
	e := m.latest[s.ID]
	stale := time.Since(e.at) > latestTTL
	if stale && !e.fetching && m.ctx.Err() == nil && s.Kind != KindClaudePlugin {
		e.fetching = true
		m.latest[s.ID] = e
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			_, _, err := m.Versions(m.ctx, s.ID, false)
			m.latestMu.Lock()
			cur := m.latest[s.ID]
			cur.fetching = false
			if err != nil {
				cur.at = time.Now() // retry after the TTL, not on every listing
			}
			m.latest[s.ID] = cur
			m.latestMu.Unlock()
		}()
	}
	m.latestMu.Unlock()
	return e.version
}

// resolveLatest returns the latest release for an install without a version.
func (m *Manager) resolveLatest(ctx context.Context, s *Spec) (string, error) {
	_, latest, err := m.Versions(ctx, s.ID, false)
	if err != nil {
		return "", fmt.Errorf("could not look up the latest version: %w", err)
	}
	if latest == "" {
		return "", errors.New("the registry lists no release")
	}
	return latest, nil
}

// ---- helpers ----

// lookPathExcept finds name on PATH, skipping the folder except.
func lookPathExcept(name, except string) string {
	except = filepath.Clean(except)
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || filepath.Clean(dir) == except {
			continue
		}
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

func sortVersionsDesc(list []string) {
	sort.SliceStable(list, func(i, j int) bool { return compareVersions(list[i], list[j]) > 0 })
}

// categoryMatches reports whether a category is named by q: its name in any case, or its slug.
func categoryMatches(category, q string) bool {
	q = strings.TrimSpace(q)
	return strings.EqualFold(category, q) || strings.EqualFold(strings.ReplaceAll(category, " ", "-"), q)
}
