//go:build !windows

package packages

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup starts cmd in a new process group and makes cancelling it kill the whole group,
// so stopping an install also stops what it started (npm lifecycle scripts, browser downloads).
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
}
