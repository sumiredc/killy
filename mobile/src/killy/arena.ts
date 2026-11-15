// Where things stand in the battle arena (DESIGN: Battle tab), as pure functions of the arena's
// size, so the layout and the strings can be checked without drawing. Sizes are DESIGN's pixels;
// positions are fractions of the arena, as in the mock's CSS.

import { anchors } from '@/art/Necromancer';
import { hand, head } from '@/art/Zombie';

export type Size = { width: number; height: number };
export type Point = { x: number; y: number };
export type Line = { x1: number; y1: number; x2: number; y2: number };

export const KILLY_WIDTH = 164;
export const NECROMANCER_WIDTH = 150;
export const MOON_SIZE = 150;
export const GROUND_HEIGHT = 44;
export const ZOMBIE_WIDTH = 48;
const zombieViewBox = { width: 56, height: 80 };
export const ZOMBIE_HEIGHT = (ZOMBIE_WIDTH * zombieViewBox.height) / zombieViewBox.width;

/** The six places on the ground: left is a fraction of the arena width, bottom is px, s the depth scale. */
export const slots = [
  { left: 0.47, bottom: 40, s: 1 },
  { left: 0.63, bottom: 40, s: 1 },
  { left: 0.79, bottom: 40, s: 1 },
  { left: 0.55, bottom: 96, s: 0.84 },
  { left: 0.71, bottom: 96, s: 0.84 },
  { left: 0.87, bottom: 96, s: 0.84 },
] as const;

/** The top-left corner of the necromancer: centred at 63% of the width, top at 27% of the height. */
export function necromancerAt(arena: Size): Point {
  return {
    x: arena.width * 0.63 - NECROMANCER_WIDTH / 2,
    y: arena.height * 0.27,
  };
}

/** The top-left corner of the moon, which sits behind the necromancer (the mock's `.moon`). */
export function moonAt(arena: Size): Point {
  return { x: arena.width * 0.63 - MOON_SIZE / 2, y: arena.height * 0.22 };
}

/** The box of the zombie in slot i before its depth scale, which shrinks it towards its bottom centre. */
export function zombieBox(i: number, arena: Size): { left: number; top: number; scale: number } {
  const slot = slots[i];
  return {
    left: arena.width * slot.left,
    top: arena.height - slot.bottom - ZOMBIE_HEIGHT,
    scale: slot.s,
  };
}

/** A point given in the zombie's viewBox, in arena coordinates, for the zombie in slot i. */
export function zombiePoint(i: number, arena: Size, p: Point): Point {
  const box = zombieBox(i, arena);
  const k = ZOMBIE_WIDTH / zombieViewBox.width;
  const cx = box.left + ZOMBIE_WIDTH / 2;
  const bottom = box.top + ZOMBIE_HEIGHT;
  return {
    x: cx + (p.x * k - ZOMBIE_WIDTH / 2) * box.scale,
    y: bottom - (ZOMBIE_HEIGHT - p.y * k) * box.scale,
  };
}

/** The control-bar anchors in arena coordinates (the necromancer's viewBox is 1:1 at 150px). */
export function anchorPoints(arena: Size): Point[] {
  const at = necromancerAt(arena);
  const k = NECROMANCER_WIDTH / 150;
  return anchors.map((a) => ({ x: at.x + a.x * k, y: at.y + a.y * k }));
}

/**
 * The marionette strings for `count` zombies on the ground: one from the anchor nearest each
 * zombie's head, one from the nearest other anchor to its front hand. "Nearest" is by x, as in
 * the mock's drawThreads.
 */
export function strings(count: number, arena: Size): Line[] {
  const from = anchorPoints(arena);
  const nearest = (p: Point, skip?: Point) =>
    from.filter((a) => a !== skip).reduce((best, a) => (Math.abs(a.x - p.x) < Math.abs(best.x - p.x) ? a : best));
  const lines: Line[] = [];
  for (let i = 0; i < Math.min(count, slots.length); i++) {
    const h = zombiePoint(i, arena, head);
    const f = zombiePoint(i, arena, hand);
    const a1 = nearest(h);
    const a2 = nearest(f, a1);
    lines.push({ x1: a1.x, y1: a1.y, x2: h.x, y2: h.y }, { x1: a2.x, y1: a2.y, x2: f.x, y2: f.y });
  }
  return lines;
}

/** The zombies drawn on the ground: the first six, and how many more the `+N` chip stands for. */
export function onGround(zombies: number[]): { shown: number[]; more: number } {
  return {
    shown: zombies.slice(0, slots.length),
    more: Math.max(0, zombies.length - slots.length),
  };
}

/** From this many parents in the battle on, the counter shows one hood and `× remaining`. */
export const COMPACT_AT = 5;

export type Counter = { kind: 'hoods'; hoods: ('done' | 'current' | 'waiting')[] } | { kind: 'compact'; remaining: number };

/**
 * The parent counter. `cleared` is how many parents this battle has put away (killed or already
 * gone), `queued` how many are still in the queue, the first of them being fought.
 */
export function counter(cleared: number, queued: number): Counter {
  if (cleared + queued >= COMPACT_AT) {
    return { kind: 'compact', remaining: queued };
  }
  const hoods: ('done' | 'current' | 'waiting')[] = [];
  for (let i = 0; i < cleared; i++) hoods.push('done');
  for (let i = 0; i < queued; i++) hoods.push(i === 0 ? 'current' : 'waiting');
  return { kind: 'hoods', hoods };
}
