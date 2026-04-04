#!/usr/bin/env bash
# engine/loop.sh — Generic cycle engine.
# Sourced by bin/lathe. Expects LATHE_DIR=".lathe" and LATHE_HOME set.

LATHE_STATE="$LATHE_DIR/state"
LATHE_HISTORY="$LATHE_STATE/history"
LATHE_SKILLS="$LATHE_DIR/skills"
PID_FILE="$LATHE_STATE/lathe.pid"
RETRO_INTERVAL=5

log() { echo "  [lathe] $(date '+%H:%M:%S') $*"; }

# ---------------------------------------------------------------------------
# State helpers
# ---------------------------------------------------------------------------

get_cycle() {
    if [[ -f "$LATHE_STATE/cycle.json" ]]; then
        python3 -c "import json; print(json.load(open('$LATHE_STATE/cycle.json')).get('cycle', 1))"
    else
        echo 1
    fi
}

set_cycle() {
    local cycle="$1"
    local status="${2:-running}"
    python3 -c "
import json
from datetime import datetime, timezone
data = {'cycle': $cycle, 'status': '$status', 'updatedAt': datetime.now(timezone.utc).isoformat()}
json.dump(data, open('$LATHE_STATE/cycle.json', 'w'), indent=2)
"
}

archive_cycle() {
    local cycle="$1"
    local cycle_dir
    cycle_dir=$(printf "%s/cycle-%03d" "$LATHE_HISTORY" "$cycle")
    mkdir -p "$cycle_dir"
    for f in snapshot.txt changelog.md; do
        [[ -f "$LATHE_STATE/$f" ]] && cp "$LATHE_STATE/$f" "$cycle_dir/"
    done
}

# ---------------------------------------------------------------------------
# Phase 1: Snapshot — run project's snapshot.sh
# ---------------------------------------------------------------------------

collect_snapshot() {
    log "Collecting project snapshot ..."
    local out="$LATHE_STATE/snapshot.txt"

    if [[ -x "$LATHE_DIR/snapshot.sh" ]]; then
        "$LATHE_DIR/snapshot.sh" > "$out" 2>&1
    else
        echo "(no snapshot script found)" > "$out"
    fi

    log "Snapshot written: $out"
}

# ---------------------------------------------------------------------------
# Phase 2: Agent — assemble prompt and call LLM
# ---------------------------------------------------------------------------

run_agent() {
    local cycle="$1"
    local tool="${2:-claude}"

    log "Running agent (cycle $cycle) ..."

    local prompt=""

    # Agent identity
    prompt+="$(cat "$LATHE_DIR/agent.md")"
    prompt+=$'\n\n'

    # All skills
    for skill_file in "$LATHE_SKILLS"/*.md; do
        if [[ -f "$skill_file" ]]; then
            prompt+="---"$'\n'
            prompt+="# Skill: $(basename "$skill_file" .md)"$'\n\n'
            prompt+="$(cat "$skill_file")"
            prompt+=$'\n\n'
        fi
    done

    # Permanent decisions
    if [[ -f "$LATHE_STATE/decisions.md" ]]; then
        prompt+="---"$'\n'
        prompt+="# PERMANENT DECISIONS — DO NOT REVISIT"$'\n\n'
        prompt+="$(cat "$LATHE_STATE/decisions.md")"
        prompt+=$'\n\n'
    fi

    # Current snapshot
    prompt+="---"$'\n'
    prompt+="# Current Project Snapshot"$'\n\n'
    if [[ -f "$LATHE_STATE/snapshot.txt" ]]; then
        prompt+="$(cat "$LATHE_STATE/snapshot.txt")"
    else
        prompt+="(no snapshot collected)"
    fi
    prompt+=$'\n\n'

    # Previous cycle changelog
    local prev_cycle=$((cycle - 1))
    local prev_dir
    prev_dir=$(printf "%s/cycle-%03d" "$LATHE_HISTORY" "$prev_cycle")
    if [[ -f "$prev_dir/changelog.md" ]]; then
        prompt+="---"$'\n'
        prompt+="# Previous Cycle Changelog (Cycle $prev_cycle)"$'\n\n'
        prompt+="$(cat "$prev_dir/changelog.md")"
        prompt+=$'\n\n'
    fi

    # Retro mode: every N cycles, inject last N changelogs
    if (( cycle > 1 )) && (( cycle % RETRO_INTERVAL == 0 )); then
        prompt+="---"$'\n'
        prompt+="# Retro Mode — Last $RETRO_INTERVAL Cycles"$'\n'
        prompt+="Review the last $RETRO_INTERVAL cycles for patterns. Are we stuck? Making progress? Repeating the same fix?"$'\n\n'
        local start_cycle=$((cycle - RETRO_INTERVAL))
        (( start_cycle < 1 )) && start_cycle=1
        for (( c=start_cycle; c<cycle; c++ )); do
            local cdir
            cdir=$(printf "%s/cycle-%03d" "$LATHE_HISTORY" "$c")
            if [[ -f "$cdir/changelog.md" ]]; then
                prompt+="## Cycle $c"$'\n'
                prompt+='```'$'\n'
                prompt+="$(cat "$cdir/changelog.md")"
                prompt+=$'\n''```'$'\n\n'
            fi
        done
    fi

    # Pre-cycle hook
    if [[ -x "$LATHE_DIR/hooks/pre-cycle.sh" ]]; then
        log "Running pre-cycle hook ..."
        "$LATHE_DIR/hooks/pre-cycle.sh" || log "WARN: pre-cycle hook failed (non-fatal)"
    fi

    # Invoke LLM
    local log_dir="$LATHE_STATE/logs"
    mkdir -p "$log_dir"
    local log_file="$log_dir/cycle-$(printf '%03d' "$cycle").log"

    log "Prompt assembled. Invoking $tool ..."
    local exit_code=0

    if [[ "$tool" == "claude" ]]; then
        echo "$prompt" | claude --dangerously-skip-permissions --print 2>&1 \
            | tee "$log_file" || exit_code=$?
    elif [[ "$tool" == "amp" ]]; then
        echo "$prompt" | amp --dangerously-allow-all 2>&1 \
            | tee "$log_file" || exit_code=$?
    else
        die "Unknown tool: $tool"
    fi

    # Rate limit detection
    if grep -q "You've hit your limit" "$log_file" 2>/dev/null; then
        log "Rate limited. Ending cycle early."
        echo "RATE_LIMITED" > "$LATHE_STATE/rate-limited"
        return 1
    fi

    rm -f "$LATHE_STATE/rate-limited"
    log "Agent complete (exit $exit_code). Log: $log_file"
    return "$exit_code"
}

# ---------------------------------------------------------------------------
# Rate limit backoff
# ---------------------------------------------------------------------------

wait_for_rate_limit() {
    if [[ ! -f "$LATHE_STATE/rate-limited" ]]; then
        return 0
    fi
    log "Rate limited from previous cycle. Waiting 5 minutes ..."
    local waited=0
    while (( waited < 300 )); do
        sleep 30 &
        wait $! || return 0
        waited=$((waited + 30))
        log "Rate limit cooldown: $((300 - waited))s remaining ..."
    done
    rm -f "$LATHE_STATE/rate-limited"
    log "Cooldown complete. Resuming."
}

# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

is_running() {
    [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null
}

engine_start() {
    local max_cycles=0
    local tool="claude"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --cycles) max_cycles="$2"; shift 2 ;;
            --tool)   tool="$2"; shift 2 ;;
            *)        die "Unknown option: $1" ;;
        esac
    done

    if is_running; then
        echo "Already running (PID $(cat "$PID_FILE")). Use 'lathe stop' first."
        exit 1
    fi

    mkdir -p "$LATHE_STATE" "$LATHE_HISTORY" "$LATHE_STATE/logs"

    local project_name
    project_name=$(basename "$(pwd)")

    echo ""
    echo "  ╔═══════════════════════════════════════════╗"
    echo "  ║  LATHE — turning $project_name"
    echo "  ╚═══════════════════════════════════════════╝"
    echo ""

    (
        trap 'exit 0' SIGTERM

        exec >> "$LATHE_STATE/logs/stream.log" 2>&1

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

            # Phase 1: Snapshot
            collect_snapshot

            # Phase 2: Agent
            run_agent "$cycle" "$tool" || true

            # Phase 3: Commit + archive
            if ! git diff --quiet HEAD 2>/dev/null || [[ -n "$(git ls-files --others --exclude-standard)" ]]; then
                git add -A
                git commit -m "lathe: cycle ${cycle}" || true
                git push origin main 2>/dev/null || log "WARN: push failed (non-fatal)"
            fi

            archive_cycle "$cycle"
            set_cycle "$cycle" "complete"
            cycle=$((cycle + 1))
            cycles_run=$((cycles_run + 1))

            if (( max_cycles > 0 )) && (( cycles_run >= max_cycles )); then
                log "Completed $cycles_run cycles. Stopping."
                exit 0
            fi

            sleep 5 &
            wait $! || exit 0
        done
    ) &

    echo $! > "$PID_FILE"
    echo "  Started (PID $!). Tool: $tool"
    echo ""
    echo "  Logs:    tail -f $LATHE_STATE/logs/stream.log"
    echo "  Status:  lathe status"
    echo "  Stop:    lathe stop"
}

engine_stop() {
    if ! is_running; then
        echo "Not running."
        [[ -f "$PID_FILE" ]] && rm -f "$PID_FILE"
        return 0
    fi
    local pid
    pid=$(cat "$PID_FILE")

    pkill -TERM -P "$pid" 2>/dev/null || true
    kill -TERM "$pid" 2>/dev/null || true

    local _
    for _ in 1 2 3 4 5; do
        kill -0 "$pid" 2>/dev/null || break
        sleep 1
    done

    if kill -0 "$pid" 2>/dev/null; then
        pkill -9 -P "$pid" 2>/dev/null || true
        kill -9 "$pid" 2>/dev/null || true
    fi

    rm -f "$PID_FILE"
    echo "Stopped (PID $pid)."
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
    else
        echo "  Stopped"
    fi

    echo ""
    if [[ -f "$LATHE_STATE/cycle.json" ]]; then
        python3 -c "
import json
c = json.load(open('$LATHE_STATE/cycle.json'))
print(f\"  Cycle: {c.get('cycle', '?')}  Status: {c.get('status', '?')}\")
print(f\"  Updated: {c.get('updatedAt', '?')[:19]}\")
"
    fi

    if [[ -f "$LATHE_STATE/rate-limited" ]]; then
        echo "  ** RATE LIMITED — waiting for cooldown **"
    fi

    echo ""
    local latest
    latest=$(ls -t "$LATHE_STATE/logs"/cycle-*.log 2>/dev/null | head -1)
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
        tail -f "$LATHE_STATE/logs/stream.log"
    else
        local latest
        latest=$(ls -t "$LATHE_STATE/logs"/cycle-*.log 2>/dev/null | head -1)
        if [[ -n "$latest" ]]; then
            echo "=== Latest: $(basename "$latest") ==="
            echo ""
            tail -80 "$latest"
            echo ""
            echo "---"
            echo "  Follow:  lathe logs --follow"
        else
            echo "  No logs yet."
        fi
    fi
}
