package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RollbackTool atomically switches the active tool version symlink to targetVersion.
func (tm *ToolManager) RollbackTool(toolID string, targetVersion string) error {
	start := time.Now()

	pkg, err := tm.GetPackage(toolID)
	if err != nil {
		return err
	}

	targetDir := filepath.Join(tm.baseDir, "versions", toolID, targetVersion)
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("version %s for tool %s not found in version store", targetVersion, toolID)
	}

	activeLink := filepath.Join(tm.baseDir, "active", toolID)
	tempLink := filepath.Join(tm.baseDir, "active", toolID+".tmp")

	// Create temporary symlink pointing to target version
	_ = os.Remove(tempLink)
	if err := os.Symlink(targetDir, tempLink); err != nil {
		return fmt.Errorf("failed to create atomic temp symlink: %w", err)
	}

	// Atomic rename to replace active symlink
	if err := os.Rename(tempLink, activeLink); err != nil {
		_ = os.Remove(tempLink)
		return fmt.Errorf("failed atomic rollback swap: %w", err)
	}

	tm.mu.Lock()
	pkg.ActiveVersion = targetVersion
	pkg.Status = ToolStatusActive
	tm.mu.Unlock()

	elapsed := time.Since(start)
	if elapsed > 1*time.Second {
		fmt.Printf("WARNING: Rollback took longer than 1s (%v)\n", elapsed)
	}

	return nil
}
