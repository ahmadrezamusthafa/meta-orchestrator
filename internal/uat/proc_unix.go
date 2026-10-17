//go:build !windows

package uat

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup starts cmd in a new process group, so stopping it also stops what it started
// (npm's install scripts, the Chromium that node launches).
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup kills cmd's process group.
func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	return cmd.Process.Kill()
}
