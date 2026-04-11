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

	// Instructions
	b.WriteString("---\n# Your Task\n\n")
	b.WriteString("Check the builder's work against the goal. Ask:\n")
	b.WriteString("1. Did the builder do what the goal asked?\n")
	b.WriteString("2. Does it actually work? Run the tests.\n")
	b.WriteString("3. What edge cases or regressions could this introduce?\n\n")
	b.WriteString("If you find gaps, fix them — commit real code (tests, edge cases, error handling).\n")
	b.WriteString("If the builder's change is solid, say so in the changelog.\n\n")
	b.WriteString("**Changelog:** Write a brief changelog to `.lathe/session/changelog.md` describing what you verified and any fixes you applied.\n\n")
	b.WriteString("**IMPORTANT — Verdict:** At the end of your changelog, write exactly one of these lines:\n")
	b.WriteString("- `VERDICT: PASS` — the goal is met, tests pass, work is solid. Move to the next goal.\n")
	b.WriteString("- `VERDICT: NEEDS_WORK` — there are issues the builder must address. Explain what's wrong above the verdict so the builder knows what to fix next round.\n\n")

	return invokeAgent(b.String(), cycle, fmt.Sprintf("verify-%d", round), tool)
}
