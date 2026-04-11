# engine/lib/state.sh — State helpers, session management, teardown
# Sourced by engine/loop.sh. Expects LATHE_DIR, LATHE_SESSION, SESSION_FILE set.

# ---------------------------------------------------------------------------
# Cycle state
# ---------------------------------------------------------------------------

get_cycle() {
    if [[ -f "$LATHE_SESSION/cycle.json" ]]; then
        python3 -c "import json; print(json.load(open('$LATHE_SESSION/cycle.json')).get('cycle', 1))"
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
json.dump(data, open('$LATHE_SESSION/cycle.json', 'w'), indent=2)
"
}

archive_cycle() {
    local cycle="$1"
    local cycle_dir
    cycle_dir=$(printf "%s/cycle-%03d" "$LATHE_HISTORY" "$cycle")
    mkdir -p "$cycle_dir"
    for f in snapshot.txt changelog.md; do
        [[ -f "$LATHE_SESSION/$f" ]] && cp "$LATHE_SESSION/$f" "$cycle_dir/"
    done
}

archive_goal() {
    local cycle="$1"
    mkdir -p "$LATHE_GOAL_HISTORY"
    # Copy the goal-setter's changelog as the goal record for this cycle
    if [[ -f "$LATHE_SESSION/changelog.md" ]]; then
        cp "$LATHE_SESSION/changelog.md" \
            "$LATHE_GOAL_HISTORY/cycle-$(printf '%03d' "$cycle").md"
    fi
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

# Create a fresh lathe branch for the next cycle of work.
# Called at the top of the cycle loop when we're on base after a merge.
create_session_branch() {
    local mode
    mode=$(get_session_field "mode")
    # Direct mode pushes to main — never create work branches
    if [[ "$mode" == "direct" ]]; then return 0; fi

    local base_branch
    base_branch=$(get_session_field "base_branch")
    local current
    current=$(git rev-parse --abbrev-ref HEAD)

    # Only create a branch if we're on base (post-merge or first cycle after restart)
    if [[ "$current" != "$base_branch" ]]; then return 0; fi
    # And only if session doesn't already have a branch
    local existing
    existing=$(get_session_field "branch")
    if [[ -n "$existing" ]]; then return 0; fi

    # Pull latest — someone else (or another lathe) may have merged since last cycle
    git pull origin "$base_branch" 2>/dev/null || true

    local ts
    ts=$(date '+%Y%m%d-%H%M%S')
    local theme=""
    [[ -f "$LATHE_SESSION/theme.txt" ]] && theme=$(cat "$LATHE_SESSION/theme.txt")
    local branch_name="lathe/${ts}"
    if [[ -n "$theme" ]]; then
        local slug
        slug=$(echo "$theme" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | cut -c1-30)
        branch_name="lathe/${slug}-${ts}"
    fi

    git checkout -b "$branch_name"
    set_session_field "branch" "$branch_name"
    set_session_field "pr_number" ""
    log "New branch: $branch_name"
}

# ---------------------------------------------------------------------------
# Session teardown — close PR, discard work, return to base, wipe state.
# ---------------------------------------------------------------------------

teardown_session() {
    local mode branch pr_number base_branch
    mode=$(get_session_field "mode")
    branch=$(get_session_field "branch")
    pr_number=$(get_session_field "pr_number")
    base_branch=$(get_session_field "base_branch")

    if [[ "$mode" == "branch" && -n "$branch" ]]; then
        # Discard any dirty working tree so checkout succeeds
        git checkout -- . 2>/dev/null || true
        git clean -fd 2>/dev/null || true

        # Switch to base branch
        if [[ -n "$base_branch" ]]; then
            git checkout "$base_branch" 2>/dev/null || true
        fi

        # Close PR (also deletes remote branch via --delete-branch)
        if [[ -n "$pr_number" ]] && command -v gh &>/dev/null; then
            gh pr close "$pr_number" --delete-branch 2>/dev/null || true
        fi

        # Delete local lathe branch
        git branch -D "$branch" 2>/dev/null || true
    fi

    # Wipe session state — clean slate
    rm -rf "$LATHE_SESSION"
}
