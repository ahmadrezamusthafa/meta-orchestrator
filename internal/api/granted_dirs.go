package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Folders outside the workspace the operator granted a task's agent access to. A grant answers a
// permission prompt for a blocked path, reaches the running session at once (addDirectories) and
// every later turn of the task through --add-dir.

// grantDirFor is the folder a grant for blockedPath covers: the path itself when it is a directory,
// else its parent. Too-broad folders (the filesystem root, the home folder) are never offered:
// those requests are allowed once instead.
func grantDirFor(blockedPath string) string {
	p := strings.TrimSpace(blockedPath)
	if p == "" || !filepath.IsAbs(p) {
		return ""
	}
	p = filepath.Clean(p)
	if st, err := os.Stat(p); err != nil || !st.IsDir() {
		p = filepath.Dir(p)
	}
	if p == filepath.Dir(p) {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == p {
		return ""
	}
	return p
}

// pathWithin reports whether p is dir or lies inside it.
func pathWithin(p, dir string) bool {
	if p == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(dir, filepath.Clean(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// grantedDirFor returns the granted folder that covers p, or "".
func grantedDirFor(p string, dirs []string) string {
	for _, d := range dirs {
		if pathWithin(p, d) {
			return d
		}
	}
	return ""
}

func (r *Router) taskGrantedDirs(taskID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.tasks[taskID]
	if t == nil || t.Metadata["granted_dirs"] == "" {
		return nil
	}
	var dirs []string
	_ = json.Unmarshal([]byte(t.Metadata["granted_dirs"]), &dirs)
	return dirs
}

// addGrantedDir records a folder grant on the task; a folder already covered by a grant is skipped.
func (r *Router) addGrantedDir(taskID, dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tasks[taskID]
	if t == nil || dir == "" {
		return
	}
	var dirs []string
	if raw := t.Metadata["granted_dirs"]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &dirs)
	}
	if grantedDirFor(dir, dirs) != "" {
		return
	}
	// A broader grant replaces the narrower ones it covers.
	kept := dirs[:0]
	for _, d := range dirs {
		if !pathWithin(d, dir) {
			kept = append(kept, d)
		}
	}
	data, _ := json.Marshal(append(kept, dir))
	if t.Metadata == nil {
		t.Metadata = map[string]string{}
	}
	t.Metadata["granted_dirs"] = string(data)
}
