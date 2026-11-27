#!/usr/bin/env bash
# Shared parts of the scripts that run the real binaries against a real PostgreSQL
# (tools/dbcheck/run.sh, tools/e2e/run.sh). Source it; it is not run by itself.
#
# It sets: work (a temp directory, removed at exit), listen and base (where the server listens),
# and the functions below. A script that sources it must `itest_init` first.

# itest_init goes to the repository root, makes the temp directory and installs the cleanup. At exit it
# stops the server and every process added with itest_track, prints the server log and the files in
# $work/*.log when the script failed, and removes $work.
itest_init() {
  set -euo pipefail
  cd "$(dirname "${BASH_SOURCE[1]}")/../.."
  : "${KILLY_DATABASE_URL:?set KILLY_DATABASE_URL to a PostgreSQL that this script may write to}"
  listen=${KILLY_DBCHECK_LISTEN:-127.0.0.1:18080}
  base="http://${listen}"
  work=$(mktemp -d)
  server_pid=
  tracked=()
  trap itest_cleanup EXIT
}

# itest_track remembers a process that the script started, so the cleanup ends it. Only processes added
# here are ever signalled by the scripts.
itest_track() { tracked+=("$1"); }

# itest_untrack forgets a process that has ended, so the cleanup does not signal its PID again.
itest_untrack() {
  local p keep=()
  for p in ${tracked[@]+"${tracked[@]}"}; do
    [ "$p" = "$1" ] || keep+=("$p")
  done
  tracked=(${keep[@]+"${keep[@]}"})
}

itest_cleanup() {
  local status=$? p
  for p in ${tracked[@]+"${tracked[@]}"} ${server_pid:+"$server_pid"}; do
    kill "$p" 2>/dev/null || true
    wait "$p" 2>/dev/null || true
  done
  if [ "$status" -ne 0 ]; then
    for f in "$work"/*.log; do
      [ -e "$f" ] || continue
      echo "--- $f" >&2
      cat "$f" >&2
    done
  fi
  rm -rf "$work"
  exit "$status"
}

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok: $*"; }

# json <jq filter> reads stdin and fails when the value is missing or null.
json() { jq -er "$1"; }

itest_build() { go build -o "$work/killy" ./cmd/killy; }

# start_server <n> starts killy server and waits until it listens. It only listens after the
# migration has succeeded, so a migration error shows up as a server that never comes up.
start_server() {
  local log="$work/server-$1.log"
  "$work/killy" server --listen "$listen" >"$log" 2>&1 &
  server_pid=$!
  for _ in $(seq 1 60); do
    if grep -qs 'msg=listening' "$log"; then return 0; fi
    if ! kill -0 "$server_pid" 2>/dev/null; then fail "the server exited before it listened (the log is below)"; fi
    sleep 0.5
  done
  fail "the server did not listen within 30 s"
}

stop_server() {
  kill -TERM "$server_pid"
  wait "$server_pid" || true
  server_pid=
}

# post <path> <token> <json body> prints the response body; it fails on an HTTP error.
post() {
  curl -fsS --max-time 15 -X POST "$base$1" -H "Authorization: Bearer $2" \
    -H 'Content-Type: application/json' -d "$3"
}

# register_device prints a device token.
register_device() {
  curl -fsS --max-time 10 -X POST "$base/app/v1/devices" -H 'Content-Type: application/json' -d '{}' | json .device_token
}

# state <device token> prints the first `state` event of the device's stream. curl ends at the time
# limit (exit 28), because the stream does not end by itself.
state() {
  local out
  out=$(curl -sN --max-time 3 -H "Authorization: Bearer $1" "$base/app/v1/events" || true)
  sed -n 's/^data: //p' <<<"$out" | head -n 1
}
