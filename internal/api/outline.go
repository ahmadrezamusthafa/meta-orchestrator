package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ast"
)

// maxOutlineBytes caps the source file an outline is made of.
const maxOutlineBytes = 4 << 20

// handleTaskOutline serves GET /api/v1/tasks/{id}/outline?repo=<name>&path=<file>: the declarations
// of one source file in the task's worktree of that repository (or the registered checkout when the
// worktree does not exist yet), from the line-based outliner in internal/ast.
func (r *Router) handleTaskOutline(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required for outline")
		return
	}
	t := r.taskSnapshot(req.PathValue("id"))
	if t == nil {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("Task %s not found", req.PathValue("id")))
		return
	}
	repo, rel := req.URL.Query().Get("repo"), req.URL.Query().Get("path")
	if repo == "" || rel == "" {
		r.writeError(w, http.StatusBadRequest, "repo and path are required")
		return
	}
	rel = filepath.FromSlash(rel)
	if !filepath.IsLocal(rel) {
		r.writeError(w, http.StatusBadRequest, "path must be relative and stay inside the repository")
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()
	root, source := "", ""
	for _, wt := range r.describeWorktrees(ctx, t) {
		if wt.Repo != repo {
			continue
		}
		switch {
		case wt.Exists && wt.Error == "":
			root, source = wt.Dir, "worktree"
		case wt.Source != "":
			root, source = wt.Source, "repository"
		}
	}
	if root == "" {
		r.writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is not a repository of %s with a checkout", repo, t.ID))
		return
	}
	full, err := containedFile(root, rel)
	if err != nil {
		r.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	slice, err := ast.NewSharder().ShardFile(full)
	if errors.Is(err, ast.ErrUnsupportedLanguage) {
		r.writeError(w, http.StatusUnsupportedMediaType, err.Error())
		return
	} else if err != nil {
		r.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slice.FilePath = filepath.ToSlash(rel)
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": t.ID, "repo": repo, "source": source, "outline": slice})
}

// containedFile resolves rel under root, following symlinks, and accepts it only when the real
// file is a regular file inside the real root and not too large.
func containedFile(root, rel string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("repository checkout not found")
	}
	real, err := filepath.EvalSymlinks(filepath.Join(realRoot, rel))
	if err != nil {
		return "", fmt.Errorf("%s not found", filepath.ToSlash(rel))
	}
	inside, err := filepath.Rel(realRoot, real)
	if err != nil || !filepath.IsLocal(inside) || strings.HasPrefix(inside, ".git"+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the repository", filepath.ToSlash(rel))
	}
	st, err := os.Stat(real)
	if err != nil || !st.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a file", filepath.ToSlash(rel))
	}
	if st.Size() > maxOutlineBytes {
		return "", fmt.Errorf("%s is too large to outline (%d bytes)", filepath.ToSlash(rel), st.Size())
	}
	return real, nil
}
