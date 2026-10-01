package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// consoleStoreVersion is bumped when the on-disk transcript format changes incompatibly.
const consoleStoreVersion = 1

// consoleFile is one task's durable transcript: the entries plus the provider session, so a
// restarted daemon shows the same Console / AI Reasoning history and resumes the conversation.
type consoleFile struct {
	Version      int                  `json:"version"`
	TaskID       string               `json:"task_id"`
	SessionID    string               `json:"session_id,omitempty"`
	SessionStage string               `json:"session_stage,omitempty"`
	Model        string               `json:"model,omitempty"`
	Entries      []types.ConsoleEntry `json:"entries"`
}

// consoleDir holds one <taskID>.json per task next to the board file. Empty keeps transcripts in
// memory only.
func (r *Router) consoleDir() string {
	if r.cfg.TaskStorePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(r.cfg.TaskStorePath), "consoles")
}

func consolePath(dir, taskID string) string {
	return filepath.Join(dir, taskID+".json")
}

// markDirty queues a task's transcript for the next persist. Caller holds h.mu.
func (h *consoleHub) markDirty(taskID string) {
	if h.dirty == nil {
		h.dirty = map[string]bool{}
	}
	h.dirty[taskID] = true
}

// loadConsoles restores transcripts for tasks on the board. Files of tasks that no longer exist
// are removed; unreadable ones are moved aside. Runs after loadBoard.
func (r *Router) loadConsoles() {
	dir := r.consoleDir()
	if dir == "" {
		return
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("[Console] Failed to read %s: %v\n", dir, err)
		}
		return
	}
	restored := 0
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		taskID := strings.TrimSuffix(name, ".json")
		path := filepath.Join(dir, name)
		if r.taskSnapshot(taskID) == nil {
			_ = os.Remove(path)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("[Console] Failed to read %s: %v\n", path, err)
			continue
		}
		var cf consoleFile
		if err := json.Unmarshal(data, &cf); err != nil {
			backup := fmt.Sprintf("%s.corrupt", path)
			_ = os.Rename(path, backup)
			fmt.Printf("[Console] Transcript %s was unreadable (%v); moved to %s\n", path, err, backup)
			continue
		}
		for i := range cf.Entries {
			settleInterrupted(&cf.Entries[i])
		}
		r.console.mu.Lock()
		c := r.console.get(taskID)
		c.entries, c.sessionID, c.sessionStage, c.model = cf.Entries, cf.SessionID, cf.SessionStage, cf.Model
		r.console.mu.Unlock()
		restored++
	}
	if restored > 0 {
		fmt.Printf("[Console] Restored %d task transcripts from %s\n", restored, dir)
	}
}

// settleInterrupted closes entries that were live when the daemon stopped: no turn or approval
// survives a restart, so they must not render as still running.
func settleInterrupted(e *types.ConsoleEntry) {
	if e.Status == types.ConsoleStreaming {
		e.Status = types.ConsoleCancelled
	}
	if e.Approval != nil && e.Approval.Decision == types.ApprovalPending {
		e.Approval.Decision = types.ApprovalExpired
	}
}

// persistConsoles writes every transcript changed since the last call. A cleared transcript
// removes its file. Failed writes stay queued for the next attempt.
func (r *Router) persistConsoles() error {
	dir := r.consoleDir()
	if dir == "" {
		return nil
	}
	h := r.console
	h.mu.Lock()
	if len(h.dirty) == 0 {
		h.mu.Unlock()
		return nil
	}
	pending := make([]consoleFile, 0, len(h.dirty))
	for taskID := range h.dirty {
		c, ok := h.consoles[taskID]
		if !ok {
			continue // task deleted; deleteTask removed the file
		}
		pending = append(pending, consoleFile{Version: consoleStoreVersion, TaskID: taskID, SessionID: c.sessionID,
			SessionStage: c.sessionStage, Model: c.model, Entries: append([]types.ConsoleEntry(nil), c.entries...)})
	}
	h.dirty = nil
	h.mu.Unlock()

	r.persistMu.Lock()
	defer r.persistMu.Unlock()
	var firstErr error
	for _, cf := range pending {
		if err := writeConsoleFile(dir, cf); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			h.mu.Lock()
			h.markDirty(cf.TaskID)
			h.mu.Unlock()
		}
	}
	return firstErr
}

func writeConsoleFile(dir string, cf consoleFile) error {
	path := consolePath(dir, cf.TaskID)
	if len(cf.Entries) == 0 && cf.SessionID == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove transcript %s: %w", cf.TaskID, err)
		}
		return nil
	}
	data, err := json.Marshal(cf)
	if err != nil {
		return fmt.Errorf("encode transcript %s: %w", cf.TaskID, err)
	}
	// 0700/0600: transcripts carry prompts, code and tool output.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create console dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write transcript %s: %w", cf.TaskID, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit transcript %s: %w", cf.TaskID, err)
	}
	return nil
}

// removeConsoleFile drops a deleted task's transcript from disk.
func (r *Router) removeConsoleFile(taskID string) {
	dir := r.consoleDir()
	if dir == "" {
		return
	}
	if err := os.Remove(consolePath(dir, taskID)); err != nil && !os.IsNotExist(err) {
		fmt.Printf("[Console] Failed to remove transcript of %s: %v\n", taskID, err)
	}
}
