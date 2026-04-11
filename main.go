package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Global paths — set once in main, used everywhere.
var (
	latheDir     = ".lathe"
	latheSession string
	latheHistory string
	goalHistory  string
	latheSkills  string
	pidFile      string
	sessionFile  string

	ciWaitTimeout  = 300 // seconds
	roundsPerCycle = 4
)

func initPaths() {
	latheSession = filepath.Join(latheDir, "session")
	latheHistory = filepath.Join(latheSession, "history")
	goalHistory = filepath.Join(latheSession, "goal-history")
	latheSkills = filepath.Join(latheDir, "skills")
	pidFile = filepath.Join(latheSession, "lathe.pid")
	sessionFile = filepath.Join(latheSession, "session.json")
}

// logWriter is where log() writes. Defaults to stderr, but engineStart
// sets it to a MultiWriter so output also goes to stream.log.
var logWriter io.Writer = os.Stderr

func log(format string, args ...any) {
	t := time.Now().Format("15:04:05")
	fmt.Fprintf(logWriter, "  [lathe] %s %s\n", t, fmt.Sprintf(format, args...))
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", fmt.Sprintf(format, args...))
	os.Exit(1)
}

func main() {
	initPaths()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		cmdInit(os.Args[2:])
	case "start":
		ensureInitialized()
		engineStart(os.Args[2:])
	case "_run":
		// Hidden command: background process entry point (called by start)
		ensureInitialized()
		engineRun(os.Args[2:])
	case "stop":
		ensureInitialized()
		engineStop()
	case "status":
		ensureInitialized()
		engineStatus(os.Args[2:])
	case "logs":
		ensureInitialized()
		engineLogs(os.Args[2:])
	default:
		die("Unknown command: %s", os.Args[1])
	}
}

func ensureInitialized() {
	if _, err := os.Stat(latheDir); os.IsNotExist(err) {
		die("Not a lathe project. Run 'lathe init' first.")
	}
	if _, err := os.Stat(filepath.Join(latheDir, "goal.md")); os.IsNotExist(err) {
		die("Missing %s/goal.md. Run 'lathe init' first.", latheDir)
	}
	if _, err := os.Stat(filepath.Join(latheDir, "snapshot.sh")); os.IsNotExist(err) {
		die("Missing %s/snapshot.sh. Run 'lathe init' first.", latheDir)
	}
}

func printUsage() {
	fmt.Println(`Usage: lathe <command> [options]

Commands:
  init     Initialize lathe for this project
  start    Start the improvement loop
  stop     Stop the loop and clean up
  status   Show current status
  logs     Show agent logs`)
}
