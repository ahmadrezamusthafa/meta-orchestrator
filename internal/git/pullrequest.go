package git

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// pushTimeout bounds a push, which talks to the remote and can be slower than local git calls.
const pushTimeout = 2 * time.Minute

// Remote identifies the hosted repository behind a git remote.
type Remote struct {
	Host  string `json:"host"`  // e.g. "bitbucket.org", "github.com"
	Owner string `json:"owner"` // workspace (Bitbucket) or owner/org (GitHub)
	Repo  string `json:"repo"`  // repository slug
}

// Provider names the pull-request API the remote speaks.
func (r Remote) Provider() string {
	switch {
	case strings.HasSuffix(r.Host, "bitbucket.org"):
		return "bitbucket"
	case r.Host == "github.com":
		return "github"
	case strings.Contains(r.Host, "gitlab"):
		return "gitlab"
	}
	return ""
}

// WebURL is the repository's browser address.
func (r Remote) WebURL() string {
	return fmt.Sprintf("https://%s/%s/%s", r.Host, r.Owner, r.Repo)
}

var scpRemote = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.+)$`)

// ParseRemote understands https, ssh:// and scp-style (git@host:owner/repo.git) remote URLs.
func ParseRemote(raw string) (Remote, error) {
	raw = strings.TrimSpace(raw)
	var host, path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return Remote{}, fmt.Errorf("unrecognized remote %q", raw)
		}
		host, path = u.Hostname(), u.Path
	} else if m := scpRemote.FindStringSubmatch(raw); m != nil {
		host, path = m[1], m[2]
	} else {
		return Remote{}, fmt.Errorf("unrecognized remote %q", raw)
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	i := strings.LastIndex(path, "/")
	if host == "" || i <= 0 || i == len(path)-1 {
		return Remote{}, fmt.Errorf("remote %q has no owner/repository path", raw)
	}
	return Remote{Host: strings.ToLower(host), Owner: path[:i], Repo: path[i+1:]}, nil
}

// OriginRemote resolves the "origin" remote of the repository at dir.
func OriginRemote(ctx context.Context, dir string) (Remote, error) {
	out, err := run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return Remote{}, fmt.Errorf("no origin remote: %w", err)
	}
	return ParseRemote(out)
}

// BranchName strips a remote prefix from a base ref ("origin/master" → "master").
func BranchName(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	ref = strings.TrimPrefix(ref, "refs/heads/")
	return strings.TrimPrefix(ref, "origin/")
}

// CommitAll stages and commits every change under pathspec (the whole tree when empty). It
// reports whether a commit was made; a clean tree is not an error.
func CommitAll(ctx context.Context, dir, pathspec, message string) (bool, error) {
	scope := []string{"--", "."}
	if pathspec != "" && pathspec != "." {
		scope = []string{"--", pathspec}
	}
	_ = ExcludeDependencyDirs(ctx, dir) // never sweep installed dependencies into the commit
	if _, err := run(ctx, dir, append([]string{"add", "-A"}, scope...)...); err != nil {
		return false, err
	}
	if _, err := run(ctx, dir, "diff", "--cached", "--quiet"); err == nil {
		return false, nil // nothing staged
	}
	if _, err := run(ctx, dir, "commit", "-q", "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

// Push publishes branch to origin and sets it as the upstream. The operator's own git
// credentials (SSH agent or credential helper) are used; nothing is passed on the command line.
func Push(ctx context.Context, dir, branch string) error {
	ctx, cancel := context.WithTimeout(ctx, pushTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "push", "-u", "origin", branch)
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0") // never hang on a password prompt
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git push: %s", msg)
	}
	return nil
}

// Unpushed counts the commits on HEAD that origin's copy of branch does not have yet, as of the
// last fetch or push. onRemote is false when the branch was never pushed (every commit is unpushed).
func Unpushed(ctx context.Context, dir, branch string) (n int, onRemote bool, err error) {
	remote := "refs/remotes/origin/" + branch
	if !refExists(ctx, dir, remote) {
		return 0, false, nil
	}
	out, err := run(ctx, dir, "rev-list", "--count", remote+"..HEAD")
	if err != nil {
		return 0, true, err
	}
	n, err = strconv.Atoi(strings.TrimSpace(out))
	return n, true, err
}

// CommitSubjects lists the subjects of commits on HEAD that are not on baseRef, oldest first.
func CommitSubjects(ctx context.Context, dir, baseRef, pathspec string) ([]string, error) {
	args := []string{"log", "--reverse", "--format=%s", "--max-count=50", baseRef + "..HEAD"}
	if pathspec != "" && pathspec != "." {
		args = append(args, "--", pathspec)
	}
	out, err := run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	var subjects []string
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			subjects = append(subjects, s)
		}
	}
	return subjects, nil
}
