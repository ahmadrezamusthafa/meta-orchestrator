package circuitbreaker

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// FrustrationListener is invoked when the breaker trips.
type FrustrationListener func(payload *types.FrustrationHaltPayload)

// CircuitBreaker halts a task that keeps failing the same way.
type CircuitBreaker struct {
	mu               sync.Mutex
	defaultThreshold int
	failures         map[string]*failureRecord
	listener         FrustrationListener
}

type failureRecord struct {
	consecutiveCount int
	lastSignature    string
}

// NewCircuitBreaker creates a circuit breaker. defaultThreshold applies when RecordFailure is not
// given one of its own; a non-positive value means 3.
func NewCircuitBreaker(defaultThreshold int, listener FrustrationListener) *CircuitBreaker {
	if defaultThreshold <= 0 {
		defaultThreshold = 3
	}
	return &CircuitBreaker{
		defaultThreshold: defaultThreshold,
		failures:         make(map[string]*failureRecord),
		listener:         listener,
	}
}

// volatile matches the parts of an error message that change between otherwise identical failures:
// UUIDs, hex ids, timestamps, durations, line:column positions and other numbers.
var volatile = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`), "<id>"},
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[t ]\d{2}:\d{2}:\d{2}(\.\d+)?(z|[+-]\d{2}:?\d{2})?`), "<time>"},
	{regexp.MustCompile(`\b\d{1,2}:\d{2}:\d{2}(\.\d+)?\b`), "<time>"},
	{regexp.MustCompile(`\b[0-9a-f]{7,}\b`), "<hex>"},
	{regexp.MustCompile(`\b\d+(\.\d+)?\s*(ns|µs|us|ms|s|m|h|sec|secs|seconds|minutes)\b`), "<dur>"},
	{regexp.MustCompile(`\d+`), "<n>"},
}

// NormalizeError strips the volatile bits of an error message so repeats of one failure compare equal.
func NormalizeError(msg string) string {
	s := strings.ToLower(strings.TrimSpace(msg))
	for _, v := range volatile {
		s = v.re.ReplaceAllString(s, v.repl)
	}
	return strings.Join(strings.Fields(s), " ")
}

// ComputeSignature is a stable hash of where a failure happened and its normalized error.
func ComputeSignature(stageID, errorMessage string) string {
	sum := sha256.Sum256([]byte(stageID + "\x00" + NormalizeError(errorMessage)))
	return hex.EncodeToString(sum[:])
}

// RecordFailure records a failure. A different signature than the previous failure restarts the
// count. When the count reaches threshold (non-positive uses the default) the breaker trips: the
// listener is called, the count restarts, and the halt payload is returned.
func (cb *CircuitBreaker) RecordFailure(taskID, stageID, signature, detail string, threshold int) (bool, *types.FrustrationHaltPayload) {
	if threshold <= 0 {
		threshold = cb.defaultThreshold
	}
	cb.mu.Lock()
	rec, exists := cb.failures[taskID]
	if !exists || rec.lastSignature != signature {
		rec = &failureRecord{lastSignature: signature}
		cb.failures[taskID] = rec
	}
	rec.consecutiveCount++
	if rec.consecutiveCount < threshold {
		cb.mu.Unlock()
		return false, nil
	}
	payload := &types.FrustrationHaltPayload{
		TaskID:           taskID,
		StageID:          stageID,
		ConsecutiveFails: rec.consecutiveCount,
		ErrorSignature:   signature,
		LastStackTrace:   detail,
	}
	delete(cb.failures, taskID) // the operator's next attempt starts a fresh count
	cb.mu.Unlock()
	if cb.listener != nil {
		cb.listener(payload)
	}
	return true, payload
}

// Count returns the consecutive identical failures recorded for a task.
func (cb *CircuitBreaker) Count(taskID string) int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if rec, ok := cb.failures[taskID]; ok {
		return rec.consecutiveCount
	}
	return 0
}

// Reset clears the failure count of a task, after a success or an operator decision.
func (cb *CircuitBreaker) Reset(taskID string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.failures, taskID)
}
