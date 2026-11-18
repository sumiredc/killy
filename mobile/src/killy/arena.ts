// Where things stand in the battle arena (DESIGN: Battle tab), as pure functions of the arena's
// size, so the layout and the strings can be checked without drawing. Sizes are DESIGN's pixels;
// positions are fractions of the arena, as in the mock's CSS.


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
  'worklet';
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
  'worklet';
  const slot = slots[i];
  return {
    left: arena.width * slot.left,
    top: arena.height - slot.bottom - ZOMBIE_HEIGHT,
    scale: slot.s,
  };
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
 * gone), `queued` how many are still in the queue, the first of them being fought unless
 * `fighting` is false (a parent has just been put away and the next has not come in yet).
 */
export function counter(cleared: number, queued: number, fighting = true): Counter {
  if (cleared + queued >= COMPACT_AT) {
    return { kind: 'compact', remaining: queued };
  }
  const hoods: ('done' | 'current' | 'waiting')[] = [];
  for (let i = 0; i < cleared; i++) hoods.push('done');
  for (let i = 0; i < queued; i++) hoods.push(i === 0 && fighting ? 'current' : 'waiting');
  return { kind: 'hoods', hoods };
}
