package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runSim(cycle int, tool string) error {
	log("Running stakeholder sim (cycle %d) ...", cycle)

	var b strings.Builder

	// Stakeholder map from goal.md
	b.WriteString("---\n# Stakeholder Map\n\n")
	b.WriteString(readFileOr(filepath.Join(latheDir, "goal.md"), "(no stakeholder map found)"))
	b.WriteString("\n\n")

	// Common: skills, refs, theme, snapshot
	b.WriteString(assembleCommon())

	// Session context
	b.WriteString(assembleSessionContext())

	// Instructions
	b.WriteString("---\n# Your Task: Stakeholder Friction Report\n\n")
	b.WriteString("You are a friction reporter. Pick one stakeholder from the map above.\n\n")
	b.WriteString("Simulate their experience encountering this project right now, based on the snapshot:\n")
	b.WriteString("- What would they try to do first?\n")
	b.WriteString("- What would work? What would fail or confuse them?\n")
	b.WriteString("- What question would they have that nothing answers?\n\n")
	b.WriteString("This is a simulation, not an analysis. Write as if you just watched someone use the project.\n")
	b.WriteString("Be specific and honest — vague praise is useless. Short and concrete is better than long and general.\n\n")
	b.WriteString("Write your findings to `.lathe/session/friction.md` in this format:\n\n")
	b.WriteString("```\n# Friction Report — Cycle N\n\n")
	b.WriteString("## Stakeholder: <name>\n\n")
	b.WriteString("## What they tried\n(their goal, in their terms)\n\n")
	b.WriteString("## What worked\n(be honest — partial credit counts)\n\n")
	b.WriteString("## Where they got stuck\n(specific friction point, not generic critique)\n\n")
	b.WriteString("## Their question\n(the one thing they'd want answered that the project doesn't answer right now)\n")
	b.WriteString("```\n\n")
	b.WriteString(fmt.Sprintf("Replace `Cycle N` with `Cycle %d`.\n\n", cycle))
	b.WriteString("Do NOT commit anything. Do NOT create a PR. Just write the friction.md file.\n")

	return invokeAgent(b.String(), cycle, "sim", tool)
}

func runGoalSetter(cycle int, tool string) error {
	log("Running goal-setter (cycle %d) ...", cycle)

	var b strings.Builder

	// Goal-setter behavioral doc
	b.WriteString(readFileOr(filepath.Join(latheDir, "goal.md"), ""))
	b.WriteString("\n\n")

	// Common: skills, refs, theme, snapshot
	b.WriteString(assembleCommon())

	// Stakeholder friction report from the sim step (if any)
	if data, err := os.ReadFile(frictionFile); err == nil && len(data) > 0 {
		b.WriteString("---\n# Stakeholder Friction Report (this cycle)\n\n")
		b.Write(data)
		b.WriteString("\n\n")
	}

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
	b.WriteString("Pick the single highest-value change for this cycle. Write a goal file describing:\n")
	b.WriteString("- **What** to change (specific, actionable)\n")
	b.WriteString("- **Which stakeholder** it helps and why\n")
	b.WriteString("- **Why now** — what in the snapshot makes this the most valuable change right now\n\n")
	b.WriteString("Commit this goal as a file the builder can read. The builder implements; you decide.\n\n")
	b.WriteString("**Changelog:** Write a brief changelog to `.lathe/session/changelog.md` describing what goal you set and why.\n\n")

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
