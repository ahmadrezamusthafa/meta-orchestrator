package security

import (
	"fmt"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// PolyglotSkillInterceptor intercepts tool executions from Claude, BMAD, Superpower, and MCP to enforce write-locks.
type PolyglotSkillInterceptor struct {
	lockMgr *WriteLockManager
}

// NewPolyglotSkillInterceptor creates an interceptor.
func NewPolyglotSkillInterceptor(lockMgr *WriteLockManager) *PolyglotSkillInterceptor {
	return &PolyglotSkillInterceptor{lockMgr: lockMgr}
}

// InterceptSkillExecution inspects incoming skill execution requests before invocation.
func (i *PolyglotSkillInterceptor) InterceptSkillExecution(
	skill *types.UniversalSkillContract,
	req *types.SkillExecutionRequest,
) error {
	if skill == nil || req == nil {
		return nil
	}

	taskID := req.TaskID
	if !i.lockMgr.IsLocked(taskID) {
		return nil // Not locked, allow
	}

	// 1. Check Native & MCP file modification tools
	lowerSkillName := strings.ToLower(skill.Name)
	if lowerSkillName == "write_to_file" || lowerSkillName == "replace_file_content" || lowerSkillName == "write_file" || lowerSkillName == "edit_file" {
		targetFile, _ := req.Parameters["target_file"].(string)
		if targetFile == "" {
			targetFile, _ = req.Parameters["path"].(string)
		}
		if targetFile != "" {
			return i.lockMgr.CheckWritePermitted(taskID, targetFile)
		}
	}

	// 2. Check Superpower and Shell command execution
	if skill.SourceFormat == types.SkillFormatSuperpower || lowerSkillName == "execute_shell_host" || lowerSkillName == "run_command" {
		cmdStr, _ := req.Parameters["command"].(string)
		if cmdStr == "" {
			cmdStr, _ = req.Parameters["cmd"].(string)
		}

		lowerCmd := strings.ToLower(cmdStr)
		// Detect destructive write operations directed at source folders
		if strings.Contains(lowerCmd, ">") || strings.Contains(lowerCmd, "sed -i") || strings.Contains(lowerCmd, "rm ") || strings.Contains(lowerCmd, "cp ") {
			if strings.Contains(lowerCmd, "src/") || strings.Contains(lowerCmd, "app/") || strings.Contains(lowerCmd, "internal/") {
				return fmt.Errorf("%w: shell command '%s' attempts to mutate write-locked source directory", ErrWriteLockActive, cmdStr)
			}
		}
	}

	return nil
}
