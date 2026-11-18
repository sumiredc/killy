import { counter, moonAt, necromancerAt, onGround, slots, ZOMBIE_HEIGHT, zombieBox } from '../killy/arena';

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
    // Between a parent put away and the next coming in, none is being fought.
    expect(counter(1, 2, false)).toEqual({ kind: 'hoods', hoods: ['done', 'waiting', 'waiting'] });
  });

  test('from five parents in the battle, cleared ones included, one hood and the number still in the queue', () => {
    expect(counter(0, 5)).toEqual({ kind: 'compact', remaining: 5 });
    expect(counter(2, 3)).toEqual({ kind: 'compact', remaining: 3 });
  });
});
