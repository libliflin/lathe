package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runGoalSetter(cycle int, tool string) error {
	log("Running goal-setter (cycle %d) ...", cycle)

	var b strings.Builder

	// Goal-setter behavioral doc
	b.WriteString(readFileOr(filepath.Join(latheDir, "goal.md"), ""))
	b.WriteString("\n\n")

	// Common: skills, refs, theme, snapshot
	b.WriteString(assembleCommon())

	// Session context
	b.WriteString(assembleSessionContext())

	// Last 4 goals for context
	if entries, err := filepath.Glob(filepath.Join(goalHistory, "*.md")); err == nil && len(entries) > 0 {
		sort.Strings(entries)
		start := 0
		if len(entries) > 4 {
			start = len(entries) - 4
		}
		b.WriteString("---\n# Previous Goals (last 4 cycles)\n\n")
		for _, f := range entries[start:] {
			name := strings.TrimSuffix(filepath.Base(f), ".md")
			b.WriteString("## " + name + "\n")
			b.WriteString(readFileOr(f, ""))
			b.WriteString("\n\n")
		}
	}

	// Recent git history
	b.WriteString("---\n# Recent Commits\n\n```\n")
	gitLog, _ := runCapture("git", "log", "--oneline", "-20")
	if gitLog == "" {
		gitLog = "(no commits)"
	}
	b.WriteString(gitLog)
	b.WriteString("\n```\n\n")

	// Instructions
	b.WriteString("---\n# Your Task\n\n")
	b.WriteString("You are the customer champion. Each cycle:\n\n")
	b.WriteString("1. If the floor is violated (CI red, build broken, tests failing), the goal is to fix that — skip straight to step 4.\n")
	b.WriteString("2. Otherwise, pick one stakeholder (rotate based on Previous Goals — prefer one under-served recently) and say who.\n")
	b.WriteString("3. **Use the project as them.** Walk their first-encounter journey — run the commands, read the output, hit the friction. Notice the emotional signal goal.md defined for them. This is not optional; it is how a champion earns the courage to name what's valuable.\n")
	b.WriteString("4. Pick the single change that would most improve their next encounter. Write a goal file describing:\n")
	b.WriteString("   - **What** to change (specific, actionable — not how)\n")
	b.WriteString("   - **Which stakeholder** it helps and why\n")
	b.WriteString("   - **Why now** — the specific moment in the journey (or snapshot signal) that makes this the most valuable change right now\n")
	b.WriteString("   - **Lived experience note** — which stakeholder you became, what you tried, what the worst/hollowest moment was\n\n")
	b.WriteString("Commit this goal as a file the builder can read. The builder implements; you decide.\n\n")
	b.WriteString("**Changelog:** Write a brief changelog to `.lathe/session/changelog.md` describing which stakeholder you became, what you experienced, the goal you set, and why.\n\n")

	return invokeAgent(b.String(), cycle, "goal", tool)
}

func runBuilder(cycle, round int, tool string) error {
	log("Running builder (cycle %d, round %d) ...", cycle, round)

	var b strings.Builder

	// Builder behavioral doc
	b.WriteString(readFileOr(filepath.Join(latheDir, "builder.md"), ""))
	b.WriteString("\n\n")

	// Common: skills, refs, theme, snapshot
	b.WriteString(assembleCommon())

	// Session context
	b.WriteString(assembleSessionContext())

	// Current goal
	b.WriteString("---\n# Current Goal\n\n")
	goalFile := filepath.Join(goalHistory, fmt.Sprintf("cycle-%03d.md", cycle))
	changelogFile := filepath.Join(latheSession, "changelog.md")
	if data, err := os.ReadFile(goalFile); err == nil {
		b.Write(data)
	} else if data, err := os.ReadFile(changelogFile); err == nil {
		b.Write(data)
	} else {
		b.WriteString("(no goal found for this cycle — use your best judgment based on the snapshot)")
	}
	b.WriteString("\n\n")

	// Verifier feedback from previous round (if any)
	if round > 1 {
		changelogFile := filepath.Join(latheSession, "changelog.md")
		if feedback, err := os.ReadFile(changelogFile); err == nil {
			b.WriteString("---\n# Verifier Feedback (previous round)\n\n")
			b.WriteString("The verifier reviewed the last round and found issues. Address them:\n\n")
			b.Write(feedback)
			b.WriteString("\n\n")
		}
	}

	// CI failure details (if any)
	ciFailFile := filepath.Join(latheSession, "ci-failure.txt")
	if data, err := os.ReadFile(ciFailFile); err == nil && len(data) > 0 {
		b.WriteString("---\n# CI Failure (must fix)\n\n")
		b.WriteString("CI failed on the previous push. Here is the failure output:\n\n```\n")
		b.Write(data)
		b.WriteString("\n```\n\n")
		b.WriteString("Fix the CI failure. Tests may pass locally (e.g. runtime tests skipped on macOS) but fail on Linux CI.\n\n")
	}

	// Instructions
	b.WriteString("---\n# Your Task\n\n")
	b.WriteString("Implement the goal above. One change, committed, validated, pushed.\n")
	b.WriteString("If CI is failing, fix CI first — that's always top priority.\n\n")
	b.WriteString("**Changelog:** Write a brief changelog to `.lathe/session/changelog.md` describing what you changed and which stakeholder it benefits.\n\n")

	return invokeAgent(b.String(), cycle, fmt.Sprintf("build-%d", round), tool)
}

func runVerifier(cycle, round int, tool string) error {
	log("Running verifier (cycle %d, round %d) ...", cycle, round)

	var b strings.Builder

	// Verifier behavioral doc
	b.WriteString(readFileOr(filepath.Join(latheDir, "verifier.md"), ""))
	b.WriteString("\n\n")

	// Common: skills, refs, theme, snapshot
	b.WriteString(assembleCommon())

	// Session context
	b.WriteString(assembleSessionContext())

	// Current goal
	b.WriteString("---\n# Current Goal\n\n")
	goalFile := filepath.Join(goalHistory, fmt.Sprintf("cycle-%03d.md", cycle))
	b.WriteString(readFileOr(goalFile, "(no goal found)"))
	b.WriteString("\n\n")

	// Builder's diff
	b.WriteString("---\n# Builder's Changes (this round)\n\n```diff\n")
	diff, _ := runCapture("git", "diff", "HEAD~1")
	if diff == "" {
		diff = "(no diff available)"
	}
	b.WriteString(diff)
	b.WriteString("\n```\n\n")

	// CI failure details (if any)
	ciFailFile := filepath.Join(latheSession, "ci-failure.txt")
	if data, err := os.ReadFile(ciFailFile); err == nil && len(data) > 0 {
		b.WriteString("---\n# CI Failure (from previous step)\n\n")
		b.WriteString("CI failed. Here is the failure output:\n\n```\n")
		b.Write(data)
		b.WriteString("\n```\n\n")
		b.WriteString("Do NOT give VERDICT: PASS if the code would cause these same CI failures. Tests may pass locally but fail on Linux CI.\n\n")
	}

	// Instructions
	b.WriteString("---\n# Your Task\n\n")
	b.WriteString("Check the builder's work against the goal. Ask:\n")
	b.WriteString("1. Did the builder do what the goal asked?\n")
	b.WriteString("2. Does it actually work? Run the tests.\n")
	b.WriteString("3. What edge cases or regressions could this introduce?\n\n")
	b.WriteString("If you find gaps, fix them — commit real code (tests, edge cases, error handling).\n\n")
	b.WriteString("**Changelog:** Write `.lathe/session/changelog.md` using this exact template:\n\n")
	b.WriteString("```\n")
	b.WriteString(fmt.Sprintf("# Verification — Cycle %d, Round %d\n", cycle, round))
	b.WriteString("\n")
	b.WriteString("## What was checked\n")
	b.WriteString("(what you tested and reviewed)\n")
	b.WriteString("\n")
	b.WriteString("## Findings\n")
	b.WriteString("(what you found — issues, gaps, or confirmation that it's solid)\n")
	b.WriteString("\n")
	b.WriteString("## Fixes applied\n")
	b.WriteString("(what you committed to fix, or \"None\" if the work was solid)\n")
	b.WriteString("\n")
	b.WriteString("VERDICT: PASS\n")
	b.WriteString("```\n\n")
	b.WriteString("Set the last line to exactly `VERDICT: PASS` or `VERDICT: NEEDS_WORK`.\n")
	b.WriteString("- **PASS** — goal is met, tests pass, work is solid. Moves to the next goal.\n")
	b.WriteString("- **NEEDS_WORK** — issues remain. The builder reads your Findings next round, so be specific about what's wrong.\n\n")

	return invokeAgent(b.String(), cycle, fmt.Sprintf("verify-%d", round), tool)
}
