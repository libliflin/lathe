package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Session tracks branch and PR state for the current run.
type Session struct {
	Mode       string `json:"mode"`
	Branch     string `json:"branch"`
	BaseBranch string `json:"base_branch"`
	PRNumber   string `json:"pr_number"`
	StartedAt  string `json:"started_at"`
}

// CycleState tracks the current cycle number and phase.
type CycleState struct {
	Cycle     int    `json:"cycle"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

func readSession() (Session, error) {
	var s Session
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func writeSession(s Session) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionFile, data, 0644)
}

func getCycle() int {
	cycleFile := filepath.Join(latheSession, "cycle.json")
	data, err := os.ReadFile(cycleFile)
	if err != nil {
		return 1
	}
	var c CycleState
	if err := json.Unmarshal(data, &c); err != nil {
		return 1
	}
	if c.Cycle < 1 {
		return 1
	}
	return c.Cycle
}

func setCycle(cycle int, status string) error {
	c := CycleState{
		Cycle:     cycle,
		Status:    status,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(latheSession, "cycle.json"), data, 0644)
}

func archiveCycle(cycle int) error {
	dir := filepath.Join(latheHistory, fmt.Sprintf("cycle-%03d", cycle))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, name := range []string{"snapshot.txt", "changelog.md"} {
		src := filepath.Join(latheSession, name)
		if _, err := os.Stat(src); err == nil {
			data, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func archiveChampion(cycle int) error {
	if err := os.MkdirAll(championHistory, 0755); err != nil {
		return err
	}
	src := filepath.Join(latheSession, "changelog.md")
	if _, err := os.Stat(src); err != nil {
		return nil // no changelog to archive
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	dst := filepath.Join(championHistory, fmt.Sprintf("cycle-%03d.md", cycle))
	return os.WriteFile(dst, data, 0644)
}

// appendToFile appends text to a file, creating it if needed.
func appendToFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

func initSessionState(mode, theme string) error {
	if err := os.MkdirAll(filepath.Join(latheSession, "logs"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(latheHistory, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(championHistory, 0755); err != nil {
		return err
	}

	baseBranch, err := runCapture("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}

	s := Session{
		Mode:       mode,
		BaseBranch: baseBranch,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	if mode == "branch" {
		ts := time.Now().Format("20060102-150405")
		branch := "lathe/" + ts
		if theme != "" {
			slug := strings.ReplaceAll(strings.ToLower(theme), " ", "-")
			if len(slug) > 30 {
				slug = slug[:30]
			}
			branch = "lathe/" + slug + "-" + ts
		}
		s.Branch = branch
		if err := runSilent("git", "checkout", "-b", branch); err != nil {
			return fmt.Errorf("create branch %s: %w", branch, err)
		}
	}

	return writeSession(s)
}

func createSessionBranch() error {
	s, err := readSession()
	if err != nil || s.Mode != "branch" {
		return err
	}

	current, err := runCapture("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}

	// If the session already has an active PR from a previous step in this cycle,
	// continue on that branch instead of cutting fresh. This keeps a single goal's
	// work on one PR across rounds — when CI takes longer than a step's waitForCI
	// budget, the next step pushes to the same PR rather than orphaning it.
	if s.Branch != "" && s.PRNumber != "" {
		if current == s.Branch {
			return nil // already on it
		}
		// Fetch in case the branch was pushed from a prior step that didn't pull back.
		_ = runSilent("git", "fetch", "origin", s.Branch)
		if err := runSilent("git", "checkout", s.Branch); err != nil {
			// Branch is gone — PR was closed/merged outside the engine. Clear and cut fresh.
			log("session branch %s no longer available — cutting fresh", s.Branch)
			s.Branch = ""
			s.PRNumber = ""
			_ = writeSession(s)
		} else {
			log("Continuing on open lathe branch %s (PR #%s)", s.Branch, s.PRNumber)
			return nil
		}
	}

	if current != s.BaseBranch {
		return nil // already on a work branch (e.g. freshly checked out by initSessionState)
	}

	// Pull latest base, cut a fresh branch for this step
	_ = runSilent("git", "pull", "--ff-only", "origin", s.BaseBranch)

	ts := time.Now().Format("20060102-150405")
	branch := "lathe/" + ts

	// Use theme if available
	themeFile := filepath.Join(latheSession, "theme.txt")
	if data, err := os.ReadFile(themeFile); err == nil {
		slug := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(string(data))), " ", "-")
		if len(slug) > 30 {
			slug = slug[:30]
		}
		branch = "lathe/" + slug + "-" + ts
	}

	if err := runSilent("git", "checkout", "-b", branch); err != nil {
		return fmt.Errorf("create branch %s: %w", branch, err)
	}

	s.Branch = branch
	s.PRNumber = ""
	return writeSession(s)
}

func discoverPR() error {
	s, err := readSession()
	if err != nil || s.Mode != "branch" || s.Branch == "" {
		return err
	}
	if s.PRNumber != "" {
		return nil // already know the PR
	}

	out, err := runCapture("gh", "pr", "list", "--head", s.Branch, "--json", "number", "--jq", ".[0].number")
	if err != nil || out == "" {
		return nil // no PR yet
	}

	s.PRNumber = out
	return writeSession(s)
}

func teardownSession() {
	s, _ := readSession()

	// Return to base branch (reset, checkout, wait, pull)
	returnToBase()

	// Close PR and delete remote branch
	if s.Mode == "branch" && s.PRNumber != "" {
		_ = runSilent("gh", "pr", "close", s.PRNumber, "--delete-branch")
	}

	// Delete local lathe branches
	if s.Mode == "branch" && s.Branch != "" {
		_ = runSilent("git", "branch", "-D", s.Branch)
	}

	// Wipe session state
	os.RemoveAll(latheSession)
}
