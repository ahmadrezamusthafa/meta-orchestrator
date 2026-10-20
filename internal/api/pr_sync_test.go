package api

import (
	"strings"
	"testing"
)

func TestPRSyncStates(t *testing.T) {
	cases := []struct {
		name  string
		d     prDraft
		state string
		note  string
	}{
		{"not opened", prDraft{Uncommitted: true}, prNotOpened, ""},
		{"in sync", prDraft{ExistingURL: "u", OnRemote: true}, prInSync, "every change"},
		{"unpushed commits", prDraft{ExistingURL: "u", OnRemote: true, Unpushed: 3}, prOutOfSync, "3 commit(s) not pushed"},
		{"uncommitted edits", prDraft{ExistingURL: "u", OnRemote: true, Uncommitted: true}, prOutOfSync, "uncommitted edits"},
		{"rewritten elsewhere, same files", prDraft{ExistingURL: "u", OnRemote: true, Unpushed: 1, SameContent: true}, prInSync, "only the history differs"},
		{"same files but edits pending", prDraft{ExistingURL: "u", OnRemote: true, SameContent: true, Uncommitted: true}, prOutOfSync, "uncommitted edits"},
		{"worktree on another branch", prDraft{ExistingURL: "u", OnRemote: true, Unpushed: 1, SourceBranch: "feat/x", CheckedOut: "feat/x-BACK",
			HeldBy: "/src/app"}, prOutOfSync, "on branch feat/x-BACK, but the pull request follows feat/x"},
		{"branch gone", prDraft{ExistingURL: "u", SourceBranch: "feat/x"}, prOutOfSync, "feat/x is not on the remote"},
	}
	for _, c := range cases {
		state, note := prSync(&c.d)
		if state != c.state || !strings.Contains(note, c.note) {
			t.Errorf("%s: got %s %q", c.name, state, note)
		}
	}
}
