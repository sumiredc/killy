#!/usr/bin/env bash
# Runs the real killy server (on PostgreSQL), the real killy agent and a real zombie-holding parent
# on this machine, and checks the kill guarantees end to end through the phone's API:
#
#   1. a parent that holds a zombie appears in the state
#   2. a kill returns "killed" and the parent is gone
#   3. the same kill ID sent again returns the first outcome and sends no second signal
#   4. a kill of a parent that is already gone returns "already_gone"
#   5. a kill of PID 1, of the agent and of an ancestor of the agent is refused, and none of them dies
#
# Needs Linux 5.3 or later (the agent signals through a pidfd), bash, curl, jq and python3, and
# KILLY_DATABASE_URL (see tools/dbcheck/run.sh; KILLY_DBCHECK_LISTEN moves the server's port). PID 1
# must reap orphans (systemd does; in a container use `docker run --init`), or the zombies of a killed
# parent stay and the kill ends as zombies_remain. The only processes it asks the agent to signal are
# the zombie parent it made, PID 1, the agent and the agent's ancestor; the last three must be
# refused. The demo mode of the agent is not covered.

# shellcheck source=../lib/itest.sh
. "$(dirname "$0")/../lib/itest.sh"
itest_init
[ "$(uname -s)" = Linux ] || fail "this check needs Linux (pidfd)"
itest_build

# starttime <pid> prints field 22 of /proc/<pid>/stat. The command name in field 2 may contain
# spaces, so everything up to the last ") " is cut first.
starttime() { sed 's/.*) //' "/proc/$1/stat" | awk '{print $20}'; }

# wait_for <seconds> <description> <command...> runs the command until it succeeds.
wait_for() {
  local n=$1 what=$2 end; shift 2
  end=$((SECONDS + n))
  while [ "$SECONDS" -lt "$end" ]; do
    if "$@"; then return 0; fi
    sleep 0.5
  done
  fail "timed out after ${n} s: $what"
}

start_server 1
ok "server up on a migrated PostgreSQL"

# Pair the agent, then link the phone to it.
pair_out=$("$work/killy" agent pair --server "$base" --config "$work/agent.json")
code=$(sed -n 's/^Pairing code: //p' <<<"$pair_out")
agent_id=$(sed -n 's/^Agent ID: //p' <<<"$pair_out")
[ -n "$code" ] && [ -n "$agent_id" ] || fail "no pairing code or agent ID in: $pair_out"
device_token=$(register_device)
post /app/v1/pairings/claim "$device_token" "{\"code\":\"$code\",\"name\":\"e2e\"}" >/dev/null
ok "agent $agent_id paired with the phone"

# A parent with a zombie child: it forks a child that exits at once and never waits for it. (A shell
# one-liner would depend on whether /bin/sh reaps its children before exec.)
cat >"$work/zparent.py" <<'PY'
import os, time
pid = os.fork()
if pid == 0:
    os._exit(0)
time.sleep(3600)
PY

# The agent runs as a child of a small Python process that holds an exited child it never waits for,
# so the agent's own ancestor is a parent with a zombie. That is what a kill of an ancestor of the
# agent must refuse. The wrapper writes the agent's PID to a file.
cat >"$work/ancestor.py" <<'PY'
import os, subprocess, sys, time
pid = os.fork()
if pid == 0:
    os._exit(0)  # exits at once; this process never waits for it
agent = subprocess.Popen(sys.argv[2:])
open(sys.argv[1], "w").write(str(agent.pid))
time.sleep(3600)
PY
python3 "$work/ancestor.py" "$work/agent.pid" "$work/killy" agent run \
  --config "$work/agent.json" --outcomes "$work/outcomes.jsonl" >"$work/agent.log" 2>&1 &
ancestor=$!
itest_track "$ancestor"
wait_for 10 "the agent to start" test -s "$work/agent.pid"
agent_pid=$(cat "$work/agent.pid")
itest_track "$agent_pid"

# parent_of <ppid> prints the parent with that PID from the phone's state, or nothing.
parent_of() {
  state "$device_token" | jq -c --argjson p "$1" '.servers[0].parents[]? | select(.pid == $p)'
}
connected() { [ "$(state "$device_token" | jq -r '.servers[0].disconnected')" = false ]; }
wait_for 30 "the agent to report" connected
ok "the agent is connected"

# Two zombie parents: the first is killed, the second proves that a repeated ID signals nothing.
python3 "$work/zparent.py" &
zparent=$!
itest_track "$zparent"
python3 "$work/zparent.py" &
zparent2=$!
itest_track "$zparent2"
have_zombie() { [ -n "$(parent_of "$zparent")" ] && [ -n "$(parent_of "$zparent2")" ]; }
wait_for 30 "the zombie parents to appear in the state" have_zombie
zstart2=$(starttime "$zparent2")
p=$(parent_of "$zparent")
zstart=$(json .starttime <<<"$p")
[ "$zstart" = "$(starttime "$zparent")" ] || fail "the reported start time differs from /proc"
[ "$(json '.zombies | length' <<<"$p")" -ge 1 ] || fail "the parent has no zombie in the report"
ok "1. the parent $zparent with its zombie is in the state"

kill_req() { # id ppid starttime
  post /app/v1/kills "$device_token" "{\"id\":\"$1\",\"server_id\":\"$agent_id\",\"ppid\":$2,\"starttime\":$3}"
}
new_id() { python3 -c 'import uuid; print(uuid.uuid4())'; }

id1=$(new_id)
r=$(kill_req "$id1" "$zparent" "$zstart")
[ "$(json .outcome <<<"$r")" = killed ] || fail "the kill did not return killed: $r"
[ "$(json .reaped <<<"$r")" -ge 1 ] || fail "no zombie was reaped: $r"
# bash reports the killed job as "Killed" on stderr; that is this step working, not an error.
gone() { [ ! -e "/proc/$zparent" ]; }
wait_for 10 "the killed parent to disappear" gone
itest_untrack "$zparent"
ok "2. the kill returned killed and the parent is gone"

r2=$(kill_req "$id1" "$zparent" "$zstart")
[ "$(json .outcome <<<"$r2")" = killed ] || fail "the same ID did not return the first outcome: $r2"
[ "$(json .reaped <<<"$r2")" = "$(json .reaped <<<"$r")" ] || fail "the second answer differs from the first: $r2"
[ "$(grep -c "$id1" "$work/outcomes.jsonl")" = 1 ] || fail "the outcome of $id1 is not stored exactly once"
# The same ID, now aimed at the second zombie parent, which qualifies for a kill: a second signal
# would kill it, so it must be answered with the first outcome and be left alone.
r2b=$(kill_req "$id1" "$zparent2" "$zstart2")
[ "$(json .outcome <<<"$r2b")" = killed ] || fail "the same ID aimed at another parent did not return the first outcome: $r2b"
[ "$(json .reaped <<<"$r2b")" = "$(json .reaped <<<"$r")" ] || fail "the answer for the same ID differs from the first: $r2b"
sleep 1
[ -e "/proc/$zparent2" ] && [ "$(starttime "$zparent2")" = "$zstart2" ] || fail "the second parent was killed by a repeated ID"
[ -n "$(parent_of "$zparent2")" ] || fail "the second parent no longer holds its zombie"
ok "3. the same ID returned the first outcome, stored it once and signalled nothing else"

r3=$(kill_req "$(new_id)" "$zparent" "$zstart")
[ "$(json .outcome <<<"$r3")" = already_gone ] || fail "a kill of a gone parent did not return already_gone: $r3"
ok "4. a kill of a parent that is gone returned already_gone"

# refused <description> <pid>: the kill is refused and the process is still alive afterwards.
refused() {
  local r
  r=$(kill_req "$(new_id)" "$2" "$(starttime "$2")")
  [ "$(json .outcome <<<"$r")" = refused ] || fail "the kill of $1 was not refused: $r"
  [ -e "/proc/$2" ] || fail "$1 is dead after a refused kill"
  echo "$r" | json .reason
}
# PID 1 and the agent hold no zombie, so the check for that comes first (killer.go) and is the reason
# given; the ancestor holds one and reaches the check for protected processes.
reason=$(refused "PID 1" 1)
[ "$reason" = not_a_zombie_parent ] || fail "PID 1 was refused for $reason, not not_a_zombie_parent"
ok "5. PID 1 was refused ($reason)"
reason=$(refused "the agent" "$agent_pid")
[ "$reason" = not_a_zombie_parent ] || fail "the agent was refused for $reason, not not_a_zombie_parent"
ok "5. the agent was refused ($reason)"
reason=$(refused "an ancestor of the agent" "$ancestor")
[ "$reason" = protected ] || fail "an ancestor of the agent was refused for $reason, not protected"
ok "5. an ancestor of the agent was refused (protected)"
