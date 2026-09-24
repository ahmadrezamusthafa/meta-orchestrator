package security

import (
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestStrictWriteLockAndPolyglotInterceptor(t *testing.T) {
	lockMgr := NewWriteLockManager()
	interceptor := NewPolyglotSkillInterceptor(lockMgr)

	taskID := "task-lock-001"
	lockMgr.LockTask(taskID)

	// 1. Direct path check on locked source file -> MUST REJECT
	err := lockMgr.CheckWritePermitted(taskID, "/workspace/backend/internal/fsm/state.go")
	if err == nil {
		t.Fatalf("expected write to internal/ to be rejected while locked")
	}

	// 2. Direct path check on test file -> MUST ALLOW
	err = lockMgr.CheckWritePermitted(taskID, "/workspace/backend/tests/e2e/checkout.spec.ts")
	if err != nil {
		t.Fatalf("expected write to tests/ to be allowed during ATDD Red Phase: %v", err)
	}

	// 3. Intercept Native / MCP tool call writing to src/ -> MUST REJECT
	mcpWriteSkill := &types.UniversalSkillContract{
		Name:         "write_file",
		SourceFormat: types.SkillFormatMCP,
	}
	mcpReq := &types.SkillExecutionRequest{
		TaskID: taskID,
		Parameters: map[string]interface{}{
			"path": "/workspace/frontend/src/views/Checkout.vue",
		},
	}
	err = interceptor.InterceptSkillExecution(mcpWriteSkill, mcpReq)
	if err == nil {
		t.Fatalf("expected MCP write_file to src/ to be rejected")
	}

	// 4. Intercept Superpower shell redirection -> MUST REJECT
	superpowerSkill := &types.UniversalSkillContract{
		Name:         "execute_shell_host",
		SourceFormat: types.SkillFormatSuperpower,
	}
	shellReq := &types.SkillExecutionRequest{
		TaskID: taskID,
		Parameters: map[string]interface{}{
			"command": "echo 'hack' > src/App.vue",
		},
	}
	err = interceptor.InterceptSkillExecution(superpowerSkill, shellReq)
	if err == nil {
		t.Fatalf("expected shell write redirection to be intercepted")
	}

	// 5. Unlock task (Red Phase passed) -> MUST ALLOW
	lockMgr.UnlockTask(taskID)
	err = lockMgr.CheckWritePermitted(taskID, "/workspace/backend/internal/fsm/state.go")
	if err != nil {
		t.Fatalf("expected write to be permitted after unlock: %v", err)
	}
}
