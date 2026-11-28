// The kill sequence (DESIGN: Kill sequence, Kill outcomes other than killed), as functions of the
// time since the hold completed. They follow the mock's kill() and its CSS keyframes: the same
// moments, the same keyframe stops, and the animation's timing function on each segment between
// stops, as CSS applies it. Everything here is a worklet, so the UI thread can call it each frame.
//
// The steps up to `defeatFrom` play at once; the rest waits for the agent's result. `at` is when
// the sequence goes on: the later of `defeatFrom` and the arrival of the result. Until then the
// necromancer trembles.

import { cubicBezier, ease, keyframes } from './idle';

/** Moments of the sequence before the result is needed, in ms from the end of the hold. */
export const HIT = 320;
export const DEFEAT_FROM = 670;

const DASH_MS = 700;
const FLASH_MS = 350;
const SIGKILL_MS = 1000;
const STRINGS_MS = 250;
const SHAKE_MS = 300;
const ZHIT_MS = 320;
const ARM_MS = 200;
const FALL_MS = 500;
const FADE_MS = 700;
const SOUL_MS = 1300;
const PID_MS = 400;
const GONE_MS = 900;
const HOP_MS = 450;
const BOW_MS = 250;
const FIDGET_MS = 450;
const FIDGET_TIMES = 3;
const POP_MS = 180;
/** How long the next parent takes to come in, and how long after Killy's bow the All clear card appears. */
export const NEXT_IN_MS = 600;
export const CLEAR_AFTER_BOW_MS = 600;
/** Where the next parent comes in from: 40px above its place, fading in (the mock's `enter`, CSS ease-out). */
export const ENTER_FROM = -40;

export type Outcome = 'defeat' | 'recoil';

/** The sequence's clock when no kill is under way: before 0, where every step is at rest. */
export const REST = -1;
/** How far the sequence's clock runs: well past the longest wait for a result (a resent request) and the defeat after it. */
export const CLOCK_MS = 120_000;

/** The outcome as the UI thread holds it in a number: 0 while the result is awaited, 1 defeat, 2 recoil. */
export function outcomeOf(code: number): Outcome | null {
  'worklet';
  return code === 1 ? 'defeat' : code === 2 ? 'recoil' : null;
}
export function outcomeCode(outcome: Outcome | null): number {
  return outcome === 'defeat' ? 1 : outcome === 'recoil' ? 2 : 0;
}

/** When the sequence goes on: the result's arrival, but not before the zombies are due to fall. */
export function settleAt(arrival: number): number {
  'worklet';
  return Math.max(DEFEAT_FROM, arrival);
}

/**
 * The rows of DESIGN's Kill sequence table after the result, for `n` zombies on the ground: each
 * `+` there is from the row before. With the result at 670 ms, `next` is 5150 + 200·n.
 */
export function defeatRows(at: number, n: number) {
  'worklet';
  const sag = at + 380 + 70 * n;
  const release = sag + 600;
  const gone = release + 1300 + 130 * n;
  const result = gone + 900;
  const next = result + 1300;
  return { fall: at, sag, release, gone, result, next };
}

/** When a refused or failed kill is over: the necromancer has recoiled and its strings are back. */
export function recoilEnd(at: number): number {
  'worklet';
  return at + SHAKE_MS + STRINGS_MS;
}

/** When the sequence ends and the battle moves on (the next parent, the All clear card, or the button again). */
export function strikeEnd(outcome: Outcome, at: number, n: number, allClear: boolean): number {
  'worklet';
  if (outcome === 'recoil') {
    return recoilEnd(at);
  }
  return defeatRows(at, n).next + (allClear ? CLEAR_AFTER_BOW_MS : 0);
}

const clamp = (x: number) => {
  'worklet';
  return x < 0 ? 0 : x > 1 ? 1 : x;
};
/** CSS `ease-out`. */
const easeOut = (x: number) => {
  'worklet';
  return cubicBezier(0, 0, 0.58, 1, x);
};
/** Progress through a step that starts at `from` and lasts `ms`, 0 before and 1 after. */
const progress = (t: number, from: number, ms: number) => {
  'worklet';
  return clamp((t - from) / ms);
};

// ---------- Killy ----------

/** Killy's dash: 70px forward and back over 700 ms (the mock's `dash`, cubic-bezier(.3, 0, .2, 1)). */
export function killyDash(t: number): number {
  'worklet';
  if (t < 0 || t >= DASH_MS) {
    return 0;
  }
  return keyframes(
    [
      [0, 0],
      [0.45, 70],
      [1, 0],
    ],
    (x) => {
      'worklet';
      return cubicBezier(0.3, 0, 0.2, 1, x);
    },
    t / DASH_MS,
  );
}

/**
 * The scythe's swing over the same 700 ms, in degrees: from the wound-up −42° to 75° and back to 0
 * (the mock's `swing`, cubic-bezier(.5, -.3, .3, 1.2)). Null outside the swing, where the hold's
 * wind-up decides.
 */
export function scytheSwing(t: number): number | null {
  'worklet';
  if (t < 0 || t >= DASH_MS) {
    return null;
  }
  return keyframes(
    [
      [0, -42],
      [0.45, 75],
      [1, 0],
    ],
    (x) => {
      'worklet';
      return cubicBezier(0.5, -0.3, 0.3, 1.2, x);
    },
    t / DASH_MS,
  );
}

/** Two small hops from `from`, 14px up, 450 ms each (the mock's `hop`, ease-out). Null outside them, where Killy bobs. */
export function killyHop(t: number, from: number): number | null {
  'worklet';
  if (t < from || t >= from + 2 * HOP_MS) {
    return null;
  }
  return keyframes(
    [
      [0, 0],
      [0.5, -14],
      [1, 0],
    ],
    easeOut,
    ((t - from) % HOP_MS) / HOP_MS,
  );
}

/** How long Killy takes to bow (the transition of the mock's `.killy .ghost`, CSS `ease`). */
export const BOW_FOR = BOW_MS;

/** How long each of Killy's bubbles stays (the mock's react() calls in kill()). */
export const BUBBLE_MS = { start: 800, killed: 1000, recoil: 1400, allClear: 3000, notice: 1400 } as const;

/** How long Killy's fidget lasts: three left-right shuffles of 450 ms (the mock's `orooro`, `.45s ease-in-out 3`). */
export const FIDGET_TOTAL = FIDGET_MS * FIDGET_TIMES;

const easeInOut = (x: number) => {
  'worklet';
  return cubicBezier(0.42, 0, 0.58, 1, x);
};

/** Killy's sideways offset, in px, t ms into the fidget: 6px left at a quarter of each shuffle, 6px right at three quarters. */
export function killyFidget(t: number): number {
  'worklet';
  if (t < 0 || t >= FIDGET_TOTAL) {
    return 0;
  }
  return keyframes(
    [
      [0, 0],
      [0.25, -6],
      [0.75, 6],
      [1, 0],
    ],
    easeInOut,
    (t % FIDGET_MS) / FIDGET_MS,
  );
}

/** How long a bubble takes to pop in or out (the mock's `.bubble` transition, .18s). */
export const BUBBLE_POP_MS = POP_MS;

/** The bubble at pop progress `p` (0 hidden, 1 shown): grows from 0.4 and fades in over the first 120 ms of the 180. */
export function bubblePop(p: number): { scale: number; opacity: number } {
  'worklet';
  return { scale: 0.4 + 0.6 * p, opacity: clamp((p * POP_MS) / 120) };
}

// ---------- The arena ----------

/** The arena's purple-100 flash at the hit, fading over 350 ms (the mock's `.flash`). */
export function flashOpacity(t: number): number {
  'worklet';
  if (t < HIT || t >= HIT + FLASH_MS) {
    return 0;
  }
  return 1 - ease(progress(t, HIT, FLASH_MS));
}

/** "SIGKILL" floating up 34px above the necromancer as it fades over 1 s (the mock's `.float`). */
export function sigkill(t: number): { opacity: number; y: number } {
  'worklet';
  if (t < HIT || t >= HIT + SIGKILL_MS) {
    return { opacity: 0, y: 0 };
  }
  const p = ease(progress(t, HIT, SIGKILL_MS));
  return { opacity: 1 - p, y: -34 * p };
}

/**
 * The strings' opacity: 0.6, fading out over 250 ms at the hit. After a refused or failed kill
 * they come back once the necromancer has recoiled (DESIGN's "strings stay" is the state after).
 */
export function stringsOpacity(t: number, outcome: Outcome | null, at: number): number {
  'worklet';
  const out = 0.6 * (1 - ease(progress(t, HIT, STRINGS_MS)));
  if (outcome !== 'recoil') {
    return out;
  }
  return out + 0.6 * ease(progress(t, at + SHAKE_MS, STRINGS_MS));
}

// ---------- The necromancer ----------

const shake = (p: number) => {
  'worklet';
  return keyframes(
    [
      [0, 0],
      [0.25, -6],
      [0.5, 6],
      [0.75, -3],
      [1, 0],
    ],
    ease,
    p,
  );
};

/**
 * The necromancer's moves: it shakes at the hit (the mock's `shake`, 300 ms), trembles from
 * `defeatFrom` until the result is in, shakes once more when a kill is refused or fails, and after
 * a defeat fades upward to 70% over 900 ms once its zombies are gone (the mock's `gone`; the blur
 * is not drawn). `outcome` is null while the result is awaited.
 */
export function necromancer(t: number, outcome: Outcome | null, at: number, n: number): { x: number; y: number; scale: number; opacity: number } {
  'worklet';
  let x = 0;
  if (t >= HIT && t < HIT + SHAKE_MS) {
    x = shake(progress(t, HIT, SHAKE_MS));
  } else if (t >= DEFEAT_FROM && (outcome === null || t < at)) {
    x = shake(((t - DEFEAT_FROM) % SHAKE_MS) / SHAKE_MS);
  } else if (outcome === 'recoil' && t >= at && t < at + SHAKE_MS) {
    x = shake(progress(t, at, SHAKE_MS));
  }
  if (outcome !== 'defeat') {
    return { x, y: 0, scale: 1, opacity: 1 };
  }
  const p = ease(progress(t, defeatRows(at, n).gone, GONE_MS));
  return { x, y: -30 * p, scale: 1 - 0.3 * p, opacity: 1 - p };
}

// ---------- The zombies ----------

export type ZombiePose = {
  /** False once the zombie is hit: its puppet jerk stops and face-dead shows. */
  alive: boolean;
  /** Both arms, in degrees around their shoulder (the mock's `.zombie.dead` arms, −65° over 200 ms ease-out). */
  arm: number;
  /** The body, around 72% across and its feet (the mock's `.zombie .body`). */
  body: { x: number; rotate: number; scaleY: number; opacity: number };
  soul: { opacity: number; x: number; y: number; scale: number };
  pidOpacity: number;
};

/**
 * Zombie i of n after a defeat (`outcome` 'defeat'; any other leaves it as it is). It is hit 70 ms
 * after the one before it: a wobble (`zhit`; its white flash is not drawn), face-dead, arms down.
 * All sag back together (`fall`), then each, 130 ms apart, fades (`fade-body`) as its soul rises
 * (`soul-up`).
 */
export function zombie(t: number, i: number, n: number, outcome: Outcome | null, at: number): ZombiePose {
  'worklet';
  const still: ZombiePose = {
    alive: true,
    arm: 0,
    body: { x: 0, rotate: 0, scaleY: 1, opacity: 1 },
    soul: { opacity: 0, x: 0, y: 0, scale: 0.5 },
    pidOpacity: 1,
  };
  if (outcome !== 'defeat') {
    return still;
  }
  const rows = defeatRows(at, n);
  const hitAt = rows.fall + 70 * i;
  if (t < hitAt) {
    return still;
  }
  const arm = -65 * easeOut(progress(t, hitAt, ARM_MS));
  let x = 0;
  let rotate = 0;
  let scaleY = 1;
  if (t < rows.sag) {
    if (t < hitAt + ZHIT_MS) {
      const p = progress(t, hitAt, ZHIT_MS);
      x = keyframes(
        [
          [0, 0],
          [0.3, 5],
          [0.6, -3],
          [1, 0],
        ],
        ease,
        p,
      );
      rotate = keyframes(
        [
          [0, 0],
          [0.3, 4],
          [0.6, 0],
          [1, 0],
        ],
        ease,
        p,
      );
    }
  } else {
    const p = progress(t, rows.sag, FALL_MS);
    const fall = (x0: number) => {
      'worklet';
      return cubicBezier(0.55, 0, 0.9, 0.45, x0);
    };
    x = keyframes(
      [
        [0, 0],
        [0.55, 6],
        [1, 4],
      ],
      fall,
      p,
    );
    rotate = keyframes(
      [
        [0, 0],
        [0.55, 30],
        [1, 22],
      ],
      fall,
      p,
    );
    scaleY = keyframes(
      [
        [0, 1],
        [0.55, 1],
        [1, 0.9],
      ],
      fall,
      p,
    );
  }
  const releaseAt = rows.release + 130 * i;
  const opacity = 1 - ease(progress(t, releaseAt, FADE_MS));
  let soul = still.soul;
  if (t >= releaseAt) {
    const p = progress(t, releaseAt, SOUL_MS);
    soul = {
      opacity: keyframes(
        [
          [0, 0],
          [0.2, 1],
          [0.7, 0.9],
          [1, 0],
        ],
        easeOut,
        p,
      ),
      x: keyframes(
        [
          [0, 0],
          [0.45, -6],
          [0.7, 5],
          [1, -2],
        ],
        easeOut,
        p,
      ),
      y: keyframes(
        [
          [0, 0],
          [0.45, -50],
          [0.7, -90],
          [1, -130],
        ],
        easeOut,
        p,
      ),
      scale: keyframes(
        [
          [0, 0.5],
          [0.45, 1],
          [0.7, 1],
          [1, 1.05],
        ],
        easeOut,
        p,
      ),
    };
  }
  return { alive: false, arm, body: { x, rotate, scaleY, opacity }, soul, pidOpacity: 1 - ease(progress(t, releaseAt, PID_MS)) };
}
