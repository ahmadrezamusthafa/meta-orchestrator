package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/artifacts"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/techdoc"
)

// maxArtifactBytes caps a text artifact returned inline.
const maxArtifactBytes = 2 << 20

var safeTaskID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var artifactContentTypes = map[string]string{
	".mp4": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".webp": "image/webp", ".svg": "image/svg+xml",
}

// stageDocOrder names stage outputs so they sort in pipeline order.
func stageDocName(stage string) string {
	if i := stageIndexOf(stage); i >= 0 {
		return fmt.Sprintf("stages/%02d-%s.md", i+1, stage)
	}
	return "stages/" + stage + ".md"
}

func (r *Router) artifactDir(taskID string) string {
	return r.cfg.ArtifactManager.TaskArtifactDir(taskID)
}

// resolveArtifactPath maps a request to a file inside the task's artifact directory, rejecting
// anything that could escape it.
func (r *Router) resolveArtifactPath(taskID, name string) (string, error) {
	if !safeTaskID.MatchString(taskID) {
		return "", fmt.Errorf("invalid task id")
	}
	clean := filepath.Clean("/" + filepath.FromSlash(name))
	if clean == "/" || strings.Contains(name, "\x00") {
		return "", fmt.Errorf("invalid artifact name")
	}
	base := r.artifactDir(taskID)
	full := filepath.Join(base, clean)
	if rel, err := filepath.Rel(base, full); err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid artifact name")
	}
	return full, nil
}

// realArtifactPath resolves symlinks in path (from resolveArtifactPath) and reports whether the file
// it names is still inside the task's artifact directory.
func (r *Router) realArtifactPath(taskID, path string) (string, bool) {
	base, err := filepath.EvalSymlinks(r.artifactDir(taskID))
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(base, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return real, true
}

// handleArtifacts serves GET /api/v1/artifacts/{task_id}/{path}: media files are streamed, text
// files are returned as JSON. Only files that exist are served — nothing is synthesized.
func (r *Router) handleArtifacts(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	taskID, name, ok := strings.Cut(strings.TrimPrefix(req.URL.Path, "/api/v1/artifacts/"), "/")
	if !ok || name == "" {
		r.writeError(w, http.StatusBadRequest, "Expected /api/v1/artifacts/{task_id}/{filename}")
		return
	}
	path, err := r.resolveArtifactPath(taskID, name)
	if err != nil {
		r.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Agents write into the artifact directory: never follow a symlink out of it.
	reqPath := path
	if real, ok := r.realArtifactPath(taskID, path); ok {
		path = real
	} else {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("%s has no artifact %s", taskID, name))
		return
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("%s has no artifact %s", taskID, name))
		return
	}
	if ct, media := artifactContentTypes[strings.ToLower(filepath.Ext(path))]; media {
		w.Header().Set("Content-Type", ct)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if ct == "image/svg+xml" { // SVG can carry script; never let it run in our origin
			w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		}
		http.ServeFile(w, req, path)
		return
	}
	if strings.EqualFold(filepath.Ext(path), ".html") {
		// Generated, self-contained reports (UAT guide): offered as a download and sandboxed so
		// nothing in them can run in the orchestrator's origin.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src data:; style-src 'unsafe-inline'")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", taskID+"-"+filepath.Base(reqPath)))
		http.ServeFile(w, req, path)
		return
	}
	if st.Size() > maxArtifactBytes {
		r.writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("%s is too large to preview (%d bytes)", name, st.Size()))
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		r.writeError(w, http.StatusInternalServerError, "Failed to read artifact")
		return
	}
	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"task_id":     taskID,
		"filename":    filepath.ToSlash(strings.TrimPrefix(reqPath, r.artifactDir(taskID)+string(filepath.Separator))),
		"content":     string(content),
		"modified_at": st.ModTime(),
	})
}

// artifactEntry describes one file in a task's artifact directory.
type artifactEntry = artifacts.Entry

// handleTaskArtifacts serves GET /api/v1/tasks/{id}/artifacts: every file the task has produced,
// as the artifact registry finds it on disk.
func (r *Router) handleTaskArtifacts(w http.ResponseWriter, req *http.Request, taskID string) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required for artifacts")
		return
	}
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": taskID, "artifacts": r.cfg.ArtifactManager.List(taskID)})
}

// saveStageDocument keeps the latest output of a stage as a reviewable document.
func (r *Router) saveStageDocument(taskID, stage, model, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	path, err := r.resolveArtifactPath(taskID, stageDocName(stage))
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	now := time.Now()
	header := fmt.Sprintf("<!-- %s · %s · generated %s by %s -->\n\n", taskID, stage, now.Format(time.RFC3339), model)
	doc := content
	if techdoc.Frames(stage) {
		meta := techdoc.Meta{TaskID: taskID, Stage: stage, Model: model, Generated: now}
		if t := r.taskSnapshot(taskID); t != nil {
			meta.Title, meta.Repos = t.Title, t.AssignedRepos
		}
		doc = techdoc.Render(meta, content)
	}
	if err := os.WriteFile(path, []byte(header+doc), 0o644); err != nil {
		return
	}
	r.checkStageDocument(taskID, stage, content)
	r.noteFollowUps(taskID, stage, content)
}
