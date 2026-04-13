package main

import (
	"os"
	"path/filepath"
	"testing"
)


func TestEnsureGitignore(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	// No .gitignore yet
	ensureGitignore()
	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if got := string(data); !contains(got, ".lathe/session/") {
		t.Errorf("expected .lathe/session/ in .gitignore, got %q", got)
	}

	// Running again should not duplicate
	ensureGitignore()
	data, _ = os.ReadFile(".gitignore")
	count := 0
	for _, line := range splitLines(string(data)) {
		if line == ".lathe/session/" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence, got %d", count)
	}
}

func contains(s, sub string) bool {
	return filepath.Base(sub) != "" && len(s) > 0 && len(sub) > 0 && stringContains(s, sub)
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
