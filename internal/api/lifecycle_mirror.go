package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// The JSON board (TaskStorePath) is the source of truth. When RouterConfig.TaskStore is set (the
// daemon sets a Redis store when REDIS_ADDR is configured) every persisted task snapshot is mirrored
// there, and at startup active tasks the board lacks — e.g. after an unreadable board file was moved
// aside — are restored from the mirror.

const mirrorTimeout = 3 * time.Second

// recoverFromMirror restores active tasks that are in the mirror but not on the board. Caller must
// not hold r.mu. It runs before reconcileIdleRunning, which suspends restored RUNNING tasks.
func (r *Router) recoverFromMirror() {
	store := r.cfg.TaskStore
	if store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), mirrorTimeout)
	defer cancel()
	machines, skipped, err := fsm.RecoverActiveTasks(ctx, store, r.cfg.WorkflowReg)
	if err != nil {
		fmt.Printf("[Recovery] Task mirror unavailable: %v\n", err)
		return
	}
	var restored []string
	r.mu.Lock()
	for _, m := range machines {
		t := m.GetTask()
		if _, onBoard := r.tasks[t.ID]; onBoard {
			continue
		}
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		task := t
		r.tasks[t.ID] = &task
		if n := taskSeqOf(t.ID); n > r.taskSeq {
			r.taskSeq = n
		}
		restored = append(restored, t.ID)
	}
	r.mu.Unlock()
	for id, why := range skipped {
		fmt.Printf("[Recovery] Mirror holds task %s that cannot be restored: %v\n", id, why)
	}
	fmt.Printf("[Recovery] Task mirror: %d active, %d restored to the board\n", len(machines), len(restored))
	for _, id := range restored {
		r.addEntry(id, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: "Restored from the task state mirror: the board file did not have this task."})
	}
	if len(restored) > 0 {
		r.saveBoardNow()
	}
	r.persistMu.Lock()
	r.mirrorTasks(r.mirrorSnapshot()) // bring the mirror up to date with the board
	r.persistMu.Unlock()
}

// mirrorSnapshot encodes every task for the mirror. It returns nil when no mirror is configured.
func (r *Router) mirrorSnapshot() map[string][]byte {
	if r.cfg.TaskStore == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	snap := make(map[string][]byte, len(r.tasks))
	for id, t := range r.tasks {
		if data, err := json.Marshal(t); err == nil {
			snap[id] = data
		}
	}
	return snap
}

// mirrorTasks writes changed task snapshots to the mirror and removes deleted tasks. Caller holds
// persistMu (or is the only writer, at startup); never r.mu.
func (r *Router) mirrorTasks(snap map[string][]byte) {
	store := r.cfg.TaskStore
	if store == nil || snap == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), mirrorTimeout)
	defer cancel()
	var failed error
	for id, data := range snap {
		if bytes.Equal(r.mirrored[id], data) {
			continue
		}
		var t types.Task
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		if err := store.SaveTask(ctx, &t); err != nil {
			failed = err
			continue
		}
		r.mirrored[id] = data
	}
	for id := range r.mirrored {
		if _, ok := snap[id]; ok {
			continue
		}
		if err := store.DeleteTask(ctx, id); err != nil {
			failed = err
			continue
		}
		delete(r.mirrored, id)
	}
	if failed != nil && !r.mirrorFailing {
		fmt.Printf("[Recovery] Task mirror write failed (will retry): %v\n", failed)
	}
	r.mirrorFailing = failed != nil
}
