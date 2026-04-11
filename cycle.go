package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runCycle executes one full cycle: goal-setter → adaptive builder/verifier rounds.
// The verifier decides when the goal is met (VERDICT: PASS). roundsPerCycle is the safety cap.
func runCycle(cycle int, tool string) error {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════")
	fmt.Printf("  CYCLE %d — %s\n", cycle, time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("═══════════════════════════════════════════════")
	fmt.Println()

	// --- Goal Setter ---
	if err := runStep(cycle, "goal", tool, func() error {
		return runGoalSetter(cycle, tool)
	}); err != nil {
		return err
	}
	archiveGoal(cycle)

	// --- Builder/Verifier Rounds ---
	for round := 1; round <= roundsPerCycle; round++ {
		// Builder
		if err := runStep(cycle, fmt.Sprintf("round-%d-build", round), tool, func() error {
			return runBuilder(cycle, round, tool)
		}); err != nil {
			return err
		}

		// Verifier
		if err := runStep(cycle, fmt.Sprintf("round-%d-verify", round), tool, func() error {
			return runVerifier(cycle, round, tool)
		}); err != nil {
			return err
		}

		// Check verifier's verdict — but CI failures override PASS
		verdict := readVerdict()
		if ciResult == "fail" {
			log("CI failed — overriding verdict. Looping builder to fix.")
			verdict = "NEEDS_WORK"
		}
		if verdict == "PASS" {
			log("Verifier passed on round %d. Moving to next goal.", round)
			break
		}
		if round < roundsPerCycle {
			log("Verifier says NEEDS_WORK. Looping builder (round %d/%d) ...", round+1, roundsPerCycle)
		} else {
			log("Max rounds reached (%d). Moving to next goal.", roundsPerCycle)
		}
	}

	setCycle(cycle, "complete")
	return nil
}

// readVerdict reads the verifier's changelog and extracts the verdict.
// Returns "PASS", "NEEDS_WORK", or "" if no verdict found.
func readVerdict() string {
	data, err := os.ReadFile(filepath.Join(latheSession, "changelog.md"))
	if err != nil {
		return ""
	}
	content := string(data)
	if strings.Contains(content, "VERDICT: PASS") {
		return "PASS"
	}
	if strings.Contains(content, "VERDICT: NEEDS_WORK") {
		return "NEEDS_WORK"
	}
	return ""
}

// runStep is the shared plumbing for every step in a cycle.
func runStep(cycle int, phase string, tool string, agentFn func() error) error {
	waitForRateLimit()
	setCycle(cycle, phase)
	log("%s ...", phase)

	if err := createSessionBranch(); err != nil {
		return fmt.Errorf("create branch: %w", err)
	}

	collectSnapshot()
	collectCIStatus()

	// Run the agent — errors are non-fatal (agent might fail, cycle continues)
	agentFn()

	archiveCycle(cycle)
	safetyNet()

	// Give GitHub time to register the push and PR
	time.Sleep(30 * time.Second)

	discoverPR()
	waitForCI()
	autoMergeIfGreen()

	return nil
}
