package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// A read outside the workspace waits for the operator, who can grant its folder: the running session
// gets it at once, later turns get it through --add-dir, and later requests inside it ask no more.
func TestGrantingABlockedPathOpensItsFolderToTheTask(t *testing.T) {
	outside := t.TempDir()
	file := filepath.Join(outside, "spec.md")
	_ = os.WriteFile(file, []byte("# spec"), 0o644)
	r, agent := interactionRouter(t, "signoff_merge", llm.ApprovalRequest{ToolName: "Read", ToolUseID: "t1",
		Input: map[string]interface{}{"file_path": file}, BlockedPath: file})
	inReview(r, "TASK-P")

	do(r, http.MethodPost, "/api/v1/tasks/TASK-P/chat", `{"message":"read the attached spec"}`)
	id := awaitApproval(t, r, "TASK-P")
	if a := lastApproval(r, "TASK-P"); a.GrantDir != outside || a.BlockedPath != file {
		t.Fatalf("the request should offer the file's folder: %+v", a)
	}
	w := do(r, http.MethodPost, "/api/v1/tasks/TASK-P/approvals/"+id, `{"decision":"grant"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("grant → %d %s", w.Code, w.Body.String())
	}
	waitChatIdle(t, r, "TASK-P")
	if d := agent.decision(); !d.Allow || !slices.Equal(d.AddDirs, []string{outside}) {
		t.Fatalf("granting should allow and hand the folder to the session: %+v", d)
	}
	if a := lastApproval(r, "TASK-P"); a.Decision != types.ApprovalGranted {
		t.Fatalf("decision = %q", a.Decision)
	}

	// The next turn starts with the folder and is not asked again.
	do(r, http.MethodPost, "/api/v1/tasks/TASK-P/chat", `{"message":"and again"}`)
	waitChatIdle(t, r, "TASK-P")
	if req := agent.reqs[len(agent.reqs)-1]; !slices.Contains(req.AddDirs, outside) {
		t.Fatalf("later turns should get the granted folder: %v", req.AddDirs)
	}
	if a := lastApproval(r, "TASK-P"); a.Decision != types.ApprovalAutomatic || !agent.decision().Allow {
		t.Fatalf("a path inside a granted folder should be auto-approved: %+v", a)
	}
}

func TestGrantNeedsAFolderToGrant(t *testing.T) {
	r, _ := interactionRouter(t, "signoff_merge", llm.ApprovalRequest{ToolName: "Bash", ToolUseID: "t1",
		Input: map[string]interface{}{"command": "make test"}})
	inReview(r, "TASK-P")
	do(r, http.MethodPost, "/api/v1/tasks/TASK-P/chat", `{"message":"run tests"}`)
	id := awaitApproval(t, r, "TASK-P")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-P/approvals/"+id, `{"decision":"grant"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("grant without a blocked path → %d, want 400", w.Code)
	}
	do(r, http.MethodPost, "/api/v1/tasks/TASK-P/approvals/"+id, `{"decision":"deny"}`)
	waitChatIdle(t, r, "TASK-P")
}

func TestGrantDirFor(t *testing.T) {
	dir := t.TempDir()
	home, _ := os.UserHomeDir()
	for in, want := range map[string]string{
		filepath.Join(dir, "a.txt"): dir, // a file: its folder
		dir:                         dir, // a folder: itself
		"/":                         "",  // never the whole filesystem
		filepath.Join(home, "x.md"): "",  // nor the whole home folder
		"relative/path":             "",
	} {
		if got := grantDirFor(in); got != want {
			t.Errorf("grantDirFor(%q) = %q, want %q", in, got, want)
		}
	}
	if !pathWithin(filepath.Join(dir, "sub", "f"), dir) || pathWithin(dir+"-other/f", dir) {
		t.Error("pathWithin must match whole path segments")
	}
}

func TestFSBrowseListsFilesOnRequest(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "notes.md"), []byte("hi"), 0o644)
	_ = os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})

	var resp BrowseFSResponse
	_ = json.Unmarshal(do(r, http.MethodGet, "/api/v1/fs/browse?path="+dir, "").Body.Bytes(), &resp)
	if len(resp.Files) != 0 {
		t.Fatalf("files are listed only when asked: %+v", resp.Files)
	}
	_ = json.Unmarshal(do(r, http.MethodGet, "/api/v1/fs/browse?files=true&path="+dir, "").Body.Bytes(), &resp)
	if len(resp.Files) != 1 || resp.Files[0].Path != filepath.Join(dir, "notes.md") || resp.Files[0].Size != 2 || len(resp.Directories) != 1 {
		t.Fatalf("browse with files = %+v", resp)
	}
}
