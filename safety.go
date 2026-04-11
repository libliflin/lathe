package main

// safetyNet catches uncommitted changes the agent left behind.
func safetyNet() {
	s, err := readSession()
	if err != nil {
		return
	}

	// Check if working tree is clean
	if err := runSilent("git", "diff", "--quiet", "HEAD"); err == nil {
		untracked, _ := runCapture("git", "ls-files", "--others", "--exclude-standard")
		if untracked == "" {
			return
		}
	}

	log("Safety net: agent left uncommitted changes")

	current, _ := runCapture("git", "rev-parse", "--abbrev-ref", "HEAD")

	if s.Mode == "branch" && current != s.Branch && s.Branch != "" {
		log("Safety net: changes on wrong branch (%s), expected %s", current, s.Branch)
		_ = runSilent("git", "stash", "--include-untracked")
		if err := runSilent("git", "checkout", s.Branch); err != nil {
			_ = runSilent("git", "checkout", "-b", s.Branch)
		}
		_ = runSilent("git", "stash", "pop")
	}

	// Commit whatever the agent left — but never commit session state
	_ = runSilent("git", "add", "-A")
	_ = runSilent("git", "reset", "HEAD", "--", ".lathe/session/")
	_ = runSilent("git", "commit", "-m", "lathe: cleanup (agent left uncommitted changes)")

	if s.Mode == "branch" && s.Branch != "" {
		if err := runSilent("git", "push", "origin", s.Branch); err != nil {
			log("WARN: push failed (non-fatal)")
		}
	} else if s.Mode == "direct" {
		if err := runSilent("git", "push", "origin", s.BaseBranch); err != nil {
			log("WARN: push failed (non-fatal)")
		}
	}
}
