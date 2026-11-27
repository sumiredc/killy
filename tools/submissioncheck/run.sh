#!/usr/bin/env bash
# Checks the hackathon's repository requirements that a change could break without anyone noticing
# (docs/PRD.md, "Fixed constraints"):
#   - LICENSE is the MIT License.
#   - .kiro/ is at the root, is not git-ignored, and the parts of it that the submission shows
#     (specs, steering, hooks) are committed and not ignored either.
# That the repository is public is a setting, not a file, and is not checked here.
set -euo pipefail

cd "$(dirname "$0")/../.."
status=0
bad() { echo "FAIL: $*" >&2; status=1; }

if [ "$(head -n 1 LICENSE 2>/dev/null)" != "MIT License" ] \
  || ! grep -q 'Permission is hereby granted, free of charge' LICENSE \
  || ! grep -q 'THE SOFTWARE IS PROVIDED "AS IS"' LICENSE; then
  bad "LICENSE is not the MIT License"
else
  echo "ok: LICENSE is the MIT License"
fi

for dir in .kiro .kiro/specs .kiro/steering .kiro/hooks; do
  if [ ! -d "$dir" ]; then
    bad "$dir does not exist"
    continue
  fi
  # --no-index: report the rule that matches the path even for a path that is already committed.
  if git check-ignore -q --no-index "$dir"; then
    bad "$dir is git-ignored ($(git check-ignore -v --no-index "$dir"))"
    continue
  fi
  if [ -z "$(git ls-files "$dir")" ]; then
    bad "no file under $dir is committed"
    continue
  fi
  echo "ok: $dir is committed and not git-ignored"
done

# A rule such as `*.md` leaves .kiro/ itself unignored but keeps new files in it out of git. A committed
# file that a rule matches shows that.
ignored=$(git ls-files -ci --exclude-standard .kiro)
if [ -n "$ignored" ]; then
  bad "files under .kiro are committed but match a .gitignore rule, so new ones would not be added:
$ignored"
else
  echo "ok: no .gitignore rule matches a committed file under .kiro"
fi

exit "$status"
