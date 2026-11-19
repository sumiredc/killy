# Killy — Privacy Policy

This policy covers the Killy mobile app (iOS and Android) and the Killy server it connects to. It says what data they handle, why, who else receives it, and how to remove it. Last updated: 2026-11-19.

Killy has no accounts, no advertising and no analytics. It does not sell or share data for any other purpose than the ones below.

## What the app sends and why

| Data | Why |
|---|---|
| A random device token, created by the Killy server when the app first starts | To recognise this phone. Only its SHA-256 hash is stored on the server; the token itself stays in the phone's secure storage |
| The phone's push token (from Apple or Google, through Expo), only if notifications are allowed | To send a notification when a server has new zombie processes |
| The app-wide notification switch, and for each server: its display name and its notification switch | To show the servers with the names chosen on this phone and to notify only when asked to |
| Pairing codes typed or scanned | To link this phone to a server. A code is used once and expires after 10 minutes |
| Kill requests: which process on which server, with a random ID | To ask that server's agent to stop that process |

The camera is used only to read the QR code that `killy agent pair` prints. No image is stored or sent; only the code read from it is used.

The app also keeps its language setting on the phone.

## What the server keeps

The Killy server stores in its database:

- for each phone: the hash of its device token, its push token if any, and its notification switch;
- for each Linux server running the Killy agent: the hash of its agent token and its hostname;
- the links between phones and servers, with the display name and notification switch of each link;
- for each parent process with zombies: its process ID, start time and when it was first seen, and which phones were notified of it. These are deleted as soon as the process no longer holds zombies.

It keeps in memory only, and loses on restart: the latest report from each agent (process IDs, start times and command lines of parent processes holding zombies, and the zombies' process IDs), pairing codes, and request counts per client address used to limit how fast requests can be made.

The agent on a Linux server keeps the outcome of each kill (its ID, the result and how many zombies were reaped) in a file on that server.

## Who else receives data

- **Expo, Apple and Google** deliver push notifications. They receive the push token and the notification text: the number of zombie processes and the server's display name.
- **Cloudflare** carries all traffic between the app, the agents and the Killy server, and sees the address each request comes from.
- The Killy server and its web server may record the address and time of requests in their logs.

## The public demo

The builds for the Kiroween 2026 hackathon judges connect to the team's staging Killy server and offer a public demo server. Processes shown in the demo are created on a demo machine only for that purpose. Kills sent to the demo are visible to everyone using it: a parent killed by one person disappears for everyone.

## Removing data

- Removing a server in the app (Settings → server → Remove) deletes that phone's link to it, with its display name and notification switch.
- Turning notifications off in the app stops pushes; uninstalling the app stops it from using its device token.
- To have a phone's record or a server's record deleted from the Killy server, open an issue at <https://github.com/sumiredc/killy/issues>.

## Children

Killy is a tool for people who run Linux servers and is not directed at children.

## Contact

Questions about this policy: <https://github.com/sumiredc/killy/issues>.
