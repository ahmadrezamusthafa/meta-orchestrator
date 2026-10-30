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

func TestWithoutEvidenceKeepsTheReviewerPartsOnly(t *testing.T) {
	doc := "## Summary\n" +
		"- Voiding a self-checkout PI now deletes its contract, behind the `rls_void_pi_delete_contract` toggle.\n" +
		"- Recorded the test evidence: 4 videos in /Users/x/.sdlc/artifacts/TASK-1/evidence\n" +
		"  - FLOW-1.mp4 passed\n" +
		"  - SUMMARY.mp4\n" +
		"- Seven new unit examples, including toggle OFF.\n\n" +
		"The storyboard was re-recorded twice.\n\n" +
		"## Test evidence\n| Case | Status |\n|---|---|\n| FLOW-1 | passed |\n\n" +
		"### Suites\nbilling: 812 passed\n\n" +
		"## How to test\n1. Turn the toggle on.\n2. Void a self-checkout PI and check its contract is gone.\n3. Watch FLOW-1.mp4 in the Evidence tab.\n"
	got := WithoutEvidence(doc)
	for _, gone := range []string{"Recorded the test evidence", "FLOW-1.mp4", "SUMMARY.mp4", "storyboard", "## Test evidence", "812 passed", "Evidence tab"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q is still in:\n%s", gone, got)
		}
	}
	for _, kept := range []string{"deletes its contract", "Seven new unit examples", "## How to test", "2. Void a self-checkout PI"} {
		if !strings.Contains(got, kept) {
			t.Errorf("%q was dropped:\n%s", kept, got)
		}
	}
	if s := Section(got, "summary"); strings.Contains(s, "evidence") || !strings.HasSuffix(s, "toggle OFF.") {
		t.Errorf("summary section = %q", s)
	}
}

func TestBodyDescribesTheTaskChangesAndEvidence(t *testing.T) {
	passed, failed := 812, 0
	in := Input{TaskID: "TASK-3", Title: "Void PI deletes contract", Context: "Voided self-checkout PIs leave orphan contracts.",
		Summary: "Voiding a PI deletes its contract.", Details: "- `void_service.rb`: deletes the contract in the void transaction.",
		Files:   []FileChange{{Path: "app/void_service.rb", Status: "modified"}, {Path: "spec/void_service_spec.rb", Status: "added"}},
		Related: []Related{{Repo: "billing-fe", URL: "https://bitbucket.org/ws/billing-fe/pull-requests/4"}, {Repo: "api"}},
		Evidence: &Evidence{Covered: 2, Total: 3,
			Cases:  []EvidenceCase{{ID: "FLOW-1", Kind: "flow", Title: "Void a PI | end to end", Status: "passed"}},
			Suites: []EvidenceSuite{{Repository: "billing", Command: "bundle exec rspec", Status: "passed", Passed: &passed, Failed: &failed}}}}
	body := Body(in)
	for _, want := range []string{"## Context\nVoided self-checkout PIs leave orphan contracts.",
		"## Changes\n- `void_service.rb`: deletes the contract in the void transaction.\n\n2 file(s)",
		"## Related pull requests\n- billing-fe: https://bitbucket.org/ws/billing-fe/pull-requests/4\n- api: not opened yet",
		"## Test evidence\n2 of 3 acceptance case(s)", "| FLOW-1 — Void a PI \\| end to end | flow | passed |",
		"| billing | `bundle exec rspec` | passed (812 passed, 0 failed) |", "- [x] Unit / integration tests added or updated",
		"- [ ] Documentation updated where needed"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	plain := Body(Input{TaskID: "TASK-4", Context: "Same text.", Summary: "Same text."})
	for _, gone := range []string{"## Test evidence", "## Related pull requests", "## Context\nSame text."} {
		if strings.Contains(plain, gone) {
			t.Errorf("%q should not be in:\n%s", gone, plain)
		}
	}
}

func TestChangesNarrowsToTheRepositoryAndDropsDiffs(t *testing.T) {
	doc := "## Summary\nx\n\n## Changes\n### billing-internal-fe\n- Shows the toggle.\n\n### billing\n- `void.rb`: deletes the contract.\n\n" +
		"```diff\n- old\n+ new\n```\n\n## How to test\n1. run\n"
	if got := Changes(doc, "billing"); got != "- `void.rb`: deletes the contract.\n" {
		t.Fatalf("billing changes = %q", got)
	}
	if got := Changes(doc, "billing-internal-fe"); got != "- Shows the toggle.\n" {
		t.Fatalf("fe changes = %q", got)
	}
	if got := Changes(doc, "other"); !strings.Contains(got, "Shows the toggle.") || strings.Contains(got, "+ new") {
		t.Fatalf("unmatched repo keeps the whole section without diffs: %q", got)
	}
	if Changes("## Summary\nx\n", "billing") != "" {
		t.Fatal("no changes section should be empty")
	}
}

func TestSummaryPromptIsScopedToTheRepository(t *testing.T) {
	patch := "diff --git a/a.rb b/a.rb\n+one\n" + "diff --git a/b.rb b/b.rb\n" + strings.Repeat("+x\n", maxPromptPatch)
	system, user := SummaryPrompt(SummaryInput{Title: "[MIB-1] Void PI", Description: "Delete the contract.", Repo: "billing",
		OtherRepos: []string{"billing-fe"}, Commits: []string{"feat(MIB-1): void"}, ReportChanges: "- `a.rb`: voids.", Patch: patch})
	for _, want := range []string{"Task: Void PI\n", "Repository of this pull request: billing", "not this one): billing-fe",
		"- feat(MIB-1): void", "- `a.rb`: voids.", "(cut short;", "+one"} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(user, "b/b.rb") || len(user) > maxPromptPatch+1000 {
		t.Errorf("the diff must be cut at a file boundary (len %d)", len(user))
	}
	if !strings.Contains(system, "Describe only what the diff contains") || !strings.Contains(system, "Never reproduce secrets") {
		t.Errorf("system prompt = %q", system)
	}
}

func TestCleanSummary(t *testing.T) {
	got := CleanSummary("## Summary\n- Voids the PI.\n\n```ruby\ncontract.destroy\n```\n\n### Notes\n- Behind a toggle.\n")
	if got != "- Voids the PI.\n\n**Notes**\n- Behind a toggle.\n" {
		t.Fatalf("CleanSummary = %q", got)
	}
	if CleanSummary("  \n```\ncode\n```\n") != "" {
		t.Fatal("nothing usable should be empty")
	}
}
