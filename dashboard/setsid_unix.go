//go:build !windows

package dashboard

import (
	"os/exec"
	"syscall"
)

func setDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
