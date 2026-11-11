---
inclusion: always
---

# Killy — what to build

Killy is a Linux zombie-process killer driven from a phone: an agent on each server reports zombies, a relay server notifies the phone, and the app lets the operator kill the zombies' parent with a hold-to-kill battle. Entry for Kiroween 2026, Frankenstein category.

Read these before planning or writing a spec:

- Requirements, guarantees, non-goals, use cases: #[[file:docs/PRD.md]]
- Components, protocol, safety checks, deployment: #[[file:docs/ARCHITECTURE.md]]

Rules that are easy to get wrong:

- Only `SIGKILL` exists as an action. Do not add other signals or game commands.
- The agent re-verifies a target (same PID and start time through a pidfd, still a zombie parent, not PID 1 / itself / its ancestors, inside summon's cgroup in demo mode) right before signalling, whatever the server says.
- There is one battle screen for all servers; do not add server switching.
- Connection status is not shown permanently; it is checked on demand from Settings.
- Anything listed under "Not guaranteed" in PRD.md is out of scope; do not build it.
