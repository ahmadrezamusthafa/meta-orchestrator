package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// maxArtifactBytes caps a text artifact returned inline.
const maxArtifactBytes = 2 << 20

var safeTaskID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var artifactContentTypes = map[string]string{
	".mp4": "video/mp4", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
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
	return filepath.Join(r.cfg.RootDir, ".sdlc", "artifacts", taskID)
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
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", taskID+"-"+filepath.Base(path)))
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
		"filename":    filepath.ToSlash(strings.TrimPrefix(path, r.artifactDir(taskID)+string(filepath.Separator))),
		"content":     string(content),
		"modified_at": st.ModTime(),
	})
}

// artifactEntry describes one file in a task's artifact directory.
type artifactEntry struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"` // "stage_output", "document", "media", "other"
	StageID    string    `json:"stage_id,omitempty"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

// handleTaskArtifacts serves GET /api/v1/tasks/{id}/artifacts: every file the task has produced.
func (r *Router) handleTaskArtifacts(w http.ResponseWriter, req *http.Request, taskID string) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required for artifacts")
		return
	}
	base := r.artifactDir(taskID)
	entries := []artifactEntry{}
	_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(base, path)
		rel = filepath.ToSlash(rel)
		e := artifactEntry{Path: rel, Name: d.Name(), Size: info.Size(), ModifiedAt: info.ModTime(), Kind: "other"}
		ext := strings.ToLower(filepath.Ext(path))
		switch {
		case strings.HasPrefix(rel, "stages/") && ext == ".md":
			e.Kind = "stage_output"
			stage := strings.TrimSuffix(d.Name(), ".md")
			if i := strings.Index(stage, "-"); i > 0 {
				stage = stage[i+1:]
			}
			e.StageID = stage
		case ext == ".md" || ext == ".txt" || ext == ".json" || ext == ".yaml" || ext == ".yml":
			e.Kind = "document"
		case artifactContentTypes[ext] != "":
			e.Kind = "media"
		}
		entries = append(entries, e)
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": taskID, "artifacts": entries})
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
	header := fmt.Sprintf("<!-- %s · %s · generated %s by %s -->\n\n", taskID, stage, time.Now().Format(time.RFC3339), model)
	_ = os.WriteFile(path, []byte(header+content), 0o644)
}
