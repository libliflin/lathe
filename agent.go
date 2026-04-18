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

	// Stale PR context — orphans inherited from the previous cycle or session
	stalePRsFile := filepath.Join(latheSession, "stale-prs.txt")
	if data, err := os.ReadFile(stalePRsFile); err == nil && len(data) > 0 {
		b.WriteString("---\n")
		b.Write(data)
		b.WriteString("\n\n")
	}

	// Instructions
	b.WriteString("---\n# Your Task\n\n")
	b.WriteString("You are the customer champion. Each cycle:\n\n")
	b.WriteString("1. If the floor is violated (CI red, build broken, tests failing), the goal is to fix that — skip straight to step 4.\n")
	b.WriteString("2. If the Stale Lathe PRs section is present above, weigh it in: is the stuck work the right next goal, or is it superseded? You can set the goal to finish a stale PR, or instruct the builder to close it as part of this cycle's fresh direction.\n")
	b.WriteString("3. Otherwise, pick one stakeholder (rotate based on Previous Goals — prefer one under-served recently) and say who.\n")
	b.WriteString("4. **Use the project as them.** Walk their first-encounter journey — run the commands, read the output, hit the friction. Notice the emotional signal goal.md defined for them. This is not optional; it is how a champion earns the courage to name what's valuable.\n")
	b.WriteString("5. Pick the single change that would most improve their next encounter. Write a goal file describing:\n")
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

	// Verifier contribution from previous round (if any)
	if round > 1 {
		changelogFile := filepath.Join(latheSession, "changelog.md")
		if feedback, err := os.ReadFile(changelogFile); err == nil {
			b.WriteString("---\n# Verifier's Contribution (previous round)\n\n")
			b.WriteString("The verifier looked at your work from the comparative lens and either added what they saw missing or stood down. Read their changelog. Their code contributions are already in the repo — run `git log --oneline -10` to see recent commits.\n\n")
			b.WriteString("Respond from your creative lens: refine their additions, extend the work, or recognize that the work stands complete and make no commit this round.\n\n")
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

	// Stale PR context — orphaned PRs from previous steps the agent can act on
	stalePRsFile := filepath.Join(latheSession, "stale-prs.txt")
	if data, err := os.ReadFile(stalePRsFile); err == nil && len(data) > 0 {
		b.WriteString("---\n")
		b.Write(data)
		b.WriteString("\n\n")
	}

	// Instructions
	b.WriteString("---\n# Your Task\n\n")
	if round == 1 {
		b.WriteString("Bring the goal into being. Implement, validate, commit, push.\n")
	} else {
		b.WriteString("Continue the dialog. Read the verifier's contribution from the previous round. From your creative lens, decide: refine, extend, or stand down. When you have something worth adding, commit and push it. When the work stands complete in your view, write the changelog with \"Applied: Nothing this round — the verifier's additions complete the work from my lens\" and skip the commit.\n")
	}
	b.WriteString("If CI is failing, fix CI first — that's always top priority.\n")
	b.WriteString("If the Stale Lathe PRs section is present above, handle those first — they block progress on this cycle's dialog.\n\n")
	b.WriteString("**Changelog:** Write a changelog to `.lathe/session/changelog.md` describing what you did this round (or explaining why you stood down).\n\n")

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
		b.WriteString("The work is not done while CI is red — from your comparative lens, adding code that closes the gap is in scope.\n\n")
	}

	// Stale PR context — orphaned PRs from previous steps the verifier can act on
	stalePRsFile := filepath.Join(latheSession, "stale-prs.txt")
	if data, err := os.ReadFile(stalePRsFile); err == nil && len(data) > 0 {
		b.WriteString("---\n")
		b.Write(data)
		b.WriteString("\n\n")
	}

	// Instructions
	b.WriteString("---\n# Your Task\n\n")
	b.WriteString("Continue the dialog. Read the builder's contribution this round and compare it against the goal from your scrutinizing lens. Ask: what's here, what was asked, where's the gap? Run the tests. Exercise the change. Try the hard cases.\n\n")
	b.WriteString("When you see gaps worth adding code to close, commit them — tests, edge cases, error handling, fills. When the work stands complete from your comparative lens, write the changelog with \"Added: Nothing this round — the work holds up against the goal from my lens\" and skip the commit.\n\n")
	b.WriteString("**Changelog:** Write `.lathe/session/changelog.md` using this template:\n\n")
	b.WriteString("```\n")
	b.WriteString(fmt.Sprintf("# Verification — Cycle %d, Round %d (Verifier)\n", cycle, round))
	b.WriteString("\n")
	b.WriteString("## What I compared\n")
	b.WriteString("(goal on one side, code on the other — what you read, ran, witnessed)\n")
	b.WriteString("\n")
	b.WriteString("## What's here, what was asked\n")
	b.WriteString("(the gap from your comparative lens, or \"matches: the work holds up against the goal\")\n")
	b.WriteString("\n")
	b.WriteString("## What I added\n")
	b.WriteString("(code you committed this round, or \"Nothing this round — the work holds up against the goal from my lens\")\n")
	b.WriteString("\n")
	b.WriteString("## Notes for the goal-setter\n")
	b.WriteString("(structural follow-ups spotted during scrutiny, or \"None\")\n")
	b.WriteString("```\n\n")
	b.WriteString("The cycle converges when a round passes with neither of you committing — the engine detects this automatically. No VERDICT line needed; your contribution (or stand-down) speaks for itself.\n\n")

	return invokeAgent(b.String(), cycle, fmt.Sprintf("verify-%d", round), tool)
}
