package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRemote(t *testing.T) {
	cases := []struct{ raw, host, owner, repo string }{
		{"git@bitbucket.org:mid-kelola-indonesia/billing.git", "bitbucket.org", "mid-kelola-indonesia", "billing"},
		{"https://someone@bitbucket.org/mid-kelola-indonesia/billing-internal-fe.git", "bitbucket.org", "mid-kelola-indonesia", "billing-internal-fe"},
		{"ssh://git@github.com/acme/app", "github.com", "acme", "app"},
		{"https://github.com/acme/app.git/", "github.com", "acme", "app"},
	}
	for _, c := range cases {
		r, err := ParseRemote(c.raw)
		if err != nil || r.Host != c.host || r.Owner != c.owner || r.Repo != c.repo {
			t.Errorf("ParseRemote(%q) = %+v, %v", c.raw, r, err)
		}
	}
	if r, _ := ParseRemote("git@bitbucket.org:ws/app.git"); r.Provider() != "bitbucket" || r.WebURL() != "https://bitbucket.org/ws/app" {
		t.Errorf("bitbucket remote: %+v", r)
	}
	for _, bad := range []string{"", "/tmp/repo", "https://github.com/only"} {
		if _, err := ParseRemote(bad); err == nil {
			t.Errorf("ParseRemote(%q) should fail", bad)
		}
	}
	if BranchName("origin/master") != "master" || BranchName("refs/heads/main") != "main" {
		t.Error("BranchName should strip remote prefixes")
	}
}

func TestCommitAllAndSubjects(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	gitT(t, dir, "config", "user.name", "t")
	gitT(t, dir, "config", "user.email", "t@t")
	gitT(t, dir, "checkout", "-q", "-b", "feat/x")
	if made, err := CommitAll(ctx, dir, "", "noop"); err != nil || made {
		t.Fatalf("clean tree → made=%v err=%v", made, err)
	}
	_ = os.WriteFile(filepath.Join(dir, "new.go"), []byte("package app\n"), 0o644)
	if made, err := CommitAll(ctx, dir, "", "feat(PAY-1): add new"); err != nil || !made {
		t.Fatalf("dirty tree → made=%v err=%v", made, err)
	}
	subjects, err := CommitSubjects(ctx, dir, "master", "")
	if err != nil || strings.Join(subjects, "|") != "feat(PAY-1): add new" {
		t.Fatalf("subjects = %v, %v", subjects, err)
	}
}

func TestUnpushedCountsCommitsOriginLacks(t *testing.T) {
	ctx := context.Background()
	origin := t.TempDir()
	gitT(t, origin, "init", "-q", "--bare")
	dir := newRepo(t)
	gitT(t, dir, "remote", "add", "origin", origin)
	gitT(t, dir, "checkout", "-q", "-b", "feat/x")

	if n, onRemote, err := Unpushed(ctx, dir, "feat/x"); err != nil || onRemote || n != 0 {
		t.Fatalf("before the first push: n=%d onRemote=%v err=%v", n, onRemote, err)
	}
	gitT(t, dir, "push", "-q", "-u", "origin", "feat/x")
	if n, onRemote, err := Unpushed(ctx, dir, "feat/x"); err != nil || !onRemote || n != 0 {
		t.Fatalf("after push: n=%d onRemote=%v err=%v", n, onRemote, err)
	}
	for i, f := range []string{"a.go", "b.go"} {
		_ = os.WriteFile(filepath.Join(dir, f), []byte("package app\n"), 0o644)
		gitT(t, dir, "add", f)
		gitT(t, dir, "commit", "-q", "-m", "change "+strings.Repeat("x", i+1))
	}
	if n, onRemote, err := Unpushed(ctx, dir, "feat/x"); err != nil || !onRemote || n != 2 {
		t.Fatalf("two local commits: n=%d onRemote=%v err=%v", n, onRemote, err)
	}
}
