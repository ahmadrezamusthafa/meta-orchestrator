package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ast"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestTaskOutlineServesSymbolsFromTheWorktree(t *testing.T) {
	r, src := worktreeRouter(t)
	r.mu.Lock()
	r.tasks["TASK-O1"] = &types.Task{ID: "TASK-O1", Title: "Refunds", CurrentStageID: "task_implementation",
		State: types.TaskStatePending, AssignedRepos: []string{"app"}, Metadata: map[string]string{}}
	r.mu.Unlock()

	// Before the worktree exists the registered checkout is outlined.
	w := do(r, http.MethodGet, "/api/v1/tasks/TASK-O1/outline?repo=app&path=main.go", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"source":"repository"`) {
		t.Fatalf("outline → %d %s", w.Code, w.Body.String())
	}

	dir := r.resolveWorkDir(r.taskSnapshot("TASK-O1")) // creates the worktree
	_ = os.MkdirAll(filepath.Join(dir, "svc"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "svc", "refund.go"), []byte("package svc\n\ntype Refunds struct{}\n\nfunc (r *Refunds) Apply(id string) error {\n\treturn nil\n}\n"), 0o644)
	w = do(r, http.MethodGet, "/api/v1/tasks/TASK-O1/outline?repo=app&path=svc/refund.go", "")
	var resp struct {
		Source  string
		Outline ast.ASTContextSlice
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != 200 || resp.Source != "worktree" || resp.Outline.FilePath != "svc/refund.go" || len(resp.Outline.Symbols) != 2 ||
		resp.Outline.Symbols[1].Name != "Apply" || resp.Outline.Symbols[1].Kind != "method" {
		t.Fatalf("outline → %d %s", w.Code, w.Body.String())
	}

	// Escapes are refused: .., absolute paths, and a symlink pointing out of the checkout.
	outside := filepath.Join(t.TempDir(), "secret.go")
	_ = os.WriteFile(outside, []byte("package x\nfunc Secret() {}\n"), 0o644)
	_ = os.Symlink(outside, filepath.Join(dir, "link.go"))
	for path, code := range map[string]int{"../" + filepath.Base(src) + "/main.go": 400, outside: 400, "link.go": 404, "README.md": 404} {
		if w := do(r, http.MethodGet, "/api/v1/tasks/TASK-O1/outline?repo=app&path="+path, ""); w.Code != code {
			t.Errorf("%s → %d %s, want %d", path, w.Code, w.Body.String(), code)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	if w := do(r, http.MethodGet, "/api/v1/tasks/TASK-O1/outline?repo=app&path=notes.txt", ""); w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("unsupported language → %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/v1/tasks/TASK-O1/outline?repo=other&path=main.go", ""); w.Code != 400 {
		t.Errorf("unassigned repo → %d", w.Code)
	}
}
