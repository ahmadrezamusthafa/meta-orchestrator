package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// fetchTimeout bounds a background fetch, which must never hold up the watcher for long.
const fetchTimeout = 30 * time.Second

// quiet runs a read-only git command that must not get in the way of the operator's own git use:
// GIT_OPTIONAL_LOCKS=0 stops `git status` from taking index.lock to refresh the index, so a
// commit, rebase or push run in a terminal at the same moment never fails on a held lock.
func quiet(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(cmd.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}

// StateSignature fingerprints what the orchestrator shows about a checkout: the checked-out
// branch and commit (detached during a rebase), ahead/behind counts, working-tree changes
// (conflicted paths included) and origin's copy of branch. It changes whenever git is used on the checkout, inside the
// orchestrator or outside it (commit, rebase, reset, push, force push, fetch).
func StateSignature(ctx context.Context, dir, branch string) (string, error) {
	status, err := quiet(ctx, dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return "", err
	}
	refs, err := quiet(ctx, dir, "for-each-ref", "--format=%(refname) %(objectname)",
		"refs/heads/"+branch, "refs/remotes/origin/"+branch)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(status + "\x00" + refs))
	return hex.EncodeToString(sum[:]), nil
}

// FetchBranch refreshes origin's copy of branch, so pushes made from another clone or machine
// show up. A branch origin does not have yet is not an error.
func FetchBranch(ctx context.Context, dir, branch string) error {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	_, err := quiet(ctx, dir, "fetch", "-q", "--no-tags", "origin", "+refs/heads/"+branch+":refs/remotes/origin/"+branch)
	if err != nil && strings.Contains(err.Error(), "couldn't find remote ref") {
		return nil
	}
	return err
}

// BranchTips resolves HEAD and origin's copy of branch as of the last fetch or push; remote is
// empty when origin has never had the branch.
func BranchTips(ctx context.Context, dir, branch string) (head, remote string) {
	if out, err := quiet(ctx, dir, "rev-parse", "HEAD"); err == nil {
		head = strings.TrimSpace(out)
	}
	if out, err := quiet(ctx, dir, "for-each-ref", "--format=%(objectname)", "refs/remotes/origin/"+branch); err == nil {
		remote = strings.TrimSpace(out)
	}
	return head, remote
}

// CurrentBranch names the branch checked out at dir; empty when HEAD is detached (e.g. mid-rebase).
func CurrentBranch(ctx context.Context, dir string) string {
	out, err := quiet(ctx, dir, "symbolic-ref", "-q", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// SameTree reports whether two commits hold exactly the same files, whatever their history.
func SameTree(ctx context.Context, dir, a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	out, err := quiet(ctx, dir, "rev-parse", a+"^{tree}", b+"^{tree}")
	if err != nil {
		return false
	}
	t := strings.Fields(out)
	return len(t) == 2 && t[0] == t[1]
}

// CheckedOutAt returns the work tree (other than dir) that has branch checked out, if any. git
// lets a branch be checked out in only one work tree of a repository.
func CheckedOutAt(ctx context.Context, dir, branch string) string {
	out, err := quiet(ctx, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return ""
	}
	self, _ := quiet(ctx, dir, "rev-parse", "--show-toplevel")
	self = strings.TrimSpace(self)
	path := ""
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case line == "branch refs/heads/"+branch && path != self:
			return path
		}
	}
	return ""
}
