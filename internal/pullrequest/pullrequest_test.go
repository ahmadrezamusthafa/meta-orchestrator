package pullrequest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTitleAndBodyAreStandard(t *testing.T) {
	in := Input{TaskID: "TASK-2", JiraKey: "MIB-12780", JiraURL: "https://jira.example/browse/MIB-12780",
		Title: "[MIB-12780] Prefill package when input SQ", TaskType: "bugfix", SourceBranch: "feat/MIB-12780-x", TargetBranch: "master",
		Summary: "Prefills the package.", Files: []FileChange{{Path: "a.go", Status: "modified", Additions: 3, Deletions: 1}},
		Commits: []string{"fix(MIB-12780): prefill"}}
	if got := Title(in); got != "[MIB-12780] fix: Prefill package when input SQ" {
		t.Fatalf("Title = %q", got)
	}
	if got := CommitMessage(in); got != "fix(MIB-12780): Prefill package when input SQ" {
		t.Fatalf("CommitMessage = %q", got)
	}
	body := Body(in)
	for _, want := range []string{"## Ticket\n[MIB-12780](https://jira.example/browse/MIB-12780)", "## Summary\nPrefills the package.",
		"- [x] Bug fix", "- [ ] New feature", "| modified | `a.go` | 3 | 1 |", "## Acceptance criteria", "## How to test",
		"## Risk & rollback", "## Checklist", "No secrets, credentials or customer PII", "`feat/MIB-12780-x` → `master`"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<details>") {
		t.Error("body must be plain markdown (Bitbucket strips HTML)")
	}
	if got := Title(Input{TaskID: "TASK-9", Title: "Add endpoint"}); got != "[TASK-9] feat: Add endpoint" {
		t.Fatalf("Title without JIRA = %q", got)
	}
}

func TestSection(t *testing.T) {
	doc := "# Impl\n\n## Changes\nlots\n\n## Summary\nDid the thing.\n\n### Detail\nmore\n\n## How to test\n1. run it\n"
	if got := Section(doc, "summary"); got != "Did the thing.\n\n### Detail\nmore" {
		t.Fatalf("summary = %q", got)
	}
	if got := Section(doc, "verif", "how to test"); got != "1. run it" {
		t.Fatalf("testing = %q", got)
	}
	if Section(doc, "risk") != "" {
		t.Fatal("missing section should be empty")
	}
}

func TestOpenBitbucketCreatesThenUpdates(t *testing.T) {
	existing := false
	var created, updated map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "me@example.com" || p != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repositories/ws/app/pullrequests":
			if !strings.Contains(r.URL.Query().Get("q"), `source.branch.name="feat/x"`) {
				t.Errorf("query = %s", r.URL.Query().Get("q"))
			}
			if existing {
				_, _ = w.Write([]byte(`{"values":[{"id":7}]}`))
			} else {
				_, _ = w.Write([]byte(`{"values":[]}`))
			}
		case r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&created)
			_, _ = w.Write([]byte(`{"id":7,"links":{"html":{"href":"https://bitbucket.org/ws/app/pull-requests/7"}}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/repositories/ws/app/pullrequests/7":
			_ = json.NewDecoder(r.Body).Decode(&updated)
			_, _ = w.Write([]byte(`{"id":7,"links":{"html":{"href":"https://bitbucket.org/ws/app/pull-requests/7"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL}
	req := Request{Owner: "ws", Repo: "app", SourceBranch: "feat/x", TargetBranch: "master", Title: "T", Body: "B"}
	creds := Credentials{Username: "me@example.com", Token: "tok"}

	res, err := c.Open(context.Background(), "bitbucket", creds, req)
	if err != nil || res.Number != 7 || res.Updated || !strings.HasSuffix(res.URL, "/pull-requests/7") {
		t.Fatalf("create → %+v, %v", res, err)
	}
	if created["title"] != "T" || created["description"] != "B" || created["close_source_branch"] != true {
		t.Fatalf("create payload = %v", created)
	}
	existing = true
	if res, err = c.Open(context.Background(), "bitbucket", creds, req); err != nil || !res.Updated {
		t.Fatalf("update → %+v, %v", res, err)
	}
	if updated["description"] != "B" || updated["source"] != nil {
		t.Fatalf("update payload = %v", updated)
	}
	if _, err := c.Open(context.Background(), "bitbucket", Credentials{Username: "me@example.com", Token: "bad"}, req); err == nil ||
		!strings.Contains(err.Error(), "401") {
		t.Fatalf("bad credentials should surface the status: %v", err)
	}
	if _, err := c.Open(context.Background(), "gitlab", creds, req); err == nil {
		t.Fatal("unsupported provider should fail")
	}
}

func TestOpenGitHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghp" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("head") != "acme:feat/x" {
				t.Errorf("head = %s", r.URL.Query().Get("head"))
			}
			_, _ = w.Write([]byte(`[]`))
		case http.MethodPost:
			var p map[string]string
			_ = json.NewDecoder(r.Body).Decode(&p)
			if p["head"] != "feat/x" || p["base"] != "main" || p["body"] != "B" {
				t.Errorf("payload = %v", p)
			}
			_, _ = w.Write([]byte(`{"number":3,"html_url":"https://github.com/acme/app/pull/3"}`))
		}
	}))
	defer srv.Close()
	res, err := (&Client{BaseURL: srv.URL}).Open(context.Background(), "github", Credentials{Token: "ghp"},
		Request{Owner: "acme", Repo: "app", SourceBranch: "feat/x", TargetBranch: "main", Title: "T", Body: "B"})
	if err != nil || res.Number != 3 || res.URL != "https://github.com/acme/app/pull/3" {
		t.Fatalf("github → %+v, %v", res, err)
	}
}
