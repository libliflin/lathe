package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTestState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Override globals for test
	latheDir = filepath.Join(dir, ".lathe")
	latheSession = filepath.Join(latheDir, "session")
	latheHistory = filepath.Join(latheSession, "history")
	championHistory = filepath.Join(latheSession, "champion-history")
	sessionFile = filepath.Join(latheSession, "session.json")
	latheSkills = filepath.Join(latheDir, "skills")

	os.MkdirAll(latheSession, 0755)
	os.MkdirAll(latheHistory, 0755)
	os.MkdirAll(championHistory, 0755)

	return dir
}

func TestReadWriteSession(t *testing.T) {
	setupTestState(t)

	s := Session{
		Mode:       "branch",
		Branch:     "lathe/20260411-120000",
		BaseBranch: "main",
		PRNumber:   "42",
		StartedAt:  "2026-04-11T12:00:00Z",
	}

	if err := writeSession(s); err != nil {
		t.Fatalf("writeSession: %v", err)
	}

	got, err := readSession()
	if err != nil {
		t.Fatalf("readSession: %v", err)
	}

	if got.Mode != "branch" {
		t.Errorf("Mode = %q, want %q", got.Mode, "branch")
	}
	if got.Branch != "lathe/20260411-120000" {
		t.Errorf("Branch = %q, want %q", got.Branch, "lathe/20260411-120000")
	}
	if got.PRNumber != "42" {
		t.Errorf("PRNumber = %q, want %q", got.PRNumber, "42")
	}
}

func TestGetSetCycle(t *testing.T) {
	setupTestState(t)

	// Default should be 1 when no file exists
	if got := getCycle(); got != 1 {
		t.Errorf("getCycle() = %d, want 1", got)
	}

	if err := setCycle(3, "running"); err != nil {
		t.Fatalf("setCycle: %v", err)
	}

	if got := getCycle(); got != 3 {
		t.Errorf("getCycle() = %d, want 3", got)
	}

	// Verify status in file
	data, _ := os.ReadFile(filepath.Join(latheSession, "cycle.json"))
	var c CycleState
	json.Unmarshal(data, &c)
	if c.Status != "running" {
		t.Errorf("Status = %q, want %q", c.Status, "running")
	}
	if c.UpdatedAt == "" {
		t.Error("UpdatedAt should not be empty")
	}
}

func TestArchiveCycle(t *testing.T) {
	setupTestState(t)

	// Write files to archive
	os.WriteFile(filepath.Join(latheSession, "snapshot.txt"), []byte("test snapshot"), 0644)
	os.WriteFile(filepath.Join(latheSession, "changelog.md"), []byte("test changelog"), 0644)

	if err := archiveCycle(1); err != nil {
		t.Fatalf("archiveCycle: %v", err)
	}

	// Check archived files
	dir := filepath.Join(latheHistory, "cycle-001")
	data, err := os.ReadFile(filepath.Join(dir, "snapshot.txt"))
	if err != nil {
		t.Fatalf("read archived snapshot: %v", err)
	}
	if string(data) != "test snapshot" {
		t.Errorf("snapshot = %q, want %q", string(data), "test snapshot")
	}

	data, err = os.ReadFile(filepath.Join(dir, "changelog.md"))
	if err != nil {
		t.Fatalf("read archived changelog: %v", err)
	}
	if string(data) != "test changelog" {
		t.Errorf("changelog = %q, want %q", string(data), "test changelog")
	}
}

func TestArchiveChampion(t *testing.T) {
	setupTestState(t)

	os.WriteFile(filepath.Join(latheSession, "changelog.md"), []byte("champion: fix tests"), 0644)

	if err := archiveChampion(2); err != nil {
		t.Fatalf("archiveChampion: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(championHistory, "cycle-002.md"))
	if err != nil {
		t.Fatalf("read archived champion report: %v", err)
	}
	if string(data) != "champion: fix tests" {
		t.Errorf("report = %q, want %q", string(data), "champion: fix tests")
	}
}

func TestArchiveChampionNoChangelog(t *testing.T) {
	setupTestState(t)

	// Should not error when no changelog exists
	if err := archiveChampion(1); err != nil {
		t.Fatalf("archiveChampion with no changelog: %v", err)
	}
}
