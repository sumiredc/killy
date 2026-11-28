// The idle loops of the battle scene (DESIGN: Idle loops), as functions of one clock in ms. They
// follow the mock's CSS keyframes: the same stops, and the timing function applied to each segment
// between two stops, as CSS does. Every function is a worklet so the UI thread can call it each frame.

import { anchors, hand, head } from '@/art/points';

import {
  type Line,
  NECROMANCER_WIDTH,
  necromancerAt,
  type Point,
  type Size,
  slots,
  ZOMBIE_HEIGHT,
  ZOMBIE_WIDTH,
  zombieBox,
} from './arena';

export const periods = { killyBob: 2800, killyBlink: 4500, necroFloat: 3200, necroWork: 1600, necroGlow: 2400, puppet: 1600 } as const;

/**
 * The idle clock wraps here, the least common multiple of the periods (16.8 minutes), so every
 * loop is at the same phase just before the wrap as at 0.
 */
export const CLOCK_WRAP = 1_008_000;

/** CSS cubic-bezier(x1, y1, x2, y2) at progress x. */
export function cubicBezier(x1: number, y1: number, x2: number, y2: number, x: number): number {
  'worklet';
  if (x <= 0 || x >= 1) {
    return x <= 0 ? 0 : 1;
  }
  const bx = (t: number) => 3 * x1 * t * (1 - t) ** 2 + 3 * x2 * t * t * (1 - t) + t ** 3;
  const by = (t: number) => 3 * y1 * t * (1 - t) ** 2 + 3 * y2 * t * t * (1 - t) + t ** 3;
  // x(t) rises monotonically for the curves CSS allows, so bisection finds t.
  let lo = 0;
  let hi = 1;
  for (let i = 0; i < 30; i++) {
    const mid = (lo + hi) / 2;
    if (bx(mid) < x) {
      lo = mid;
    } else {
      hi = mid;
    }
  }
  return by((lo + hi) / 2);
}

/** CSS `ease-in-out`. */
export function easeInOut(x: number): number {
  'worklet';
  return cubicBezier(0.42, 0, 0.58, 1, x);
}

/** CSS `ease`, the timing function when a CSS animation names none. */
export function ease(x: number): number {
  'worklet';
  return cubicBezier(0.25, 0.1, 0.25, 1, x);
}

/** Where `t` falls in a loop of `period` that started `delay` ms before 0, as a fraction 0 ≤ p < 1. */
export function phase(t: number, period: number, delay = 0): number {
  'worklet';
  const p = ((t + delay) % period) / period;
  return p < 0 ? p + 1 : p;
}

/** The value of CSS keyframes at phase p: stops are [offset, value], from 0 to 1, in order. */
export function keyframes(stops: readonly (readonly [number, number])[], easing: (x: number) => number, p: number): number {
  'worklet';
  for (let i = 1; i < stops.length; i++) {
    const [o1, v1] = stops[i];
    const [o0, v0] = stops[i - 1];
    if (p <= o1) {
      return o1 === o0 ? v1 : v0 + (v1 - v0) * easing((p - o0) / (o1 - o0));
    }
  }
  return stops[stops.length - 1][1];
}

/** Killy's bob: up 5px and back (`bob`, 2.8 s). */
export function killyBob(t: number): number {
  'worklet';
  return keyframes(
    [
      [0, 0],
      [0.5, -5],
      [1, 0],
    ],
    easeInOut,
    phase(t, periods.killyBob),
  );
}

/** Killy's eyes, scaled vertically: shut for an instant every 4.5 s (`blink`). */
export function killyBlink(t: number): number {
  'worklet';
  return keyframes(
    [
      [0, 1],
      [0.94, 1],
      [0.97, 0.1],
      [1, 1],
    ],
    ease,
    phase(t, periods.killyBlink),
  );
}

/** The necromancer's float: up 8px and back (`float`, 3.2 s). */
export function necroFloat(t: number): number {
  'worklet';
  return keyframes(
    [
      [0, 0],
      [0.5, -8],
      [1, 0],
    ],
    easeInOut,
    phase(t, periods.necroFloat),
  );
}

/**
 * The necromancer working the control bar, in degrees: it rocks to −4° and +4° (`work`, 1.6 s).
 * The mock turns the whole figure around 50% across and 20% down.
 */
export function necroWork(t: number): number {
  'worklet';
  return keyframes(
    [
      [0, 0],
      [0.25, -4],
      [0.75, 4],
      [1, 0],
    ],
    easeInOut,
    phase(t, periods.necroWork),
  );
}

/** Where the necromancer turns, in its own 150px box: the mock's transform-origin 50% 20%. */
export const workOrigin = { x: NECROMANCER_WIDTH * 0.5, y: NECROMANCER_WIDTH * 0.2 };

/** The necromancer's eyes pulsing down to 45% and back (`glow`, 2.4 s). */
export function necroGlow(t: number): number {
  'worklet';
  return keyframes(
    [
      [0, 1],
      [0.5, 0.45],
      [1, 1],
    ],
    easeInOut,
    phase(t, periods.necroGlow),
  );
}

/**
 * The puppet jerk of the zombie in slot i: up 5px tilted −4°, back, up 3px tilted 3°, back
 * (`puppet`, 1.6 s). Every second zombie runs half a cycle ahead (`animation-delay: -.8s`).
 */
export function puppet(t: number, i: number): { y: number; rotate: number } {
  'worklet';
  const p = phase(t, periods.puppet, i % 2 === 1 ? periods.puppet / 2 : 0);
  const y = keyframes(
    [
      [0, 0],
      [0.25, -5],
      [0.5, 0],
      [0.75, -3],
      [1, 0],
    ],
    easeInOut,
    p,
  );
  const rotate = keyframes(
    [
      [0, 0],
      [0.25, -4],
      [0.5, 0],
      [0.75, 3],
      [1, 0],
    ],
    easeInOut,
    p,
  );
  return { y, rotate };
}

function rotateAround(p: Point, o: Point, deg: number): Point {
  'worklet';
  const r = (deg * Math.PI) / 180;
  const dx = p.x - o.x;
  const dy = p.y - o.y;
  return { x: o.x + dx * Math.cos(r) - dy * Math.sin(r), y: o.y + dx * Math.sin(r) + dy * Math.cos(r) };
}

/** The control-bar anchors at time t, floating and rocking with the necromancer, moved down by dy while it comes in. */
export function anchorsAt(t: number, arena: Size, dy = 0): Point[] {
  'worklet';
  const at = necromancerAt(arena);
  const lift = necroFloat(t) + dy;
  const turn = necroWork(t);
  const k = NECROMANCER_WIDTH / 150;
  return anchors.map((a) => {
    const p = rotateAround({ x: a.x * k, y: a.y * k }, workOrigin, turn);
    return { x: at.x + p.x, y: at.y + lift + p.y };
  });
}

/**
 * A point given in the zombie's viewBox, in arena coordinates at time t. The zombie is scaled by
 * its depth, lifted and tilted by its puppet jerk, all around its bottom centre, as the mock's
 * `transform: scale(s) translateY(y) rotate(r)` with transform-origin 50% 100%.
 */
export function zombiePointAt(t: number, i: number, arena: Size, p: Point, drop = 0): Point {
  'worklet';
  const box = zombieBox(i, arena);
  const k = ZOMBIE_WIDTH / 56;
  const origin = { x: box.left + ZOMBIE_WIDTH / 2, y: box.top + ZOMBIE_HEIGHT };
  // `drop` lifts a zombie that is coming in, in the same units as the jerk.
  const jerk = puppet(t, i);
  jerk.y += drop;
  const turned = rotateAround({ x: box.left + p.x * k, y: box.top + p.y * k }, origin, jerk.rotate);
  return { x: origin.x + (turned.x - origin.x) * box.scale, y: origin.y + (turned.y - origin.y + jerk.y) * box.scale };
}

/**
 * The marionette strings at time t for `count` zombies on the ground: from the anchor nearest
 * each zombie's head to the head, and from the nearest other anchor to its front hand, as the
 * mock's drawThreads does every frame. `dy` is how far the necromancer is moved down from its place (while it comes in); `drops` is how far each zombie that is coming in is above its place.
 */
export function stringsAt(t: number, count: number, arena: Size, dy = 0, drops?: readonly number[]): Line[] {
  'worklet';
  const from = anchorsAt(t, arena, dy);
  const nearest = (p: Point, skip: number) => {
    let best = -1;
    for (let j = 0; j < from.length; j++) {
      if (j !== skip && (best < 0 || Math.abs(from[j].x - p.x) < Math.abs(from[best].x - p.x))) {
        best = j;
      }
    }
    return best;
  };
  const lines: Line[] = [];
  for (let i = 0; i < Math.min(count, slots.length); i++) {
    const h = zombiePointAt(t, i, arena, head, drops?.[i] ?? 0);
    const f = zombiePointAt(t, i, arena, hand, drops?.[i] ?? 0);
    const a1 = nearest(h, -1);
    const a2 = nearest(f, a1);
    lines.push({ x1: from[a1].x, y1: from[a1].y, x2: h.x, y2: h.y }, { x1: from[a2].x, y1: from[a2].y, x2: f.x, y2: f.y });
  }
  return lines;
}
