package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// errMaxRounds signals the engine to stop — the cycle exhausted all
// builder/verifier rounds without CI passing.
var errMaxRounds = errors.New("max rounds exhausted")

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

		verdict := readVerdict()

		// If verifier said PASS but CI failed, the verifier got it wrong.
		// Re-run the verifier with CI failure logs so it can investigate.
		if verdict == "PASS" && ciResult == "fail" {
			log("CI failed but verifier said PASS — re-running verifier to investigate CI failure.")
			if err := runStep(cycle, fmt.Sprintf("round-%d-verify-ci", round), tool, func() error {
				return runVerifier(cycle, round, tool)
			}); err != nil {
				return err
			}
			verdict = readVerdict()
		}

		if verdict == "PASS" {
			log("Verifier passed on round %d. Moving to next goal.", round)
			break
		}
		if round < roundsPerCycle {
			log("Verifier says NEEDS_WORK. Looping builder (round %d/%d) ...", round+1, roundsPerCycle)
		} else {
			log("Max rounds reached (%d). Stopping — goal not resolved.", roundsPerCycle)
			setCycle(cycle, "failed")
			return errMaxRounds
		}
	}

	setCycle(cycle, "complete")
	return nil
}

// readVerdict reads the verifier's changelog and extracts the verdict.
// Returns "PASS", "NEEDS_WORK", or "" if no verdict found.
// Anchors to line-start to avoid matching the word inside prose or code blocks.
func readVerdict() string {
	data, err := os.ReadFile(filepath.Join(latheSession, "changelog.md"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "VERDICT: PASS" {
			return "PASS"
		}
		if trimmed == "VERDICT: NEEDS_WORK" {
			return "NEEDS_WORK"
		}
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

	// Capture or clear CI failure logs
	ciFailFile := filepath.Join(latheSession, "ci-failure.txt")
	if ciResult == "fail" {
		captureCIFailureLogs()
	} else {
		os.Remove(ciFailFile) // clear stale failure from previous step
	}

	autoMergeIfGreen()

	return nil
}
