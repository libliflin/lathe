package main

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// isRunning returns true if the PID file points to a live process.
func isRunning() bool {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check.
	return proc.Signal(syscall.Signal(0)) == nil
}

// killTree kills a process and all its children.
func killTree(sig syscall.Signal, pid int) {
	// Find children first
	out, err := runCapture("pgrep", "-P", strconv.Itoa(pid))
	if err == nil && out != "" {
		for _, line := range strings.Split(out, "\n") {
			childPid, err := strconv.Atoi(strings.TrimSpace(line))
			if err == nil {
				killTree(sig, childPid)
			}
		}
	}
	// Then kill the root
	if proc, err := os.FindProcess(pid); err == nil {
		proc.Signal(sig)
	}
}

// findLatheAgent finds lathe's claude/amp agent process by its distinctive flags.
func findLatheAgent() (int, bool) {
	out, err := runCapture("pgrep", "-f", "claude.*--dangerously-skip-permissions.*--print")
	if err != nil || out == "" {
		return 0, false
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, false
	}
	return pid, true
}
