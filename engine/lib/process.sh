# engine/lib/process.sh — Process management (kill tree, find agent, is_running)
# Sourced by engine/loop.sh. Expects LATHE_SESSION, PID_FILE set.

# Walk the process tree from a root PID and kill everything (leaves first).
# Claude CLI talks to a daemon via IPC — we can't kill the daemon, but killing
# the CLI process (and its children) is sufficient. The daemon abandons work
# when the client disconnects.
kill_tree() {
    local sig="${1:-TERM}"
    local pid="$2"
    local children
    children=$(pgrep -P "$pid" 2>/dev/null || true)
    for child in $children; do
        kill_tree "$sig" "$child"
    done
    kill -"$sig" "$pid" 2>/dev/null || true
}

is_running() {
    [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null
}

_find_lathe_agent() {
    # Find claude processes spawned by lathe (--dangerously-skip-permissions --print)
    # whose cwd is this repo. Outputs: PID ELAPSED  (one line per match, or nothing)
    local repo_dir
    repo_dir=$(pwd -P)

    # Find claude processes with lathe's distinctive flags
    local pids
    pids=$(pgrep -f 'claude.*--dangerously-skip-permissions.*--print' 2>/dev/null) || return 0

    local pid cwd elapsed
    for pid in $pids; do
        # Get process cwd via lsof
        cwd=$(lsof -p "$pid" 2>/dev/null | awk '$4 == "cwd" {print $NF}') || true
        [[ -n "$cwd" ]] || continue
        cwd=$(cd "$cwd" 2>/dev/null && pwd -P) || continue

        case "$cwd" in
            "$repo_dir"|"$repo_dir"/*)
                elapsed=$(ps -p "$pid" -o etime= 2>/dev/null | tr -d ' ' || echo "?")
                echo "$pid $elapsed"
                ;;
        esac
    done
}
