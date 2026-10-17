//go:build !windows

package uat

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// A runner that times out must stop everything it started (npm's scripts, node's Chromium) and
// return: a grandchild that inherited the output pipe must neither outlive the run nor block it.
func TestCommandTimeoutStopsTheWholeProcessTree(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := command(ctx, t.TempDir(), "sh", "-c", "sleep 30 & sleep 30")
	if err == nil {
		t.Fatal("a timed-out command must fail")
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("the timed-out command blocked for %s: its child kept the output pipe open", d)
	}
}
