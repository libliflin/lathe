# engine/lib/ci.sh — CI polling, auto-merge, CI status collection
# Sourced by engine/loop.sh. Expects LATHE_DIR, LATHE_SESSION, SESSION_FILE,
# CI_WAIT_TIMEOUT, DIRECT_CI_CHECK_NAME_DEFAULT set.

# ---------------------------------------------------------------------------
# Direct-mode CI helpers
# ---------------------------------------------------------------------------

# SECURITY: Direct-mode CI polling reads workflow status by querying check
# runs scoped to a specific commit SHA on the protected base branch. This is
# the only safe path — never read /actions/runs, never read free-text fields
# (display titles, branch names, commit messages from those records). See
# the "Reading CI status safely" rule that lathe init writes into the agent docs.

_direct_ci_check_name() {
    if [[ -f "$LATHE_DIR/ci-check-name" ]]; then
        local name
        name=$(tr -d '[:space:]' < "$LATHE_DIR/ci-check-name")
        if [[ -n "$name" ]]; then
            echo "$name"
            return 0
        fi
    fi
    echo "$DIRECT_CI_CHECK_NAME_DEFAULT"
}

_direct_repo_full_name() {
    # Prefer gh which uses the user's auth context. Fall back to parsing
    # the origin remote URL. Both are local-only — no attacker-controlled.
    local repo
    repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null || true)
    if [[ -n "$repo" ]]; then
        echo "$repo"
        return 0
    fi
    local url
    url=$(git remote get-url origin 2>/dev/null || true)
    [[ -z "$url" ]] && return 0
    # Strip protocol and trailing .git, normalize to owner/repo
    url=${url#git@github.com:}
    url=${url#https://github.com/}
    url=${url#http://github.com/}
    url=${url%.git}
    echo "$url"
}

_direct_pushed_sha() {
    # Fetch the latest from origin and return the HEAD commit on the base
    # branch — that is the SHA the agent just pushed.
    local base_branch
    base_branch=$(get_session_field "base_branch")
    [[ -z "$base_branch" ]] && base_branch="main"
    git fetch --quiet origin "$base_branch" 2>/dev/null || true
    git rev-parse "origin/$base_branch" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# CI polling — block until checks complete or timeout
# ---------------------------------------------------------------------------

# Poll a single named check run on the latest base-branch HEAD commit.
# Sets CI_RESULT to: pass, fail, pending (only at timeout), none, timeout, skip.
wait_for_ci_direct() {
    CI_RESULT="skip"
    if ! command -v gh &>/dev/null; then return 0; fi

    local repo
    repo=$(_direct_repo_full_name)
    if [[ -z "$repo" ]]; then
        log "Direct CI: could not determine repo full_name — skipping CI poll"
        return 0
    fi

    local sha
    sha=$(_direct_pushed_sha)
    if [[ -z "$sha" ]]; then
        log "Direct CI: could not determine pushed SHA — skipping CI poll"
        return 0
    fi

    local check_name
    check_name=$(_direct_ci_check_name)

    log "Waiting for '$check_name' check on ${repo}@${sha:0:7} (timeout: ${CI_WAIT_TIMEOUT}s) ..."
    local waited=0
    local interval=15

    # SECURITY: this is the one safe path. /commits/<sha>/check-runs is
    # scoped to a commit on a protected branch — attacker fork-PR runs
    # never appear here regardless of what they are named. We read only
    # structured fields (status, conclusion, name) via --jq.
    local jq_filter
    jq_filter='[.check_runs[] | select(.name == $name)]'
    jq_filter+=' | if length == 0 then "none"'
    jq_filter+='   elif any(.status != "completed") then "pending"'
    jq_filter+='   elif any(.conclusion == "failure" or .conclusion == "timed_out" or .conclusion == "cancelled" or .conclusion == "action_required") then "fail"'
    jq_filter+='   elif all(.conclusion == "success" or .conclusion == "neutral" or .conclusion == "skipped") then "pass"'
    jq_filter+='   else "none"'
    jq_filter+='   end'

    while (( waited < CI_WAIT_TIMEOUT )); do
        local result
        result=$(gh api "repos/${repo}/commits/${sha}/check-runs" \
            --jq "$jq_filter" --arg name "$check_name" 2>/dev/null || echo "none")

        case "$result" in
            pass)
                log "Direct CI: '$check_name' passed on ${sha:0:7}"
                CI_RESULT="pass"
                return 0
                ;;
            fail)
                log "Direct CI: '$check_name' failed on ${sha:0:7}"
                CI_RESULT="fail"
                return 0
                ;;
            pending)
                sleep "$interval" &
                wait $! || return 0
                waited=$((waited + interval))
                log "Direct CI: '$check_name' still running ... (${waited}s / ${CI_WAIT_TIMEOUT}s)"
                ;;
            none|*)
                # Check might not exist yet (workflow still queueing) — retry.
                sleep "$interval" &
                wait $! || return 0
                waited=$((waited + interval))
                log "Direct CI: '$check_name' not yet present on ${sha:0:7} (${waited}s / ${CI_WAIT_TIMEOUT}s) ..."
                ;;
        esac
    done

    log "Direct CI: '$check_name' timed out after ${CI_WAIT_TIMEOUT}s on ${sha:0:7} — treating as signal"
    CI_RESULT="timeout"
    return 0
}

# Returns CI status via the CI_RESULT variable: pass, fail, timeout, none, skip
wait_for_ci() {
    CI_RESULT="skip"
    if ! command -v gh &>/dev/null; then return 0; fi

    local mode
    mode=$(get_session_field "mode")
    if [[ "$mode" == "direct" ]]; then
        wait_for_ci_direct
        return 0
    fi

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
                # Check WHY there are no checks — merge conflicts prevent CI from running
                local merge_state
                merge_state=$(gh pr view "$pr_number" --json mergeStateStatus --jq '.mergeStateStatus' 2>/dev/null || echo "UNKNOWN")
                if [[ "$merge_state" == "DIRTY" ]]; then
                    log "No CI checks on PR #$pr_number — PR has merge conflicts (needs rebase)"
                else
                    log "No CI checks found for PR #$pr_number (merge state: $merge_state)"
                fi
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

# ---------------------------------------------------------------------------
# Auto-merge — merge PR when CI passes
# ---------------------------------------------------------------------------

# Engine merges the PR when CI passes, returns to base branch.
# Does NOT create the next branch — that's the cycle loop's job.
auto_merge_if_green() {
    if [[ "$CI_RESULT" != "pass" ]]; then return 0; fi

    local mode
    mode=$(get_session_field "mode")
    if [[ "$mode" != "branch" ]]; then return 0; fi

    local pr_number
    pr_number=$(get_session_field "pr_number")
    if [[ -z "$pr_number" ]]; then return 0; fi

    # Ensure PR targets the base branch, not another lathe branch
    local base_branch
    base_branch=$(get_session_field "base_branch")
    local pr_base
    pr_base=$(gh pr view "$pr_number" --json baseRefName --jq '.baseRefName' 2>/dev/null || true)
    if [[ -n "$pr_base" && "$pr_base" != "$base_branch" ]]; then
        log "PR #$pr_number targets '$pr_base' instead of '$base_branch' — fixing ..."
        gh pr edit "$pr_number" --base "$base_branch" 2>/dev/null || true
    fi

    log "CI green on PR #$pr_number — merging ..."
    local merge_output
    merge_output=$(gh pr merge "$pr_number" --squash --delete-branch 2>&1)
    local merge_rc=$?
    if [[ $merge_rc -ne 0 ]]; then
        log "WARN: auto-merge failed on PR #$pr_number (exit $merge_rc): $merge_output"
        return 0
    fi
    log "Merged PR #$pr_number"

    # Return to base branch — session/ is gitignored so checkout is clean
    local old_branch
    old_branch=$(get_session_field "branch")
    git checkout "$base_branch" 2>/dev/null || true
    if [[ -n "$old_branch" ]]; then
        git branch -D "$old_branch" 2>/dev/null || true
    fi

    # Give GitHub time to propagate the merge before fetching
    sleep 10
    git pull origin "$base_branch" 2>/dev/null || true

    # Clear branch/PR from session — we're on base now
    set_session_field "branch" ""
    set_session_field "pr_number" ""
}

# ---------------------------------------------------------------------------
# CI status collection — append CI info to snapshot
# ---------------------------------------------------------------------------

# SECURITY MODEL: The snapshot feeds directly into the LLM prompt.
# Everything fetched from GitHub is a potential prompt injection vector.
# Rules:
# - Only fetch structured fields (numbers, statuses, booleans, timestamps)
# - Never fetch free-text fields (title, body, comments, commit messages, displayTitle)
# - Only list PRs authored by the current authenticated gh user
# - Init should verify branch protection settings
collect_ci_status() {
    if ! command -v gh &>/dev/null; then
        echo "" >> "$LATHE_SESSION/snapshot.txt"
        echo "## CI/CD Status" >> "$LATHE_SESSION/snapshot.txt"
        echo "(gh CLI not installed — no CI visibility)" >> "$LATHE_SESSION/snapshot.txt"
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

    # Direct (push-to-main) mode: read check runs scoped to the latest
    # base-branch HEAD commit. SECURITY: only structured fields, only the
    # commit-scoped endpoint, never /actions/runs.
    if [[ "$mode" == "direct" ]]; then
        local repo sha check_name
        repo=$(_direct_repo_full_name)
        sha=$(_direct_pushed_sha)
        check_name=$(_direct_ci_check_name)
        local base_branch
        base_branch=$(get_session_field "base_branch")

        ci_section+=$'\n'"### Direct mode: pushing to \`${base_branch:-main}\`"$'\n'
        if [[ -n "$repo" && -n "$sha" ]]; then
            ci_section+="Latest pushed commit: \`${sha:0:12}\` on \`${repo}\`"$'\n'
            ci_section+=$'\n'"Polling check name: \`${check_name}\` (override via \`.lathe/ci-check-name\`)"$'\n'
            ci_section+=$'\n'"### Check runs on ${sha:0:7}"$'\n'
            ci_section+='```'$'\n'
            ci_section+="$(gh api "repos/${repo}/commits/${sha}/check-runs" \
                --jq '.check_runs[] | "\(.name): \(.status)/\(.conclusion // "—")"' \
                2>/dev/null || echo "(could not fetch check runs)")"
            ci_section+=$'\n''```'$'\n'
        else
            ci_section+="(could not determine repo or pushed SHA)"$'\n'
        fi

        ci_section+=$'\n'"### CI Configuration"$'\n'
        if ls .github/workflows/*.yml &>/dev/null 2>&1 || ls .github/workflows/*.yaml &>/dev/null 2>&1; then
            ci_section+="Workflows found:"$'\n'
            ci_section+="$(ls .github/workflows/*.yml .github/workflows/*.yaml 2>/dev/null)"$'\n'
        else
            ci_section+="**No CI/CD configuration found.** The project has no automated validation. Creating a build workflow on push to main is the highest-value first change."$'\n'
        fi

        echo "$ci_section" >> "$LATHE_SESSION/snapshot.txt"
        return
    fi

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

        # Surface merge conflicts prominently so the agent can't miss them
        local pr_merge_status
        pr_merge_status=$(gh pr view "$pr_number" --json mergeStateStatus --jq '.mergeStateStatus' 2>/dev/null || true)
        if [[ "$pr_merge_status" == "DIRTY" ]]; then
            ci_section+=$'\n'"**WARNING: PR #$pr_number has merge conflicts with the base branch. CI will not run until conflicts are resolved. You must rebase onto the base branch: \`git fetch origin main && git rebase origin/main\`, resolve any conflicts, then force-push.**"$'\n'
        fi
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

    echo "$ci_section" >> "$LATHE_SESSION/snapshot.txt"
}
