#!/bin/bash
# Builds the introduction site's images from mock/art.js and the app icon: the animated scene
# (scene.svg in English, scene-ja.svg in Japanese), the share image (ogp.png), the favicon and
# the Apple touch icon. The art is placed as the mock draws it; nothing is redrawn. Needs node
# and rsvg-convert. Run from anywhere; it writes into site/.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
out=$repo/site
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$out"

node - "$repo" "$work" <<'JS'
const fs = require('fs');
const [repo, work] = process.argv.slice(2);
const art = new Function(`${fs.readFileSync(`${repo}/mock/art.js`, 'utf8')}\nreturn { killySVG, parentSVG, zombieSVG };`)();

// DESIGN.md tokens.
const C = { elev1: '#19161f', line: '#3a3443', muted: '#958f9e', text: '#fafafa', purple: '#9046ff', purple300: '#bf9eff', purple100: '#e2d4ff' };
const MONO = "'Geist Mono', ui-monospace, 'SF Mono', Menlo, Consolas, 'Hiragino Sans', monospace";
const SANS = "'Geist', system-ui, 'Hiragino Sans', sans-serif";

// The arena of the battle screen, widened: Killy bottom-left facing right, the necromancer in front
// of the moon, six zombies on the ground in the app's two rows.
const W = 720, H = 360, ZW = 48, ZH = ZW * 80 / 56;
const place = (svg, x, y, w, h) => svg.replace('<svg ', `<svg x="${x}" y="${y}" width="${w}" height="${h}" `);

const killyW = 164, killyH = killyW * 0.75, killyX = 24, killyY = H - 36 - killyH;
const parentW = 150, parentX = 0.63 * W - parentW / 2, parentY = 20;
const slots = [
  { left: 47, bottom: 40, s: 1 }, { left: 63, bottom: 40, s: 1 }, { left: 79, bottom: 40, s: 1 },
  { left: 55, bottom: 96, s: 0.84 }, { left: 71, bottom: 96, s: 0.84 }, { left: 87, bottom: 96, s: 0.84 },
];
const pids = [3812, 3813, 3814, 3815, 3816, 3817];
const anchors = [12, 44, 75, 106, 138].map((x, i) => [parentX + x, parentY + (i === 2 ? 109 : 96)]);

const zombies = slots.map((slot, i) => {
  const w = ZW * slot.s, h = ZH * slot.s;
  const x = (slot.left / 100) * W + ZW / 2 - w / 2, y = H - slot.bottom - h;
  const at = (px, py) => [x + (px / 56) * w, y + (py / 80) * h];
  return { slot, i, x, y, w, h, head: at(28, 5), hand: at(3.5, 46.5), pid: pids[i] };
});
const nearest = (px, skip) => anchors.map((a, k) => [k, Math.abs(a[0] - px)]).filter(([k]) => k !== skip).sort((a, b) => a[1] - b[1])[0][0];
const strings = zombies.flatMap((z) => {
  const k = nearest(z.head[0], -1);
  // The far end goes into the zombie (8px into the head, 2px into the hand), so the string stays
  // attached while the necromancer floats.
  return [[anchors[k], z.head, 8], [anchors[nearest(z.hand[0], k)], z.hand, 2]]
    .map(([a, b, into]) => `<line x1="${a[0].toFixed(1)}" y1="${a[1].toFixed(1)}" x2="${b[0].toFixed(1)}" y2="${(b[1] + into).toFixed(1)}"/>`);
});

const stars = [[18, 42], [42, 30], [58, 48], [30, 58], [88, 26], [12, 70]]
  .map(([l, t], i) => `<circle cx="${(l / 100 * W).toFixed(1)}" cy="${(t / 100 * H).toFixed(1)}" r="${[1.2, 1, 1.4, 1, 1.2, 1][i]}" fill="#fff" opacity="${[.53, .4, .47, .33, .4, .27][i]}"/>`).join('');

const moonX = 0.63 * W, moonY = 8 + 75;

const css = `
@media (prefers-reduced-motion: no-preference) {
  .bob { animation: bob 2.8s ease-in-out infinite; }
  .float { animation: float 3.2s ease-in-out infinite; }
  .work { transform-box: fill-box; transform-origin: 50% 20%; animation: work 1.6s ease-in-out infinite; }
  .glow { animation: glow 2.4s ease-in-out infinite; }
  .puppet { transform-box: fill-box; transform-origin: 50% 100%; animation: puppet 1.6s ease-in-out infinite; }
  .puppet.odd { animation-delay: -.8s; }
  .eye { transform-box: fill-box; transform-origin: center; animation: blink 4.5s infinite; }
}
@keyframes bob { 50% { transform: translateY(-5px); } }
@keyframes float { 50% { transform: translateY(-8px); } }
@keyframes work { 25% { transform: rotate(-4deg); } 75% { transform: rotate(4deg); } }
@keyframes glow { 50% { opacity: .45; } }
@keyframes puppet { 25% { transform: translateY(-5px) rotate(-4deg); } 50% { transform: none; } 75% { transform: translateY(-3px) rotate(3deg); } }
@keyframes blink { 0%, 94%, 100% { transform: scaleY(1); } 97% { transform: scaleY(.1); } }`;

const scene = (label, animated) => `
<defs>
  <clipPath id="card"><rect width="${W}" height="${H}" rx="24"/></clipPath>
  <radialGradient id="glow" cx=".7" cy=".7" r=".6"><stop offset="0" stop-color="${C.purple}" stop-opacity=".22"/><stop offset="1" stop-color="${C.purple}" stop-opacity="0"/></radialGradient>
  <radialGradient id="moon" cx=".38" cy=".36" r=".7"><stop offset="0" stop-color="#f3edff"/><stop offset=".55" stop-color="#cdb8ff"/><stop offset="1" stop-color="#9f7ae6"/></radialGradient>
  <linearGradient id="ground" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#211c29"/><stop offset="1" stop-color="#15121a"/></linearGradient>
  <filter id="halo" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="22"/></filter>
</defs>
<style>.face-dead { display: none; }${animated ? css : ''}</style>
<g clip-path="url(#card)">
  <rect width="${W}" height="${H}" fill="${C.elev1}"/>
  <rect width="${W}" height="${H}" fill="url(#glow)"/>
  ${stars}
  <circle cx="${moonX}" cy="${moonY}" r="80" fill="${C.purple300}" opacity=".3" filter="url(#halo)"/>
  <circle cx="${moonX}" cy="${moonY}" r="75" fill="url(#moon)" opacity=".85"/>
  <circle cx="${moonX + 30}" cy="${moonY + 20}" r="9" fill="#7859be" opacity=".35"/>
  <circle cx="${moonX - 40}" cy="${moonY + 30}" r="6" fill="#7859be" opacity=".25"/>
  <rect y="${H - 44}" width="${W}" height="44" fill="url(#ground)"/>
  <rect y="${H - 44}" width="${W}" height="1" fill="${C.line}"/>

  <g font-family="${MONO}">
    <rect x="20" y="20" width="118" height="24" rx="12" fill="${C.purple}" fill-opacity=".14"/>
    <circle cx="33" cy="32" r="3.5" fill="${C.purple300}"/>
    <text x="43" y="36" font-size="12.5" fill="${C.purple100}">demo-linux-01</text>
    <text x="20" y="98" font-size="56" font-weight="600" letter-spacing="-2" fill="${C.text}">6</text>
    <text x="62" y="98" font-size="14" fill="${C.muted}">${label}</text>
    <text x="20" y="122" font-size="12.5" fill="${C.purple300}">PID 3801</text>
    <text x="20" y="140" font-size="12" fill="${C.muted}">node evil_daemon.js</text>
  </g>

  <g class="float" stroke="${C.purple300}" stroke-width="1" opacity=".6">${strings.join('')}</g>

  <g class="bob">
    ${place(art.killySVG(), killyX, killyY, killyW, killyH)}
    <g>
      <rect x="36" y="172" width="72" height="26" rx="12" fill="${C.text}"/>
      <path d="M87 197 H96 V207 Z" fill="${C.text}"/>
      <text x="72" y="190" font-family="${SANS}" font-size="13" text-anchor="middle" fill="#000">……！</text>
    </g>
  </g>

  ${zombies.filter((z) => z.i >= 3).map((z) => `<g class="puppet${z.i % 2 ? ' odd' : ''}">${place(art.zombieSVG(), z.x, z.y, z.w, z.h)}</g>`).join('')}
  ${zombies.filter((z) => z.i < 3).map((z) => `<g class="puppet${z.i % 2 ? ' odd' : ''}">${place(art.zombieSVG(), z.x, z.y, z.w, z.h)}</g>`).join('')}
  <g font-family="${MONO}" font-size="10" fill="${C.muted}" text-anchor="middle">
    ${zombies.map((z) => `<text x="${(z.x + z.w / 2).toFixed(1)}" y="${(H - z.slot.bottom + 13).toFixed(1)}">${z.pid}</text>`).join('')}
  </g>

  <g class="float"><g class="work" style="filter: drop-shadow(0 0 18px rgba(144,70,255,.35))">${place(art.parentSVG(), parentX, parentY, parentW, parentW)}</g></g>
</g>
<rect x=".5" y=".5" width="${W - 1}" height="${H - 1}" rx="23.5" fill="none" stroke="${C.line}"/>`;

const doc = (label, animated) =>
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}">${scene(label, animated)}\n</svg>\n`;
fs.writeFileSync(`${work}/scene.svg`, doc('zombie processes', true));
fs.writeFileSync(`${work}/scene-ja.svg`, doc('体のゾンビプロセス', true));

// The share image: the same scene, still, on black, 1200 × 630.
const still = scene('zombie processes', false);
fs.writeFileSync(`${work}/ogp.svg`,
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 630" width="1200" height="630"><rect width="1200" height="630" fill="#000"/>` +
  `<svg x="24" y="27" width="1152" height="576" viewBox="0 0 ${W} ${H}">${still}</svg></svg>\n`);
JS

cp "$work/scene.svg" "$work/scene-ja.svg" "$out/"
rsvg-convert -w 1200 -h 630 "$work/ogp.svg" -o "$out/ogp.png"

# The favicon and the Apple touch icon are the app icon, scaled.
icon=$(base64 < "$repo/mobile/assets/images/icon.png" | tr -d '\n')
for spec in favicon.png:64 apple-touch-icon.png:180; do
  name=${spec%%:*}; size=${spec##*:}
  printf '<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="%s" height="%s" viewBox="0 0 %s %s"><image width="%s" height="%s" xlink:href="data:image/png;base64,%s"/></svg>' \
    "$size" "$size" "$size" "$size" "$size" "$size" "$icon" > "$work/icon.svg"
  rsvg-convert "$work/icon.svg" -o "$out/$name"
done
ls -la "$out"
