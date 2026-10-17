package atdd

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/security"
)

// RedPhaseVerificationResult details the verified red phase test failure.
type RedPhaseVerificationResult struct {
	TaskID         string    `json:"task_id"`
	Passed         bool      `json:"passed"` // True if tests failed as expected (Red phase verified)
	ExitCode       int       `json:"exit_code"`
	FailureSummary string    `json:"failure_summary"`
	VerifiedAt     time.Time `json:"verified_at"`
}

// RedPhaseVerifier judges an acceptance-test run and releases the task's write lock once the red
// phase is verified.
type RedPhaseVerifier struct {
	lockMgr *security.WriteLockManager
}

// NewRedPhaseVerifier creates a verifier.
func NewRedPhaseVerifier(lockMgr *security.WriteLockManager) *RedPhaseVerifier {
	return &RedPhaseVerifier{lockMgr: lockMgr}
}

// testCommand matches shell commands that run a test suite.
var testCommand = regexp.MustCompile(`(^|[\s;&|(/])(` +
	`playwright\s+test|cypress\s+run|go\s+test|(npm|pnpm|yarn|bun)\s+(run\s+)?(test|e2e)[\w:-]*|npx\s+(jest|vitest|playwright|cypress)|` +
	`jest|vitest|pytest|python3?\s+-m\s+(pytest|unittest)|(bundle\s+exec\s+)?(rspec|cucumber|rails\s+test)|` +
	`mvn\w*\s+(\S+\s+)*(test|verify)|(\./)?gradlew?\s+(\S+\s+)*test|cargo\s+test|flutter\s+test|dart\s+test|` +
	`phpunit|make\s+(test|e2e)[\w-]*)(\s|$|;|&|\|)`)

// IsTestCommand reports whether a shell command runs a test suite.
func IsTestCommand(cmd string) bool {
	return testCommand.MatchString(strings.TrimSpace(cmd))
}

// harnessBroken are output fragments that mean the suite never reached its assertions.
var harnessBroken = []string{
	"syntaxerror", "parseerror", "parse error", "cannot find module", "module not found", "modulenotfounderror",
	"importerror", "cannot use import statement", "command not found", "no such file or directory",
	"no tests found", "no test files", "no tests ran", "no tests to run", "unknown command",
	"error: unknown option", "is not recognized as an internal or external command",
}

// VerifyRedPhase judges one acceptance-test run made before the feature is implemented: the suite
// must fail (non-zero exit) on its assertions, not on a broken harness. A verified run releases
// the task's write lock.
func (v *RedPhaseVerifier) VerifyRedPhase(ctx context.Context, taskID string, exitCode int, output string) (*RedPhaseVerificationResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	res := &RedPhaseVerificationResult{TaskID: taskID, ExitCode: exitCode, VerifiedAt: time.Now()}
	if exitCode == 0 {
		res.FailureSummary = "Invalid red phase: the acceptance tests passed before the feature was implemented, so they do not test it"
		return res, fmt.Errorf("red phase assertion failed: tests passed with exit code 0")
	}
	lowerOut := strings.ToLower(output)
	if exitCode == 127 {
		lowerOut += " command not found"
	}
	for _, s := range harnessBroken {
		if strings.Contains(lowerOut, s) {
			res.FailureSummary = fmt.Sprintf("Test harness broken (%q in the output): the tests did not reach their assertions", s)
			return res, fmt.Errorf("test suite rejected: harness broken (%s)", s)
		}
	}
	res.Passed = true
	res.FailureSummary = fmt.Sprintf("Verified red phase: the acceptance tests ran and failed (exit code %d) before implementation", exitCode)
	if v.lockMgr != nil {
		v.lockMgr.UnlockTask(taskID)
	}
	return res, nil
}
