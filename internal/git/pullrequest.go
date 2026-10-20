package git

import (
	"bytes"
	"context"
	"errors"
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
// A push origin refuses because the branch history diverged (e.g. after a rebase) fails with
// ErrPushRejected, so the caller can diagnose it and offer ForcePushWithLease.
func Push(ctx context.Context, dir, branch string) error {
	return push(ctx, dir, "-u", "origin", branch)
}

// ForcePushWithLease overwrites origin's branch only while it still points at expectSHA, the
// commit the operator reviewed; anything pushed there since makes the push fail instead.
func ForcePushWithLease(ctx context.Context, dir, branch, expectSHA string) error {
	if expectSHA == "" {
		return fmt.Errorf("a force push needs the remote commit it replaces")
	}
	return push(ctx, dir, "--force-with-lease=refs/heads/"+branch+":"+expectSHA, "-u", "origin", branch)
}

// ErrPushRejected marks a push origin refused because it is not a fast-forward.
var ErrPushRejected = errors.New("push rejected: the remote branch has commits the local branch does not")

func push(ctx context.Context, dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, pushTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "push"}, args...)...)
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0") // never hang on a password prompt
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		if nonFastForward(msg) {
			return fmt.Errorf("git push: %w: %s", ErrPushRejected, msg)
		}
		return fmt.Errorf("git push: %s", msg)
	}
	return nil
}

// nonFastForward recognizes git's refusal to replace remote history. A stale lease is not one:
// it means the remote moved after the operator reviewed it.
func nonFastForward(stderr string) bool {
	if strings.Contains(stderr, "stale info") {
		return false
	}
	return strings.Contains(stderr, "non-fast-forward") || (strings.Contains(stderr, "[rejected]") && strings.Contains(stderr, "fetch first"))
}

// RemoteCommit is a commit on origin's branch that HEAD lacks.
type RemoteCommit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	// Rewritten is true when HEAD holds the same patch under another commit (a rebased copy),
	// so overwriting the remote loses nothing.
	Rewritten bool `json:"rewritten"`
}

// Divergence explains how a local branch and origin's copy of it parted.
type Divergence struct {
	Branch      string         `json:"branch"`
	RemoteSHA   string         `json:"remote_sha"` // origin's tip when diagnosed: the force-push lease
	LocalSHA    string         `json:"local_sha"`
	LocalOnly   int            `json:"local_only"` // commits HEAD has that origin lacks
	RemoteOnly  []RemoteCommit `json:"remote_only"`
	Foreign     int            `json:"foreign"` // remote-only commits with no copy in HEAD: lost on a force push
	MergeBase   string         `json:"merge_base,omitempty"`
	SafeToForce bool           `json:"safe_to_force"`
	Explanation string         `json:"explanation"`
}

// Diverge fetches origin's branch and compares it with HEAD. Remote-only commits whose patch
// HEAD already carries (git cherry) are the old copies a rebase or amend replaced; any other
// remote-only commit is someone else's work that a force push would discard.
func Diverge(ctx context.Context, dir, branch string) (Divergence, error) {
	d := Divergence{Branch: branch}
	if _, err := run(ctx, dir, "fetch", "-q", "origin", "+refs/heads/"+branch+":refs/remotes/origin/"+branch); err != nil {
		return d, err
	}
	remote := "refs/remotes/origin/" + branch
	out, err := run(ctx, dir, "rev-parse", remote, "HEAD")
	if err != nil {
		return d, err
	}
	shas := strings.Fields(out)
	if len(shas) != 2 {
		return d, fmt.Errorf("could not resolve %s and HEAD", remote)
	}
	d.RemoteSHA, d.LocalSHA = shas[0], shas[1]
	if out, err := run(ctx, dir, "merge-base", remote, "HEAD"); err == nil {
		d.MergeBase = strings.TrimSpace(out)
	}
	if out, err := run(ctx, dir, "rev-list", "--count", remote+"..HEAD"); err == nil {
		d.LocalOnly, _ = strconv.Atoi(strings.TrimSpace(out))
	}

	// `git cherry HEAD remote` lists remote-only commits; "-" marks a patch HEAD already has.
	out, err = run(ctx, dir, "cherry", "HEAD", remote)
	if err != nil {
		return d, err
	}
	rewritten := map[string]bool{}
	var order []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		order = append(order, f[1])
		rewritten[f[1]] = f[0] == "-"
	}
	for _, sha := range order {
		c := RemoteCommit{SHA: sha, Rewritten: rewritten[sha]}
		if info, err := run(ctx, dir, "log", "-1", "--format=%an%x00%s", sha); err == nil {
			if parts := strings.SplitN(strings.TrimSpace(info), "\x00", 2); len(parts) == 2 {
				c.Author, c.Subject = parts[0], parts[1]
			}
		}
		if !c.Rewritten {
			d.Foreign++
		}
		d.RemoteOnly = append(d.RemoteOnly, c)
	}
	d.SafeToForce = d.Foreign == 0
	d.Explanation = d.explain()
	return d, nil
}

func (d Divergence) explain() string {
	switch {
	case len(d.RemoteOnly) == 0:
		return fmt.Sprintf("origin/%s has nothing HEAD lacks; a plain push should succeed now.", d.Branch)
	case d.Foreign == 0:
		return fmt.Sprintf("The %d commit(s) only on origin/%s are earlier copies of commits HEAD already carries — the branch "+
			"history was rewritten (rebase or amend). Pulling would merge the old and new copies and duplicate them; "+
			"force-push with a lease instead.", len(d.RemoteOnly), d.Branch)
	default:
		return fmt.Sprintf("%d of the %d commit(s) only on origin/%s have no copy in HEAD — someone else may have pushed "+
			"to the branch. A force push discards them; integrate them first unless you are sure they are obsolete.",
			d.Foreign, len(d.RemoteOnly), d.Branch)
	}
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
