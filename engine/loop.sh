#!/usr/bin/env bash
# engine/loop.sh — Generic cycle engine.
# Sourced by bin/lathe. Expects LATHE_DIR=".lathe" and LATHE_HOME set.

LATHE_STATE="$LATHE_DIR/state"
LATHE_HISTORY="$LATHE_STATE/history"
LATHE_SKILLS="$LATHE_DIR/skills"
PID_FILE="$LATHE_STATE/lathe.pid"
SESSION_FILE="$LATHE_STATE/session.json"
RETRO_INTERVAL=5
CI_WAIT_TIMEOUT=120  # seconds to wait for CI before treating as timeout

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
# Session state — branch and PR tracking
# ---------------------------------------------------------------------------

get_session_field() {
    local field="$1"
    if [[ -f "$SESSION_FILE" ]]; then
        python3 -c "import json; print(json.load(open('$SESSION_FILE')).get('$field', ''))" 2>/dev/null
    fi
}

set_session_field() {
    local field="$1"
    local value="$2"
    python3 -c "
import json, os
path = '$SESSION_FILE'
data = {}
if os.path.exists(path):
    data = json.load(open(path))
data['$field'] = '$value'
json.dump(data, open(path, 'w'), indent=2)
"
}

init_session() {
    local mode="$1"
    local theme="$2"

    if [[ "$mode" == "direct" ]]; then
        python3 -c "
import json
from datetime import datetime, timezone
data = {
    'mode': 'direct',
    'base_branch': '$(git rev-parse --abbrev-ref HEAD)',
    'started_at': datetime.now(timezone.utc).isoformat()
}
json.dump(data, open('$SESSION_FILE', 'w'), indent=2)
"
        return
    fi

    local base_branch
    base_branch=$(git rev-parse --abbrev-ref HEAD)
    local ts
    ts=$(date '+%Y%m%d-%H%M%S')
    local branch_name="lathe/${ts}"
    if [[ -n "$theme" ]]; then
        local slug
        slug=$(echo "$theme" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | cut -c1-30)
        branch_name="lathe/${slug}-${ts}"
    fi

    git checkout -b "$branch_name"
    log "Created branch: $branch_name (base: $base_branch)"

    python3 -c "
import json
from datetime import datetime, timezone
data = {
    'mode': 'branch',
    'branch': '$branch_name',
    'base_branch': '$base_branch',
    'pr_number': '',
    'started_at': datetime.now(timezone.utc).isoformat()
}
json.dump(data, open('$SESSION_FILE', 'w'), indent=2)
"
}

discover_pr() {
    local branch
    branch=$(get_session_field "branch")
    if [[ -z "$branch" ]]; then return; fi

    local pr_number
    pr_number=$(gh pr list --head "$branch" --json number --jq '.[0].number' 2>/dev/null || true)
    if [[ -n "$pr_number" ]]; then
        set_session_field "pr_number" "$pr_number"
        log "Discovered PR #$pr_number for branch $branch"
    fi
}

# ---------------------------------------------------------------------------
# CI polling — block until checks complete or timeout
# ---------------------------------------------------------------------------

# Returns CI status via the CI_RESULT variable: pass, fail, timeout, none, skip
wait_for_ci() {
    CI_RESULT="skip"
    if ! command -v gh &>/dev/null; then return 0; fi

    local pr_number
    pr_number=$(get_session_field "pr_number")
    if [[ -z "$pr_number" ]]; then return 0; fi

    log "Waiting for CI on PR #$pr_number (timeout: ${CI_WAIT_TIMEOUT}s) ..."
    local waited=0
    local interval=15

    while (( waited < CI_WAIT_TIMEOUT )); do
        local status
        status=$(gh pr checks "$pr_number" --json bucket --jq 'map(.bucket) | if length == 0 then "none" elif any(. == "fail") then "fail" elif any(. == "pending") then "pending" elif all(. == "pass" or . == "skipping") then "pass" else "none" end' 2>/dev/null || echo "none")

        case "$status" in
            pass)
                log "CI passed on PR #$pr_number"
                CI_RESULT="pass"
                return 0
                ;;
            fail)
                log "CI failed on PR #$pr_number"
                CI_RESULT="fail"
                return 0
                ;;
            none)
                log "No CI checks found for PR #$pr_number"
                CI_RESULT="none"
                return 0
                ;;
            pending)
                sleep "$interval" &
                wait $! || return 0
                waited=$((waited + interval))
                log "CI still running ... (${waited}s / ${CI_WAIT_TIMEOUT}s)"
                ;;
        esac
    done

    log "CI timed out after ${CI_WAIT_TIMEOUT}s on PR #$pr_number — treating as signal"
    CI_RESULT="timeout"
    return 0
}

# Engine merges the PR when CI passes, pulls, creates fresh branch.
# This is plumbing — no LLM tokens spent on mechanical bookkeeping.
auto_merge_if_green() {
    if [[ "$CI_RESULT" != "pass" ]]; then return 0; fi

    local mode
    mode=$(get_session_field "mode")
    if [[ "$mode" != "branch" ]]; then return 0; fi

    local pr_number
    pr_number=$(get_session_field "pr_number")
    if [[ -z "$pr_number" ]]; then return 0; fi

    log "CI green on PR #$pr_number — merging ..."
    if ! gh pr merge "$pr_number" --squash --delete-branch 2>/dev/null; then
        log "WARN: auto-merge failed on PR #$pr_number — agent will see this in snapshot"
        return 0
    fi
    log "Merged PR #$pr_number"

    # Switch to base, pull, create fresh branch
    local base_branch
    base_branch=$(get_session_field "base_branch")
    git checkout "$base_branch" 2>/dev/null || true
    git pull origin "$base_branch" 2>/dev/null || true

    local ts
    ts=$(date '+%Y%m%d-%H%M%S')
    local theme=""
    [[ -f "$LATHE_STATE/theme.txt" ]] && theme=$(cat "$LATHE_STATE/theme.txt")
    local branch_name="lathe/${ts}"
    if [[ -n "$theme" ]]; then
        local slug
        slug=$(echo "$theme" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | cut -c1-30)
        branch_name="lathe/${slug}-${ts}"
    fi

    git checkout -b "$branch_name"
    set_session_field "branch" "$branch_name"
    set_session_field "pr_number" ""
    log "New branch: $branch_name — ready for next change"
}

# SECURITY MODEL: The snapshot feeds directly into the LLM prompt.
# Everything fetched from GitHub is a potential prompt injection vector.
# Rules:
# - Only fetch structured fields (numbers, statuses, booleans, timestamps)
# - Never fetch free-text fields (title, body, comments, commit messages, displayTitle)
# - Only list PRs authored by the current authenticated gh user
# - Init should verify branch protection settings
collect_ci_status() {
    if ! command -v gh &>/dev/null; then
        echo "" >> "$LATHE_STATE/snapshot.txt"
        echo "## CI/CD Status" >> "$LATHE_STATE/snapshot.txt"
        echo "(gh CLI not installed — no CI visibility)" >> "$LATHE_STATE/snapshot.txt"
        return
    fi

    local ci_section=""
    ci_section+=$'\n'"## CI/CD Status"$'\n'

    local mode
    mode=$(get_session_field "mode")
    local branch
    branch=$(get_session_field "branch")
    local pr_number
    pr_number=$(get_session_field "pr_number")

    # SECURITY: Only fetch structured fields (numbers, statuses, booleans).
    # Never fetch free-text fields (title, body, comments, commit messages)
    # as they are prompt injection vectors via PR comments or commit messages.

    if [[ -n "$pr_number" ]]; then
        ci_section+=$'\n'"### Primary PR: #$pr_number (branch: $branch)"$'\n'
        ci_section+='```'$'\n'
        ci_section+="$(gh pr checks "$pr_number" --json name,bucket,startedAt,completedAt --jq '.[] | "\(.name): \(.bucket)"' 2>/dev/null || echo "(could not fetch checks)")"
        ci_section+=$'\n''```'$'\n'

        ci_section+=$'\n'"### PR State"$'\n'
        ci_section+='```'$'\n'
        ci_section+="$(gh pr view "$pr_number" --json number,state,mergeable,mergeStateStatus --jq '{number,state,mergeable,mergeStateStatus}' 2>/dev/null || echo "(could not fetch PR state)")"
        ci_section+=$'\n''```'$'\n'
    elif [[ "$mode" == "branch" ]]; then
        ci_section+="Current branch: $branch (no PR created yet)"$'\n'
    fi

    # Show all open PRs by the current gh user — structured fields only
    ci_section+=$'\n'"### All Open PRs (by current user)"$'\n'
    ci_section+='```'$'\n'
    ci_section+="$(gh pr list --author '@me' --json number,headRefName,state,statusCheckRollup --jq '.[] | "#\(.number) [\(.headRefName)] state:\(.state) checks:\((.statusCheckRollup // []) | map(.bucket // .state) | if length == 0 then "none" elif any(. == "fail" or . == "FAILURE") then "FAILING" elif any(. == "pending" or . == "PENDING") then "pending" else "pass" end)"' 2>/dev/null || echo "(could not list PRs)")"
    ci_section+=$'\n''```'$'\n'

    ci_section+=$'\n'"### CI Configuration"$'\n'
    if ls .github/workflows/*.yml &>/dev/null 2>&1 || ls .github/workflows/*.yaml &>/dev/null 2>&1; then
        ci_section+="Workflows found:"$'\n'
        ci_section+="$(ls .github/workflows/*.yml .github/workflows/*.yaml 2>/dev/null)"$'\n'
    elif [[ -f ".gitlab-ci.yml" ]]; then
        ci_section+="GitLab CI config found: .gitlab-ci.yml"$'\n'
    else
        ci_section+="**No CI/CD configuration found.** The project has no automated validation beyond local commands. Creating CI is likely the highest-value first step."$'\n'
    fi

    # Workflow runs: only structured fields (no displayTitle which could contain injection)
    ci_section+=$'\n'"### Recent Workflow Runs"$'\n'
    ci_section+='```'$'\n'
    ci_section+="$(gh run list --limit 5 --json databaseId,status,conclusion,event,headBranch,createdAt --jq '.[] | "#\(.databaseId) \(.status)/\(.conclusion // "—") event:\(.event) branch:\(.headBranch) at:\(.createdAt[:19])"' 2>/dev/null || echo "(no workflow runs)")"
    ci_section+=$'\n''```'$'\n'

    echo "$ci_section" >> "$LATHE_STATE/snapshot.txt"
}

# ---------------------------------------------------------------------------
# Safety net — catch uncommitted changes the agent left behind
# ---------------------------------------------------------------------------

safety_net() {
    local mode
    mode=$(get_session_field "mode")

    # Nothing to do if working tree is clean
    if git diff --quiet HEAD 2>/dev/null && [[ -z "$(git ls-files --others --exclude-standard)" ]]; then
        return 0
    fi

    log "Safety net: agent left uncommitted changes"

    local current_branch
    current_branch=$(git rev-parse --abbrev-ref HEAD)
    local session_branch
    session_branch=$(get_session_field "branch")

    if [[ "$mode" == "branch" && "$current_branch" != "$session_branch" ]]; then
        log "Safety net: changes on wrong branch ($current_branch), expected $session_branch"
        # Stash, switch, apply
        git stash --include-untracked
        git checkout "$session_branch" 2>/dev/null || git checkout -b "$session_branch"
        git stash pop
    fi

    # Commit whatever the agent left
    git add -A
    git commit -m "lathe: cycle cleanup (agent left uncommitted changes)" || true

    if [[ "$mode" == "branch" && -n "$session_branch" ]]; then
        git push origin "$session_branch" 2>/dev/null || log "WARN: push failed (non-fatal)"
    elif [[ "$mode" == "direct" ]]; then
        local base
        base=$(get_session_field "base_branch")
        git push origin "$base" 2>/dev/null || log "WARN: push failed (non-fatal)"
    fi
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

    # Theme — why the user put this on the lathe today
    if [[ -f "$LATHE_STATE/theme.txt" ]]; then
        local theme_text
        theme_text=$(cat "$LATHE_STATE/theme.txt")
        prompt+="---"$'\n'
        prompt+="# Theme"$'\n\n'
        prompt+="The user started this session with a purpose: **$theme_text**"$'\n\n'
        prompt+="Use this to guide your pick step. The stakeholder framework in agent.md still applies — the theme tells you where to focus within it, not to override it."$'\n\n'
    fi

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

    # Session context — branch, PR, workflow
    local session_mode
    session_mode=$(get_session_field "mode")
    if [[ "$session_mode" == "branch" ]]; then
        local session_branch
        session_branch=$(get_session_field "branch")
        local session_pr
        session_pr=$(get_session_field "pr_number")
        local session_base
        session_base=$(get_session_field "base_branch")

        prompt+="---"$'\n'
        prompt+="# Session Context"$'\n\n'
        prompt+="You are working on branch \`$session_branch\` (base: \`$session_base\`)."$'\n\n'

        if [[ -n "$session_pr" ]]; then
            prompt+="There is an open PR: #$session_pr. Push your commits to this branch. The CI status is in the snapshot above."$'\n\n'
        else
            prompt+="No PR exists yet. After your first commit and push, create one with \`gh pr create\`."$'\n\n'
        fi

        prompt+="**Your responsibilities this cycle:**"$'\n'
        prompt+="- If CI failed on the previous PR: fixing the failure is your top priority. Read the failure, understand it, fix it. Push the fix to this branch."$'\n'
        prompt+="- If CI timed out (took >2 minutes): that's a signal. Consider making CI faster as a priority."$'\n'
        prompt+="- If there is no CI: creating a basic CI workflow (GitHub Actions, etc.) is likely the highest-value first change. Start minimal — just run the project's test command."$'\n'
        prompt+="- If CI is failing for external reasons (dependency outage, vulnerability scanner, upstream issue): use your judgment. Sometimes a workaround is right. Sometimes you keep working on the current branch. Explain your reasoning in the changelog."$'\n'
        prompt+="- Otherwise: implement your one change, commit, push to \`$session_branch\`."$'\n\n'
        prompt+="The engine handles merging PRs when CI passes and creating fresh branches. You never need to merge PRs or create branches yourself."$'\n'
        prompt+="After implementing your change: \`git add\`, \`git commit\`, \`git push origin $session_branch\`. If no PR exists yet, create one with \`gh pr create\`."$'\n\n'
    fi

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
        prompt+="Review the last $RETRO_INTERVAL cycles for patterns:"$'\n'
        prompt+="- Are we stuck? Making progress? Repeating the same fix?"$'\n'
        prompt+="- Which stakeholder benefited from each cycle? Is any stakeholder being neglected?"$'\n'
        prompt+="- Are we still aligned with the theme (if set) and the stakeholder priorities in agent.md?"$'\n\n'
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
    local theme=""
    local mode="branch"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --cycles)  max_cycles="$2"; shift 2 ;;
            --tool)    tool="$2"; shift 2 ;;
            --theme)   theme="$2"; shift 2 ;;
            --direct)  mode="direct"; shift ;;
            *)         die "Unknown option: $1" ;;
        esac
    done

    # Persist theme so it survives across the background process boundary
    if [[ -n "$theme" ]]; then
        echo "$theme" > "$LATHE_STATE/theme.txt"
    else
        rm -f "$LATHE_STATE/theme.txt"
    fi

    if is_running; then
        echo "Already running (PID $(cat "$PID_FILE")). Use 'lathe stop' first."
        exit 1
    fi

    mkdir -p "$LATHE_STATE" "$LATHE_HISTORY" "$LATHE_STATE/logs"

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

            # Phase 1.5: Wait for CI, auto-merge if green
            wait_for_ci
            auto_merge_if_green

            # Phase 1.6: Re-snapshot after potential merge (fresh branch state)
            # and append CI status
            if [[ "$CI_RESULT" == "pass" ]]; then
                collect_snapshot
            fi
            collect_ci_status

            # Phase 2: Agent — always gets a clean slate after merge
            run_agent "$cycle" "$tool" || true

            # Phase 3: Post-cycle cleanup
            safety_net
            discover_pr

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
