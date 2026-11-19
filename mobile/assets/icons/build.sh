#!/bin/bash
# Builds the app icon, Android's adaptive and themed icon layers, the splash image and the Android
# notification icon from killySVG in mock/art.js, drawn with the Enter-key scythe (DESIGN: App icon,
# splash and notification icon). Needs node and rsvg-convert. Run from anywhere; it writes
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

// The mock's push icon: in a 40 × 40 box the 200-wide art is 76 wide, 18 left and 2 down, so Killy's
// head peeks in from the lower left with the scythe's collar and shaft in front of it.
const crop = 'translate(-18 2) scale(0.38)';
// Android's adaptive layers are 108 dp, of which the launcher shows the middle 72.
const layer = 'translate(18 18) scale(1.8)';
const file = (name, viewBox, inner) =>
  fs.writeFileSync(`${work}/${name}.svg`, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}">${inner}</svg>`);

file('icon', '0 0 40 40', `<rect width="40" height="40" fill="#000"/><clipPath id="box"><rect width="40" height="40"/></clipPath><g clip-path="url(#box)"><g transform="${crop}">${killy}</g></g>`);
file('android-icon-foreground', '0 0 108 108', `<g transform="${layer}"><g transform="${crop}">${killy}</g></g>`);
file('android-icon-monochrome', '0 0 108 108', `<g transform="${layer}"><g transform="${crop}">${solid('#fff')}</g></g>`);
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
