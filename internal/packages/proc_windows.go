//go:build windows

package packages

import "os/exec"

func ownProcessGroup(cmd *exec.Cmd) {}
