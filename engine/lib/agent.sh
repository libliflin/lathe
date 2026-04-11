# engine/lib/agent.sh — Snapshot, falsification, prompt assembly, agent invocation, safety net
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
# Falsification suite — run .lathe/falsify.sh and record the result
#
# The engine runs the project's own falsification suite each
# cycle and surfaces the result in the snapshot. A failing claim is treated by
# the agent the same way as a failing CI check — top priority, fix first.
#
# `falsify.sh` is written by lathe init alongside `claims.md`. It exits 0 if
# all load-bearing claims hold, non-zero if any are violated.
# ---------------------------------------------------------------------------

collect_falsification() {
    local out="$LATHE_SESSION/snapshot.txt"

    if [[ ! -x "$LATHE_DIR/falsify.sh" ]]; then
        echo "" >> "$out"
        echo "## Falsification" >> "$out"
        echo "(no .lathe/falsify.sh — falsification suite not installed)" >> "$out"
        return 0
    fi

    log "Running falsification suite ..."
    local fal_output
    local fal_rc=0
    fal_output=$("$LATHE_DIR/falsify.sh" 2>&1) || fal_rc=$?

    echo "" >> "$out"
    echo "## Falsification" >> "$out"
    echo '```' >> "$out"
    if (( fal_rc == 0 )); then
        echo "PASS — all claims hold" >> "$out"
    else
        echo "FAIL (exit $fal_rc) — one or more claims violated" >> "$out"
    fi
    if [[ -n "$fal_output" ]]; then
        echo "" >> "$out"
        echo "$fal_output" >> "$out"
    fi
    echo '```' >> "$out"

    if [[ -f "$LATHE_DIR/claims.md" ]]; then
        echo "" >> "$out"
        echo "(see .lathe/claims.md for the registry of claims being tested)" >> "$out"
    fi

    if (( fal_rc == 0 )); then
        log "Falsification: PASS"
    else
        log "Falsification: FAIL (exit $fal_rc)"
    fi
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

    # Commit whatever the agent left — but never commit session state
    git add -A
    git reset HEAD -- .lathe/session/ 2>/dev/null || true
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
# Agent — assemble prompt and call LLM
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

    # Reference documents (external specs, standards)
    for ref_file in "$LATHE_DIR/refs"/*.md; do
        if [[ -f "$ref_file" ]]; then
            prompt+="---"$'\n'
            prompt+="# Reference: $(basename "$ref_file" .md)"$'\n\n'
            prompt+="$(cat "$ref_file")"
            prompt+=$'\n\n'
        fi
    done

    # Theme — why the user put this on the lathe today
    if [[ -f "$LATHE_SESSION/theme.txt" ]]; then
        local theme_text
        theme_text=$(cat "$LATHE_SESSION/theme.txt")
        prompt+="---"$'\n'
        prompt+="# Theme"$'\n\n'
        prompt+="The user started this session with a purpose: **$theme_text**"$'\n\n'
        prompt+="Use this to guide your pick step. The stakeholder framework in agent.md still applies — the theme tells you where to focus within it, not to override it."$'\n\n'
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
            prompt+="No PR exists yet. After your first commit and push, create one with \`gh pr create --base $session_base\`."$'\n\n'
        fi

        prompt+="**Your responsibilities this cycle:**"$'\n'
        prompt+="- If CI failed on the previous PR: fixing the failure is your top priority. Read the failure, understand it, fix it. Push the fix to this branch."$'\n'
        prompt+="- If CI timed out (took >2 minutes): that's a signal. Consider making CI faster as a priority."$'\n'
        prompt+="- If there is no CI: creating a basic CI workflow (GitHub Actions, etc.) is likely the highest-value first change. Start minimal — just run the project's test command."$'\n'
        prompt+="- If CI is failing for external reasons (dependency outage, vulnerability scanner, upstream issue): use your judgment. Sometimes a workaround is right. Sometimes you keep working on the current branch. Explain your reasoning in the changelog."$'\n'
        prompt+="- Otherwise: implement your one change, commit, push to \`$session_branch\`."$'\n\n'
        prompt+="The engine handles merging PRs when CI passes and creating fresh branches. You never need to merge PRs or create branches yourself."$'\n'
        prompt+="After implementing your change: \`git add\`, \`git commit\`, \`git push origin $session_branch\`. If no PR exists yet, create one with \`gh pr create --base $session_base\`."$'\n\n'
        prompt+="**Changelog:** After completing your work, write a brief changelog to \`.lathe/session/changelog.md\` describing what you changed and which stakeholder it benefits. This is read by the engine for retros — if you don't write it, the retro has nothing to review."$'\n\n'
    elif [[ "$session_mode" == "direct" ]]; then
        local session_base
        session_base=$(get_session_field "base_branch")

        prompt+="---"$'\n'
        prompt+="# Session Context"$'\n\n'
        prompt+="You are working in **direct mode**: every cycle's commit goes straight to \`${session_base:-main}\`. There are no PRs and no branches. The repo's lockdown workflows (close-prs.yml, verify-author-signature.yml) enforce this — only the maintainer's signed commits land on main."$'\n\n'
        prompt+="**CI flow:** After you push, the engine waits for the build workflow's check on the exact SHA you just pushed. It queries \`/repos/<owner>/<repo>/commits/<sha>/check-runs\` (commit-scoped, not run-scoped) and reads only structured fields. The result is in the \"## CI/CD Status\" section of the snapshot above."$'\n\n'
        prompt+="**Your responsibilities this cycle:**"$'\n'
        prompt+="- If the build check failed on the previous push: fixing that failure is your top priority. Read the failure (it is in the snapshot), understand it, fix it, commit, push to \`${session_base:-main}\`."$'\n'
        prompt+="- If the build check timed out: that is a signal. CI is too slow or wedged — diagnose before adding new work."$'\n'
        prompt+="- If there is no build workflow at all: creating \`.github/workflows/build.yml\` (push-to-main, single job named \`build\`) is the highest-value change. The engine polls for a check run named \`build\` by default; change it via \`.lathe/ci-check-name\` if you use a different name."$'\n'
        prompt+="- Otherwise: implement your one change, commit (signed), push to \`${session_base:-main}\`."$'\n\n'
        prompt+="The engine never merges anything for you (there is nothing to merge). It just polls the build check and surfaces the result in the next cycle's snapshot."$'\n'
        prompt+="After implementing your change: \`git add\`, \`git commit -S\`, \`git push origin ${session_base:-main}\`."$'\n\n'
        prompt+="**Changelog:** After completing your work, write a brief changelog to \`.lathe/session/changelog.md\` describing what you changed and which stakeholder it benefits. This is read by the engine for retros — if you don't write it, the retro has nothing to review."$'\n\n'
    fi

    # Red-team section: runs every cycle alongside normal work.
    # The agent implements its change AND reviews the falsification suite.
    # CI output is included so the agent sees build health as part of the
    # adversarial picture — a red-team that ignores CI is incomplete.
    if (( cycle > 0 )); then
        prompt+="---"$'\n'
        prompt+="# Red-Team Review"$'\n\n'
        prompt+="Every cycle includes a red-team pass. After implementing your change, review \`.lathe/claims.md\` and do at least one of these:"$'\n\n'
        prompt+="1. **Try to break a claim.** Pick one that hasn't been adversarially tested recently. Construct an input or scenario that would falsify it. If it breaks, fix it (or document it as a known limitation in \`claims.md\`) and add the case to \`falsify.sh\`."$'\n'
        prompt+="2. **Strengthen the fence.** Extend \`falsify.sh\` with a case that would have caught a plausible regression. Adversarial cases defend claims; easy-path cases don't."$'\n'
        prompt+="3. **Surface a missing claim.** If a load-bearing property isn't in \`claims.md\` yet, add it (with stakeholder and whether behavioral/structural) and add a falsification case."$'\n\n'
        prompt+="**CI output is part of the red-team picture.** The \`## CI/CD Status\` and \`## Falsification\` sections in the snapshot above show the current build and claim health. A failing CI check or falsification failure is a red flag — address it before adding new claims. A passing build with clean falsification is the baseline; push on it."$'\n\n'
        prompt+="In the changelog, note which claim you tested and what regression the cycle now prevents."$'\n\n'
    fi

    # Pre-cycle hook
    if [[ -x "$LATHE_DIR/hooks/pre-cycle.sh" ]]; then
        log "Running pre-cycle hook ..."
        "$LATHE_DIR/hooks/pre-cycle.sh" || log "WARN: pre-cycle hook failed (non-fatal)"
    fi

    # Invoke LLM
    local log_dir="$LATHE_SESSION/logs"
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
        echo "RATE_LIMITED" > "$LATHE_SESSION/rate-limited"
        return 1
    fi

    rm -f "$LATHE_SESSION/rate-limited"
    log "Agent complete (exit $exit_code). Log: $log_file"
    return "$exit_code"
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
