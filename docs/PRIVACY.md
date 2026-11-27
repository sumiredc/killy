# Killy — Privacy Policy

This policy covers the Killy mobile app (iOS and Android), the Web demo at <https://killy.sumire-sakamoto.co.jp/demo/>, the Killy server they connect to and the contact form on the support page. It says what data they handle, why, who else receives it, and how to remove it. Last updated: 2026-11-27.

Killy has no accounts, no advertising and no analytics. It does not sell or share data for any other purpose than the ones below.

## What the app sends and why

| Data | Why |
|---|---|
| A random device token, created by the Killy server when the app first starts | To recognise this phone. Only its SHA-256 hash is stored on the server; the token itself stays in the phone's secure storage (in the Web demo, in the browser's local storage; see "The public demo") |
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

## What the contact form collects

The contact form on the support page (<https://killy.sumire-sakamoto.co.jp/support/>) is a Google Form. It asks for:

| Data | Why |
|---|---|
| Email address | To reply to your message |
| Name (optional) | To address you in the reply |
| Type of inquiry | To route the message |
| Device (optional) | To reproduce a problem on the app |
| App version (optional) | To reproduce a problem on the app |
| The server's operating system and CPU (optional) | To reproduce a problem with the agent |
| Your message | To answer it |

We use these answers only to reply to you and to carry out requests to delete data. Google stores the answers of the form. The form is opened on Google's site; the Killy app, the Killy server and the support page do not send it anything else.

## Who else receives data

- **Expo, Apple and Google** deliver push notifications. They receive the push token and the notification text: the number of zombie processes and the server's display name.
- **Google** stores the answers to the contact form (see above) and receives what you enter into it.
- **Cloudflare** carries all traffic between the app, the agents and the Killy server, and sees the address each request comes from.
- The Killy server and its web server may record the address and time of requests in their logs.

## The public demo

This section applies only to the builds made for the Kiroween 2026 hackathon judges and to the Web demo (below). They connect to the team's staging Killy server and offer a public demo server. The store release of the app (App Store and Google Play) connects to the production Killy server and has no demo. Processes shown in the demo are created on a demo machine only for that purpose. Kills sent to the demo are visible to everyone using it: a parent killed by one person disappears for everyone.

The Web demo, at <https://killy.sumire-sakamoto.co.jp/demo/>, is the same app in a browser, for the demo only. It connects to the staging Killy server and adds the demo server by itself when it opens, with no pairing code. It keeps the device token and the language setting in the browser's local storage (`localStorage`) for the site killy.sumire-sakamoto.co.jp, where the phone app uses its secure storage. It sends no push token and uses no camera. The page and its scripts, images and fonts come from the introduction site only, and the page loads nothing from a third party; the introduction site's web server may record the address and time of requests in its logs.

## Removing data

- Removing a server in the app (Settings → server → Remove) deletes that phone's link to it, with its display name and notification switch.
- Turning notifications off in the app stops pushes; uninstalling the app stops it from using its device token.
- In the Web demo, clearing the site data of killy.sumire-sakamoto.co.jp in the browser deletes the device token and the language setting; the next visit creates a new device. The old device's record on the Killy server is deleted on request, as below.
- To have a phone's record or a server's record deleted from the Killy server, send a request through the contact form on the support page, <https://killy.sumire-sakamoto.co.jp/support/>. Answers you gave in the form are deleted on request too.

## Children

Killy is a tool for people who run Linux servers and is not directed at children.

## Contact

Questions about this policy: the contact form on the support page, <https://killy.sumire-sakamoto.co.jp/support/>.
