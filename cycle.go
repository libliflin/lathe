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

	// A new cycle is a new goal. Clear any PR left active from the previous cycle
	// so the goal step cuts a fresh branch. The orphan (if any) stays visible to
	// the agents via stale-prs.txt.
	if s, err := readSession(); err == nil && (s.Branch != "" || s.PRNumber != "") {
		s.Branch = ""
		s.PRNumber = ""
		_ = writeSession(s)
	}

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
			// Don't declare convergence while PRs from this dialog are still open.
			// One more merge sweep, then check.
			resolveStalePRs()
			if openPRs := countOpenLathePRs(); openPRs > 0 {
				log("No commits this round but %d lathe PR(s) still open — continuing dialog.", openPRs)
			} else {
				log("Convergence reached at round %d. Both lenses stood down — goal complete.", round)
				break
			}
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
			log("Oscillation cap reached (%d rounds) — entering error state for human review.", roundsPerCycle)
			writeErrorState(cycle, round, "oscillation-cap",
				fmt.Sprintf("Builder and verifier did not converge after %d rounds of dialog. Both kept contributing without reaching a stable state.", roundsPerCycle))
			setCycle(cycle, "error")
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

// writeErrorState captures everything a human (or a Claude Code session) needs
// to diagnose and unstick a lathe that couldn't converge. Written to
// .lathe/session/error.md, read by `lathe status` afterwards.
func writeErrorState(cycle, round int, kind, detail string) {
	var b strings.Builder

	fmt.Fprintf(&b, "# Lathe Error State\n\n")
	fmt.Fprintf(&b, "**When:** %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "**Cycle:** %d, Round: %d\n", cycle, round)
	fmt.Fprintf(&b, "**Kind:** %s\n\n", kind)
	fmt.Fprintf(&b, "## What happened\n\n%s\n\n", detail)

	latestGoal := filepath.Join(goalHistory, fmt.Sprintf("cycle-%03d.md", cycle))
	if data, err := os.ReadFile(latestGoal); err == nil {
		b.WriteString("## Goal of the stuck cycle\n\n")
		b.Write(data)
		b.WriteString("\n\n")
	}

	changelog := filepath.Join(latheSession, "changelog.md")
	if data, err := os.ReadFile(changelog); err == nil {
		b.WriteString("## Last round's changelog\n\n")
		b.Write(data)
		b.WriteString("\n\n")
	}

	stalePRs := filepath.Join(latheSession, "stale-prs.txt")
	if data, err := os.ReadFile(stalePRs); err == nil {
		b.Write(data)
		b.WriteString("\n\n")
	}

	b.WriteString("## What to do from here\n\n")
	b.WriteString("Open Claude Code in this project directory and ask it to investigate. Tell it to read this file and the stale-prs.txt context.\n\n")
	b.WriteString("Typical resolutions:\n")
	b.WriteString("- **PRs going in circles**: close them (`gh pr close <N> --delete-branch`). The next cycle's goal-setter will pick a new angle.\n")
	b.WriteString("- **Goal was malformed**: close the related PRs; lathe's next cycle will pick a fresh goal.\n")
	b.WriteString("- **Real blocker** (a dep conflict, a flaky test, a credential issue): fix it in the repo, then restart.\n\n")
	b.WriteString("When you've resolved things: `lathe start` to resume. `preStartCleanup` will merge any greens that appeared, leave the rest for the new session's agents to see.\n")

	_ = os.WriteFile(filepath.Join(latheSession, "error.md"), []byte(b.String()), 0644)
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

	// Clean up any lathe PRs from earlier steps whose CI has since completed.
	// Without this, a PR whose CI took longer than this step's waitForCI budget
	// sits orphaned forever.
	resolveStalePRs()

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
