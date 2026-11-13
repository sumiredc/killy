#!/bin/sh
# Kiro PreToolUse hook for the shell tool (#60). Runs before every shell command, so a command
# without the word `merge` passes here without starting Go or calling gh. Kiro blocks only on
# exit 2 and lets the command run on any other code, so every failure here becomes exit 2.
advice='Do not merge another way (gh api, git push); wait for CI to finish, or ask the user.'
fail() {
	echo "Merge blocked (#60): $1" >&2
	echo "$advice" >&2
	exit 2
}

event=$(cat) || fail 'cannot read the hook event'
printf '%s' "$event" | grep -q merge
case $? in
0) ;;
1) exit 0 ;;
*) fail 'cannot search the hook event' ;;
esac

# Built rather than `go run`, because `go run` turns the gate's exit 2 into 1.
dir=$(mktemp -d) || fail 'cannot make a directory for the gate'
trap 'rm -rf "$dir"' EXIT
go build -o "$dir/mergegate" ./tools/mergegate || fail 'the gate could not be built (is Go installed?)'
printf '%s' "$event" | "$dir/mergegate"
code=$?
case $code in
0) exit 0 ;;
2) exit 2 ;;
*) fail "the gate stopped with exit $code" ;;
esac
