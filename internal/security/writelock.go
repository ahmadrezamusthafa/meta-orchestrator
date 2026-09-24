package security

import (
	"fmt"
	"strings"
	"sync"
)

// ErrWriteLockActive is returned when a write is attempted while the workspace is write-locked.
var ErrWriteLockActive = fmt.Errorf("operation rejected: workspace source tree is strictly write-locked (ATDD Red Phase verification required)")

// WriteLockManager tracks and enforces write-lock state per task.
type WriteLockManager struct {
	mu           sync.RWMutex
	lockedTasks  map[string]bool
	protectedDirs []string
}

// NewWriteLockManager creates a write-lock manager.
func NewWriteLockManager() *WriteLockManager {
	return &WriteLockManager{
		lockedTasks:   make(map[string]bool),
		protectedDirs: []string{"src/", "app/", "lib/", "internal/", "pkg/"},
	}
}

// LockTask activates strict write-locking for task.
func (m *WriteLockManager) LockTask(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lockedTasks[taskID] = true
}

// UnlockTask releases write-locking (called when Red Phase verification passes).
func (m *WriteLockManager) UnlockTask(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.lockedTasks, taskID)
}

// IsLocked checks whether task workspace is currently locked.
func (m *WriteLockManager) IsLocked(taskID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lockedTasks[taskID]
}

// CheckWritePermitted asserts whether target path can be modified.
func (m *WriteLockManager) CheckWritePermitted(taskID string, targetPath string) error {
	if !m.IsLocked(taskID) {
		return nil
	}

	normPath := strings.ToLower(strings.ReplaceAll(targetPath, "\\", "/"))

	// Tests and artifacts are always permitted to write during Red Phase
	if strings.Contains(normPath, "test") ||
		strings.Contains(normPath, "spec") ||
		strings.Contains(normPath, ".sdlc/") ||
		strings.Contains(normPath, ".scratch/") {
		return nil
	}

	for _, dir := range m.protectedDirs {
		if strings.Contains(normPath, dir) {
			return fmt.Errorf("%w: cannot write to '%s'", ErrWriteLockActive, targetPath)
		}
	}

	return nil
}
