package toolchain

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Status is what this machine has of a tool.
type Status struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	// Problem explains a tool that is present but unusable, e.g. a stub that fails to run.
	Problem   string    `json:"problem,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// host is everything a check touches on the machine, so tests can fake it.
type host struct {
	goos     string
	lookPath func(string) (string, error)
	run      func(ctx context.Context, name string, args ...string) (string, error)
	getenv   func(string) string
	home     string
	isDir    func(string) bool
}

func realHost() host {
	home, _ := os.UserHomeDir()
	return host{
		goos:     runtime.GOOS,
		lookPath: exec.LookPath,
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out // java prints its version on stderr
			err := cmd.Run()
			return out.String(), err
		},
		getenv: os.Getenv,
		home:   home,
		isDir: func(p string) bool {
			st, err := os.Stat(p)
			return err == nil && st.IsDir()
		},
	}
}

// Checker checks tools and caches the answers briefly: briefs and stage gates ask often, while
// a tool only appears when someone installs it (which invalidates the cache).
type Checker struct {
	host host
	ttl  time.Duration
	mu   sync.Mutex
	seen map[string]Status
	gen  int // bumped by Invalidate: a check started before it is not cached
}

const statusTTL = time.Minute

// NewChecker checks tools on this machine.
func NewChecker() *Checker {
	return &Checker{host: realHost(), ttl: statusTTL, seen: map[string]Status{}}
}

// Invalidate forgets every cached answer, e.g. after an install or when the operator re-checks.
func (c *Checker) Invalidate() {
	c.mu.Lock()
	c.seen = map[string]Status{}
	c.gen++
	c.mu.Unlock()
}

// Check reports one tool, from the cache when it is fresh.
func (c *Checker) Check(t Tool) Status {
	c.mu.Lock()
	if s, ok := c.seen[t.ID]; ok && time.Since(s.CheckedAt) < c.ttl {
		c.mu.Unlock()
		return s
	}
	gen := c.gen
	c.mu.Unlock()
	s := c.check(t)
	c.mu.Lock()
	if gen == c.gen {
		c.seen[t.ID] = s
	}
	c.mu.Unlock()
	return s
}

// CheckAll checks tools in parallel; version commands of JVM and Flutter tools take seconds.
func (c *Checker) CheckAll(tools []Tool) map[string]Status {
	out := make(map[string]Status, len(tools))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, t := range tools {
		wg.Add(1)
		go func(t Tool) {
			defer wg.Done()
			s := c.Check(t)
			mu.Lock()
			out[t.ID] = s
			mu.Unlock()
		}(t)
	}
	wg.Wait()
	return out
}

func (c *Checker) check(t Tool) Status {
	s := Status{CheckedAt: time.Now()}
	if t.Binary == "" {
		if dir := c.sdkDir(t); dir != "" {
			s.Installed, s.Path = true, dir
		}
		return s
	}
	p, err := c.host.lookPath(t.Binary)
	if err != nil {
		return s
	}
	s.Path = p
	timeout := t.Timeout
	if timeout == 0 {
		timeout = defaultCheckTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := c.host.run(ctx, p, t.VersionArgs...)
	line := firstLine(out)
	switch {
	case ctx.Err() != nil:
		s.Problem = fmt.Sprintf("`%s %s` did not answer within %s", t.Binary, strings.Join(t.VersionArgs, " "), timeout)
	case err != nil:
		s.Problem = fmt.Sprintf("found at %s, but `%s %s` failed", p, t.Binary, strings.Join(t.VersionArgs, " "))
		if line != "" {
			s.Problem += ": " + line
		}
	default:
		s.Installed, s.Version = true, line
	}
	return s
}

func (c *Checker) sdkDir(t Tool) string {
	for _, env := range t.SDKEnv {
		if v := c.host.getenv(env); v != "" && c.host.isDir(v) {
			return v
		}
	}
	for _, rel := range t.SDKDirs {
		if p := filepath.Join(c.host.home, rel); c.host.home != "" && c.host.isDir(p) {
			return p
		}
	}
	return ""
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if len(l) > 120 {
				l = l[:117] + "…"
			}
			return l
		}
	}
	return ""
}

// managerAvailable reports whether a package manager can run here.
func (c *Checker) managerAvailable(m Method) (bool, string) {
	if len(m.OS) > 0 && !contains(m.OS, c.host.goos) {
		return false, "not for " + c.host.goos
	}
	bin := map[string]string{"brew": "brew", "npm": "npm", "go": "go", "apt": "apt-get", "snap": "snap"}[m.Manager]
	if bin == "" {
		return false, "unknown package manager " + m.Manager
	}
	if _, err := c.host.lookPath(bin); err != nil {
		switch m.Manager {
		case "brew":
			return false, "Homebrew is not installed (https://brew.sh)"
		case "npm":
			return false, "needs Node.js (npm) first"
		case "go":
			return false, "needs Go first"
		}
		return false, bin + " is not available on this machine"
	}
	return true, ""
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// EnsurePath adds the folders package managers install into to this process's PATH, when they
// exist and are missing. A daemon started from a GUI or a service manager often lacks them, so a
// freshly installed tool would stay invisible to it and to the agents it starts.
func EnsurePath() {
	home, _ := os.UserHomeDir()
	gopath := os.Getenv("GOPATH")
	if gopath == "" && home != "" {
		gopath = filepath.Join(home, "go")
	}
	candidates := []string{"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin", "/snap/bin"}
	if gopath != "" {
		candidates = append(candidates, filepath.Join(strings.Split(gopath, string(os.PathListSeparator))[0], "bin"))
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".linuxbrew", "bin"), filepath.Join(home, ".bun", "bin"),
			filepath.Join(home, ".pub-cache", "bin"), filepath.Join(home, ".local", "bin"))
	}
	current := filepath.SplitList(os.Getenv("PATH"))
	have := map[string]bool{}
	for _, p := range current {
		have[filepath.Clean(p)] = true
	}
	changed := false
	for _, dir := range candidates {
		if st, err := os.Stat(dir); err == nil && st.IsDir() && !have[filepath.Clean(dir)] {
			current = append(current, dir) // after the operator's own entries: theirs win
			have[filepath.Clean(dir)] = true
			changed = true
		}
	}
	if changed {
		_ = os.Setenv("PATH", strings.Join(current, string(os.PathListSeparator)))
	}
}
