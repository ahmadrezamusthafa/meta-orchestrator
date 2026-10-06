package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// gitTimeout bounds every git invocation so a hung repository never stalls the daemon.
const gitTimeout = 30 * time.Second

// MaxPatchBytes caps the unified diff returned for one repository.
const MaxPatchBytes = 1 << 20

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// TaskBranchName builds the worktree branch for a task. A JIRA key leads the name so JIRA's
// development panel links the branch (e.g. "feat/PAY-123-express-checkout"); otherwise the task ID
// is used ("feat/task-12-express-checkout").
func TaskBranchName(jiraKey, taskID, title string) string {
	prefix := strings.ToLower(taskID)
	if k := strings.ToUpper(strings.TrimSpace(jiraKey)); k != "" {
		prefix = k
	}
	// Drop a leading "[KEY]" so the key is not repeated in the slug.
	t := strings.ToLower(title)
	if jiraKey != "" {
		t = strings.ReplaceAll(t, strings.ToLower(jiraKey), "")
	}
	slug := strings.Trim(nonSlug.ReplaceAllString(t, "-"), "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
		if i := strings.LastIndex(slug, "-"); i > 20 {
			slug = slug[:i]
		}
	}
	if slug == "" {
		return "feat/" + prefix
	}
	return "feat/" + prefix + "-" + slug
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}

// TopLevel returns the root of the git work tree containing dir.
func TopLevel(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(dir string) bool {
	out, err := run(context.Background(), dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

func refExists(ctx context.Context, dir, ref string) bool {
	_, err := run(ctx, dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

// DefaultBaseRef picks the branch new work is based on: the remote default branch when known,
// else a local master/main, else HEAD.
func DefaultBaseRef(ctx context.Context, repo string) string {
	if out, err := run(ctx, repo, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		if ref := strings.TrimPrefix(strings.TrimSpace(out), "refs/remotes/"); ref != "" && refExists(ctx, repo, ref) {
			return ref
		}
	}
	for _, ref := range []string{"master", "main", "origin/master", "origin/main"} {
		if refExists(ctx, repo, ref) {
			return ref
		}
	}
	return "HEAD"
}

// EnsureWorktree makes dir a worktree of repo on branch, creating branch from base when it does
// not exist yet. An existing worktree at dir is reused as-is.
func EnsureWorktree(ctx context.Context, repo, dir, branch, base string) (created bool, err error) {
	if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return false, err
	}
	if refExists(ctx, repo, "refs/heads/"+branch) {
		_, err = run(ctx, repo, "worktree", "add", dir, branch)
	} else {
		_, err = run(ctx, repo, "worktree", "add", "-b", branch, dir, base)
	}
	return err == nil, err
}

// IsDirty reports uncommitted or untracked changes in dir.
func IsDirty(ctx context.Context, dir string) (bool, error) {
	out, err := run(ctx, dir, "status", "--porcelain")
	return strings.TrimSpace(out) != "", err
}

// ConflictState is what is left of a merge conflict in a work tree.
type ConflictState struct {
	Operation string   // merge, rebase, cherry-pick or revert still in progress; empty when none
	Unmerged  []string // paths git still marks as conflicted
	Markers   []string // "path:line" of conflict markers left in files (committed or not)
}

// Unresolved reports whether anything needs the operator's attention.
func (c ConflictState) Unresolved() bool {
	return c.Operation != "" || len(c.Unmerged) > 0 || len(c.Markers) > 0
}

// Conflicts inspects dir for an unfinished merge-like operation, unmerged paths, and conflict
// markers left in files changed since baseRef (HEAD when empty).
func Conflicts(ctx context.Context, dir, baseRef string) (ConflictState, error) {
	var c ConflictState
	for _, op := range []struct{ ref, name string }{{"MERGE_HEAD", "merge"}, {"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"}} {
		if refExists(ctx, dir, op.ref) {
			c.Operation = op.name
		}
	}
	for _, d := range []string{"rebase-merge", "rebase-apply"} {
		if out, err := run(ctx, dir, "rev-parse", "--git-path", d); err == nil {
			p := strings.TrimSpace(out)
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			if _, err := os.Stat(p); err == nil {
				c.Operation = "rebase"
			}
		}
	}
	out, err := run(ctx, dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return c, err
	}
	c.Unmerged = strings.Fields(out)
	if baseRef == "" || !refExists(ctx, dir, baseRef) {
		baseRef = "HEAD"
	}
	// --check exits non-zero when it finds problems, so its output matters, not its error.
	out, _ = run(ctx, dir, "diff", "--check", baseRef)
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, ": leftover conflict marker"); i > 0 {
			c.Markers = append(c.Markers, line[:i])
		}
	}
	return c, nil
}

// RemoveWorktreeIfClean detaches a worktree that holds no uncommitted work. The branch (and its
// commits) is always kept. It reports whether the worktree was removed.
func RemoveWorktreeIfClean(ctx context.Context, repo, dir string) (bool, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return false, nil
	}
	if dirty, err := IsDirty(ctx, dir); err != nil || dirty {
		return false, err
	}
	if _, err := run(ctx, repo, "worktree", "remove", dir); err != nil {
		return false, err
	}
	_, _ = run(ctx, repo, "worktree", "prune")
	return true, nil
}

// DiffAgainst selects what a diff compares the working tree with.
type DiffAgainst string

const (
	// DiffAgainstBase shows everything on the branch since it forked from the base branch,
	// including uncommitted work — what a pull request would contain.
	DiffAgainstBase DiffAgainst = "base"
	// DiffAgainstHead shows only uncommitted work.
	DiffAgainstHead DiffAgainst = "head"
)

// DiffFile summarizes one changed path.
type DiffFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Status    string `json:"status"` // added, modified, deleted, renamed, untracked
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
}

// DiffCommit is a commit on the branch that is not on the base.
type DiffCommit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// RepoDiff is the change set of one worktree.
type RepoDiff struct {
	Branch     string       `json:"branch"`
	BaseRef    string       `json:"base_ref"`
	CompareRef string       `json:"compare_ref"` // resolved commit the working tree is compared with
	Commits    []DiffCommit `json:"commits"`
	Files      []DiffFile   `json:"files"`
	Additions  int          `json:"additions"`
	Deletions  int          `json:"deletions"`
	Patch      string       `json:"patch"`
	Truncated  bool         `json:"truncated,omitempty"`
}

// Diff compares the working tree in dir with its base branch or with HEAD. A non-empty pathspec
// limits the diff to that subdirectory (for projects that live inside a larger repository).
func Diff(ctx context.Context, dir, baseRef string, against DiffAgainst, pathspec string) (*RepoDiff, error) {
	scope := []string{"--"}
	if pathspec != "" && pathspec != "." {
		scope = append(scope, pathspec)
	}
	branch, _ := run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	d := &RepoDiff{Branch: strings.TrimSpace(branch), BaseRef: baseRef, Commits: []DiffCommit{}, Files: []DiffFile{}}

	compare := "HEAD"
	if against == DiffAgainstBase {
		if baseRef == "" {
			baseRef = "HEAD"
		}
		mb, err := run(ctx, dir, "merge-base", baseRef, "HEAD")
		if err != nil {
			return nil, fmt.Errorf("cannot find where %s forked from %s: %w", d.Branch, baseRef, err)
		}
		compare = strings.TrimSpace(mb)
		log, err := run(ctx, dir, append([]string{"log", "--format=%h%x09%s", "--max-count=100", compare + "..HEAD"}, scope...)...)
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
				if sha, subject, ok := strings.Cut(line, "\t"); ok {
					d.Commits = append(d.Commits, DiffCommit{SHA: sha, Subject: subject})
				}
			}
		}
	} else if !refExists(ctx, dir, "HEAD") {
		return nil, errors.New("the worktree has no commits yet")
	}
	full, _ := run(ctx, dir, "rev-parse", "--short", compare)
	d.CompareRef = strings.TrimSpace(full)

	// Tracked changes: working tree (staged + unstaged) against the compare commit.
	nameStatus, err := run(ctx, dir, append([]string{"diff", "--name-status", "-M", "-z", compare}, scope...)...)
	if err != nil {
		return nil, err
	}
	numstat, err := run(ctx, dir, append([]string{"diff", "--numstat", "-M", "-z", compare}, scope...)...)
	if err != nil {
		return nil, err
	}
	d.Files = parseNameStatus(nameStatus)
	applyNumstat(d.Files, numstat)

	var patch strings.Builder
	p, err := run(ctx, dir, append([]string{"diff", "--no-color", "--no-ext-diff", "-M", compare}, scope...)...)
	if err != nil {
		return nil, err
	}
	patch.WriteString(p)

	// Untracked files are part of the change set but invisible to git diff.
	untracked, _ := run(ctx, dir, append([]string{"ls-files", "--others", "--exclude-standard", "-z"}, scope...)...)
	for _, path := range strings.Split(untracked, "\x00") {
		if path == "" {
			continue
		}
		f := DiffFile{Path: path, Status: "untracked"}
		// --no-index exits 1 when files differ, which they always do against /dev/null.
		fp, _ := run(ctx, dir, "diff", "--no-color", "--no-index", "--numstat", "--", os.DevNull, path)
		if add, _, binary := parseNumstatLine(fp); binary {
			f.Binary = true
		} else {
			f.Additions = add
		}
		d.Files = append(d.Files, f)
		if patch.Len() < MaxPatchBytes {
			fp, _ := run(ctx, dir, "diff", "--no-color", "--no-index", "--", os.DevNull, path)
			patch.WriteString(fp)
		}
	}
	for _, f := range d.Files {
		d.Additions += f.Additions
		d.Deletions += f.Deletions
	}
	d.Patch = patch.String()
	if len(d.Patch) > MaxPatchBytes {
		d.Patch = d.Patch[:MaxPatchBytes]
		if i := strings.LastIndex(d.Patch, "\ndiff --git "); i > 0 {
			d.Patch = d.Patch[:i+1]
		}
		d.Truncated = true
	}
	return d, nil
}

// parseNameStatus reads `git diff --name-status -z` output.
func parseNameStatus(out string) []DiffFile {
	fields := strings.Split(out, "\x00")
	files := []DiffFile{}
	for i := 0; i < len(fields); i++ {
		code := fields[i]
		if code == "" {
			continue
		}
		f := DiffFile{}
		switch code[0] {
		case 'A':
			f.Status = "added"
		case 'D':
			f.Status = "deleted"
		case 'R', 'C':
			f.Status = "renamed"
			if i+2 < len(fields) {
				f.OldPath, i = fields[i+1], i+1
			}
		default:
			f.Status = "modified"
		}
		if i+1 < len(fields) {
			f.Path, i = fields[i+1], i+1
		}
		files = append(files, f)
	}
	return files
}

// applyNumstat merges `git diff --numstat -z` counts into files. Renames are emitted as
// "add\tdel\t" followed by old and new paths as separate NUL-terminated fields.
func applyNumstat(files []DiffFile, out string) {
	byPath := make(map[string]*DiffFile, len(files))
	for i := range files {
		byPath[files[i].Path] = &files[i]
	}
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		parts := strings.SplitN(fields[i], "\t", 3)
		if len(parts) < 3 {
			continue
		}
		path := parts[2]
		if path == "" && i+2 < len(fields) { // rename: old, new follow
			path, i = fields[i+2], i+2
		}
		f := byPath[path]
		if f == nil {
			continue
		}
		if parts[0] == "-" {
			f.Binary = true
			continue
		}
		f.Additions, _ = strconv.Atoi(parts[0])
		f.Deletions, _ = strconv.Atoi(parts[1])
	}
}

func parseNumstatLine(out string) (add, del int, binary bool) {
	parts := strings.SplitN(strings.TrimSpace(out), "\t", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	if parts[0] == "-" {
		return 0, 0, true
	}
	add, _ = strconv.Atoi(parts[0])
	del, _ = strconv.Atoi(parts[1])
	return add, del, false
}
