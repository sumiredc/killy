#!/bin/bash
# Builds the app icon, Android's adaptive and themed icon layers, the splash image and the Android
# notification icon from killySVG in mock/art.js: the whole Killy with the Enter-key scythe (DESIGN:
# App icon, splash and notification icon). Needs node and rsvg-convert. Run from anywhere; it writes
# into mobile/assets/images.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../../.." && pwd)
out=$repo/mobile/assets/images
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

node - "$repo/mock/art.js" "$work" <<'JS'
const fs = require('fs');
const [art, work] = process.argv.slice(2);
const svg = new Function(`${fs.readFileSync(art, 'utf8')}\nreturn killySVG();`)();
// Killy as the mock draws it, in its 200 × 150 viewBox, without the outer <svg>.
const killy = svg.replace(/^[\s\S]*?<svg[^>]*>/, '').replace(/<\/svg>\s*$/, '');
const eyes = [...svg.matchAll(/<ellipse class="eye"[^>]*\/>/g)].map((m) => m[0].replace(' class="eye"', ''));
if (eyes.length !== 2) throw new Error('expected two eyes');

// Killy in one colour with the eyes cut out, for icons Android tints: every drawn pixel of the art
// becomes white in the mask, then the eyes are drawn black over it.
const solid = (colour) =>
  `<filter id="white"><feColorMatrix type="matrix" values="0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  0 0 0 1 0"/></filter>` +
  `<mask id="cut" maskUnits="userSpaceOnUse" x="-50" y="-50" width="300" height="300"><rect x="-50" y="-50" width="300" height="300" fill="#000"/><g filter="url(#white)">${killy}</g>${eyes.join('').replace(/fill="[^"]*"/g, 'fill="#000"')}</mask>` +
  `<rect x="-50" y="-50" width="300" height="300" fill="${colour}" mask="url(#cut)"/>`;

// The whole Killy, scythe included (x 8–199, y −8–150 of the art), centred in a 100 × 100 box.
const fit = 'translate(6.6 20.4) scale(0.42)';
// A purple glow behind Killy on black, as behind the arena's necromancer.
const glow =
  `<radialGradient id="glow" gradientUnits="userSpaceOnUse" cx="56" cy="46" r="56"><stop offset="0" stop-color="#9046FF" stop-opacity="0.5"/><stop offset="1" stop-color="#9046FF" stop-opacity="0"/></radialGradient>`;
// Android's adaptive layers are 108 dp; whatever the mask, the launcher keeps the circle of 66 dp in
// the middle, so the 100 box is drawn 66 dp wide there.
const layer = 'translate(21 21) scale(0.66)';
const file = (name, viewBox, inner) =>
  fs.writeFileSync(`${work}/${name}.svg`, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}">${inner}</svg>`);

file('icon', '0 0 100 100', `<defs>${glow}</defs><rect width="100" height="100" fill="#000"/><rect width="100" height="100" fill="url(#glow)"/><g transform="${fit}">${killy}</g>`);
file(
  'android-icon-foreground',
  '0 0 108 108',
  `<defs>${glow}</defs><g transform="translate(-4 -4) scale(1.16)"><rect width="100" height="100" fill="url(#glow)"/></g><g transform="${layer}"><g transform="${fit}">${killy}</g></g>`,
);
file('android-icon-monochrome', '0 0 108 108', `<g transform="${layer}"><g transform="${fit}">${solid('#fff')}</g></g>`);
// The whole Killy with a little room around it.
file('splash-icon', '2 -14 204 170', killy);
file('notification-icon', '0 -26 210 210', solid('#fff'));
JS

render() { rsvg-convert -w "$2" -h "$3" "$work/$1.svg" -o "$out/$1.png"; }
render android-icon-foreground 1024 1024
render android-icon-monochrome 1024 1024
render splash-icon 480 400
render notification-icon 96 96
# The App Store refuses icons with transparency: render the iOS icon onto black.
rsvg-convert -w 1024 -h 1024 -b '#000000' "$work/icon.svg" -o "$out/icon.png"
echo "wrote $(cd "$out" && ls icon.png android-icon-*.png splash-icon.png notification-icon.png | tr '\n' ' ')"
