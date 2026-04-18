package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// invokeAgent calls the LLM with a prompt, tees output to a log file, and detects rate limits.
// cycleID is the timestamp-based cycle identity used to name per-step log files.
func invokeAgent(prompt string, cycleID string, label string, tool string) error {
	// Pre-cycle hook
	hook := filepath.Join(latheDir, "hooks", "pre-cycle.sh")
	if info, err := os.Stat(hook); err == nil && info.Mode()&0111 != 0 {
		log("Running pre-cycle hook ...")
		if err := runSilent(hook); err != nil {
			log("WARN: pre-cycle hook failed (non-fatal)")
		}
	}

	logDir := filepath.Join(latheSession, "logs")
	os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, fmt.Sprintf("cycle-%s-%s.log", cycleID, label))

	log("Invoking %s (%s) ...", tool, label)

	var exitCode int
	var err error

	switch tool {
	case "claude":
		exitCode, err = runPipe(prompt, logFile, "claude", "--dangerously-skip-permissions", "--print")
	case "amp":
		exitCode, err = runPipe(prompt, logFile, "amp", "--dangerously-allow-all")
	default:
		return fmt.Errorf("unknown tool: %s", tool)
	}

	if err != nil {
		return fmt.Errorf("invoke %s: %w", tool, err)
	}

	// Rate limit detection
	if data, err := os.ReadFile(logFile); err == nil {
		if strings.Contains(string(data), "You've hit your limit") {
			log("Rate limited. Ending early.")
			os.WriteFile(filepath.Join(latheSession, "rate-limited"), []byte("RATE_LIMITED"), 0644)
			return fmt.Errorf("rate limited")
		}
	}

	os.Remove(filepath.Join(latheSession, "rate-limited"))
	log("Agent complete (%s, exit %d). Log: %s", label, exitCode, logFile)
	return nil
}

// waitForRateLimit blocks if the rate-limited sentinel exists.
func waitForRateLimit() {
	flag := filepath.Join(latheSession, "rate-limited")
	if _, err := os.Stat(flag); os.IsNotExist(err) {
		return
	}
	sleepUntilRateLimitLifts()
	os.Remove(flag)
}

// sleepUntilRateLimitLifts shows a sleeping animation and probes every 3 minutes
// until the rate limit has lifted. Used by both the engine and init.
func sleepUntilRateLimitLifts() {
	log("Rate limited. Sleeping until it lifts (checking every 3 minutes) ...")

	frames := []string{"💤", "😴", "🌙", "⭐"}
	start := time.Now()
	attempt := 0
	isTTY := isTerminal()

	for {
		// Sleeping animation for 3 minutes
		deadline := time.Now().Add(3 * time.Minute)
		i := 0
		for time.Now().Before(deadline) {
			elapsed := time.Since(start).Truncate(time.Second)
			mins := int(elapsed.Minutes())
			secs := int(elapsed.Seconds()) % 60
			if isTTY {
				fmt.Fprintf(os.Stderr, "\r  %s  Rate limited — sleeping ... %dm%02ds", frames[i%len(frames)], mins, secs)
			}
			i++
			time.Sleep(500 * time.Millisecond)
		}
		if isTTY {
			fmt.Fprintf(os.Stderr, "\r\033[K")
		}

		attempt++
		elapsed := time.Since(start).Truncate(time.Second)
		mins := int(elapsed.Minutes())
		secs := int(elapsed.Seconds()) % 60
		log("Checking if rate limit lifted (attempt %d, %dm%02ds elapsed) ...", attempt, mins, secs)

		// Probe: ask claude a trivial question to see if we're still limited
		output, err := runCaptureAll("claude", "-p", "say ok")
		if err == nil && !strings.Contains(output, "You've hit your limit") {
			log("Rate limit lifted after %dm%02ds. Resuming.", mins, secs)
			return
		}

		log("Still rate limited. Sleeping again ...")
	}
}

// isTerminal reports whether stderr is a terminal (for animation support).
func isTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
