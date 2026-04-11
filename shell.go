package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// run executes a command, inheriting stdout/stderr.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runCapture executes a command and returns its stdout, trimmed.
func runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), err
}

// runSilent executes a command, discarding all output.
func runSilent(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

// runCaptureAll executes a command and returns combined stdout+stderr.
func runCaptureAll(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// runPipe executes a command, piping stdin from a string, and teeing output to a writer and a file.
func runPipe(input string, logFile string, name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)

	f, err := os.Create(logFile)
	if err != nil {
		return 1, fmt.Errorf("create log file: %w", err)
	}
	defer f.Close()

	// Tee to both stdout and log file
	cmd.Stdout = io.MultiWriter(os.Stdout, f)
	cmd.Stderr = io.MultiWriter(os.Stderr, f)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}
