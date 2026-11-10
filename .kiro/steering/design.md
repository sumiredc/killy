---
inclusion: fileMatch
fileMatchPattern: "mobile/**"
---

# Killy app design

When working on the mobile app, follow #[[file:docs/DESIGN.md]].

- `mock/` is a working HTML prototype of every screen. Open `mock/index.html` to see the intended look and motion; match it.
- Port artwork from `mock/art.js` to `react-native-svg` without redrawing it, and keep the class-named parts (`scythe`, `eye`, `face-dead`, `arm-front`, `hand`, `head-pt`, `anchor`, …) as separate elements so they can be animated.
- Take colours, fonts and timings from DESIGN.md, not from guesses.
- Some decided behaviour is not in the mock (queue order, more than 6 zombies, the demo button, kill outcomes other than killed, an unreachable Killy server, notifications blocked by the OS); DESIGN.md marks each with "not in the mock".
- Do not ship `mock/` in the app.
