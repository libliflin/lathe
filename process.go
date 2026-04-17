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

// getPidCwd returns the working directory of the given PID, or "" when it can't be determined.
// Uses lsof, which is available on macOS and most Linux systems.
func getPidCwd(pid int) string {
	out, err := runCapture("lsof", "-p", strconv.Itoa(pid), "-a", "-d", "cwd", "-Fn")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
}

// findLatheAgent finds lathe's claude/amp agent process running in THIS project's directory.
// Scoping by cwd prevents a global pgrep match from picking up agents belonging to other
// projects — e.g. `lathe status` in project A surfacing project B's active subprocess as an orphan,
// or `lathe stop` in A killing B's live cycle.
func findLatheAgent() (int, bool) {
	out, err := runCapture("pgrep", "-f", "claude.*--dangerously-skip-permissions.*--print")
	if err != nil || out == "" {
		return 0, false
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			continue
		}
		if getPidCwd(pid) == cwd {
			return pid, true
		}
	}
	return 0, false
}
