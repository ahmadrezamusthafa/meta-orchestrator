//go:build !windows

package connectors

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup starts cmd in a new process group, so stopping it also stops what it started
// (npx and mcp-remote run the real server as a child).
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup kills cmd's process group.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
	}
}
