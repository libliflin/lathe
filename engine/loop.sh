#!/usr/bin/env bash
# engine/loop.sh — Generic cycle engine.
# Sourced by bin/lathe. Expects LATHE_DIR=".lathe" and LATHE_HOME set.

# Session state — ephemeral, gitignored, wiped on stop
LATHE_SESSION="$LATHE_DIR/session"
LATHE_HISTORY="$LATHE_SESSION/history"
# Durable state — tracked, committed by agent, wiped on stop
LATHE_DECISIONS="$LATHE_DIR/decisions.md"

LATHE_SKILLS="$LATHE_DIR/skills"
PID_FILE="$LATHE_SESSION/lathe.pid"
SESSION_FILE="$LATHE_SESSION/session.json"
RETRO_INTERVAL=5
CI_WAIT_TIMEOUT=300  # seconds to wait for CI (5 min — container pulls alone can take 1-2 min)

# In direct mode, the loop polls a single named check run on the latest
# main HEAD commit. The default name is "build"; projects can override by
# writing a different name to .lathe/ci-check-name.
DIRECT_CI_CHECK_NAME_DEFAULT="build"

log() { echo "  [lathe] $(date '+%H:%M:%S') $*"; }

# ---------------------------------------------------------------------------
# Load library modules
# ---------------------------------------------------------------------------

source "$LATHE_HOME/engine/lib/process.sh"
source "$LATHE_HOME/engine/lib/state.sh"
source "$LATHE_HOME/engine/lib/ci.sh"
source "$LATHE_HOME/engine/lib/agent.sh"

# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

engine_start() {
    local max_cycles=0
    local tool="claude"
    local theme=""
    local mode="branch"

    # Project-level mode override. If .lathe/mode exists and contains
    # "direct", the engine defaults to direct (push-to-main) mode for
    # this repo. The --direct CLI flag still works and overrides the file.
    if [[ -f "$LATHE_DIR/mode" ]]; then
        local file_mode
        file_mode=$(tr -d '[:space:]' < "$LATHE_DIR/mode")
        if [[ "$file_mode" == "direct" || "$file_mode" == "branch" ]]; then
            mode="$file_mode"
        fi
    fi

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --cycles)  max_cycles="$2"; shift 2 ;;
            --tool)    tool="$2"; shift 2 ;;
            --theme)   theme="$2"; shift 2 ;;
            --direct)  mode="direct"; shift ;;
            *)         die "Unknown option: $1" ;;
        esac
    done

    if is_running; then
        echo "Already running (PID $(cat "$PID_FILE")). Use 'lathe stop' first."
        exit 1
    fi

    # Clean slate — wipe any stale session from a previous run or crashed stop
    rm -rf "$LATHE_SESSION"
    mkdir -p "$LATHE_SESSION/logs" "$LATHE_SESSION/history"

    # Persist theme so it survives across the background process boundary
    if [[ -n "$theme" ]]; then
        echo "$theme" > "$LATHE_SESSION/theme.txt"
    fi

    # Initialize session (creates branch in branch mode)
    init_session "$mode" "$theme"

    local project_name
    project_name=$(basename "$(pwd)")

    echo ""
    echo "  ╔═══════════════════════════════════════════╗"
    echo "  ║  LATHE — turning $project_name"
    echo "  ╚═══════════════════════════════════════════╝"
    echo ""

    (
        # Disable set -e inside the cycle loop — the engine must not silently
        # die because a gh/ls/python3 command returned non-zero. Each phase
        # handles its own errors explicitly.
        set +e
        trap 'exit 0' SIGTERM

        exec >> "$LATHE_SESSION/logs/stream.log" 2>&1

        local cycle
        cycle=$(get_cycle)
        local cycles_run=0

        while true; do
            echo ""
            echo "═══════════════════════════════════════════════"
            echo "  CYCLE $cycle — $(date '+%Y-%m-%d %H:%M:%S')"
            echo "═══════════════════════════════════════════════"
            echo ""

            wait_for_rate_limit
            set_cycle "$cycle" "running"

            # If we're on base (post-merge or first cycle), create a work branch
            create_session_branch

            # Phase 1: Snapshot + CI status + falsification suite
            collect_snapshot
            collect_ci_status
            collect_falsification

            # Phase 2: Agent implements one change
            run_agent "$cycle" "$tool" || true

            # Phase 3: Post-cycle cleanup
            # Archive first so safety_net commits history along with stragglers
            archive_cycle "$cycle"
            safety_net
            discover_pr

            # Give GitHub time to register the latest push before polling CI
            sleep 30

            # Phase 4: Wait for CI, merge if green
            # This makes each cycle self-contained: do work, then land it.
            # When the loop exits, the last cycle's work is already merged
            # (if CI passed) — teardown only closes work that didn't pass.
            wait_for_ci
            auto_merge_if_green

            set_cycle "$cycle" "complete"
            cycle=$((cycle + 1))
            cycles_run=$((cycles_run + 1))

            if (( max_cycles > 0 )) && (( cycles_run >= max_cycles )); then
                log "Completed $cycles_run cycles."
                exit 0
            fi

            sleep 5 &
            wait $! || exit 0
        done
    ) &

    echo $! > "$PID_FILE"
    echo "  Started (PID $!). Tool: $tool, Mode: $mode"
    if [[ "$mode" == "branch" ]]; then
        local branch
        branch=$(get_session_field "branch")
        echo "  Branch:  $branch"
    fi
    echo ""
    echo "  Logs:    lathe logs --follow"
    echo "  Status:  lathe status"
    echo "  Stop:    lathe stop"
}

engine_stop() {
    # ---------------------------------------------------------------------------
    # 1. Kill the process tree
    # ---------------------------------------------------------------------------
    local pid=""
    if [[ -f "$PID_FILE" ]]; then
        pid=$(cat "$PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            log "Stopping process tree (root PID $pid) ..."
            kill_tree "TERM" "$pid"

            # Wait for tree to die
            local attempts=0
            while kill -0 "$pid" 2>/dev/null && (( attempts < 5 )); do
                sleep 1
                attempts=$((attempts + 1))
            done

            # Force kill if still alive
            if kill -0 "$pid" 2>/dev/null; then
                log "Force-killing process tree ..."
                kill_tree "9" "$pid"
                sleep 1
            fi
        fi
        rm -f "$PID_FILE"
    fi

    # ---------------------------------------------------------------------------
    # 1b. Kill orphaned agent process if engine tree didn't catch it
    # ---------------------------------------------------------------------------
    local orphans
    orphans=$(_find_lathe_agent)
    if [[ -n "$orphans" ]]; then
        while IFS= read -r line; do
            local agent_pid
            agent_pid=$(echo "$line" | awk '{print $1}')
            log "Killing orphaned agent (PID $agent_pid) ..."
            kill_tree "TERM" "$agent_pid"
            sleep 2
            if kill -0 "$agent_pid" 2>/dev/null; then
                kill_tree "9" "$agent_pid"
            fi
        done <<< "$orphans"
    fi

    # ---------------------------------------------------------------------------
    # 2. Git + state teardown
    # ---------------------------------------------------------------------------
    teardown_session

    echo "Stopped."
}

engine_status() {
    local follow=false
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --follow|-f) follow=true; shift ;;
            *)           shift ;;
        esac
    done

    if $follow; then
        trap 'exit 0' INT
        while true; do
            clear
            _print_status
            sleep 15 &
            wait $! || exit 0
        done
    else
        _print_status
    fi
}

_print_status() {
    local project_name
    project_name=$(basename "$(pwd)")
    echo "=== Lathe: $project_name ==="

    if is_running; then
        local pid
        pid=$(cat "$PID_FILE")
        local elapsed
        elapsed=$(ps -p "$pid" -o etime= 2>/dev/null | tr -d ' ' || echo "?")
        echo "  Running — PID $pid, uptime $elapsed"
    elif [[ ! -f "$SESSION_FILE" ]]; then
        echo "  No active session. Run 'lathe start' to begin."
    else
        echo "  Stopped (session state exists — may need 'lathe stop' to clean up)"
    fi

    # Detect lathe's own agent process
    local agent_info
    agent_info=$(_find_lathe_agent)
    if [[ -n "$agent_info" ]]; then
        local agent_pid agent_elapsed
        agent_pid=$(echo "$agent_info" | awk '{print $1}')
        agent_elapsed=$(echo "$agent_info" | awk '{print $2}')
        if is_running; then
            echo "  Agent  — PID $agent_pid, uptime $agent_elapsed"
        else
            echo ""
            echo "  ** ORPHANED AGENT — PID $agent_pid, uptime $agent_elapsed **"
            echo "  Engine is dead but the agent process is still running."
            echo "  Kill with: kill $agent_pid"
        fi
    fi

    echo ""
    if [[ -f "$SESSION_FILE" ]]; then
        python3 -c "
import json
s = json.load(open('$SESSION_FILE'))
mode = s.get('mode', '?')
print(f\"  Mode: {mode}\")
if mode == 'branch':
    print(f\"  Branch: {s.get('branch', '?')}\")
    pr = s.get('pr_number', '')
    if pr:
        print(f\"  PR: #{pr}\")
    else:
        print(f\"  PR: (not yet created)\")
print(f\"  Base: {s.get('base_branch', '?')}\")
"
    fi

    if [[ -f "$LATHE_SESSION/cycle.json" ]]; then
        python3 -c "
import json
c = json.load(open('$LATHE_SESSION/cycle.json'))
print(f\"  Cycle: {c.get('cycle', '?')}  Status: {c.get('status', '?')}\")
print(f\"  Updated: {c.get('updatedAt', '?')[:19]}\")
"
    fi

    if [[ -f "$LATHE_SESSION/rate-limited" ]]; then
        echo "  ** RATE LIMITED — waiting for cooldown **"
    fi

    echo ""
    local latest
    latest=$(ls -t "$LATHE_SESSION/logs"/cycle-*.log 2>/dev/null | head -1)
    if [[ -n "$latest" ]]; then
        echo "  Latest log: $latest"
        echo "  Last 5 lines:"
        tail -5 "$latest" | sed 's/^/    /'
    fi
}

engine_logs() {
    local follow=false
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --follow|-f) follow=true; shift ;;
            *)           shift ;;
        esac
    done

    if $follow; then
        if [[ ! -f "$LATHE_SESSION/logs/stream.log" ]]; then
            echo "  No active session. Start one with 'lathe start'."
            return 0
        fi
        tail -f "$LATHE_SESSION/logs/stream.log"
    else
        local latest
        latest=$(ls -t "$LATHE_SESSION/logs"/cycle-*.log 2>/dev/null | head -1)
        if [[ -n "$latest" ]]; then
            echo "=== Latest: $(basename "$latest") ==="
            echo ""
            tail -80 "$latest"
            echo ""
            echo "---"
            echo "  Follow:  lathe logs --follow"
        else
            echo "  No logs. Start a session with 'lathe start'."
        fi
    fi
}
