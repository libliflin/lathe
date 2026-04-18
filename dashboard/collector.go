package dashboard

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Lathe is the snapshot of one running lathe process, as shown on the dashboard.
type Lathe struct {
	PID             int        `json:"pid"`
	AgentPID        int        `json:"agent_pid,omitempty"`
	Project         string     `json:"project"`
	CWD             string     `json:"cwd"`
	Branch          string     `json:"branch"`
	BaseBranch      string     `json:"base_branch"`
	PRNumber        string     `json:"pr_number,omitempty"`
	Mode            string     `json:"mode"`
	CycleID        string     `json:"cycle_id,omitempty"` // timestamp-based ID e.g. 20260418-083045
	Phase          string     `json:"phase,omitempty"`
	ElapsedSeconds int64      `json:"elapsed_seconds"`
	RateLimited    bool       `json:"rate_limited"`
	RecentCommits  []Commit   `json:"recent_commits"`
	RecentLogs     []LogLine  `json:"recent_logs"`
	CycleStats     CycleStats `json:"cycle_stats"`
	Journey        string     `json:"journey,omitempty"`    // champion's journey for the current cycle
	Whiteboard     string     `json:"whiteboard,omitempty"` // shared scratchpad (current state)
	RepoURL        string     `json:"repo_url,omitempty"`
}

type Commit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

type LogLine struct {
	Time    string `json:"time"`
	Message string `json:"message"`
}

// CycleStats summarizes the round-to-converge history for a given lathe.
// In the dialog model, the earliest possible convergence is round 2 (round 1
// always has the builder contributing their first implementation), so "clean"
// means converged in ≤2 rounds. Legacy field names (pass_one / pass_late) are
// kept on the wire for backwards compat with older dashboard bundles.
type CycleStats struct {
	Total    int      `json:"total"`     // cycles touched (includes in-progress)
	PassOne  int      `json:"pass_one"`  // cycles that converged cleanly (round ≤ 2)
	PassLate int      `json:"pass_late"` // cycles that needed a deeper dialog (round ≥ 3)
	Failed   int      `json:"failed"`    // cycles that hit the oscillation cap
	Verdicts []string `json:"verdicts"`  // chronological, for sparkline ("P2", "P3", "P4", "F")
}

type Snapshot struct {
	Lathes      []Lathe   `json:"lathes"`
	CollectedAt time.Time `json:"collected_at"`
}

// Collect discovers all lathes running on the machine by scanning for `lathe _run`
// processes, then reads each one's session state from disk.
func Collect() Snapshot {
	snap := Snapshot{CollectedAt: time.Now()}

	out, err := exec.Command("pgrep", "-f", "lathe _run").Output()
	if err != nil {
		return snap
	}

	seen := map[int]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || seen[pid] {
			continue
		}
		seen[pid] = true
		if l, ok := readLathe(pid); ok {
			snap.Lathes = append(snap.Lathes, l)
		}
	}
	return snap
}

func readLathe(pid int) (Lathe, bool) {
	cwd := getPidCwd(pid)
	if cwd == "" {
		return Lathe{}, false
	}

	sessionPath := filepath.Join(cwd, ".lathe", "session", "session.json")
	sdata, err := os.ReadFile(sessionPath)
	if err != nil {
		return Lathe{}, false
	}
	var sess struct {
		Branch     string `json:"branch"`
		BaseBranch string `json:"base_branch"`
		PRNumber   string `json:"pr_number"`
		Mode       string `json:"mode"`
	}
	if err := json.Unmarshal(sdata, &sess); err != nil {
		return Lathe{}, false
	}

	l := Lathe{
		PID:        pid,
		CWD:        cwd,
		Project:    filepath.Base(cwd),
		Branch:     sess.Branch,
		BaseBranch: sess.BaseBranch,
		PRNumber:   sess.PRNumber,
		Mode:       sess.Mode,
	}

	if cdata, err := os.ReadFile(filepath.Join(cwd, ".lathe", "session", "cycle.json")); err == nil {
		// New shape: {id, phase, started_at, updated_at}
		var cNew struct {
			ID    string `json:"id"`
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal(cdata, &cNew); err == nil && (cNew.ID != "" || cNew.Phase != "") {
			l.CycleID = cNew.ID
			l.Phase = cNew.Phase
		} else {
			// Legacy shape: {cycle, status} — handle old lathes still running
			var cOld struct {
				Cycle  int    `json:"cycle"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(cdata, &cOld); err == nil {
				l.Phase = cOld.Status
			}
		}
	}

	if _, err := os.Stat(filepath.Join(cwd, ".lathe", "session", "rate-limited")); err == nil {
		l.RateLimited = true
	}

	l.ElapsedSeconds = getPidElapsed(pid)
	l.AgentPID = findAgentForCwd(cwd)
	l.RecentCommits = getRecentCommits(cwd, 5)
	l.RecentLogs = tailStreamLog(cwd, 40)
	l.CycleStats = computeCycleStats(cwd)
	l.Journey = readSessionFile(cwd, "journey.md", 2000)
	l.Whiteboard = readSessionFile(cwd, "whiteboard.md", 2000)
	l.RepoURL = getRepoURL(cwd)

	return l, true
}

// getPidCwd returns the working directory of a PID using lsof. Returns "" on failure.
func getPidCwd(pid int) string {
	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid), "-a", "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
}

// getPidElapsed returns elapsed seconds since PID started. Uses `ps -o etime=`
// because `etimes` (raw-seconds variant) is Linux-only — BSD/macOS ps doesn't
// support it and silently fails, returning 0. `etime` outputs `[[dd-]hh:]mm:ss`
// on both platforms.
func getPidElapsed(pid int) int64 {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "etime=").Output()
	if err != nil {
		return 0
	}
	return parseEtime(strings.TrimSpace(string(out)))
}

// parseEtime converts ps's etime format `[[dd-]hh:]mm:ss` into elapsed seconds.
// Handles: "12:34" (mm:ss), "01:12:34" (hh:mm:ss), "2-01:12:34" (d-hh:mm:ss).
func parseEtime(s string) int64 {
	if s == "" {
		return 0
	}
	var days int64
	if i := strings.Index(s, "-"); i >= 0 {
		days, _ = strconv.ParseInt(s[:i], 10, 64)
		s = s[i+1:]
	}
	parts := strings.Split(s, ":")
	var h, m, sec int64
	switch len(parts) {
	case 1:
		sec, _ = strconv.ParseInt(parts[0], 10, 64)
	case 2:
		m, _ = strconv.ParseInt(parts[0], 10, 64)
		sec, _ = strconv.ParseInt(parts[1], 10, 64)
	case 3:
		h, _ = strconv.ParseInt(parts[0], 10, 64)
		m, _ = strconv.ParseInt(parts[1], 10, 64)
		sec, _ = strconv.ParseInt(parts[2], 10, 64)
	}
	return days*86400 + h*3600 + m*60 + sec
}

// findAgentForCwd returns the PID of the claude/amp subprocess whose cwd matches.
func findAgentForCwd(cwd string) int {
	out, err := exec.Command("pgrep", "-f", "claude.*--dangerously-skip-permissions.*--print").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			continue
		}
		if getPidCwd(pid) == cwd {
			return pid
		}
	}
	return 0
}

func getRecentCommits(cwd string, n int) []Commit {
	// --no-pager so git doesn't try to invoke a pager; safe when stdout is a pipe.
	cmd := exec.Command("git", "-C", cwd, "log", "-n", strconv.Itoa(n),
		"--pretty=format:%h|%s|%cr")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		commits = append(commits, Commit{
			SHA:     parts[0],
			Message: parts[1],
			Time:    parts[2],
		})
	}
	return commits
}

func getRepoURL(cwd string) string {
	cmd := exec.Command("git", "-C", cwd, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	// Normalize git@github.com:owner/repo.git to https://github.com/owner/repo
	if strings.HasPrefix(url, "git@") {
		url = strings.Replace(url, ":", "/", 1)
		url = strings.Replace(url, "git@", "https://", 1)
	}
	url = strings.TrimSuffix(url, ".git")
	return url
}

// tailStreamLog returns the last n lines of the session stream log, parsed into LogLines.
// Lines look like: "  [lathe] 18:26:30 Message text"
func tailStreamLog(cwd string, n int) []LogLine {
	path := filepath.Join(cwd, ".lathe", "session", "logs", "stream.log")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	start := len(lines) - n - 1
	if start < 0 {
		start = 0
	}
	var out []LogLine
	for _, raw := range lines[start:] {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		// Parse `[lathe] HH:MM:SS rest...`
		t, msg := "", raw
		if i := strings.Index(raw, "[lathe]"); i >= 0 {
			rest := strings.TrimSpace(raw[i+len("[lathe]"):])
			parts := strings.SplitN(rest, " ", 2)
			if len(parts) == 2 {
				t, msg = parts[0], parts[1]
			} else {
				msg = rest
			}
		}
		out = append(out, LogLine{Time: t, Message: msg})
	}
	return out
}

// computeCycleStats scans stream.log for convergence markers and cycle boundaries.
// Tracks how many rounds each cycle took to converge — the dialog shape.
// Accepts both the dialog-model phrases and the legacy VERDICT phrases so the
// dashboard works across binary versions.
func computeCycleStats(cwd string) CycleStats {
	path := filepath.Join(cwd, ".lathe", "session", "logs", "stream.log")
	data, err := os.ReadFile(path)
	if err != nil {
		return CycleStats{}
	}
	var stats CycleStats
	roundsThisCycle := 0
	inCycle := false

	converge := func(round int) {
		// "Clean" = converged in the fewest possible rounds for the dialog model.
		// Round 1 always has the builder contributing (first impl), so the
		// earliest a round can end with both sides standing down is round 2.
		if round <= 2 {
			stats.PassOne++
		} else {
			stats.PassLate++
		}
		stats.Verdicts = append(stats.Verdicts, "P"+strconv.Itoa(round))
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "CYCLE ") && strings.Contains(line, "—") {
			if inCycle {
				stats.Total++
			}
			inCycle = true
			roundsThisCycle = 0
		}
		if strings.Contains(line, "round-") && strings.Contains(line, "-build ...") {
			roundsThisCycle++
		}
		// Dialog-model: "Convergence reached at round N."
		// Legacy: "Verifier passed on round N. Moving to next goal."
		if strings.Contains(line, "Convergence reached at round") || strings.Contains(line, "Verifier passed on round") {
			converge(roundsThisCycle)
		}
		// Dialog-model: "Oscillation cap reached"
		// Legacy: "Max rounds reached"
		if strings.Contains(line, "Oscillation cap reached") || strings.Contains(line, "Max rounds reached") {
			stats.Failed++
			stats.Verdicts = append(stats.Verdicts, "F")
		}
	}
	if inCycle {
		stats.Total++
	}
	if len(stats.Verdicts) > 24 {
		stats.Verdicts = stats.Verdicts[len(stats.Verdicts)-24:]
	}
	return stats
}

// readSessionFile returns the truncated contents of a named file inside the
// project's .lathe/session/ directory. Empty when the file is missing.
func readSessionFile(cwd, name string, maxBytes int) string {
	data, err := os.ReadFile(filepath.Join(cwd, ".lathe", "session", name))
	if err != nil {
		return ""
	}
	s := string(data)
	if len(s) > maxBytes {
		s = s[:maxBytes] + "\n... (truncated)"
	}
	return s
}
