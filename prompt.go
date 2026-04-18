package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// assembleCommon builds the shared prompt block: skills + refs + theme + snapshot.
func assembleCommon() string {
	var b strings.Builder

	// Skills
	if entries, err := filepath.Glob(filepath.Join(latheSkills, "*.md")); err == nil {
		for _, f := range entries {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			name := strings.TrimSuffix(filepath.Base(f), ".md")
			b.WriteString("---\n# Skill: " + name + "\n\n")
			b.Write(data)
			b.WriteString("\n\n")
		}
	}

	// References
	if entries, err := filepath.Glob(filepath.Join(latheDir, "refs", "*.md")); err == nil {
		for _, f := range entries {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			name := strings.TrimSuffix(filepath.Base(f), ".md")
			b.WriteString("---\n# Reference: " + name + "\n\n")
			b.Write(data)
			b.WriteString("\n\n")
		}
	}

	// Brand — project character, read by champion and builder as a tint on decisions.
	// Lives at .lathe/brand.md (not under agents/) because it's a reference doc loaded
	// into every prompt, not a role that runs in the loop.
	if data, err := os.ReadFile(filepath.Join(latheDir, "brand.md")); err == nil {
		b.WriteString("---\n# Brand\n\n")
		b.Write(data)
		b.WriteString("\n\n")
	}

	// Theme
	themeFile := filepath.Join(latheSession, "theme.txt")
	if data, err := os.ReadFile(themeFile); err == nil {
		theme := strings.TrimSpace(string(data))
		if theme != "" {
			b.WriteString("---\n# Theme\n\n")
			b.WriteString("The user started this session with a purpose: **" + theme + "**\n\n")
		}
	}

	// Snapshot (truncated to maxSnapshotChars to keep agents at decision-making altitude)
	b.WriteString("---\n# Current Project Snapshot\n\n")
	snapshotFile := filepath.Join(latheSession, "snapshot.txt")
	if data, err := os.ReadFile(snapshotFile); err == nil {
		snapshot := string(data)
		if len(snapshot) > maxSnapshotChars {
			b.WriteString(snapshot[:maxSnapshotChars])
			b.WriteString(fmt.Sprintf("\n\n⚠ SNAPSHOT TRUNCATED — %d of %d chars shown. You are missing context.\n", maxSnapshotChars, len(snapshot)))
			b.WriteString("Fix this: edit `.lathe/snapshot.sh` to produce a shorter, crisper report.\n")
			b.WriteString("Summarize (pass/fail counts, not raw output). The full snapshot is at `.lathe/session/snapshot.txt`.\n")
		} else {
			b.WriteString(snapshot)
		}
	} else {
		b.WriteString("(no snapshot collected)")
	}
	b.WriteString("\n\n")

	return b.String()
}

// assembleSessionContext builds the branch/PR/CI session context.
func assembleSessionContext() string {
	s, err := readSession()
	if err != nil {
		return ""
	}

	var b strings.Builder

	switch s.Mode {
	case "branch":
		b.WriteString("---\n# Session Context\n\n")
		b.WriteString("You are working on branch `" + s.Branch + "` (base: `" + s.BaseBranch + "`).\n\n")
		if s.PRNumber != "" {
			b.WriteString("There is an open PR: #" + s.PRNumber + ". Push your commits to this branch.\n\n")
		} else {
			b.WriteString("No PR exists yet. After your first commit and push, create one with `gh pr create --base " + s.BaseBranch + "`.\n\n")
		}
		b.WriteString("After your work: `git add`, `git commit`, `git push origin " + s.Branch + "`. If no PR exists yet, create one with `gh pr create --base " + s.BaseBranch + "`.\n\n")

	case "direct":
		base := s.BaseBranch
		if base == "" {
			base = "main"
		}
		b.WriteString("---\n# Session Context\n\n")
		b.WriteString("You are working in **direct mode**: commits go straight to `" + base + "`.\n\n")
		b.WriteString("After your work: `git add`, `git commit -S`, `git push origin " + base + "`.\n\n")
	}

	return b.String()
}

// readFileOr reads a file and returns its content, or a fallback string.
func readFileOr(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	return string(data)
}
