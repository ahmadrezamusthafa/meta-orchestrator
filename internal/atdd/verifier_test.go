package atdd

import (
	"context"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/security"
)

func TestRedPhaseVerification(t *testing.T) {
	lockMgr := security.NewWriteLockManager()
	verifier := NewRedPhaseVerifier(lockMgr)

	taskID := "task-red-01"
	lockMgr.LockTask(taskID)
	ctx := context.Background()

	// 1. Tests pass before implementation -> MUST FAIL Red Phase
	_, err := verifier.VerifyRedPhase(ctx, taskID, 0, "All 5 tests passed")
	if err == nil {
		t.Fatalf("expected error when tests unexpectedly pass in Red Phase")
	}
	if !lockMgr.IsLocked(taskID) {
		t.Errorf("task should remain locked after failed verification")
	}

	// 2. Tests fail due to syntax error -> MUST FAIL Red Phase
	_, err = verifier.VerifyRedPhase(ctx, taskID, 1, "SyntaxError: Unexpected token '{' at checkout.spec.ts:12")
	if err == nil {
		t.Fatalf("expected error when tests fail with syntax error")
	}

	// 3. Genuine feature failure (expected element not found) -> MUST PASS Red Phase and unlock
	res, err := verifier.VerifyRedPhase(ctx, taskID, 1, "Error: expect(locator('#order-confirmed')).toBeVisible() failed (timeout 5000ms)")
	if err != nil || !res.Passed {
		t.Fatalf("expected genuine assertion failure to pass verification: %v", err)
	}

	// Verify write-lock is released
	if lockMgr.IsLocked(taskID) {
		t.Errorf("task should be unlocked after successful Red Phase verification")
	}
}
