package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ciResult is set by waitForCI and read by autoMergeIfGreen.
var ciResult string // "pass", "fail", "pending", "none", "timeout", "skip"

// waitForCI polls CI status until it resolves or times out.
func waitForCI() {
	s, err := readSession()
	if err != nil {
		ciResult = "skip"
		return
	}

	if s.Mode == "direct" {
		waitForCIDirect()
		return
	}

	if s.PRNumber == "" {
		log("No PR — skipping CI wait")
		ciResult = "none"
		return
	}

	log("Waiting for CI on PR #%s ...", s.PRNumber)
	waited := 0

	for waited < ciWaitTimeout {
		out, err := runCapture("gh", "pr", "checks", s.PRNumber, "--json", "bucket")
		if err != nil {
			time.Sleep(15 * time.Second)
			waited += 15
			continue
		}

		var checks []struct {
			Bucket string `json:"bucket"`
		}
		if err := json.Unmarshal([]byte(out), &checks); err != nil {
			time.Sleep(15 * time.Second)
			waited += 15
			continue
		}

		if len(checks) == 0 {
			time.Sleep(15 * time.Second)
			waited += 15
			continue
		}

		allDone := true
		anyFail := false
		for _, c := range checks {
			switch c.Bucket {
			case "fail":
				anyFail = true
			case "pending":
				allDone = false
			}
		}

		if anyFail {
			ciResult = "fail"
			log("CI: FAIL")
			return
		}
		if allDone {
			ciResult = "pass"
			log("CI: PASS")
			return
		}

		time.Sleep(15 * time.Second)
		waited += 15
	}

	ciResult = "timeout"
	log("CI: timeout after %ds", ciWaitTimeout)
}

// waitForCIDirect polls check runs on the base branch HEAD (direct mode).
func waitForCIDirect() {
	checkName := "build"
	nameFile := filepath.Join(latheDir, "ci-check-name")
	if data, err := os.ReadFile(nameFile); err == nil {
		checkName = strings.TrimSpace(string(data))
	}

	repo, err := runCapture("gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		ciResult = "skip"
		return
	}

	s, _ := readSession()
	base := s.BaseBranch
	if base == "" {
		base = "main"
	}

	sha, err := runCapture("git", "rev-parse", "origin/"+base)
	if err != nil {
		ciResult = "skip"
		return
	}

	log("Waiting for check '%s' on %s ...", checkName, sha[:8])
	waited := 0

	for waited < ciWaitTimeout {
		out, err := runCapture("gh", "api",
			fmt.Sprintf("/repos/%s/commits/%s/check-runs", repo, sha),
			"--jq", fmt.Sprintf(".check_runs[] | select(.name==\"%s\") | .status + \" \" + .conclusion", checkName))
		if err != nil || out == "" {
			time.Sleep(15 * time.Second)
			waited += 15
			continue
		}

		parts := strings.Fields(out)
		if len(parts) >= 1 && parts[0] == "completed" {
			conclusion := ""
			if len(parts) >= 2 {
				conclusion = parts[1]
			}
			if conclusion == "success" {
				ciResult = "pass"
				log("CI: PASS")
				return
			}
			ciResult = "fail"
			log("CI: FAIL (%s)", conclusion)
			return
		}

		time.Sleep(15 * time.Second)
		waited += 15
	}

	ciResult = "timeout"
	log("CI: timeout after %ds", ciWaitTimeout)
}

// preStartCleanup is called once at the top of `lathe start`, before session
// state is wiped. It surfaces any lathe/* PRs left open by a prior session
// (crash, kill, or CI timing out after lathe stop), merges the greens to
// preserve that work, and reports the rest so the user knows what's inherited.
// Fail/pending orphans are picked up by the first step's resolveStalePRs and
// exposed to the new session's agents via stale-prs.txt.
func preStartCleanup() {
	out, err := runCapture("gh", "pr", "list",
		"--state", "open",
		"--author", "@me",
		"--json", "number,headRefName",
		"--jq", `.[] | select(.headRefName | startswith("lathe/")) | [.number, .headRefName] | @tsv`)
	if err != nil || strings.TrimSpace(out) == "" {
		return
	}

	fmt.Println()
	fmt.Println("  Checking for unfinished business from a previous session ...")

	merged, fails, pendings := 0, 0, 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		prNum := strings.TrimSpace(parts[0])
		branch := strings.TrimSpace(parts[1])

		switch probePRCI(prNum) {
		case "pass":
			if err := runSilent("gh", "pr", "merge", prNum, "--squash", "--delete-branch"); err == nil {
				_ = runSilent("git", "branch", "-D", branch)
				merged++
				fmt.Printf("  ✓  Merged stale PR #%s (%s)\n", prNum, branch)
			} else {
				fmt.Printf("  ✗  Merge failed for stale PR #%s (%s): %v\n", prNum, branch, err)
			}
		case "fail":
			fails++
			fmt.Printf("  ⚠  Stale PR #%s (%s): CI failing — first cycle's agents will see it\n", prNum, branch)
		case "pending":
			pendings++
			fmt.Printf("  …  Stale PR #%s (%s): CI pending — will re-probe during the session\n", prNum, branch)
		}
	}

	if merged > 0 {
		base, _ := runCapture("git", "rev-parse", "--abbrev-ref", "HEAD")
		base = strings.TrimSpace(base)
		if base != "" {
			_ = runSilent("git", "fetch", "origin", base)
			_ = runSilent("git", "pull", "--ff-only", "origin", base)
		}
	}
	if merged+fails+pendings == 0 {
		fmt.Println("  Clean.")
	} else {
		fmt.Printf("  Summary: %d merged, %d failing, %d pending.\n", merged, fails, pendings)
	}
	fmt.Println()
}

// resolveStalePRs finds any open lathe-branch PRs (from this or earlier steps)
// and merges the ones whose CI has since turned green. Handles the case where
// CI took longer than waitForCI's per-step budget — the PR sat green but unmerged
// until the next step picked it up. Called at the top of each runStep.
//
// For PRs still failing or pending, writes context to session/stale-prs.txt so the
// next agent's prompt includes the failure log output and concrete instructions on
// how to fix or close each one. Without this file the agent would only know open
// PRs exist from the snapshot's bucket rollup — enough to notice, not enough to act.
func resolveStalePRs() {
	s, _ := readSession()
	if s.Mode != "branch" {
		return
	}

	out, err := runCapture("gh", "pr", "list",
		"--state", "open",
		"--author", "@me",
		"--json", "number,headRefName",
		"--jq", `.[] | select(.headRefName | startswith("lathe/")) | [.number, .headRefName] | @tsv`)
	if err != nil || strings.TrimSpace(out) == "" {
		clearStalePRsContext()
		return
	}

	merged := 0
	var remaining []stalePR

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		prNum := strings.TrimSpace(parts[0])
		branch := strings.TrimSpace(parts[1])

		status := probePRCI(prNum)
		switch status {
		case "pass":
			log("Resolving stale PR #%s (%s) — CI passed, merging ...", prNum, branch)
			if err := runSilent("gh", "pr", "merge", prNum, "--squash", "--delete-branch"); err != nil {
				log("WARN: merge of stale PR #%s failed: %v", prNum, err)
				remaining = append(remaining, stalePR{prNum, branch, "merge-failed"})
			} else {
				_ = runSilent("git", "branch", "-D", branch)
				merged++
				// If this was the session's active PR, clear session state so
				// createSessionBranch doesn't try to check out a branch that no
				// longer exists.
				if s.PRNumber == prNum {
					s.Branch = ""
					s.PRNumber = ""
					_ = writeSession(s)
				}
			}
		case "fail":
			log("Stale PR #%s (%s): CI failed — adding to agent context.", prNum, branch)
			remaining = append(remaining, stalePR{prNum, branch, "fail"})
		case "pending":
			log("Stale PR #%s (%s): CI still running — adding to agent context.", prNum, branch)
			remaining = append(remaining, stalePR{prNum, branch, "pending"})
		}
	}

	if merged > 0 {
		base := s.BaseBranch
		if base == "" {
			base = "main"
		}
		_ = runSilent("git", "fetch", "origin", base)
		_ = runSilent("git", "pull", "--ff-only", "origin", base)
	}

	writeStalePRsContext(remaining)
}

// stalePR describes an orphan PR passed between resolveStalePRs and writeStalePRsContext.
type stalePR struct {
	num, branch, status string
}

// writeStalePRsContext produces session/stale-prs.txt with concrete handling
// instructions per stale PR. The next agent's prompt reads this and can act.
func writeStalePRsContext(entries []stalePR) {
	if len(entries) == 0 {
		clearStalePRsContext()
		return
	}

	var b strings.Builder
	b.WriteString("# Stale Lathe PRs (from previous dialog rounds)\n\n")
	b.WriteString(fmt.Sprintf("%d open lathe PR(s) need your attention.\n\n", len(entries)))
	b.WriteString("These are orphans from earlier rounds — the engine didn't merge them because CI was still running or failed. Decide per PR:\n\n")
	b.WriteString("- **Failing CI**: check out the branch with `gh pr checkout <N>`, push a fix commit (goes to the same PR), then push. The next engine sweep will merge it once CI turns green. Or, if the work is no longer relevant, close it with `gh pr close <N> --delete-branch`.\n")
	b.WriteString("- **Pending CI**: the engine re-probes at the start of each step — leave these alone, the next sweep will merge them.\n")
	b.WriteString("- **merge-failed**: usually a conflict with base. Check out and rebase, or close if superseded.\n\n")

	for _, e := range entries {
		b.WriteString(fmt.Sprintf("## PR #%s — %s [%s]\n\n", e.num, e.branch, e.status))

		// Short title + summary
		title, _ := runCapture("gh", "pr", "view", e.num, "--json", "title", "--jq", ".title")
		if t := strings.TrimSpace(title); t != "" {
			b.WriteString("Title: " + t + "\n")
		}

		if e.status == "fail" {
			failLog := fetchPRFailureLog(e.num)
			if failLog != "" {
				b.WriteString("\n```\n")
				b.WriteString(failLog)
				b.WriteString("\n```\n")
			}
		}
		b.WriteString("\n")
	}

	// Cap overall size so prompts stay reasonable.
	s := b.String()
	const maxBytes = 8000
	if len(s) > maxBytes {
		s = s[:maxBytes] + "\n\n(truncated — inspect full detail with `gh pr view <N>` and `gh run view <runID> --log-failed`)\n"
	}
	_ = os.WriteFile(filepath.Join(latheSession, "stale-prs.txt"), []byte(s), 0644)
}

func clearStalePRsContext() {
	_ = os.Remove(filepath.Join(latheSession, "stale-prs.txt"))
}

// fetchPRFailureLog returns a compact failure summary + truncated log for a specific
// failing PR. Returns empty string when it can't be resolved.
func fetchPRFailureLog(prNumber string) string {
	checksJSON, err := runCapture("gh", "pr", "checks", prNumber, "--json", "name,bucket,link")
	if err != nil {
		return ""
	}
	var checks []struct {
		Name, Bucket, Link string
	}
	if err := json.Unmarshal([]byte(checksJSON), &checks); err != nil {
		return ""
	}

	var out strings.Builder
	for _, c := range checks {
		if c.Bucket == "fail" {
			out.WriteString(fmt.Sprintf("FAILED: %s\n", c.Name))
		}
	}

	for _, c := range checks {
		if c.Bucket != "fail" || c.Link == "" {
			continue
		}
		parts := strings.Split(c.Link, "/")
		for i, p := range parts {
			if p == "runs" && i+1 < len(parts) {
				runID := parts[i+1]
				failLog, err := runCaptureAll("gh", "run", "view", runID, "--log-failed")
				if err == nil && failLog != "" {
					// Keep per-PR log modest; tail is where the real error lives.
					if len(failLog) > 2500 {
						failLog = failLog[len(failLog)-2500:]
					}
					out.WriteString("\n--- Failed log (tail) ---\n")
					out.WriteString(failLog)
				}
				break
			}
		}
		break
	}
	return out.String()
}

// probePRCI returns "pass", "fail", or "pending" for a PR's current CI status.
// Does not wait — a single probe.
func probePRCI(prNumber string) string {
	out, err := runCapture("gh", "pr", "checks", prNumber, "--json", "bucket")
	if err != nil {
		return "pending"
	}
	var checks []struct {
		Bucket string `json:"bucket"`
	}
	if err := json.Unmarshal([]byte(out), &checks); err != nil {
		return "pending"
	}
	if len(checks) == 0 {
		return "pending"
	}
	for _, c := range checks {
		if c.Bucket == "fail" {
			return "fail"
		}
	}
	for _, c := range checks {
		if c.Bucket == "pending" {
			return "pending"
		}
	}
	return "pass"
}

// countOpenLathePRs returns how many open lathe-branch PRs (authored by us) exist.
// Used as part of convergence detection — the dialog isn't done while PRs are still open.
func countOpenLathePRs() int {
	out, err := runCapture("gh", "pr", "list",
		"--state", "open",
		"--author", "@me",
		"--json", "headRefName",
		"--jq", `[.[] | select(.headRefName | startswith("lathe/"))] | length`)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}

// autoMergeIfGreen merges the PR when CI passes and returns to base.
func autoMergeIfGreen() {
	if ciResult != "pass" {
		returnToBase()
		return
	}

	s, err := readSession()
	if err != nil || s.Mode != "branch" || s.PRNumber == "" {
		returnToBase()
		return
	}

	log("CI passed. Merging PR #%s ...", s.PRNumber)
	if err := runSilent("gh", "pr", "merge", s.PRNumber, "--squash", "--delete-branch"); err != nil {
		log("WARN: merge failed: %v", err)
		returnToBase()
		return
	}

	oldBranch := s.Branch

	// Clear branch and PR for next step
	s.Branch = ""
	s.PRNumber = ""
	writeSession(s)

	// Return to base, wait for merge to propagate, pull
	returnToBase()

	// Clean up local branch
	if oldBranch != "" {
		_ = runSilent("git", "branch", "-D", oldBranch)
	}
}

// returnToBase checks out the base branch, waits for any pending merge
// to propagate, and pulls the latest. Always leaves us on base with
// the latest code, ready for the next step.
func returnToBase() {
	s, _ := readSession()

	base := s.BaseBranch
	if base == "" {
		base = "main"
	}

	current, _ := runCapture("git", "rev-parse", "--abbrev-ref", "HEAD")
	if current == base {
		// Already on base — just pull
		_ = runSilent("git", "pull", "--ff-only", "origin", base)
		return
	}

	// Discard any uncommitted state on the work branch
	_ = runSilent("git", "reset", "--hard")
	_ = runSilent("git", "clean", "-fd")

	// Switch to base
	if err := runSilent("git", "checkout", base); err != nil {
		log("WARN: checkout %s failed, trying main", base)
		if runSilent("git", "checkout", "main") != nil {
			_ = runSilent("git", "checkout", "master")
		}
	}

	// Wait for GitHub to register the merge
	time.Sleep(5 * time.Second)

	// Pull latest (includes the just-merged squash commit)
	_ = runSilent("git", "pull", "--ff-only", "origin", base)
}

// captureCIFailureLogs fetches CI failure output and writes it to session/ci-failure.txt.
// Called after waitForCI determines CI failed, so agents can see what broke.
func captureCIFailureLogs() {
	s, err := readSession()
	if err != nil {
		return
	}

	outFile := filepath.Join(latheSession, "ci-failure.txt")

	if s.Mode == "branch" && s.PRNumber != "" {
		// Get failed check details
		checksJSON, err := runCapture("gh", "pr", "checks", s.PRNumber, "--json", "name,bucket,link")
		if err != nil {
			return
		}

		var checks []struct {
			Name   string `json:"name"`
			Bucket string `json:"bucket"`
			Link   string `json:"link"`
		}
		if err := json.Unmarshal([]byte(checksJSON), &checks); err != nil {
			return
		}

		var out strings.Builder
		for _, c := range checks {
			if c.Bucket == "fail" {
				out.WriteString(fmt.Sprintf("FAILED: %s\n", c.Name))
			}
		}

		// Try to get the failed log output from the most recent run
		// The link URL contains the run ID: https://github.com/owner/repo/actions/runs/12345/job/67890
		for _, c := range checks {
			if c.Bucket != "fail" || c.Link == "" {
				continue
			}
			// Extract run ID from link
			parts := strings.Split(c.Link, "/")
			for i, p := range parts {
				if p == "runs" && i+1 < len(parts) {
					runID := parts[i+1]
					failLog, err := runCaptureAll("gh", "run", "view", runID, "--log-failed")
					if err == nil && failLog != "" {
						// Truncate to ~4000 chars to fit in prompt
						if len(failLog) > 4000 {
							failLog = failLog[len(failLog)-4000:]
						}
						out.WriteString("\n--- Failed log output ---\n")
						out.WriteString(failLog)
					}
					break
				}
			}
			break // only need logs from first failed check
		}

		if out.Len() > 0 {
			os.WriteFile(outFile, []byte(out.String()), 0644)
		}
	} else if s.Mode == "direct" {
		// For direct mode, just record the failure — detailed logs require more API work
		os.WriteFile(outFile, []byte("CI failed in direct mode. Check the repository's Actions tab for details.\n"), 0644)
	}
}

// collectCIStatus appends CI info to the snapshot.
func collectCIStatus() {
	s, err := readSession()
	if err != nil {
		return
	}

	out := filepath.Join(latheSession, "snapshot.txt")
	f, err := os.OpenFile(out, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintln(f, "\n## CI/CD Status")

	if s.Mode == "branch" && s.PRNumber != "" {
		checks, err := runCapture("gh", "pr", "checks", s.PRNumber, "--json", "name,bucket")
		if err == nil {
			fmt.Fprintf(f, "PR #%s checks:\n```\n%s\n```\n", s.PRNumber, checks)
		}

		prState, err := runCapture("gh", "pr", "view", s.PRNumber, "--json", "state,mergeable,mergeStateStatus")
		if err == nil {
			fmt.Fprintf(f, "PR state: %s\n", prState)
		}
	} else if s.Mode == "direct" {
		fmt.Fprintln(f, "Direct mode — CI polled on commit SHA after push.")
		if ciResult != "" {
			fmt.Fprintf(f, "Last CI result: %s\n", ciResult)
		}
	} else {
		fmt.Fprintln(f, "(no PR yet — CI status will appear after first push)")
	}

	// My open PRs — gives agents visibility into orphaned/failed PRs
	fmt.Fprintln(f, "\n## My Open PRs")
	prs, err := runCapture("gh", "pr", "list", "--state", "open", "--author", "@me", "--json", "number,title,headRefName,statusCheckRollup", "--jq",
		`.[] | "#\(.number) [\(if .statusCheckRollup then (.statusCheckRollup | map(.conclusion // .status) | join(",")) else "no-checks" end)] \(.title)"`)
	if err == nil && prs != "" {
		fmt.Fprintf(f, "```\n%s\n```\n", prs)
	} else {
		fmt.Fprintln(f, "(none)")
	}
}
