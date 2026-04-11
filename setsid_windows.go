package main

import "os/exec"

func setDetach(cmd *exec.Cmd) {
	// Windows doesn't support Setsid; the process is already detached
	// when started without inheriting the console.
}
