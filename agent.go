package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runChampion(cycle int, tool string) error {
	log("Running champion (cycle %d) ...", cycle)

	var b strings.Builder

	// Champion's playbook (stable reference doc — read, not written)
	b.WriteString(readFileOr(filepath.Join(latheAgents, "champion.md"), ""))
	b.WriteString("\n\n")

	// Common: skills, refs, theme, snapshot
	b.WriteString(assembleCommon())

	// Session context
	b.WriteString(assembleSessionContext())

	// Last 4 cycles' reports for context
	if entries, err := filepath.Glob(filepath.Join(championHistory, "*.md")); err == nil && len(entries) > 0 {
		sort.Strings(entries)
		start := 0
		if len(entries) > 4 {
			start = len(entries) - 4
		}
		b.WriteString("---\n# Previous Cycles (last 4)\n\n")
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
	b.WriteString("You are the champion. Each cycle:\n\n")
	b.WriteString("1. When the floor is violated (CI red, build broken, tests failing), target that in the report — skip straight to step 4.\n")
	b.WriteString("2. When the Stale Lathe PRs section is present above, weigh it in: is the stuck work the right next target, or is it superseded? The report can target finishing a stale PR, or instruct the builder to close it as part of this cycle's fresh direction.\n")
	b.WriteString("3. Otherwise, pick one stakeholder (rotate based on Previous Cycles — prefer one under-served recently) and name them.\n")
	b.WriteString("4. **Become that person.** Walk their first-encounter journey — run the commands, read the output, hit the friction. Notice the emotional signal your playbook defined for them. Walking is the role; it's what earns you the standing to name what matters.\n")
	b.WriteString("5. Write your report to `.lathe/session/changelog.md` using the Output Format from your playbook (champion.md). Your reference doc `.lathe/champion.md` is not the output target — it is stable and you read from it, not write to it.\n\n")
	b.WriteString("The engine archives your report to `.lathe/session/champion-history/` for the builder.\n\n")

	return invokeAgent(b.String(), cycle, "champion", tool)
}

func runBuilder(cycle, round int, tool string) error {
	log("Running builder (cycle %d, round %d) ...", cycle, round)

	var b strings.Builder

	// Builder behavioral doc
	b.WriteString(readFileOr(filepath.Join(latheAgents, "builder.md"), ""))
	b.WriteString("\n\n")

	// Common: skills, refs, theme, snapshot
	b.WriteString(assembleCommon())

	// Session context
	b.WriteString(assembleSessionContext())

	// Current champion report
	b.WriteString("---\n# Champion's Report (this cycle)\n\n")
	reportFile := filepath.Join(championHistory, fmt.Sprintf("cycle-%03d.md", cycle))
	changelogFile := filepath.Join(latheSession, "changelog.md")
	if data, err := os.ReadFile(reportFile); err == nil {
		b.Write(data)
	} else if data, err := os.ReadFile(changelogFile); err == nil {
		b.Write(data)
	} else {
		b.WriteString("(no champion report found for this cycle — use your best judgment based on the snapshot)")
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
	b.WriteString(readFileOr(filepath.Join(latheAgents, "verifier.md"), ""))
	b.WriteString("\n\n")

	// Common: skills, refs, theme, snapshot
	b.WriteString(assembleCommon())

	// Session context
	b.WriteString(assembleSessionContext())

	// Champion's report for this cycle
	b.WriteString("---\n# Champion's Report (this cycle)\n\n")
	reportFile := filepath.Join(championHistory, fmt.Sprintf("cycle-%03d.md", cycle))
	b.WriteString(readFileOr(reportFile, "(no champion report found)"))
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
	b.WriteString("## Notes for the champion\n")
	b.WriteString("(structural follow-ups spotted during scrutiny, or \"None\")\n")
	b.WriteString("```\n\n")
	b.WriteString("The cycle converges when a round passes with neither of you committing — the engine detects this automatically. No VERDICT line needed; your contribution (or stand-down) speaks for itself.\n\n")

	return invokeAgent(b.String(), cycle, fmt.Sprintf("verify-%d", round), tool)
}
