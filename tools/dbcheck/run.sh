#!/usr/bin/env bash
# Runs killy server against a real PostgreSQL and checks what the unit tests (which use an in-memory
# store) cannot: that the embedded migrations apply, and that registrations survive a restart.
#
#   KILLY_DATABASE_URL=postgres://postgres:killy@127.0.0.1:55432/killy?sslmode=disable \
#     tools/dbcheck/run.sh
#
# `make dbcheck` does the same against the PostgreSQL that `make db-up` starts. The database may be
# empty or already migrated. The server listens on 127.0.0.1:18080; KILLY_DBCHECK_LISTEN changes that
# when the port is taken.
set -euo pipefail

cd "$(dirname "$0")/../.."

: "${KILLY_DATABASE_URL:?set KILLY_DATABASE_URL to a PostgreSQL that this script may write to}"
listen=${KILLY_DBCHECK_LISTEN:-127.0.0.1:18080}
base="http://${listen}"
work=$(mktemp -d)
pid=

cleanup() {
  status=$?
  if [ -n "$pid" ]; then kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  if [ "$status" -ne 0 ]; then
    echo "--- server log" >&2
    cat "$work"/server-*.log >&2 2>/dev/null || true
  fi
  rm -rf "$work"
  exit "$status"
}
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok: $*"; }

go build -o "$work/killy" ./cmd/killy

# start <n> starts the server and waits until it listens. It only listens after the migration has
# succeeded, so a migration error shows up here as a server that never comes up.
start() {
  local log="$work/server-$1.log"
  "$work/killy" server --listen "$listen" >"$log" 2>&1 &
  pid=$!
  for _ in $(seq 1 60); do
    if grep -qs 'msg=listening' "$log"; then return 0; fi
    if ! kill -0 "$pid" 2>/dev/null; then fail "the server exited before it listened (the log is below)"; fi
    sleep 0.5
  done
  fail "the server did not listen within 30 s"
}

stop() {
  kill -TERM "$pid"
  wait "$pid" || true
  pid=
}

# json <jq filter> reads stdin.
json() { jq -er "$1"; }

start 1
grep -q 'database migrated' "$work/server-1.log" || fail "no 'database migrated' in the log"
ok "migrations applied and the server listens"

reg=$(curl -fsS --max-time 10 -X POST "$base/agent/v1/pairings" -H 'Content-Type: application/json' -d '{"hostname":"dbcheck-host"}')
agent_token=$(json .agent_token <<<"$reg")
agent_id=$(json .agent_id <<<"$reg")
code=$(json .code <<<"$reg")
ok "agent registered ($agent_id)"

dev=$(curl -fsS --max-time 10 -X POST "$base/app/v1/devices" -H 'Content-Type: application/json' -d '{}')
device_token=$(json .device_token <<<"$dev")
ok "device registered"

claim=$(curl -fsS --max-time 10 -X POST "$base/app/v1/pairings/claim" -H "Authorization: Bearer $device_token" \
  -H 'Content-Type: application/json' -d "{\"code\":\"$code\",\"name\":\"dbcheck\"}")
[ "$(json .id <<<"$claim")" = "$agent_id" ] || fail "the claim returned another server"
ok "pairing code claimed"

# state prints the servers of the first `state` event of the device's stream. curl ends at the
# time limit (exit 28), because the stream does not end by itself.
state() {
  local out
  out=$(curl -sN --max-time 3 -H "Authorization: Bearer $device_token" "$base/app/v1/events" || true)
  sed -n 's/^data: //p' <<<"$out" | head -n 1 | jq -c '[.servers[] | {id, name}]'
}

want="[{\"id\":\"$agent_id\",\"name\":\"dbcheck\"}]"
got=$(state)
[ "$got" = "$want" ] || fail "the state before the restart is wrong: $got"
ok "the linked server is in the state"

stop
start 2
ok "the server restarted on the same database"

got=$(state)
[ "$got" = "$want" ] || fail "the link did not survive the restart: $got"
ok "the link and the display name survived the restart"

again=$(curl -fsS --max-time 10 -X POST "$base/agent/v1/pairings" -H "Authorization: Bearer $agent_token" \
  -H 'Content-Type: application/json' -d '{"hostname":"dbcheck-host"}')
[ "$(json .agent_id <<<"$again")" = "$agent_id" ] || fail "the agent token was not recognised after the restart"
ok "the agent token survived the restart"
