package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/libliflin/lathe/dashboard"
)

// Global paths — set once in main, used everywhere.
var (
	latheDir     = ".lathe"
	latheAgents  string
	latheSession string
	latheHistory string
	latheSkills  string
	pidFile      string
	sessionFile  string

	ciWaitTimeout    = 300   // seconds
	roundsPerCycle   = 20    // oscillation cap — a dialog that hasn't converged by 20 rounds needs human review
	maxSnapshotChars = 6000 // truncate snapshot — tight cap pressures crisp snapshots
)

func initPaths() {
	latheAgents = filepath.Join(latheDir, "agents")
	latheSession = filepath.Join(latheDir, "session")
	latheHistory = filepath.Join(latheSession, "history")
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
	case "dashboard":
		// Machine-wide dashboard — does not require a lathe-initialized project.
		dashboard.Command(os.Args[2:])
	case "_dashboard_serve":
		// Hidden: background HTTP server entry for the dashboard daemon.
		dashboard.Serve(os.Args[2:])
	case "update":
		cmdUpdate()
	case "version", "--version", "-v":
		cmdVersion()
	default:
		die("Unknown command: %s", os.Args[1])
	}
}

func ensureInitialized() {
	if _, err := os.Stat(latheDir); os.IsNotExist(err) {
		die("Not a lathe project. Run 'lathe init' first.")
	}
	// Accept any of three possible locations for the champion's playbook during the transition:
	// 1. .lathe/agents/champion.md (current)
	// 2. .lathe/champion.md (previous refactor, pre-agents/ move)
	// 3. .lathe/goal.md (original name)
	// preStartCleanup handles the migration forward.
	candidates := []string{
		filepath.Join(latheAgents, "champion.md"),
		filepath.Join(latheDir, "champion.md"),
		filepath.Join(latheDir, "goal.md"),
	}
	found := false
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			found = true
			break
		}
	}
	if !found {
		die("Missing %s/agents/champion.md. Run 'lathe init' first.", latheDir)
	}
	if _, err := os.Stat(filepath.Join(latheDir, "snapshot.sh")); os.IsNotExist(err) {
		die("Missing %s/snapshot.sh. Run 'lathe init' first.", latheDir)
	}
}

func printUsage() {
	fmt.Println(`Usage: lathe <command> [options]

Commands:
  init       Initialize lathe for this project
  start      Start the improvement loop
  stop       Stop the loop and clean up
  status     Show current status
  logs       Show agent logs
  dashboard  Start/stop the machine-wide web dashboard
  update     Update to the latest version
  version    Show current version`)
}
