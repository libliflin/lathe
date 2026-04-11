package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssembleCommonWithSkillsAndRefs(t *testing.T) {
	dir := setupTestState(t)

	// Create skills
	os.MkdirAll(latheSkills, 0755)
	os.WriteFile(filepath.Join(latheSkills, "testing.md"), []byte("Run tests with: go test ./..."), 0644)

	// Create refs
	os.MkdirAll(filepath.Join(latheDir, "refs"), 0755)
	os.WriteFile(filepath.Join(latheDir, "refs", "api.md"), []byte("API spec v2"), 0644)

	// Create theme
	os.WriteFile(filepath.Join(latheSession, "theme.txt"), []byte("harden edge cases"), 0644)

	// Create snapshot
	os.WriteFile(filepath.Join(latheSession, "snapshot.txt"), []byte("all tests pass"), 0644)

	_ = dir
	result := assembleCommon()

	if !strings.Contains(result, "# Skill: testing") {
		t.Error("expected skill header")
	}
	if !strings.Contains(result, "go test ./...") {
		t.Error("expected skill content")
	}
	if !strings.Contains(result, "# Reference: api") {
		t.Error("expected reference header")
	}
	if !strings.Contains(result, "API spec v2") {
		t.Error("expected reference content")
	}
	if !strings.Contains(result, "harden edge cases") {
		t.Error("expected theme")
	}
	if !strings.Contains(result, "all tests pass") {
		t.Error("expected snapshot")
	}
}

func TestAssembleCommonNoSnapshot(t *testing.T) {
	setupTestState(t)

	result := assembleCommon()
	if !strings.Contains(result, "(no snapshot collected)") {
		t.Error("expected fallback when no snapshot exists")
	}
}

func TestAssembleSessionContextBranch(t *testing.T) {
	setupTestState(t)

	writeSession(Session{
		Mode:       "branch",
		Branch:     "lathe/test-branch",
		BaseBranch: "main",
		PRNumber:   "99",
	})

	result := assembleSessionContext()
	if !strings.Contains(result, "lathe/test-branch") {
		t.Error("expected branch name")
	}
	if !strings.Contains(result, "#99") {
		t.Error("expected PR number")
	}
}

func TestAssembleSessionContextDirect(t *testing.T) {
	setupTestState(t)

	writeSession(Session{
		Mode:       "direct",
		BaseBranch: "main",
	})

	result := assembleSessionContext()
	if !strings.Contains(result, "direct mode") {
		t.Error("expected direct mode context")
	}
}
