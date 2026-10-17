package deps

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// npmName is a valid npm package name; anything else is never passed to npm.
var npmName = regexp.MustCompile(`^(@[a-z0-9][a-z0-9._~-]*/)?[a-z0-9][a-z0-9._~-]*$`)

const (
	latestTTL         = time.Hour
	latestConcurrency = 4
	latestTimeout     = 30 * time.Second
	latestMaxNames    = 200
)

// Latest looks up the newest published version of npm packages with `npm view <name> version`,
// at most four at a time, and remembers each answer for an hour. It only runs on request.
type Latest struct {
	mu    sync.Mutex
	cache map[string]latestEntry
	sem   chan struct{}
}

type latestEntry struct {
	version string
	at      time.Time
}

// NewLatest returns an empty checker.
func NewLatest() *Latest {
	return &Latest{cache: map[string]latestEntry{}, sem: make(chan struct{}, latestConcurrency)}
}

// Fill sets Latest on the node dependencies of ecos, in place. root is the repository the scan
// ran on; npm runs in each ecosystem's folder so the repository's .npmrc registry applies.
func (l *Latest) Fill(ctx context.Context, root string, ecos []Ecosystem, lookPath func(string) (string, error)) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	npm, err := lookPath("npm")
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	asked := 0
	for ei := range ecos {
		if ecos[ei].Ecosystem != "node" {
			continue
		}
		dir := abs(root, ecos[ei].Dir)
		for di := range ecos[ei].Dependencies {
			d := &ecos[ei].Dependencies[di]
			if !npmName.MatchString(d.Name) {
				continue
			}
			if v, ok := l.cached(d.Name); ok {
				d.Latest = v
				continue
			}
			if asked >= latestMaxNames {
				continue
			}
			asked++
			wg.Add(1)
			go func(d *Dependency) {
				defer wg.Done()
				select {
				case l.sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				v := npmView(ctx, npm, dir, d.Name)
				<-l.sem
				l.store(d.Name, v)
				mu.Lock()
				d.Latest = v
				mu.Unlock()
			}(d)
		}
	}
	wg.Wait()
}

func (l *Latest) cached(name string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.cache[name]
	if !ok || time.Since(e.at) > latestTTL {
		return "", false
	}
	return e.version, true
}

func (l *Latest) store(name, version string) {
	l.mu.Lock()
	l.cache[name] = latestEntry{version: version, at: time.Now()}
	l.mu.Unlock()
}

func npmView(ctx context.Context, npm, dir, name string) string {
	ctx, cancel := context.WithTimeout(ctx, latestTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, npm, "view", name, "version")
	if isDir(dir) {
		cmd.Dir = filepath.Clean(dir)
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Fields(strings.TrimSpace(string(out)))
	if len(lines) == 0 {
		return ""
	}
	v := lines[len(lines)-1]
	if len(v) > 64 {
		return ""
	}
	return v
}
