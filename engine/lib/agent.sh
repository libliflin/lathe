# engine/lib/agent.sh — Snapshot, prompt assembly, agent invocation, safety net
# Sourced by engine/loop.sh. Expects LATHE_DIR, LATHE_SESSION, LATHE_SKILLS,
# LATHE_HISTORY set.

# ---------------------------------------------------------------------------
# Snapshot — run project's snapshot.sh
# ---------------------------------------------------------------------------

collect_snapshot() {
    log "Collecting project snapshot ..."
    local out="$LATHE_SESSION/snapshot.txt"

    if [[ -x "$LATHE_DIR/snapshot.sh" ]]; then
        "$LATHE_DIR/snapshot.sh" > "$out" 2>&1
    else
        echo "(no snapshot script found)" > "$out"
    fi

    log "Snapshot written: $out"
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
        git stash --include-untracked
        git checkout "$session_branch" 2>/dev/null || git checkout -b "$session_branch"
        git stash pop
    fi

    # Commit whatever the agent left — but never commit session state
    git add -A
    git reset HEAD -- .lathe/session/ 2>/dev/null || true
    git commit -m "lathe: cleanup (agent left uncommitted changes)" || true

    if [[ "$mode" == "branch" && -n "$session_branch" ]]; then
        git push origin "$session_branch" 2>/dev/null || log "WARN: push failed (non-fatal)"
    elif [[ "$mode" == "direct" ]]; then
        local base
        base=$(get_session_field "base_branch")
        git push origin "$base" 2>/dev/null || log "WARN: push failed (non-fatal)"
    fi
}

# ---------------------------------------------------------------------------
# Shared prompt helpers
# ---------------------------------------------------------------------------

# Assemble the common prompt block: skills + refs + theme + snapshot
_assemble_common() {
    local prompt=""

    # All skills
    for skill_file in "$LATHE_SKILLS"/*.md; do
        if [[ -f "$skill_file" ]]; then
            prompt+="---"$'\n'
            prompt+="# Skill: $(basename "$skill_file" .md)"$'\n\n'
            prompt+="$(cat "$skill_file")"
            prompt+=$'\n\n'
        fi
    done

    # Reference documents
    for ref_file in "$LATHE_DIR/refs"/*.md; do
        if [[ -f "$ref_file" ]]; then
            prompt+="---"$'\n'
            prompt+="# Reference: $(basename "$ref_file" .md)"$'\n\n'
            prompt+="$(cat "$ref_file")"
            prompt+=$'\n\n'
        fi
    done

    # Theme
    if [[ -f "$LATHE_SESSION/theme.txt" ]]; then
        local theme_text
        theme_text=$(cat "$LATHE_SESSION/theme.txt")
        prompt+="---"$'\n'
        prompt+="# Theme"$'\n\n'
        prompt+="The user started this session with a purpose: **$theme_text**"$'\n\n'
    fi

    # Current snapshot
    prompt+="---"$'\n'
    prompt+="# Current Project Snapshot"$'\n\n'
    if [[ -f "$LATHE_SESSION/snapshot.txt" ]]; then
        prompt+="$(cat "$LATHE_SESSION/snapshot.txt")"
    else
        prompt+="(no snapshot collected)"
    fi
    prompt+=$'\n\n'

    echo "$prompt"
}

# Assemble session context (branch/PR/CI instructions)
_assemble_session_context() {
    local prompt=""
    local session_mode
    session_mode=$(get_session_field "mode")

    if [[ "$session_mode" == "branch" ]]; then
        local session_branch session_pr session_base
        session_branch=$(get_session_field "branch")
        session_pr=$(get_session_field "pr_number")
        session_base=$(get_session_field "base_branch")

        prompt+="---"$'\n'
        prompt+="# Session Context"$'\n\n'
        prompt+="You are working on branch \`$session_branch\` (base: \`$session_base\`)."$'\n\n'

        if [[ -n "$session_pr" ]]; then
            prompt+="There is an open PR: #$session_pr. Push your commits to this branch."$'\n\n'
        else
            prompt+="No PR exists yet. After your first commit and push, create one with \`gh pr create --base $session_base\`."$'\n\n'
        fi

        prompt+="After your work: \`git add\`, \`git commit\`, \`git push origin $session_branch\`. If no PR exists yet, create one with \`gh pr create --base $session_base\`."$'\n\n'

    elif [[ "$session_mode" == "direct" ]]; then
        local session_base
        session_base=$(get_session_field "base_branch")

        prompt+="---"$'\n'
        prompt+="# Session Context"$'\n\n'
        prompt+="You are working in **direct mode**: commits go straight to \`${session_base:-main}\`."$'\n\n'
        prompt+="After your work: \`git add\`, \`git commit -S\`, \`git push origin ${session_base:-main}\`."$'\n\n'
    fi

    echo "$prompt"
}

# Invoke the LLM with a prompt. Handles logging and rate limit detection.
_invoke_agent() {
    local prompt="$1"
    local cycle="$2"
    local label="$3"    # e.g. "goal", "build-1", "verify-2"
    local tool="${4:-claude}"

    # Pre-cycle hook
    if [[ -x "$LATHE_DIR/hooks/pre-cycle.sh" ]]; then
        log "Running pre-cycle hook ..."
        "$LATHE_DIR/hooks/pre-cycle.sh" || log "WARN: pre-cycle hook failed (non-fatal)"
    fi

    local log_dir="$LATHE_SESSION/logs"
    mkdir -p "$log_dir"
    local log_file="$log_dir/cycle-$(printf '%03d' "$cycle")-${label}.log"

    log "Invoking $tool ($label) ..."
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
        log "Rate limited. Ending early."
        echo "RATE_LIMITED" > "$LATHE_SESSION/rate-limited"
        return 1
    fi

    rm -f "$LATHE_SESSION/rate-limited"
    log "Agent complete ($label, exit $exit_code). Log: $log_file"
    return "$exit_code"
}

# ---------------------------------------------------------------------------
# Goal Setter — pick the highest-value change
# ---------------------------------------------------------------------------

run_goal_setter() {
    local cycle="$1"
    local tool="${2:-claude}"

    log "Running goal-setter (cycle $cycle) ..."

    local prompt=""

    # Goal-setter behavioral doc
    if [[ -f "$LATHE_DIR/goal.md" ]]; then
        prompt+="$(cat "$LATHE_DIR/goal.md")"
        prompt+=$'\n\n'
    else
        log "WARN: no .lathe/goal.md found"
    fi

    # Common: skills, refs, theme, snapshot
    prompt+="$(_assemble_common)"

    # Session context (so goal-setter can commit its goal)
    prompt+="$(_assemble_session_context)"

    # Last 4 goals for context
    local goal_history="$LATHE_SESSION/goal-history"
    if [[ -d "$goal_history" ]]; then
        local goal_files
        goal_files=$(ls -1 "$goal_history"/*.md 2>/dev/null | tail -4)
        if [[ -n "$goal_files" ]]; then
            prompt+="---"$'\n'
            prompt+="# Previous Goals (last 4 cycles)"$'\n\n'
            while IFS= read -r gf; do
                prompt+="## $(basename "$gf" .md)"$'\n'
                prompt+="$(cat "$gf")"
                prompt+=$'\n\n'
            done <<< "$goal_files"
        fi
    fi

    # Recent git history
    prompt+="---"$'\n'
    prompt+="# Recent Commits"$'\n\n'
    prompt+='```'$'\n'
    prompt+="$(git log --oneline -20 2>/dev/null || echo '(no commits)')"
    prompt+=$'\n''```'$'\n\n'

    # Instructions
    prompt+="---"$'\n'
    prompt+="# Your Task"$'\n\n'
    prompt+="Pick the single highest-value change for this cycle. Write a goal file describing:"$'\n'
    prompt+="- **What** to change (specific, actionable)"$'\n'
    prompt+="- **Which stakeholder** it helps and why"$'\n'
    prompt+="- **Why now** — what in the snapshot makes this the most valuable change right now"$'\n\n'
    prompt+="Commit this goal as a file the builder can read. The builder implements; you decide."$'\n\n'
    prompt+="**Changelog:** Write a brief changelog to \`.lathe/session/changelog.md\` describing what goal you set and why."$'\n\n'

    _invoke_agent "$prompt" "$cycle" "goal" "$tool"
}

# ---------------------------------------------------------------------------
# Builder — implement the goal
# ---------------------------------------------------------------------------

run_builder() {
    local cycle="$1"
    local round="$2"
    local tool="${3:-claude}"

    log "Running builder (cycle $cycle, round $round) ..."

    local prompt=""

    # Builder behavioral doc
    if [[ -f "$LATHE_DIR/builder.md" ]]; then
        prompt+="$(cat "$LATHE_DIR/builder.md")"
        prompt+=$'\n\n'
    else
        log "WARN: no .lathe/builder.md found"
    fi

    # Common: skills, refs, theme, snapshot
    prompt+="$(_assemble_common)"

    # Session context
    prompt+="$(_assemble_session_context)"

    # Current goal — find it in the repo (goal-setter committed it)
    prompt+="---"$'\n'
    prompt+="# Current Goal"$'\n\n'
    # Look for the most recent goal-setter changelog or committed goal file
    if [[ -f "$LATHE_SESSION/goal-history/cycle-$(printf '%03d' "$cycle").md" ]]; then
        prompt+="$(cat "$LATHE_SESSION/goal-history/cycle-$(printf '%03d' "$cycle").md")"
    elif [[ -f "$LATHE_SESSION/changelog.md" ]]; then
        prompt+="$(cat "$LATHE_SESSION/changelog.md")"
    else
        prompt+="(no goal found for this cycle — use your best judgment based on the snapshot)"
    fi
    prompt+=$'\n\n'

    # Instructions
    prompt+="---"$'\n'
    prompt+="# Your Task"$'\n\n'
    prompt+="Implement the goal above. One change, committed, validated, pushed."$'\n'
    prompt+="If CI is failing, fix CI first — that's always top priority."$'\n\n'
    prompt+="**Changelog:** Write a brief changelog to \`.lathe/session/changelog.md\` describing what you changed and which stakeholder it benefits."$'\n\n'

    _invoke_agent "$prompt" "$cycle" "build-$round" "$tool"
}

# ---------------------------------------------------------------------------
# Verifier — check the builder's work
# ---------------------------------------------------------------------------

run_verifier() {
    local cycle="$1"
    local round="$2"
    local tool="${3:-claude}"

    log "Running verifier (cycle $cycle, round $round) ..."

    local prompt=""

    # Verifier behavioral doc
    if [[ -f "$LATHE_DIR/verifier.md" ]]; then
        prompt+="$(cat "$LATHE_DIR/verifier.md")"
        prompt+=$'\n\n'
    else
        log "WARN: no .lathe/verifier.md found"
    fi

    # Common: skills, refs, theme, snapshot (refreshed after builder)
    prompt+="$(_assemble_common)"

    # Session context
    prompt+="$(_assemble_session_context)"

    # Current goal
    prompt+="---"$'\n'
    prompt+="# Current Goal"$'\n\n'
    if [[ -f "$LATHE_SESSION/goal-history/cycle-$(printf '%03d' "$cycle").md" ]]; then
        prompt+="$(cat "$LATHE_SESSION/goal-history/cycle-$(printf '%03d' "$cycle").md")"
    else
        prompt+="(no goal found)"
    fi
    prompt+=$'\n\n'

    # Builder's diff — what the builder just changed
    prompt+="---"$'\n'
    prompt+="# Builder's Changes (this round)"$'\n\n'
    prompt+='```diff'$'\n'
    prompt+="$(git diff HEAD~1 2>/dev/null || echo '(no diff available)')"
    prompt+=$'\n''```'$'\n\n'

    # Instructions
    prompt+="---"$'\n'
    prompt+="# Your Task"$'\n\n'
    prompt+="Check the builder's work against the goal. Ask:"$'\n'
    prompt+="1. Did the builder do what the goal asked?"$'\n'
    prompt+="2. Does it actually work? Run the tests."$'\n'
    prompt+="3. What edge cases or regressions could this introduce?"$'\n\n'
    prompt+="If you find gaps, fix them — commit real code (tests, edge cases, error handling)."$'\n'
    prompt+="If the builder's change is solid, say so in the changelog."$'\n\n'
    prompt+="**Changelog:** Write a brief changelog to \`.lathe/session/changelog.md\` describing what you verified and any fixes you applied."$'\n\n'

    _invoke_agent "$prompt" "$cycle" "verify-$round" "$tool"
}

# ---------------------------------------------------------------------------
# Rate limit backoff
# ---------------------------------------------------------------------------

wait_for_rate_limit() {
    if [[ ! -f "$LATHE_SESSION/rate-limited" ]]; then
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
    rm -f "$LATHE_SESSION/rate-limited"
    log "Cooldown complete. Resuming."
}
