import { anchors } from '../art/Necromancer';
import { hand, head } from '../art/Zombie';
import {
  anchorPoints,
  counter,
  moonAt,
  necromancerAt,
  onGround,
  slots,
  strings,
  ZOMBIE_HEIGHT,
  ZOMBIE_WIDTH,
  zombieBox,
  zombiePoint,
} from '../killy/arena';

// The reference arena on a 390 × 844 screen: 16px gutters, between the safe area and the kill button.
const arena = { width: 358, height: 600 };

describe('placement (DESIGN: Battle tab)', () => {
  test('the necromancer is 150px wide, centred at 63% of the width with its top at 27% of the height', () => {
    const at = necromancerAt(arena);
    expect(at.x + 150 / 2).toBeCloseTo(arena.width * 0.63);
    expect(at.y).toBeCloseTo(arena.height * 0.27);
  });

  test('the 150px moon is centred on the same vertical line, behind the necromancer', () => {
    const m = moonAt(arena);
    expect(m.x + 150 / 2).toBeCloseTo(necromancerAt(arena).x + 150 / 2);
    expect(m.y).toBeCloseTo(arena.height * 0.22);
  });

  test.each([
    [0, 47, 40, 1],
    [1, 63, 40, 1],
    [2, 79, 40, 1],
    [3, 55, 96, 0.84],
    [4, 71, 96, 0.84],
    [5, 87, 96, 0.84],
  ])('zombie slot %i stands at left %i%%, bottom %ipx, scale %f', (i, left, bottom, scale) => {
    const box = zombieBox(i, arena);
    expect(box.left).toBeCloseTo((arena.width * left) / 100);
    expect(arena.height - (box.top + ZOMBIE_HEIGHT)).toBeCloseTo(bottom);
    expect(box.scale).toBe(scale);
  });

  test('a scaled zombie keeps its feet on its slot and shrinks towards them', () => {
    const feet = zombiePoint(3, arena, { x: 28, y: 80 });
    expect(feet.x).toBeCloseTo(zombieBox(3, arena).left + ZOMBIE_WIDTH / 2);
    expect(arena.height - feet.y).toBeCloseTo(96);
    const top = zombiePoint(3, arena, { x: 28, y: 0 });
    expect(feet.y - top.y).toBeCloseTo(ZOMBIE_HEIGHT * 0.84);
  });
});

describe('strings', () => {
  test('each zombie on the ground has two: one to its head and one to its front hand', () => {
    const lines = strings(4, arena);
    expect(lines).toHaveLength(8);
    for (let i = 0; i < 4; i++) {
      expect({ x: lines[2 * i].x2, y: lines[2 * i].y2 }).toEqual(zombiePoint(i, arena, head));
      expect({ x: lines[2 * i + 1].x2, y: lines[2 * i + 1].y2 }).toEqual(zombiePoint(i, arena, hand));
    }
  });

  test('a string starts at the anchor nearest its end, and the hand takes a different anchor from the head', () => {
    const from = anchorPoints(arena);
    const lines = strings(6, arena);
    for (let i = 0; i < lines.length; i += 2) {
      const [toHead, toHand] = [lines[i], lines[i + 1]];
      const nearestHead = Math.min(...from.map((a) => Math.abs(a.x - toHead.x2)));
      expect(Math.abs(toHead.x1 - toHead.x2)).toBeCloseTo(nearestHead);
      expect(from).toContainEqual({ x: toHead.x1, y: toHead.y1 });
      expect(from).toContainEqual({ x: toHand.x1, y: toHand.y1 });
      expect({ x: toHand.x1, y: toHand.y1 }).not.toEqual({ x: toHead.x1, y: toHead.y1 });
    }
  });

  test('the anchors are the five control-bar points of the necromancer', () => {
    const at = necromancerAt(arena);
    expect(anchorPoints(arena)).toEqual(anchors.map((a) => ({ x: at.x + a.x, y: at.y + a.y })));
  });

  test('no more than the six zombies drawn get strings', () => {
    expect(strings(9, arena)).toHaveLength(12);
  });
});

describe('zombies on the ground', () => {
  test.each([
    [1, 1, 0],
    [6, 6, 0],
    [7, 6, 1],
    [8, 6, 2],
    [13, 6, 7],
  ])('%i zombies: %i drawn and +%i', (n, drawn, more) => {
    const pids = Array.from({ length: n }, (_, i) => 100 + i);
    const g = onGround(pids);
    expect(g.shown).toEqual(pids.slice(0, drawn));
    expect(g.more).toBe(more);
    expect(slots).toHaveLength(6);
  });
});

describe('parent counter', () => {
  test('below five parents in the battle, one hood each: cleared ones first, then the current, then the waiting', () => {
    expect(counter(0, 1)).toEqual({ kind: 'hoods', hoods: ['current'] });
    expect(counter(1, 3)).toEqual({ kind: 'hoods', hoods: ['done', 'current', 'waiting', 'waiting'] });
  });

  test('from five parents in the battle, cleared ones included, one hood and the number still in the queue', () => {
    expect(counter(0, 5)).toEqual({ kind: 'compact', remaining: 5 });
    expect(counter(2, 3)).toEqual({ kind: 'compact', remaining: 3 });
  });
});
