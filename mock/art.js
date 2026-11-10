// Flat vector art. Killy faces right (toward the zombies), has no mouth,
// and only reacts with symbols and gestures.

function killySVG() {
  return `
<svg viewBox="0 0 200 150" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
  <defs>
    <linearGradient id="killyBody" x1="1" y1="0" x2="0" y2="1">
      <stop offset="0.45" stop-color="#ffffff"/>
      <stop offset="1" stop-color="#d9d0f0"/>
    </linearGradient>
    <linearGradient id="killyBlade" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#b6a9da"/>
      <stop offset="0.55" stop-color="#e3ddf5"/>
      <stop offset="1" stop-color="#f8f6ff"/>
    </linearGradient>
  </defs>

  <g class="ghost">
    <path d="M80 16 C102 16 116 34 116 56 C116 74 112 88 108 100 Q100 112 92 102 Q84 114 74 104 Q64 116 54 106 Q40 113 26 106 Q14 101 8 88 Q28 95 40 84 C46 77 46 66 46 56 C46 34 58 16 80 16 Z" fill="url(#killyBody)"/>
    <ellipse class="eye" cx="88" cy="56" rx="4.8" ry="7.2" fill="#000"/>
    <ellipse class="eye" cx="105" cy="55" rx="3.8" ry="6.6" fill="#000"/>
  </g>

  <g class="scythe">
    <!-- snath: a gently curved shaft with a highlight -->
    <path d="M100 146 Q119 82 127 16" fill="none" stroke="#2f2740" stroke-width="7" stroke-linecap="round"/>
    <path d="M98.6 143 Q117 82 125 18" fill="none" stroke="#5c4f78" stroke-width="2" stroke-linecap="round"/>
    <circle cx="100" cy="146" r="4.5" fill="#9046ff"/>

    <!-- blade: broad at the heel, sweeping forward to a sharp point -->
    <path d="M124 6 C148 -8 184 -2 199 46 C186 24 160 16 130 24 Z" fill="url(#killyBlade)"/>
    <path d="M131 21 C160 13 185 21 197 41" fill="none" stroke="#ffffff" stroke-width="2.4" stroke-linecap="round"/>
    <path d="M126 5.5 C150 -6.5 183 -1.5 198 43" fill="none" stroke="#8a7db3" stroke-width="1.6" stroke-linecap="round"/>
    <path d="M150 6 C164 5 176 10 184 20" fill="none" stroke="#ffffff" stroke-width="1.6" stroke-linecap="round" opacity=".7"/>

    <!-- the Enter key works as the collar that joins blade and shaft -->
    <g transform="translate(127 15) rotate(8)">
      <rect x="-13" y="-10" width="26" height="23" rx="5" fill="#5b22b8"/>
      <rect x="-13" y="-12" width="26" height="21" rx="5" fill="#9046ff"/>
      <rect x="-10" y="-10" width="20" height="15" rx="3.5" fill="#a46dff"/>
      <path d="M5 -6.5 V-1 H-5 M-2.4 -3.6 L-5 -1 L-2.4 1.6" fill="none" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
    </g>
  </g>

  <circle class="arm" cx="115" cy="82" r="7.5" fill="#ffffff"/>
</svg>`;
}

// The parent process: a necromancer floating above, facing left toward Killy,
// working its children with a marionette control.
function parentSVG() {
  return `
<svg viewBox="0 0 150 150" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
  <path d="M86 6 C64 4 46 18 39 40 C28 50 17 66 12 82 Q26 76 38 78 C38 100 48 118 60 134 Q67 126 75 142 Q83 126 90 134 C102 118 112 100 112 78 Q124 76 138 82 C133 66 121 52 111 44 C113 30 110 18 102 10 Q112 6 118 12 Q110 2 86 6 Z" fill="#2a2433" stroke="#4a4157" stroke-width="2" stroke-linejoin="round"/>
  <path d="M90 18 Q100 36 94 62" fill="none" stroke="#1e1a25" stroke-width="2.4" stroke-linecap="round"/>
  <path d="M58 70 Q66 96 64 126 M92 70 Q84 96 86 126" fill="none" stroke="#1e1a25" stroke-width="2"/>
  <ellipse cx="61" cy="42" rx="15" ry="18" fill="#000" transform="rotate(-8 61 42)"/>
  <ellipse class="glow" cx="54" cy="43" rx="3.4" ry="4.6" fill="#bf9eff"/>
  <ellipse class="glow" cx="66" cy="42" rx="2.6" ry="4.2" fill="#bf9eff"/>

  <!-- marionette control: a cross bar held in both hands -->
  <rect x="8" y="91" width="134" height="6" rx="3" fill="#6b5a8e"/>
  <rect x="72" y="80" width="6" height="30" rx="3" fill="#6b5a8e"/>
  <circle cx="13" cy="86" r="6.5" fill="#b8a9cc"/>
  <circle cx="137" cy="86" r="6.5" fill="#b8a9cc"/>
  <circle class="anchor" cx="12" cy="96" r="2.2" fill="#bf9eff"/>
  <circle class="anchor" cx="44" cy="96" r="2.2" fill="#bf9eff"/>
  <circle class="anchor" cx="75" cy="109" r="2.2" fill="#bf9eff"/>
  <circle class="anchor" cx="106" cy="96" r="2.2" fill="#bf9eff"/>
  <circle class="anchor" cx="138" cy="96" r="2.2" fill="#bf9eff"/>
</svg>`;
}

// A zombie child, facing left with its arms out. String points sit on the head and the front hand.
// The face swaps to .face-dead and the arms go limp when the strings are cut.
function zombieSVG() {
  return `
<svg viewBox="0 0 56 80" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
  <rect x="20" y="58" width="8" height="16" rx="3" fill="#3b4360"/>
  <rect x="32" y="58" width="8" height="16" rx="3" fill="#3b4360"/>
  <rect x="15" y="72" width="13" height="6" rx="3" fill="#262b3d"/>
  <rect x="28" y="72" width="13" height="6" rx="3" fill="#262b3d"/>
  <g class="arm-back"><rect x="4" y="37" width="22" height="7" rx="3.5" fill="#6f9a5c"/></g>
  <path d="M16 33 H44 V60 L40 56 L36 61 L31 56 L26 61 L21 56 L16 60 Z" fill="#6b5a8e"/>
  <rect x="12" y="4" width="32" height="30" rx="13" fill="#8fbf74"/>
  <path d="M15 11 Q21 2 29 6 Q36 0 43 9 Q35 8 30 11 Q23 7 15 11 Z" fill="#3d6b35"/>
  <path d="M36 23 L40 27 M40 23 L36 27" stroke="#3d6b35" stroke-width="1.4" stroke-linecap="round"/>
  <path d="M17 27.5 H28 M20 25.5 V29.5 M24 25.5 V29.5" stroke="#2f5228" stroke-width="1.5" stroke-linecap="round"/>
  <g class="face-normal">
    <circle cx="20" cy="19" r="4.2" fill="#fff"/>
    <circle cx="19" cy="19.6" r="1.9" fill="#1c1922"/>
    <circle cx="31" cy="18" r="3" fill="#fff"/>
    <circle cx="30.2" cy="18.5" r="1.4" fill="#1c1922"/>
  </g>
  <g class="face-dead">
    <path d="M16.5 15.5 L23 22 M23 15.5 L16.5 22 M28.5 15 L33.5 20 M33.5 15 L28.5 20" stroke="#1c1922" stroke-width="2" stroke-linecap="round"/>
  </g>
  <g class="arm-front">
    <rect x="1" y="43" width="25" height="7" rx="3.5" fill="#8fbf74"/>
    <circle class="hand" cx="3.5" cy="46.5" r="3.6" fill="#8fbf74"/>
  </g>
  <circle class="head-pt" cx="28" cy="5" r="1" fill="none"/>
</svg>`;
}

// The zombie's soul: a small, calm wisp with closed eyes that rises when it is released.
function soulSVG() {
  return `
<svg viewBox="0 0 40 46" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
  <path d="M20 3 C10 3 5 11 5 20 V36 Q9 43 13 37 Q16.5 43 20 37 Q23.5 43 27 37 Q31 43 35 36 V20 C35 11 30 3 20 3 Z" fill="#e2d4ff" opacity=".9"/>
  <path d="M12.5 20 Q15 17.5 17.5 20 M22.5 20 Q25 17.5 27.5 20" fill="none" stroke="#6a4fb0" stroke-width="1.6" stroke-linecap="round"/>
</svg>`;
}

// A tiny hood used as a "remaining parents" counter, like lives in a game.
function hoodIconSVG() {
  return `
<svg viewBox="0 0 20 22" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
  <path d="M11 1 C5 1 2 6 2 11 L1 21 H19 L18 11 C18 6 16 2 13 1.4 Q16 0.6 17 2 Q15 0 11 1 Z" fill="currentColor"/>
  <ellipse cx="8.6" cy="10" rx="4.2" ry="4.8" fill="#000"/>
  <circle class="hood-eye" cx="7" cy="10.2" r="1.1"/>
  <circle class="hood-eye" cx="10" cy="10" r="0.9"/>
</svg>`;
}
