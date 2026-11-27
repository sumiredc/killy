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

# shellcheck source=../lib/itest.sh
. "$(dirname "$0")/../lib/itest.sh"
itest_init
itest_build

start_server 1
grep -q 'database migrated' "$work/server-1.log" || fail "no 'database migrated' in the log"
ok "migrations applied and the server listens"

reg=$(curl -fsS --max-time 10 -X POST "$base/agent/v1/pairings" -H 'Content-Type: application/json' -d '{"hostname":"dbcheck-host"}')
agent_token=$(json .agent_token <<<"$reg")
agent_id=$(json .agent_id <<<"$reg")
code=$(json .code <<<"$reg")
ok "agent registered ($agent_id)"

device_token=$(register_device)
ok "device registered"

claim=$(post /app/v1/pairings/claim "$device_token" "{\"code\":\"$code\",\"name\":\"dbcheck\"}")
[ "$(json .id <<<"$claim")" = "$agent_id" ] || fail "the claim returned another server"
ok "pairing code claimed"

servers() { state "$device_token" | jq -c '[.servers[] | {id, name}]'; }
want="[{\"id\":\"$agent_id\",\"name\":\"dbcheck\"}]"
got=$(servers)
[ "$got" = "$want" ] || fail "the state before the restart is wrong: $got"
ok "the linked server is in the state"

stop_server
start_server 2
ok "the server restarted on the same database"

got=$(servers)
[ "$got" = "$want" ] || fail "the link did not survive the restart: $got"
ok "the link and the display name survived the restart"

again=$(post /agent/v1/pairings "$agent_token" '{"hostname":"dbcheck-host"}')
[ "$(json .agent_id <<<"$again")" = "$agent_id" ] || fail "the agent token was not recognised after the restart"
ok "the agent token survived the restart"
