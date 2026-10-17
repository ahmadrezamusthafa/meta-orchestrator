package circuitbreaker

import (
	"sync/atomic"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestCircuitBreakerThreshold(t *testing.T) {
	var tripCount int32
	cb := NewCircuitBreaker(3, func(p *types.FrustrationHaltPayload) { atomic.AddInt32(&tripCount, 1) })

	sig := ComputeSignature("task_implementation", "Error: Element not found: #checkout-btn")
	for i := 1; i <= 2; i++ {
		if tripped, p := cb.RecordFailure("task-loop-1", "task_implementation", sig, "trace", 0); tripped || p != nil {
			t.Fatalf("tripped on attempt %d", i)
		}
	}
	tripped, p := cb.RecordFailure("task-loop-1", "task_implementation", sig, "trace", 0)
	if !tripped || p == nil || p.ConsecutiveFails != 3 || p.ErrorSignature != sig || p.LastStackTrace != "trace" {
		t.Fatalf("expected trip on attempt 3, got %v %+v", tripped, p)
	}
	if atomic.LoadInt32(&tripCount) != 1 {
		t.Errorf("listener called %d times", tripCount)
	}
	if cb.Count("task-loop-1") != 0 {
		t.Errorf("a trip should restart the count")
	}

	cb.RecordFailure("task-loop-1", "task_implementation", sig, "trace", 0)
	cb.Reset("task-loop-1")
	if tripped, _ := cb.RecordFailure("task-loop-1", "task_implementation", sig, "trace", 2); tripped {
		t.Fatalf("expected count cleared after Reset")
	}
	if tripped, _ := cb.RecordFailure("task-loop-1", "task_implementation", sig, "trace", 2); !tripped {
		t.Fatalf("expected per-call threshold 2 to trip")
	}
}

func TestCircuitBreakerDifferentErrorsRestartCount(t *testing.T) {
	cb := NewCircuitBreaker(3, nil)
	sig1 := ComputeSignature("task_implementation", "cannot find package")
	sig2 := ComputeSignature("task_implementation", "syntax error")
	cb.RecordFailure("task-diff", "task_implementation", sig1, "", 0)
	cb.RecordFailure("task-diff", "task_implementation", sig1, "", 0)
	if tripped, _ := cb.RecordFailure("task-diff", "task_implementation", sig2, "", 0); tripped {
		t.Fatalf("expected no trip when signature changes")
	}
}

func TestSignatureIgnoresVolatileDetails(t *testing.T) {
	a := ComputeSignature("e2e_validation", "request 3f2b1c9a-1111-2222-3333-444455556666 failed after 1.52s at 2026-10-17T10:00:01Z (line 42)")
	b := ComputeSignature("e2e_validation", "request 9a9a9a9a-aaaa-bbbb-cccc-ddddeeeeffff failed after 300ms at 2026-10-18 11:22:33 (line 7)")
	if a != b {
		t.Fatalf("signatures differ for the same failure:\n%s\n%s", NormalizeError("request 3f2b1c9a-1111-2222-3333-444455556666 failed after 1.52s"), NormalizeError("request 9a9a9a9a-aaaa-bbbb-cccc-ddddeeeeffff failed after 300ms"))
	}
	if a == ComputeSignature("task_implementation", "request 3f2b1c9a-1111-2222-3333-444455556666 failed after 1.52s at 2026-10-17T10:00:01Z (line 42)") {
		t.Fatalf("the stage must be part of the signature")
	}
	if ComputeSignature("x", "permission denied") == ComputeSignature("x", "provider unavailable") {
		t.Fatalf("different errors must differ")
	}
}
