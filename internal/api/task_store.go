package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// taskStoreVersion is bumped when the on-disk board format changes incompatibly.
const taskStoreVersion = 1

// taskPersistInterval bounds how long a state change made outside an explicit save (stage runs,
// gate reviews, token accounting) can stay unsaved.
const taskPersistInterval = 2 * time.Second

// boardFile is the durable Kanban board: every task plus the bookkeeping that must survive a
// daemon restart.
type boardFile struct {
	Version           int           `json:"version"`
	NextTaskSeq       int           `json:"next_task_seq"`
	Tasks             []*types.Task `json:"tasks"`
	DismissedJiraKeys []string      `json:"dismissed_jira_keys,omitempty"`
}

// loadBoard restores the board from TaskStorePath. A missing file is an empty board; an unreadable
// one is moved aside so a corrupt write never silently wipes the operator's tasks.
func (r *Router) loadBoard() {
	path := r.cfg.TaskStorePath
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("[Tasks] Failed to read board %s: %v\n", path, err)
		}
		return
	}
	var bf boardFile
	if err := json.Unmarshal(data, &bf); err != nil {
		backup := fmt.Sprintf("%s.corrupt-%d", path, time.Now().Unix())
		_ = os.Rename(path, backup)
		fmt.Printf("[Tasks] Board file was unreadable (%v); moved to %s and starting empty\n", err, backup)
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range bf.Tasks {
		if t == nil || t.ID == "" {
			continue
		}
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		r.tasks[t.ID] = t
		if n := taskSeqOf(t.ID); n > r.taskSeq {
			r.taskSeq = n
		}
	}
	if bf.NextTaskSeq-1 > r.taskSeq {
		r.taskSeq = bf.NextTaskSeq - 1
	}
	for _, k := range bf.DismissedJiraKeys {
		r.dismissedJira[strings.ToUpper(k)] = true
	}
	r.lastPersisted = data
	fmt.Printf("[Tasks] Restored %d tasks from %s\n", len(r.tasks), path)
}

// taskSeqOf extracts N from "TASK-N"; 0 when the ID has another shape.
func taskSeqOf(id string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "TASK-"))
	if err != nil || !strings.HasPrefix(id, "TASK-") {
		return 0
	}
	return n
}

// nextTaskIDLocked allocates a task ID that is never reused, even after deletes. Caller holds r.mu.
func (r *Router) nextTaskIDLocked() string {
	for {
		r.taskSeq++
		id := fmt.Sprintf("TASK-%d", r.taskSeq)
		if _, taken := r.tasks[id]; !taken {
			return id
		}
	}
}

// persistBoard writes the board atomically when it changed since the last write.
func (r *Router) persistBoard() error {
	path := r.cfg.TaskStorePath
	if path == "" {
		return nil
	}
	r.mu.RLock()
	bf := boardFile{Version: taskStoreVersion, NextTaskSeq: r.taskSeq + 1, Tasks: make([]*types.Task, 0, len(r.tasks))}
	for _, t := range r.tasks {
		bf.Tasks = append(bf.Tasks, t)
	}
	sort.Slice(bf.Tasks, func(i, j int) bool {
		if !bf.Tasks[i].CreatedAt.Equal(bf.Tasks[j].CreatedAt) {
			return bf.Tasks[i].CreatedAt.Before(bf.Tasks[j].CreatedAt)
		}
		return bf.Tasks[i].ID < bf.Tasks[j].ID
	})
	for k := range r.dismissedJira {
		bf.DismissedJiraKeys = append(bf.DismissedJiraKeys, k)
	}
	sort.Strings(bf.DismissedJiraKeys)
	data, err := json.MarshalIndent(bf, "", "  ")
	r.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("encode board: %w", err)
	}

	r.persistMu.Lock()
	defer r.persistMu.Unlock()
	if bytes.Equal(data, r.lastPersisted) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create board dir: %w", err)
	}
	tmp := path + ".tmp"
	// 0600: imported issue descriptions can carry customer details.
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write board: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit board: %w", err)
	}
	r.lastPersisted = data
	return nil
}

// saveBoardNow persists immediately after operator-visible changes (create, import, delete) so a
// crash right after the response can't lose them.
func (r *Router) saveBoardNow() {
	if err := r.persistBoard(); err != nil {
		fmt.Printf("[Tasks] Failed to save board: %v\n", err)
	}
}

// startBoardPersister snapshots the board periodically; persistBoard skips unchanged boards.
func (r *Router) startBoardPersister() {
	if r.cfg.TaskStorePath == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(taskPersistInterval)
		defer ticker.Stop()
		for {
			select {
			case <-r.stop:
				return
			case <-ticker.C:
				r.saveBoardNow()
				r.saveConsolesNow()
			}
		}
	}()
}

// Close stops background work and flushes the board. Safe to call more than once.
func (r *Router) Close() error {
	r.closeOnce.Do(func() { close(r.stop) })
	r.saveConsolesNow()
	return r.persistBoard()
}

// saveConsolesNow flushes changed task transcripts.
func (r *Router) saveConsolesNow() {
	if err := r.persistConsoles(); err != nil {
		fmt.Printf("[Console] Failed to save transcripts: %v\n", err)
	}
}

// deleteTask removes a task from the board, stopping any live agent turn first. Deleting a
// JIRA-linked task dismisses its issue so the next sync does not bring it back.
func (r *Router) deleteTask(taskID string) (*types.Task, error) {
	r.interruptAndWait(taskID, 5*time.Second)

	r.mu.Lock()
	task, ok := r.tasks[taskID]
	if !ok {
		r.mu.Unlock()
		return nil, errTaskNotFound
	}
	removed := cloneTask(task)
	delete(r.tasks, taskID)
	delete(r.taskProcesses, taskID)
	if key := task.Metadata["jira_key"]; key != "" {
		r.dismissedJira[strings.ToUpper(key)] = true
	}
	// Dependents must not wait forever on a task that no longer exists.
	var released []*types.Task
	for _, other := range r.tasks {
		kept := other.Dependencies[:0]
		dropped := false
		for _, d := range other.Dependencies {
			if d == taskID {
				dropped = true
				continue
			}
			kept = append(kept, d)
		}
		if !dropped {
			continue
		}
		other.Dependencies = kept
		if other.State == types.TaskStateWaitingDependency && r.depsSatisfiedLocked(other) {
			r.setTaskState(other, types.TaskStatePending)
			delete(other.Metadata, "unmet_dependencies")
		}
		other.UpdatedAt = time.Now()
		released = append(released, cloneTask(other))
	}
	r.mu.Unlock()

	r.console.mu.Lock()
	delete(r.console.consoles, taskID)
	delete(r.console.dirty, taskID)
	r.console.mu.Unlock()
	r.removeConsoleFile(taskID)

	if kept := r.removeTaskWorktrees(removed); len(kept) > 0 {
		fmt.Printf("[Tasks] Kept worktrees with uncommitted work for deleted %s: %s\n", taskID, strings.Join(kept, ", "))
	}

	r.saveBoardNow()
	if r.cfg.WSHub != nil {
		r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{Type: types.EventTaskDeleted, TaskID: taskID, Timestamp: time.Now(),
			Payload: map[string]string{"id": taskID}})
	}
	for _, t := range released {
		r.broadcastTask(t)
	}
	return removed, nil
}

// depsSatisfiedLocked reports whether every dependency of t is completed. Caller holds r.mu.
func (r *Router) depsSatisfiedLocked(t *types.Task) bool {
	for _, d := range t.Dependencies {
		if dt, ok := r.tasks[d]; !ok || dt.State != types.TaskStateCompleted {
			return false
		}
	}
	return true
}
