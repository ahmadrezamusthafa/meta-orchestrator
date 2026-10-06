package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newRepo creates a repository whose default branch is master with one commit.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "-b", "master")
	_ = os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app\n\nfunc A() {}\n"), 0o644)
	gitT(t, dir, "add", ".")
	gitT(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func TestTaskBranchName(t *testing.T) {
	cases := []struct{ key, id, title, want string }{
		{"PAY-123", "TASK-7", "[PAY-123] Express checkout: Apple Pay & Google Pay", "feat/PAY-123-express-checkout-apple-pay-google-pay"},
		{"", "TASK-7", "Add refund endpoint", "feat/task-7-add-refund-endpoint"},
		{"", "TASK-8", "!!!", "feat/task-8"},
		{"PAY-9", "TASK-9", "A very long summary that keeps going well beyond any sensible branch length", "feat/PAY-9-a-very-long-summary-that-keeps-going"},
	}
	for _, c := range cases {
		if got := TaskBranchName(c.key, c.id, c.title); got != c.want {
			t.Errorf("TaskBranchName(%q,%q,%q) = %q, want %q", c.key, c.id, c.title, got, c.want)
		}
	}
}

func TestWorktreeDiffLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	if base := DefaultBaseRef(ctx, repo); base != "master" {
		t.Fatalf("base = %q, want master", base)
	}
	wt := filepath.Join(t.TempDir(), "wt", "app")
	created, err := EnsureWorktree(ctx, repo, wt, "feat/PAY-1-x", "master")
	if err != nil || !created {
		t.Fatalf("EnsureWorktree: created=%v err=%v", created, err)
	}
	if again, err := EnsureWorktree(ctx, repo, wt, "feat/PAY-1-x", "master"); err != nil || again {
		t.Fatalf("second EnsureWorktree should reuse: created=%v err=%v", again, err)
	}

	// One committed change, one uncommitted edit, one untracked file.
	_ = os.WriteFile(filepath.Join(wt, "app.go"), []byte("package app\n\nfunc A() {}\n\nfunc B() {}\n"), 0o644)
	gitT(t, wt, "commit", "-q", "-am", "add B")
	_ = os.WriteFile(filepath.Join(wt, "app.go"), []byte("package app\n\nfunc A() { _ = 1 }\n\nfunc B() {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(wt, "new.txt"), []byte("hello\n"), 0o644)
	// Master moves on independently; it must not show up in the branch diff.
	_ = os.WriteFile(filepath.Join(repo, "other.go"), []byte("package app\n"), 0o644)
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "master work")

	base, err := Diff(ctx, wt, "master", DiffAgainstBase, "")
	if err != nil {
		t.Fatal(err)
	}
	if base.Branch != "feat/PAY-1-x" || len(base.Commits) != 1 || base.Commits[0].Subject != "add B" {
		t.Fatalf("branch/commits: %+v", base)
	}
	paths := map[string]DiffFile{}
	for _, f := range base.Files {
		paths[f.Path] = f
	}
	if len(paths) != 2 || paths["app.go"].Status != "modified" || paths["new.txt"].Status != "untracked" || paths["new.txt"].Additions != 1 {
		t.Fatalf("files vs base: %+v", base.Files)
	}
	if _, leaked := paths["other.go"]; leaked || !strings.Contains(base.Patch, "+func B() {}") || !strings.Contains(base.Patch, "+hello") {
		t.Fatalf("patch vs base wrong:\n%s", base.Patch)
	}

	head, err := Diff(ctx, wt, "master", DiffAgainstHead, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(head.Patch, "+func B() {}") || !strings.Contains(head.Patch, "_ = 1") || len(head.Commits) != 0 {
		t.Fatalf("head diff should only hold uncommitted work:\n%s", head.Patch)
	}

	if removed, _ := RemoveWorktreeIfClean(ctx, repo, wt); removed {
		t.Fatal("a dirty worktree must never be removed")
	}
	gitT(t, wt, "add", ".")
	gitT(t, wt, "commit", "-q", "-m", "wip")
	if removed, err := RemoveWorktreeIfClean(ctx, repo, wt); err != nil || !removed {
		t.Fatalf("clean worktree should be removed: %v %v", removed, err)
	}
	if !refExists(ctx, repo, "refs/heads/feat/PAY-1-x") {
		t.Fatal("removing the worktree must keep the branch")
	}
}

func TestConflictsFindsUnfinishedMergesAndLeftoverMarkers(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	if c, err := Conflicts(ctx, repo, "master"); err != nil || c.Unresolved() {
		t.Fatalf("clean repo reported %+v %v", c, err)
	}

	// Both branches change the same line: the merge stops with a conflict.
	gitT(t, repo, "checkout", "-q", "-b", "feat")
	_ = os.WriteFile(filepath.Join(repo, "app.go"), []byte("package app\n\nfunc A() { feat() }\n"), 0o644)
	gitT(t, repo, "commit", "-q", "-am", "feat")
	gitT(t, repo, "checkout", "-q", "master")
	_ = os.WriteFile(filepath.Join(repo, "app.go"), []byte("package app\n\nfunc A() { master() }\n"), 0o644)
	gitT(t, repo, "commit", "-q", "-am", "master")
	gitT(t, repo, "checkout", "-q", "feat")
	_ = exec.Command("git", "-C", repo, "merge", "master").Run()
	c, err := Conflicts(ctx, repo, "master")
	if err != nil || c.Operation != "merge" || len(c.Unmerged) != 1 || c.Unmerged[0] != "app.go" {
		t.Fatalf("unfinished merge = %+v %v", c, err)
	}

	// Committing the file with its markers still in it ends the merge but not the conflict.
	gitT(t, repo, "commit", "-q", "-am", "merge")
	c, _ = Conflicts(ctx, repo, "master")
	if c.Operation != "" || len(c.Unmerged) != 0 || len(c.Markers) == 0 || !strings.HasPrefix(c.Markers[0], "app.go:") {
		t.Fatalf("committed markers = %+v", c)
	}
}
