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
	lockMgr.LockTask(taskID, t.TempDir())
	ctx := context.Background()

	if res, err := verifier.VerifyRedPhase(ctx, taskID, 0, "All 5 tests passed"); err == nil || res.Passed {
		t.Fatalf("expected error when tests unexpectedly pass in Red Phase")
	}
	if !lockMgr.IsLocked(taskID) {
		t.Errorf("task should remain locked after failed verification")
	}
	for _, out := range []string{
		"SyntaxError: Unexpected token '{' at checkout.spec.ts:12",
		"Error: No tests found.\nMake sure that arguments are regular expressions",
		"sh: playwright: command not found",
		"ModuleNotFoundError: No module named 'app'",
		"?   \tgithub.com/x/y\t[no test files]",
	} {
		if res, err := verifier.VerifyRedPhase(ctx, taskID, 1, out); err == nil || res.Passed {
			t.Errorf("harness failure %q should not verify", out)
		}
	}
	if res, err := verifier.VerifyRedPhase(ctx, taskID, 127, ""); err == nil || res.Passed {
		t.Errorf("exit 127 should not verify")
	}
	if !lockMgr.IsLocked(taskID) {
		t.Fatal("still locked after broken runs")
	}

	res, err := verifier.VerifyRedPhase(ctx, taskID, 1, "Error: expect(locator('#order-confirmed')).toBeVisible() failed (timeout 5000ms)")
	if err != nil || !res.Passed {
		t.Fatalf("expected genuine assertion failure to pass verification: %v", err)
	}
	if lockMgr.IsLocked(taskID) {
		t.Errorf("task should be unlocked after successful Red Phase verification")
	}
}

func TestIsTestCommand(t *testing.T) {
	for cmd, want := range map[string]bool{
		"npx playwright test tests/e2e":                 true,
		"cd web && npm test -- --run":                   true,
		"go test ./internal/... 2>&1 | tail":            true,
		"pnpm run test:e2e":                             true,
		"bundle exec rspec spec/requests":               true,
		"./gradlew test":                                true,
		"python -m pytest -q":                           true,
		"cat tests/e2e/checkout.spec.ts":                false,
		"go build ./...":                                false,
		"npm install":                                   false,
		"grep -rn 'test' internal/":                     false,
		"git log --oneline -- tests/e2e/latest.spec.ts": false,
	} {
		if got := IsTestCommand(cmd); got != want {
			t.Errorf("IsTestCommand(%q) = %v, want %v", cmd, got, want)
		}
	}
}
