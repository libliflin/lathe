//go:build windows

package dashboard

import "os/exec"

func setDetach(cmd *exec.Cmd) {
	// No-op on Windows; detachment works differently.
}
