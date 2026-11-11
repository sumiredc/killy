# Killy — Product Requirements

This document is the source of truth for **what Killy must do and must not do**: purpose, guarantees, non-goals, constraints and use cases.
How the system is built (components, protocol, deployment) is in [ARCHITECTURE.md](ARCHITECTURE.md). What the app looks like and how it moves is in [DESIGN.md](DESIGN.md). Killy's character rules are in [.kiro/steering/killy-character.md](../.kiro/steering/killy-character.md).

## Purpose

Killy lets the people who run Linux servers find zombie processes and put them to rest from their phone, through a small game.

A zombie process (`<defunct>`) is a child that has exited but whose parent never collected it with `wait()`. Sending a signal to the zombie does nothing; the only fix is to deal with the **parent** (PPID). Today an operator has to notice the problem, SSH into the server, run `ps` to find the zombies and their parent, decide whether that parent can go, and kill it by hand. Away from a terminal this does not happen at all, and zombies pile up until the process table or monitoring complains.

Killy removes those steps:

- an agent on each server watches for zombies and reports them,
- the operator's phone is notified as soon as a new parent with zombies appears,
- the app shows which parent, on which server, holds how many zombies,
- the operator kills that parent with one deliberate gesture (hold to send `SIGKILL`), presented as a battle between Killy, a silent little reaper, and a necromancer working the zombies like marionettes.

Killy is our entry to the **Kiroween 2026** hackathon in the **Frankenstein** category: it stitches together Linux process control, a mobile game and push notifications.

## Users

| User | What they want |
|---|---|
| Operator | Runs one or more Linux servers. Installs the agent, pairs the phone, gets notified and kills zombie parents. |
| Judge / visitor | Has no server of their own. Installs the app (TestFlight or APK) and tries Killy against the public demo environment. |

## Guarantees

### Security

- Only a phone paired with a server can see that server's zombies or send it a kill.
- The agent sends `SIGKILL` only to a process that, at the moment of sending, is still the same process that was shown (same PID and same start time) and is still the parent of at least one zombie on that server.
- The agent never kills PID 1, itself, or any of its own ancestors, whatever the Killy server asks.
- An agent started in demo mode kills only parents created by `killy summon`.
- The agent opens no inbound port; it only makes outbound connections to the Killy server.

### Duplicates and identity

- A kill request carries an ID (a UUID v4) chosen by the phone. Sending the same ID again, at any later time and even after the agent restarts, never sends a second signal; it returns the outcome of the first.
- If the parent is already gone when a kill arrives, or has already been killed and is waiting to be reaped, no signal is sent and the outcome is "already gone". Two kills of the same parent arriving together send one signal: the first is "killed", the other "already gone".
- A pairing code can be used once and expires 10 minutes after it is shown.
- A parent is identified by server, PID and process start time, so a reused PID is treated as a new parent.
- The phone is notified once per parent (per server, PID and start time), not on every report, and a restart of the Killy server does not repeat it. The one exception: if the Killy server stops right after Expo Push Service accepts a push and before the server records it, that push is sent once more after the restart.

### Consistency

- The zombies shown on the phone are the latest report from each agent. After any reconnection the phone receives a full snapshot that replaces what it had.
- When the outcome of a kill is not known within the timeout, the app says so and refreshes; it never shows an unconfirmed kill as a success.

### Timeliness

- A new parent with zombies triggers a notification within 30 seconds of the zombie appearing, while the agent and the Killy server are connected.
- A kill on a connected server returns its outcome within 10 seconds.

### Observability

- The operator can check, per server, whether a command can reach the agent and come back, and is told which part failed (see UC4).
- Every kill ends with an outcome the operator can see: killed, already gone, refused (with the reason), failed, or unknown.

## Not guaranteed

- **Signals other than `SIGKILL`.** No `SIGCHLD` "persuade" or `SIGTERM` "attack": the game has one action to stay simple.
- **Judging whether a parent is safe to kill.** Killy shows what it will kill (server, PID, command line); the decision is the operator's.
- **Zombies in other PID namespaces.** The agent sees only processes visible in its own namespace (containers on the host are not inspected separately).
- **Non-Linux servers.** No macOS or Windows agent.
- **Slack, Discord or other chat notifications.** Push notifications to the app only.
- **Choosing which server to fight.** There is one battle screen; parents from all servers form one queue.
- **A permanent connection-status indicator.** Connection is checked on demand from Settings (UC4).
- **Accounts or sharing between people.** A device is the identity; there is no login.
- **History, statistics or analytics** of past zombies and kills.
- **Peer-to-peer connections** between phone and agent; everything is relayed by the Killy server.
- **High availability.** The Killy server runs as a single instance; a restart drops live connections until agents and phones reconnect.

## Fixed constraints

| Kind | Constraint |
|---|---|
| Hackathon | Submission deadline **2026-12-04 14:00 PT**. Category: Frankenstein. Public repository under the MIT License, with `.kiro/` at the root and not git-ignored. Demo video under 3 minutes. |
| Repository | `github.com/sumiredc/killy`: private during development, made public before submission. |
| Agent platform | Linux 5.3 or later (the agent signals through a pidfd); zombies are read from `/proc`. Real machine or VM. |
| Languages | Backend (agent, server, summon) in Go 1.27 (toolchain go1.27.3). Mobile app in React Native with Expo. |
| Hosting | Killy server on the team's home Kubernetes cluster at `killy.sumire.pink`, exposed through the existing Cloudflare Tunnel, one replica. The home cluster gives no uptime guarantee. |
| Notifications | Expo Push Service (APNs and FCM credentials managed by EAS). |
| Distribution | iOS through a TestFlight public link, Android as a directly downloaded APK. |
| Character | Killy never speaks. See the character rules. |

## Use cases

### UC1 Add a server

The operator wants a server to be watched.

- Preconditions: the agent binary is installed on the server; the server can reach the Killy server over HTTPS.
- Main flow: the operator runs `killy agent pair` on the server; it shows a 6-character code and a QR code. The operator opens Settings → Add server, enters the code (or scans the QR) and optionally a display name, and taps Add. The app waits for the agent and shows "Added ‹name›".
- Alternative: no display name is given → the server's hostname is used.
- Alternative: the same server is paired from a second phone → both phones see it and are notified.
- Exception: the code is unknown, already used or expired → the app shows that and asks to run `killy agent pair` again; nothing is added.
- Exception: the phone cannot reach the Killy server → the app says so; nothing is added.
- Postconditions: the server appears in Settings → Servers; its zombies, if any, join the battle queue.

### UC2 Get notified of zombies

The operator wants to know when a server has zombies, even with the app closed.

- Preconditions: app notifications and the server's notifications are on; the OS allows notifications.
- Main flow: a parent with zombies appears on a server; the phone gets a push notification whose text is a system-log line (zombie count, parent command, PID, server) followed by a Killy symbol. Tapping it opens the battle screen.
- Alternative: the app is open → no push is needed; the battle screen updates live.
- Exception: notifications are off for the app or the server → no push; the battle screen still shows the zombies when opened.
- Exception: the same parent is reported again → no second notification.
- Postconditions: the parent is in the battle queue.

### UC3 Kill zombie parents (battle)

The operator wants the zombies gone.

- Preconditions: at least one connected server has a parent with zombies.
- Main flow: parents from all connected servers are fought one at a time, oldest detection first (the detection time survives a Killy server restart). The battle screen shows the current parent: its server, zombie count, PID and command line. The operator holds the kill button for 1 second; `SIGKILL` is sent to that parent on that server. The zombies are released and the next parent appears. After the last one, "All clear" shows how many parents were killed and zombies released.
- Alternative: the operator releases the button before 1 second → nothing is sent.
- Alternative: new zombies of the current parent are reported during the battle → they appear on screen and the count updates.
- Alternative: a new parent is reported during the battle → it is added to the end of the queue (it is the most recently detected).
- Exception: the parent is already gone → outcome "already gone"; the app moves to the next parent.
- Exception: the agent refuses (no longer a zombie parent, PID reused, protected process, not a demo process, pidfd not available on the server) → no signal is sent; the reason is shown; the parent stays in the queue only if it is still reported.
- Exception: the signal fails (e.g. permission denied) → the error is shown; the parent stays.
- Exception: the server's agent is not connected, or no outcome arrives within 10 seconds → "unknown"; the app refreshes from the latest snapshot.
- Exception: the phone gets no response from the Killy server → the app resends the same kill (same ID) once; if that also fails, it says so under the kill button and the button can be held again.
- Postconditions on success: the parent process no longer exists on the server; its zombies are no longer in the process table; the server's zombie count in Settings is updated.
- Postconditions in all cases: no process other than the shown parent received a signal.

### UC4 Check the connection to a server

The operator wants to know whether Killy can reach a server right now.

- Preconditions: the server is paired.
- Main flow: in Settings → server → Connection, the operator taps Check. The app shows "Connected (round trip in ms / agent version)".
- Exception: the phone cannot reach the Killy server → "Check your network".
- Exception: the agent is not connected to the Killy server → "No response from the agent. Last heard ‹n› min ago. Check that killy agent is running on the server."
- Exception: the agent is connected but does not answer within 5 seconds → "Commands are not reaching the agent. Restart the agent."
- Postconditions: nothing on the server changes.

### UC5 Manage a server

- Rename: the new display name is shown everywhere in the app. An empty name keeps the previous one.
- Mute: turning off the server's notifications stops pushes for that server only; its zombies still appear in the battle.
- Remove: needs a second tap to confirm. The server disappears from this phone, its parents leave the battle queue, and the agent keeps running (other paired phones are unaffected).

### UC6 Try the public demo

A judge wants to try Killy without a server.

- Main flow: in Add server, the judge taps "Try the demo server"; the public demo server is added without a pairing code. Demo zombies appear and can be killed.
- Alternative: another visitor kills the same demo parent first → it disappears from everyone's screen ("already gone" for the late one).
- Exception: no public demo server is configured → the app says the demo server is not available; nothing is added.
- Postconditions: only parents created by `killy summon` on the demo machine are ever killed; new ones keep appearing.

### UC7 Run a demo environment

The team wants a server that always has safe zombies to kill.

- Main flow: on the demo Linux machine, the agent runs in demo mode and `killy summon` keeps creating parents (with varied command names) that leave zombies, up to a limit.
- Postconditions: there is usually at least one demo parent to kill; nothing else on the machine can be killed through Killy or appears in the app.

### UC8 App settings

- Language: Japanese or English; the default follows the phone's language.
- Notifications: one switch for the whole app, in addition to the per-server switch.

## Should-have

Built only if time allows before the deadline; not part of the guarantees above.

- **Signed kill commands.** At pairing, the phone registers a public key with the agent; the phone signs every kill command and the agent rejects commands without a valid signature. The Killy server could then relay kills but never forge one.
