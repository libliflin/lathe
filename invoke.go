package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// invokeAgent calls the LLM with a prompt, tees output to a log file, and detects rate limits.
func invokeAgent(prompt string, cycle int, label string, tool string) error {
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
	logFile := filepath.Join(logDir, fmt.Sprintf("cycle-%03d-%s.log", cycle, label))

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
	log("Rate limited from previous cycle. Waiting 5 minutes ...")
	waited := 0
	for waited < 300 {
		time.Sleep(30 * time.Second)
		waited += 30
		log("Rate limit cooldown: %ds remaining ...", 300-waited)
	}
	os.Remove(flag)
	log("Cooldown complete. Resuming.")
}
