package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
