package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestToolInstallationAndRollback(t *testing.T) {
	tmpDir := t.TempDir()
	var progressSteps []string

	tm := NewToolManager(tmpDir, func(toolID string, stage string, pct int, msg string) {
		progressSteps = append(progressSteps, stage)
	})

	ctx := context.Background()

	// 1. Install tool v1.0.0
	err := tm.InstallTool(ctx, "bmad-methodology", "1.3.1")
	if err != nil {
		t.Fatalf("failed to install v1.3.1: %v", err)
	}

	pkg, _ := tm.GetPackage("bmad-methodology")
	if pkg.ActiveVersion != "1.3.1" {
		t.Errorf("expected active version 1.3.1, got %s", pkg.ActiveVersion)
	}

	activeLink := filepath.Join(tmpDir, "active", "bmad-methodology")
	target, err := os.Readlink(activeLink)
	if err != nil || filepath.Base(target) != "1.3.1" {
		t.Errorf("expected symlink pointing to 1.3.1, got %s", target)
	}

	// 2. Upgrade to v1.4.2
	err = tm.InstallTool(ctx, "bmad-methodology", "1.4.2")
	if err != nil {
		t.Fatalf("failed to install v1.4.2: %v", err)
	}
	if pkg.ActiveVersion != "1.4.2" {
		t.Errorf("expected active version 1.4.2, got %s", pkg.ActiveVersion)
	}

	// 3. Rollback to v1.3.1 (< 1 second)
	start := time.Now()
	err = tm.RollbackTool("bmad-methodology", "1.3.1")
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}
	if duration > 1*time.Second {
		t.Errorf("rollback took %v, exceeded 1s target", duration)
	}
	if pkg.ActiveVersion != "1.3.1" {
		t.Errorf("expected active version rolled back to 1.3.1, got %s", pkg.ActiveVersion)
	}

	target, _ = os.Readlink(activeLink)
	if filepath.Base(target) != "1.3.1" {
		t.Errorf("expected symlink swapped back to 1.3.1, got %s", target)
	}
}

func TestBestFitResolution(t *testing.T) {
	tm := NewToolManager(t.TempDir(), nil)

	res, err := tm.ResolveBestFit("treesitter-parsers")
	if err != nil {
		t.Fatalf("failed to resolve best fit for treesitter: %v", err)
	}

	if res.ResolvedVersion != "0.22.6" {
		t.Errorf("expected treesitter 0.22.6, got %s", res.ResolvedVersion)
	}
	if !res.IsCompatible {
		t.Errorf("expected compatible flag to be true")
	}

	all, err := tm.ResolveAllBestFit()
	if err != nil || len(all) < 4 {
		t.Errorf("expected at least 4 best-fit resolutions, got %d", len(all))
	}
}
