package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectType(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	// No files → generic
	if got := detectType(); got != "generic" {
		t.Errorf("empty dir: got %q, want %q", got, "generic")
	}

	// go.mod → go
	os.WriteFile("go.mod", []byte("module test"), 0644)
	if got := detectType(); got != "go" {
		t.Errorf("go.mod: got %q, want %q", got, "go")
	}
	os.Remove("go.mod")

	// Cargo.toml → rust
	os.WriteFile("Cargo.toml", []byte("[package]"), 0644)
	if got := detectType(); got != "rust" {
		t.Errorf("Cargo.toml: got %q, want %q", got, "rust")
	}
	os.Remove("Cargo.toml")

	// package.json → node
	os.WriteFile("package.json", []byte("{}"), 0644)
	if got := detectType(); got != "node" {
		t.Errorf("package.json: got %q, want %q", got, "node")
	}
	os.Remove("package.json")

	// requirements.txt → python
	os.WriteFile("requirements.txt", []byte("flask"), 0644)
	if got := detectType(); got != "python" {
		t.Errorf("requirements.txt: got %q, want %q", got, "python")
	}
	os.Remove("requirements.txt")

	// k8s yaml
	os.WriteFile("deploy.yaml", []byte("apiVersion: apps/v1"), 0644)
	if got := detectType(); got != "k8s" {
		t.Errorf("k8s yaml: got %q, want %q", got, "k8s")
	}
	os.Remove("deploy.yaml")
}

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
