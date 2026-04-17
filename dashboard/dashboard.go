// Package dashboard is the Lathe web UI. It is self-contained: it discovers running
// lathes on the machine via process introspection, reads each project's .lathe/session/
// state directly from disk, and serves a localhost web dashboard with real-time updates
// via Server-Sent Events. Nothing here reaches into the main lathe package's state.
package dashboard

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

// daemonState is persisted at ~/.lathe/dashboard.json so subsequent `start` invocations
// can detect a running instance and just open the browser at the existing URL.
type daemonState struct {
	PID  int    `json:"pid"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

func stateFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "lathe-dashboard.json")
	}
	return filepath.Join(home, ".lathe", "dashboard.json")
}

func logPath() string {
	return filepath.Join(filepath.Dir(stateFile()), "dashboard.log")
}

func readDaemonState() (*daemonState, error) {
	data, err := os.ReadFile(stateFile())
	if err != nil {
		return nil, err
	}
	var s daemonState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func writeDaemonState(s *daemonState) error {
	path := stateFile()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, _ := json.Marshal(s)
	return os.WriteFile(path, data, 0644)
}

func clearDaemonState() {
	os.Remove(stateFile())
}

func isAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// Command dispatches `lathe dashboard <start|stop|status>`.
func Command(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: lathe dashboard <start|stop|status> [--host H] [--port P]")
		os.Exit(1)
	}
	switch args[0] {
	case "start":
		cmdStart(args[1:])
	case "stop":
		cmdStop()
	case "status":
		cmdStatus()
	default:
		fmt.Fprintf(os.Stderr, "Unknown dashboard command: %s\n", args[0])
		fmt.Println("Usage: lathe dashboard <start|stop|status> [--host H] [--port P]")
		os.Exit(1)
	}
}

func cmdStart(args []string) {
	host := "127.0.0.1"
	port := 0

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			i++
			if i < len(args) {
				host = args[i]
			}
		case "--port":
			i++
			if i < len(args) {
				p, err := strconv.Atoi(args[i])
				if err != nil || p <= 0 || p > 65535 {
					fmt.Fprintf(os.Stderr, "Invalid --port: %s\n", args[i])
					os.Exit(1)
				}
				port = p
			}
		default:
			fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
			os.Exit(1)
		}
	}

	if s, err := readDaemonState(); err == nil && isAlive(s.PID) {
		url := fmt.Sprintf("http://%s:%d", s.Host, s.Port)
		fmt.Printf("Dashboard already running (PID %d) at %s\n", s.PID, url)
		openBrowser(url)
		return
	}
	clearDaemonState()

	if port == 0 {
		port = findFreePort()
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot resolve executable: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(stateFile()), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create state dir: %v\n", err)
		os.Exit(1)
	}

	logF, err := os.Create(logPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create log: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(exe, "_dashboard_serve",
		"--host", host,
		"--port", strconv.Itoa(port),
	)
	cmd.Stdout = logF
	cmd.Stderr = logF
	setDetach(cmd)

	if err := cmd.Start(); err != nil {
		logF.Close()
		fmt.Fprintf(os.Stderr, "Error: start dashboard: %v\n", err)
		os.Exit(1)
	}

	pid := cmd.Process.Pid
	cmd.Process.Release()
	logF.Close()

	writeDaemonState(&daemonState{PID: pid, Host: host, Port: port})

	url := fmt.Sprintf("http://%s:%d", host, port)
	if waitForReady(host, port, 5*time.Second) {
		fmt.Printf("Dashboard started (PID %d) at %s\n", pid, url)
		openBrowser(url)
	} else {
		fmt.Printf("Dashboard started (PID %d) but not yet responding at %s\n", pid, url)
		fmt.Printf("Log: %s\n", logPath())
	}
}

func cmdStop() {
	s, err := readDaemonState()
	if err != nil {
		fmt.Println("Dashboard not running.")
		return
	}
	if !isAlive(s.PID) {
		clearDaemonState()
		fmt.Println("Dashboard not running (cleared stale state).")
		return
	}
	proc, _ := os.FindProcess(s.PID)
	proc.Signal(syscall.SIGTERM)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !isAlive(s.PID) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if isAlive(s.PID) {
		proc.Signal(syscall.SIGKILL)
	}
	clearDaemonState()
	fmt.Printf("Dashboard stopped (PID %d).\n", s.PID)
}

func cmdStatus() {
	s, err := readDaemonState()
	if err != nil {
		fmt.Println("Dashboard not running.")
		return
	}
	if !isAlive(s.PID) {
		fmt.Printf("Dashboard not running (stale PID %d).\n", s.PID)
		return
	}
	fmt.Printf("Dashboard running — PID %d at http://%s:%d\n", s.PID, s.Host, s.Port)
}

// findFreePort asks the OS for an available port by binding :0 then releasing.
func findFreePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 45000 + (int(time.Now().UnixNano()) % 10000)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForReady(host string, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
