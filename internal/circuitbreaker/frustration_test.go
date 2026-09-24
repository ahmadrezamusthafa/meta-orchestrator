package circuitbreaker

import (
	"sync/atomic"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestCircuitBreakerThreshold(t *testing.T) {
	var tripCount int32

	cb := NewCircuitBreaker(3, func(p *types.FrustrationHaltPayload) {
		atomic.AddInt32(&tripCount, 1)
	})

	sig := ComputeSignature("playwright_test", "--spec=order.spec.ts", "Error: Element not found: #checkout-btn", 1)

	// Attempt 1: Should not trip
	tripped, p := cb.RecordFailure("task-loop-1", "IMPLEMENTATION_GREEN", sig, "stacktrace at line 42")
	if tripped || p != nil {
		t.Fatalf("expected not tripped on attempt 1")
	}

	// Attempt 2: Should not trip
	tripped, p = cb.RecordFailure("task-loop-1", "IMPLEMENTATION_GREEN", sig, "stacktrace at line 42")
	if tripped || p != nil {
		t.Fatalf("expected not tripped on attempt 2")
	}

	// Attempt 3: MUST trip
	tripped, p = cb.RecordFailure("task-loop-1", "IMPLEMENTATION_GREEN", sig, "stacktrace at line 42")
	if !tripped || p == nil {
		t.Fatalf("expected circuit breaker to trip on attempt 3")
	}
	if p.ConsecutiveFails != 3 {
		t.Errorf("expected 3 consecutive fails, got %d", p.ConsecutiveFails)
	}
	if p.ErrorSignature != sig {
		t.Errorf("expected matching signature, got %s", p.ErrorSignature)
	}
	if atomic.LoadInt32(&tripCount) != 1 {
		t.Errorf("expected listener called once, got %d", tripCount)
	}

	// Reset via RecordSuccess
	cb.RecordSuccess("task-loop-1")
	tripped, _ = cb.RecordFailure("task-loop-1", "IMPLEMENTATION_GREEN", sig, "stacktrace")
	if tripped {
		t.Fatalf("expected breaker reset after RecordSuccess")
	}
}

func TestCircuitBreakerNonConsecutiveDifferentErrors(t *testing.T) {
	cb := NewCircuitBreaker(3, nil)

	sig1 := ComputeSignature("go_test", "./...", "cannot find package", 1)
	sig2 := ComputeSignature("go_test", "./...", "syntax error", 2)

	cb.RecordFailure("task-diff", "IMPLEMENTATION_GREEN", sig1, "trace 1")
	cb.RecordFailure("task-diff", "IMPLEMENTATION_GREEN", sig1, "trace 1")

	// Interrupted by different error signature
	tripped, _ := cb.RecordFailure("task-diff", "IMPLEMENTATION_GREEN", sig2, "trace 2")
	if tripped {
		t.Fatalf("expected no trip when signature changes")
	}
}
