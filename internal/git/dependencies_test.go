package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownUnderlineIsNotAConflict(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	_ = os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("Release 1.0\n=======\n\n* first\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app\n<<<<<<< ours\nfunc A() {}\n=======\nfunc B() {}\n>>>>>>> theirs\n"), 0o644)
	gitT(t, dir, "add", ".")
	c, err := Conflicts(ctx, dir, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(c.Markers, ",")
	if strings.Contains(joined, "CHANGELOG.md") {
		t.Fatalf("a Markdown heading underline was reported as a conflict: %v", c.Markers)
	}
	if !strings.Contains(joined, "app.go:2") || !strings.Contains(joined, "app.go:6") {
		t.Fatalf("the real conflict hunk was not reported: %v", c.Markers)
	}
}

func TestCommitAllLeavesInstalledDependenciesOut(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	gitT(t, dir, "config", "user.name", "t")
	gitT(t, dir, "config", "user.email", "t@t")
	for _, p := range []string{"vendor/bundle/ruby/3.1.0/gems/x/README.md", "web/node_modules/y/index.js", "coverage/index.html"} {
		_ = os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755)
		_ = os.WriteFile(filepath.Join(dir, p), []byte("x\n"), 0o644)
	}
	_ = os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app\n\nfunc A() { _ = 1 }\n"), 0o644)

	if ok, err := CommitAll(ctx, dir, "", "change"); err != nil || !ok {
		t.Fatalf("CommitAll = %v, %v", ok, err)
	}
	files, _ := run(ctx, dir, "show", "--name-only", "--format=", "HEAD")
	if strings.TrimSpace(files) != "app.go" {
		t.Fatalf("commit must only hold the real change, got:\n%s", files)
	}
	// Running it again must not duplicate the exclude entries.
	if err := ExcludeDependencyDirs(ctx, dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	if strings.Count(string(raw), "**/vendor/bundle/") != 1 {
		t.Fatalf("exclude file:\n%s", raw)
	}
}

func TestAddedDependencyFiles(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	for _, p := range []string{"vendor/bundle/ruby/a.rb", "vendor/bundle/ruby/b.rb", "api/node_modules/z.js", "lib/vendor/thing.rb"} {
		_ = os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755)
		_ = os.WriteFile(filepath.Join(dir, p), []byte("x\n"), 0o644)
	}
	gitT(t, dir, "add", "-f", ".")
	gitT(t, dir, "commit", "-q", "-m", "deps")
	got, err := AddedDependencyFiles(ctx, dir, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	if got["vendor/bundle"] != 2 || got["api/node_modules"] != 1 || len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}
