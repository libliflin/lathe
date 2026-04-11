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
		return
	}

	s, err := readSession()
	if err != nil || s.Mode != "branch" || s.PRNumber == "" {
		return
	}

	log("CI passed. Merging PR #%s ...", s.PRNumber)
	if err := runSilent("gh", "pr", "merge", s.PRNumber, "--squash", "--delete-branch"); err != nil {
		log("WARN: merge failed: %v", err)
		return
	}

	// Return to base
	_ = runSilent("git", "checkout", s.BaseBranch)
	_ = runSilent("git", "pull", "--ff-only", "origin", s.BaseBranch)
	_ = runSilent("git", "branch", "-D", s.Branch)

	// Wait for GitHub propagation
	time.Sleep(10 * time.Second)

	// Clear branch and PR for next step
	s.Branch = ""
	s.PRNumber = ""
	writeSession(s)
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
}
