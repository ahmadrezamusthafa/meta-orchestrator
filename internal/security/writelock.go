package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ErrWriteLockActive is returned when a write is attempted while the workspace is write-locked.
var ErrWriteLockActive = errors.New("operation rejected: production source is write-locked during the ATDD red phase — write only tests, specs, fixtures or .sdlc files")

// WriteLockManager tracks and enforces write-lock state per task. A locked task may write only
// test files inside its workspace root.
type WriteLockManager struct {
	mu    sync.RWMutex
	roots map[string]string // task id → workspace root
}

// NewWriteLockManager creates a write-lock manager.
func NewWriteLockManager() *WriteLockManager {
	return &WriteLockManager{roots: make(map[string]string)}
}

// LockTask write-locks the task's workspace rooted at root.
func (m *WriteLockManager) LockTask(taskID, root string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roots[taskID] = realPath(root)
}

// UnlockTask releases the task's write lock.
func (m *WriteLockManager) UnlockTask(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.roots, taskID)
}

// IsLocked reports whether the task's workspace is write-locked.
func (m *WriteLockManager) IsLocked(taskID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.roots[taskID]
	return ok
}

func (m *WriteLockManager) root(taskID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.roots[taskID]
	return r, ok
}

// CheckWritePermitted reports whether the task may modify targetPath. A relative path is resolved
// against dir, or the workspace root when dir is empty. While locked, only test files inside the
// workspace may be written: anything outside it, and every non-test file, is refused.
func (m *WriteLockManager) CheckWritePermitted(taskID, dir, targetPath string) error {
	root, locked := m.root(taskID)
	if !locked {
		return nil
	}
	rel, err := relativeTo(root, dir, targetPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWriteLockActive, err)
	}
	if !IsTestPath(rel) {
		return fmt.Errorf("%w: '%s' is not a test file", ErrWriteLockActive, targetPath)
	}
	return nil
}

// relativeTo resolves target (relative to dir, itself relative to root) to a slash path inside root.
func relativeTo(root, dir, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("empty path")
	}
	if strings.HasPrefix(target, "~") {
		return "", fmt.Errorf("'%s' is outside the workspace", target)
	}
	base := root
	if dir != "" {
		if filepath.IsAbs(dir) {
			base = realPath(dir)
		} else {
			base = filepath.Join(root, dir)
		}
	}
	abs := filepath.FromSlash(target)
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(base, abs)
	}
	abs = realPath(filepath.Clean(abs))
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("'%s' is outside the workspace", target)
	}
	return filepath.ToSlash(rel), nil
}

// realPath resolves symlinks in the longest existing prefix of p, so /var and /private/var (or a
// symlinked checkout) compare equal. Missing trailing elements are kept as given.
func realPath(p string) string {
	p = filepath.Clean(p)
	var rest []string
	for cur := p; ; cur = filepath.Dir(cur) {
		if _, err := os.Lstat(cur); err == nil {
			if resolved, err := filepath.EvalSymlinks(cur); err == nil {
				return filepath.Join(append([]string{resolved}, rest...)...)
			}
			return p
		}
		if filepath.Dir(cur) == cur {
			return p
		}
		rest = append([]string{filepath.Base(cur)}, rest...)
	}
}

// testDirs are directory names whose contents are tests, fixtures or orchestrator artifacts.
var testDirs = map[string]bool{
	"test": true, "tests": true, "__tests__": true, "e2e": true, "spec": true, "specs": true, "testdata": true,
	"fixtures": true, "__fixtures__": true, "__mocks__": true, "acceptance": true, ".sdlc": true,
}

// testFileSuffixes and testConfigPrefixes recognise test files by name.
var (
	testFileSuffixes = []string{
		"_test.go", "_test.py", "_test.dart", "_spec.rb", "_test.rb", ".feature",
	}
	// JVM test classes are named in CamelCase (OrderTest.java); matched case-sensitively so
	// latest.java is not a test.
	jvmTestSuffixes    = []string{"Test.java", "Tests.java", "IT.java", "Test.kt", "Tests.kt", "Spec.kt", "Spec.groovy"}
	testInfixes        = []string{".test.", ".spec.", ".e2e."}
	testConfigPrefixes = []string{"playwright.config.", "cypress.config.", "vitest.config.", "jest.config."}
)

// IsTestPath reports whether a workspace-relative slash path is a test, spec, fixture or .sdlc
// file. Everything else is production source.
func IsTestPath(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, dir := range parts[:len(parts)-1] {
		if testDirs[strings.ToLower(dir)] {
			return true
		}
	}
	for _, s := range jvmTestSuffixes {
		if strings.HasSuffix(parts[len(parts)-1], s) {
			return true
		}
	}
	name := strings.ToLower(parts[len(parts)-1])
	if strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py") {
		return true
	}
	for _, s := range testFileSuffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	for _, s := range testInfixes {
		if strings.Contains(name, s) {
			return true
		}
	}
	for _, s := range testConfigPrefixes {
		if strings.HasPrefix(name, s) {
			return true
		}
	}
	return false
}
