package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func engineStart(args []string) {
	maxCycles := 0
	tool := "claude"
	theme := ""
	mode := "branch"

	// Project-level mode override
	if data, err := os.ReadFile(filepath.Join(latheDir, "mode")); err == nil {
		m := strings.TrimSpace(string(data))
		if m == "direct" || m == "branch" {
			mode = m
		}
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cycles":
			i++
			if i < len(args) {
				maxCycles, _ = strconv.Atoi(args[i])
			}
		case "--tool":
			i++
			if i < len(args) {
				tool = args[i]
			}
		case "--theme":
			i++
			if i < len(args) {
				theme = args[i]
			}
		case "--direct":
			mode = "direct"
		default:
			die("Unknown option: %s", args[i])
		}
	}

	if isRunning() {
		data, _ := os.ReadFile(pidFile)
		fmt.Printf("Already running (PID %s). Use 'lathe stop' first.\n", strings.TrimSpace(string(data)))
		os.Exit(1)
	}

	// Clean slate
	os.RemoveAll(latheSession)
	os.MkdirAll(filepath.Join(latheSession, "logs"), 0755)
	os.MkdirAll(latheHistory, 0755)
	os.MkdirAll(goalHistory, 0755)

	if theme != "" {
		os.WriteFile(filepath.Join(latheSession, "theme.txt"), []byte(theme), 0644)
	}

	if err := initSessionState(mode, theme); err != nil {
		die("init session: %v", err)
	}

	projectName := filepath.Base(".")
	if cwd, err := os.Getwd(); err == nil {
		projectName = filepath.Base(cwd)
	}

	fmt.Println()
	fmt.Println("  ╔═══════════════════════════════════════════╗")
	fmt.Printf("  ║  LATHE — turning %s\n", projectName)
	fmt.Println("  ╚═══════════════════════════════════════════╝")
	fmt.Println()

	// Fork the cycle loop into a background goroutine, write PID
	// For now, run in foreground (the bash version used a subshell &).
	// Go doesn't fork — we run in-process and write our own PID.
	os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644)

	// Redirect log output to stream log
	logFile := filepath.Join(latheSession, "logs", "stream.log")
	f, err := os.Create(logFile)
	if err != nil {
		die("create stream log: %v", err)
	}
	defer f.Close()
	// Log to both stderr and file
	origStderr := os.Stderr
	_ = origStderr // keep for future use

	fmt.Printf("  Started (PID %d). Tool: %s, Mode: %s\n", os.Getpid(), tool, mode)
	if mode == "branch" {
		s, _ := readSession()
		fmt.Printf("  Branch:  %s\n", s.Branch)
	}
	fmt.Println()
	fmt.Println("  Logs:    lathe logs --follow")
	fmt.Println("  Status:  lathe status")
	fmt.Println("  Stop:    lathe stop")

	// Run the cycle loop
	cycle := getCycle()
	cyclesRun := 0

	for {
		if err := runCycle(cycle, tool); err != nil {
			log("Cycle %d error: %v", cycle, err)
		}

		cycle++
		cyclesRun++

		if maxCycles > 0 && cyclesRun >= maxCycles {
			log("Completed %d cycles.", cyclesRun)
			return
		}

		time.Sleep(5 * time.Second)
	}
}

func engineStop() {
	// Kill process tree
	if data, err := os.ReadFile(pidFile); err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil {
			if proc, err := os.FindProcess(pid); err == nil {
				if proc.Signal(syscall.Signal(0)) == nil && pid != os.Getpid() {
					log("Stopping process tree (PID %d) ...", pid)
					killTree(syscall.SIGTERM, pid)

					// Wait for tree to die
					for i := 0; i < 5; i++ {
						time.Sleep(1 * time.Second)
						if proc.Signal(syscall.Signal(0)) != nil {
							break
						}
					}
					// Force kill if still alive
					if proc.Signal(syscall.Signal(0)) == nil {
						killTree(syscall.SIGKILL, pid)
						time.Sleep(1 * time.Second)
					}
				}
			}
		}
		os.Remove(pidFile)
	}

	// Kill orphaned agent
	if agentPid, found := findLatheAgent(); found {
		log("Killing orphaned agent (PID %d) ...", agentPid)
		killTree(syscall.SIGTERM, agentPid)
		time.Sleep(2 * time.Second)
		if proc, err := os.FindProcess(agentPid); err == nil {
			if proc.Signal(syscall.Signal(0)) == nil {
				killTree(syscall.SIGKILL, agentPid)
			}
		}
	}

	teardownSession()
	fmt.Println("Stopped.")
}

func engineStatus(args []string) {
	projectName := "."
	if cwd, err := os.Getwd(); err == nil {
		projectName = filepath.Base(cwd)
	}
	fmt.Printf("=== Lathe: %s ===\n", projectName)

	if isRunning() {
		data, _ := os.ReadFile(pidFile)
		fmt.Printf("  Running — PID %s\n", strings.TrimSpace(string(data)))
	} else if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		fmt.Println("  No active session. Run 'lathe start' to begin.")
	} else {
		fmt.Println("  Stopped (session state exists — may need 'lathe stop' to clean up)")
	}

	if agentPid, found := findLatheAgent(); found {
		if isRunning() {
			fmt.Printf("  Agent  — PID %d\n", agentPid)
		} else {
			fmt.Printf("\n  ** ORPHANED AGENT — PID %d **\n", agentPid)
		}
	}

	fmt.Println()

	if s, err := readSession(); err == nil {
		fmt.Printf("  Mode: %s\n", s.Mode)
		if s.Mode == "branch" {
			fmt.Printf("  Branch: %s\n", s.Branch)
			if s.PRNumber != "" {
				fmt.Printf("  PR: #%s\n", s.PRNumber)
			} else {
				fmt.Println("  PR: (not yet created)")
			}
		}
		fmt.Printf("  Base: %s\n", s.BaseBranch)
	}

	cycleFile := filepath.Join(latheSession, "cycle.json")
	if data, err := os.ReadFile(cycleFile); err == nil {
		var c CycleState
		if json.Unmarshal(data, &c) == nil {
			fmt.Printf("  Cycle: %d  Status: %s\n", c.Cycle, c.Status)
		}
	}

	if _, err := os.Stat(filepath.Join(latheSession, "rate-limited")); err == nil {
		fmt.Println("  ** RATE LIMITED — waiting for cooldown **")
	}

	fmt.Println()

	// Show latest log snippet
	entries, _ := filepath.Glob(filepath.Join(latheSession, "logs", "cycle-*.log"))
	if len(entries) > 0 {
		latest := entries[len(entries)-1]
		fmt.Printf("  Latest log: %s\n", latest)
		if data, err := os.ReadFile(latest); err == nil {
			lines := strings.Split(string(data), "\n")
			start := len(lines) - 5
			if start < 0 {
				start = 0
			}
			fmt.Println("  Last 5 lines:")
			for _, line := range lines[start:] {
				if line != "" {
					fmt.Printf("    %s\n", line)
				}
			}
		}
	}
}

func engineLogs(args []string) {
	follow := false
	for _, a := range args {
		if a == "--follow" || a == "-f" {
			follow = true
		}
	}

	if follow {
		streamLog := filepath.Join(latheSession, "logs", "stream.log")
		if _, err := os.Stat(streamLog); os.IsNotExist(err) {
			fmt.Println("  No active session. Start one with 'lathe start'.")
			return
		}
		// Tail -f equivalent
		run("tail", "-f", streamLog)
	} else {
		entries, _ := filepath.Glob(filepath.Join(latheSession, "logs", "cycle-*.log"))
		if len(entries) == 0 {
			fmt.Println("  No logs. Start a session with 'lathe start'.")
			return
		}
		latest := entries[len(entries)-1]
		fmt.Printf("=== Latest: %s ===\n\n", filepath.Base(latest))
		run("tail", "-80", latest)
		fmt.Println("\n---")
		fmt.Println("  Follow:  lathe logs --follow")
	}
}
