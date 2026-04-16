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

// assembleFrictionBlock mirrors the friction-inclusion logic in runGoalSetter.
// It's extracted here so we can test the behaviour without invoking an LLM.
func assembleFrictionBlock() string {
	var b strings.Builder
	if data, err := os.ReadFile(frictionFile); err == nil && len(data) > 0 {
		b.WriteString("---\n# Stakeholder Friction Report (this cycle)\n\n")
		b.Write(data)
		b.WriteString("\n\n")
	}
	return b.String()
}

func TestFrictionIncludedWhenPresent(t *testing.T) {
	setupTestState(t)

	content := "## Stakeholder: Developer\n\nGot stuck on missing docs."
	if err := os.WriteFile(frictionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := assembleFrictionBlock()
	if !strings.Contains(result, "# Stakeholder Friction Report (this cycle)") {
		t.Error("expected friction section header")
	}
	if !strings.Contains(result, "Got stuck on missing docs.") {
		t.Error("expected friction file content")
	}
}

func TestFrictionOmittedWhenAbsent(t *testing.T) {
	setupTestState(t)
	// frictionFile does not exist

	result := assembleFrictionBlock()
	if strings.Contains(result, "Friction Report") {
		t.Error("expected no friction section when file is absent")
	}
}

func TestFrictionOmittedWhenEmpty(t *testing.T) {
	setupTestState(t)

	if err := os.WriteFile(frictionFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	result := assembleFrictionBlock()
	if strings.Contains(result, "Friction Report") {
		t.Error("expected no friction section when file is empty")
	}
}
