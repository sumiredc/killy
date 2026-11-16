#!/bin/bash
# Builds the app icon, Android's adaptive and themed icon layers, the splash image and the Android
# notification icon from Killy's ghost in mock/art.js (DESIGN: App icon, splash and notification
# icon). Needs node and rsvg-convert. Run from anywhere; it writes into mobile/assets/images.
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
const pick = (re) => {
  const m = re.exec(svg);
  if (!m) throw new Error(`not found in killySVG: ${re}`);
  return m[0];
};
const body = pick(/<linearGradient id="killyBody"[\s\S]*?<\/linearGradient>/);
const outline = pick(/<path d="M80 16[^"]*"/).slice('<path d="'.length, -1);
const eyes = [...svg.matchAll(/<ellipse class="eye"[^>]*\/>/g)].map((m) => m[0].replace(' class="eye"', ''));
if (eyes.length !== 2) throw new Error('expected two eyes');

const purple = '#9046FF';
// The ghost as drawn in the app: the body gradient and black eyes.
const ghost = `<path d="${outline}" fill="url(#killyBody)"/>${eyes.join('')}`;
// The ghost in one colour with the eyes cut out, for icons Android tints.
const solid = (colour) =>
  `<mask id="cut" maskUnits="userSpaceOnUse" x="-50" y="-50" width="300" height="300"><rect x="-50" y="-50" width="300" height="300" fill="#000"/><path d="${outline}" fill="#fff"/>${eyes.join('')}</mask><rect x="-50" y="-50" width="300" height="300" fill="${colour}" mask="url(#cut)"/>`;
// The mock's push icon: in a 40 × 40 box the 200-wide art is 76 wide, 18 left and 2 down.
const crop = 'translate(-18 2) scale(0.38)';
// Android's adaptive layers are 108 dp, of which the launcher shows the middle 72.
const layer = 'translate(18 18) scale(1.8)';
const file = (name, viewBox, inner) =>
  fs.writeFileSync(`${work}/${name}.svg`, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}"><defs>${body}</defs>${inner}</svg>`);

file('icon', '0 0 40 40', `<rect width="40" height="40" fill="${purple}"/><clipPath id="box"><rect width="40" height="40"/></clipPath><g clip-path="url(#box)"><g transform="${crop}">${ghost}</g></g>`);
file('android-icon-foreground', '0 0 108 108', `<g transform="${layer}"><g transform="${crop}">${ghost}</g></g>`);
file('android-icon-monochrome', '0 0 108 108', `<g transform="${layer}"><g transform="${crop}">${solid('#fff')}</g></g>`);
// The whole ghost, head to tail (x 8–116, y 16–114 in the art), with a little room around it.
file('splash-icon', '4 10 116 108', ghost);
file('notification-icon', '2 5 120 120', solid('#fff'));
JS

render() { rsvg-convert -w "$2" -h "$3" "$work/$1.svg" -o "$out/$1.png"; }
render android-icon-foreground 1024 1024
render android-icon-monochrome 1024 1024
render splash-icon 480 447
render notification-icon 96 96
# The App Store refuses icons with transparency: render the iOS icon onto the purple background.
rsvg-convert -w 1024 -h 1024 -b '#9046FF' "$work/icon.svg" -o "$out/icon.png"
rm -f "$out/android-icon-background.png"
echo "wrote $(cd "$out" && ls icon.png android-icon-*.png splash-icon.png notification-icon.png | tr '\n' ' ')"
