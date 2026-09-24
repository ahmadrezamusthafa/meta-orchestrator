package circuitbreaker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// FrustrationListener is invoked when the breaker trips.
type FrustrationListener func(payload *types.FrustrationHaltPayload)

// CircuitBreaker monitors task iterations and halts repetitive loops.
type CircuitBreaker struct {
	mu           sync.Mutex
	maxThreshold int
	failures     map[string]*failureRecord
	listener     FrustrationListener
}

type failureRecord struct {
	consecutiveCount int
	lastSignature    string
	lastStackTrace   string
}

// NewCircuitBreaker creates a circuit breaker with specified failure threshold.
func NewCircuitBreaker(threshold int, listener FrustrationListener) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 3
	}
	return &CircuitBreaker{
		maxThreshold: threshold,
		failures:     make(map[string]*failureRecord),
		listener:     listener,
	}
}

// ComputeSignature calculates a stable SHA-256 hash of error details and tool calls.
func ComputeSignature(toolName string, rawArgs string, errorMessage string, exitCode int) string {
	hasher := sha256.New()
	normError := strings.TrimSpace(errorMessage)
	normArgs := strings.TrimSpace(rawArgs)
	hasher.Write([]byte(fmt.Sprintf("%s:%s:%d:%s", toolName, normArgs, exitCode, normError)))
	return hex.EncodeToString(hasher.Sum(nil))
}

// RecordFailure records a failure instance. If threshold is reached, trips breaker.
func (cb *CircuitBreaker) RecordFailure(taskID string, stageID string, signature string, stackTrace string) (bool, *types.FrustrationHaltPayload) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	rec, exists := cb.failures[taskID]
	if !exists {
		rec = &failureRecord{
			consecutiveCount: 1,
			lastSignature:    signature,
			lastStackTrace:   stackTrace,
		}
		cb.failures[taskID] = rec
	} else {
		if rec.lastSignature == signature {
			rec.consecutiveCount++
		} else {
			// Different error signature, reset consecutive counter
			rec.consecutiveCount = 1
			rec.lastSignature = signature
		}
		rec.lastStackTrace = stackTrace
	}

	if rec.consecutiveCount >= cb.maxThreshold {
		payload := &types.FrustrationHaltPayload{
			TaskID:           taskID,
			StageID:          stageID,
			ConsecutiveFails: rec.consecutiveCount,
			ErrorSignature:   signature,
			LastStackTrace:   stackTrace,
		}
		if cb.listener != nil {
			cb.listener(payload)
		}
		return true, payload
	}

	return false, nil
}

// RecordSuccess clears failure count for a task upon a successful iteration.
func (cb *CircuitBreaker) RecordSuccess(taskID string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.failures, taskID)
}

// Reset clears state for a specific task.
func (cb *CircuitBreaker) Reset(taskID string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.failures, taskID)
}
