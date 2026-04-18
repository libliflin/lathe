package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Override globals for test
	latheDir = filepath.Join(dir, ".lathe")
	latheSession = filepath.Join(latheDir, "session")
	latheHistory = filepath.Join(latheSession, "history")
	sessionFile = filepath.Join(latheSession, "session.json")
	latheSkills = filepath.Join(latheDir, "skills")

	os.MkdirAll(latheSession, 0755)
	os.MkdirAll(latheHistory, 0755)

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

func TestSetPhase(t *testing.T) {
	setupTestState(t)

	id := "20260418-120000"
	if err := setPhase(id, "champion"); err != nil {
		t.Fatalf("setPhase: %v", err)
	}

	c, err := readCycleState()
	if err != nil {
		t.Fatalf("readCycleState: %v", err)
	}
	if c.ID != id {
		t.Errorf("ID = %q, want %q", c.ID, id)
	}
	if c.Phase != "champion" {
		t.Errorf("Phase = %q, want %q", c.Phase, "champion")
	}
	if c.UpdatedAt == "" {
		t.Error("UpdatedAt should not be empty")
	}
}

func TestArchiveCycle(t *testing.T) {
	setupTestState(t)

	os.WriteFile(filepath.Join(latheSession, "snapshot.txt"), []byte("test snapshot"), 0644)
	os.WriteFile(filepath.Join(latheSession, "journey.md"), []byte("test journey"), 0644)
	os.WriteFile(filepath.Join(latheSession, "whiteboard.md"), []byte("test whiteboard"), 0644)

	id := "20260418-120000"
	if err := archiveCycle(id); err != nil {
		t.Fatalf("archiveCycle: %v", err)
	}

	dir := filepath.Join(latheHistory, id)
	for _, name := range []string{"snapshot.txt", "journey.md", "whiteboard.md"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read archived %s: %v", name, err)
		}
		if !strings.Contains(string(data), "test ") {
			t.Errorf("%s missing expected content: %q", name, string(data))
		}
	}
}

func TestRecentJourneys(t *testing.T) {
	setupTestState(t)

	ids := []string{"20260418-100000", "20260418-110000", "20260418-120000"}
	for _, id := range ids {
		dir := filepath.Join(latheHistory, id)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "journey.md"), []byte("journey from "+id), 0644)
	}

	got := recentJourneys(2)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != ids[1] || got[1].ID != ids[2] {
		t.Errorf("IDs = %v, want last two of %v", []string{got[0].ID, got[1].ID}, ids)
	}
}

func TestWipeWhiteboard(t *testing.T) {
	setupTestState(t)

	path := filepath.Join(latheSession, "whiteboard.md")
	os.WriteFile(path, []byte("some content"), 0644)
	wipeWhiteboard()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read whiteboard: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("whiteboard not wiped: %q", string(data))
	}
}
