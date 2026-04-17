package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// errMaxRounds signals the engine to stop — the dialog hit the oscillation cap
// without converging.
var errMaxRounds = errors.New("oscillation cap reached without convergence")

// runCycle executes one full cycle: goal-setter → dialog between builder and verifier.
// Each round both contribute (or stand down). The cycle converges when a round passes
// with neither committing. roundsPerCycle caps the dialog to prevent oscillation.
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

	baseBranch := getBaseBranch()

	// --- Builder/Verifier Dialog ---
	for round := 1; round <= roundsPerCycle; round++ {
		builderHead := getHead(baseBranch)
		if err := runStep(cycle, fmt.Sprintf("round-%d-build", round), tool, func() error {
			return runBuilder(cycle, round, tool)
		}); err != nil {
			return err
		}
		builderContributed := getHead(baseBranch) != builderHead

		verifierHead := getHead(baseBranch)
		if err := runStep(cycle, fmt.Sprintf("round-%d-verify", round), tool, func() error {
			return runVerifier(cycle, round, tool)
		}); err != nil {
			return err
		}
		verifierContributed := getHead(baseBranch) != verifierHead

		// CI failure invalidates "converged" — if CI is red, the work isn't done even
		// if neither agent committed this round. Re-run the verifier to investigate.
		if !builderContributed && !verifierContributed && ciResult == "fail" {
			log("Neither contributed but CI failed — re-running verifier to investigate.")
			if err := runStep(cycle, fmt.Sprintf("round-%d-verify-ci", round), tool, func() error {
				return runVerifier(cycle, round, tool)
			}); err != nil {
				return err
			}
			verifierContributed = getHead(baseBranch) != verifierHead
		}

		if !builderContributed && !verifierContributed {
			log("Convergence reached at round %d. Both lenses stood down — goal complete.", round)
			break
		}

		if round < roundsPerCycle {
			who := ""
			switch {
			case builderContributed && verifierContributed:
				who = "both contributed"
			case builderContributed:
				who = "builder contributed"
			case verifierContributed:
				who = "verifier contributed"
			}
			log("%s — dialog continues (round %d/%d) ...", who, round+1, roundsPerCycle)
		} else {
			log("Oscillation cap reached (%d rounds). Handing dialog to next goal-setter.", roundsPerCycle)
			setCycle(cycle, "oscillated")
			return errMaxRounds
		}
	}

	setCycle(cycle, "complete")
	return nil
}

// getBaseBranch returns the session's base branch, defaulting to "main" when unknown.
func getBaseBranch() string {
	if s, err := readSession(); err == nil && s.BaseBranch != "" {
		return s.BaseBranch
	}
	return "main"
}

// getHead returns the SHA of the given branch locally, or "" if unknown.
// This is the convergence signal: HEAD of the base branch moves only when a
// step's PR squash-merges, which only happens when the agent committed real work.
func getHead(branch string) string {
	out, err := runCapture("git", "rev-parse", branch)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
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
