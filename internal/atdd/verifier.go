package atdd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/security"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// RedPhaseVerificationResult details the verified red phase test failure.
type RedPhaseVerificationResult struct {
	TaskID         string    `json:"task_id"`
	Passed         bool      `json:"passed"` // True if tests failed as expected (Red phase verified)
	ExitCode       int       `json:"exit_code"`
	FailureSummary string    `json:"failure_summary"`
	VerifiedAt     time.Time `json:"verified_at"`
}

// RedPhaseVerifier runs test suites and confirms expected failure signatures.
type RedPhaseVerifier struct {
	lockMgr *security.WriteLockManager
}

// NewRedPhaseVerifier creates a verifier.
func NewRedPhaseVerifier(lockMgr *security.WriteLockManager) *RedPhaseVerifier {
	return &RedPhaseVerifier{lockMgr: lockMgr}
}

// VerifyRedPhase evaluates test output to confirm expected business logic assertion failure.
func (v *RedPhaseVerifier) VerifyRedPhase(ctx context.Context, taskID string, exitCode int, output string) (*RedPhaseVerificationResult, error) {
	if exitCode == 0 {
		return &RedPhaseVerificationResult{
			TaskID:         taskID,
			Passed:         false,
			ExitCode:       exitCode,
			FailureSummary: "Invalid Red Phase: test suite passed unexpectedly before implementation code was written",
			VerifiedAt:     time.Now(),
		}, fmt.Errorf("red phase assertion failed: tests passed with exit code 0")
	}

	lowerOut := strings.ToLower(output)

	// Guard against syntax errors or harness crash
	if strings.Contains(lowerOut, "syntaxerror") || strings.Contains(lowerOut, "parseerror") || strings.Contains(lowerOut, "cannot find module") {
		return &RedPhaseVerificationResult{
			TaskID:         taskID,
			Passed:         false,
			ExitCode:       exitCode,
			FailureSummary: "Test harness broken: syntax error or missing module in test file",
			VerifiedAt:     time.Now(),
		}, fmt.Errorf("test suite rejected: harness crashed with syntax or module error")
	}

	// Legitimate business assertion failure (e.g. element not found, expected 200 got 404, assertion failed)
	res := &RedPhaseVerificationResult{
		TaskID:         taskID,
		Passed:         true,
		ExitCode:       exitCode,
		FailureSummary: "Verified genuine Red Phase test failure (unimplemented feature)",
		VerifiedAt:     time.Now(),
	}

	// Unlock write lock on source tree
	if v.lockMgr != nil {
		v.lockMgr.UnlockTask(taskID)
	}

	_ = types.TaskStateRunning
	return res, nil
}
