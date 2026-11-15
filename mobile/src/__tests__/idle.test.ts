import { anchors, hand, head } from '../art/points';
import { necromancerAt, ZOMBIE_HEIGHT, ZOMBIE_WIDTH, zombieBox } from '../killy/arena';
import {
  anchorsAt,
  CLOCK_WRAP,
  ease,
  easeInOut,
  killyBlink,
  killyBob,
  necroFloat,
  necroGlow,
  necroWork,
  periods,
  puppet,
  stringsAt,
  zombiePointAt,
} from '../killy/idle';

// The times are given by the tests; DESIGN: Idle loops is the source of the periods and swings.
const arena = { width: 358, height: 600 };

/** The least and greatest value of f over one period, sampled every ms. */
function range(f: (t: number) => number, period: number): [number, number] {
  let lo = Infinity;
  let hi = -Infinity;
  for (let t = 0; t < period; t++) {
    lo = Math.min(lo, f(t));
    hi = Math.max(hi, f(t));
  }
  return [lo, hi];
}

describe('timing functions', () => {
  test('ease-in-out and ease are the CSS curves', () => {
    expect(easeInOut(0)).toBeCloseTo(0);
    expect(easeInOut(0.5)).toBeCloseTo(0.5);
    expect(easeInOut(1)).toBeCloseTo(1);
    // Values of the CSS curves, worked out separately (Newton's method on x(t)).
    expect(easeInOut(0.25)).toBeCloseTo(0.1292, 3);
    expect(ease(0.25)).toBeCloseTo(0.4085, 3);
    expect(ease(0.5)).toBeCloseTo(0.8024, 3);
  });
});

describe('idle loops (DESIGN: Idle loops)', () => {
  test.each([
    ['Killy bobs 5px', killyBob, 2800, -5, 0],
    ['the necromancer floats 8px', necroFloat, 3200, -8, 0],
    ['the control bar rocks ±4°', necroWork, 1600, -4, 4],
    ['the eyes pulse down to 45%', necroGlow, 2400, 0.45, 1],
    ['Killy blinks, the eyes squashed to 10%', killyBlink, 4500, 0.1, 1],
  ] as const)('%s, over %i ms', (_, f, period, lo, hi) => {
    const [min, max] = range(f, period);
    expect(min).toBeCloseTo(lo, 2);
    expect(max).toBeCloseTo(hi, 2);
    for (const t of [0, 123, 777, 1999]) {
      expect(f(t + period)).toBeCloseTo(f(t));
    }
  });

  test('each loop rests at 0 ms, which is where the clock stays when motion is reduced', () => {
    expect([killyBob(0), necroFloat(0), necroWork(0), necroGlow(0), killyBlink(0)]).toEqual([0, 0, 0, 1, 1]);
    for (let i = 0; i < 6; i++) {
      expect(puppet(0, i).y).toBeCloseTo(0);
      expect(puppet(0, i).rotate).toBeCloseTo(0);
    }
  });

  test('the peaks are where the mock puts them', () => {
    expect(killyBob(1400)).toBeCloseTo(-5);
    expect(necroFloat(1600)).toBeCloseTo(-8);
    expect(necroWork(400)).toBeCloseTo(-4);
    expect(necroWork(1200)).toBeCloseTo(4);
    expect(necroGlow(1200)).toBeCloseTo(0.45);
    expect(killyBlink(4500 * 0.97)).toBeCloseTo(0.1);
    expect(killyBlink(4000)).toBe(1);
  });

  test('a zombie jerks up 5px tilted −4°, then up 3px tilted 3°, every 1.6 s', () => {
    expect(puppet(400, 0)).toEqual({ y: expect.closeTo(-5), rotate: expect.closeTo(-4) });
    expect(puppet(800, 0)).toEqual({ y: expect.closeTo(0), rotate: expect.closeTo(0) });
    expect(puppet(1200, 0)).toEqual({ y: expect.closeTo(-3), rotate: expect.closeTo(3) });
    expect(puppet(400 + periods.puppet, 0)).toEqual(puppet(400, 0));
  });

  test('alternate zombies are half a cycle apart', () => {
    for (const t of [0, 250, 400, 1100]) {
      expect(puppet(t, 1)).toEqual(puppet(t + 800, 0));
      expect(puppet(t, 2)).toEqual(puppet(t, 0));
      expect(puppet(t, 5)).toEqual(puppet(t, 1));
    }
  });

  test('the clock wraps where every loop is back at its start', () => {
    for (const f of [killyBob, killyBlink, necroFloat, necroWork, necroGlow]) {
      expect(f(CLOCK_WRAP)).toBeCloseTo(f(0));
    }
    expect(puppet(CLOCK_WRAP, 1)).toEqual({ y: expect.closeTo(puppet(0, 1).y), rotate: expect.closeTo(puppet(0, 1).rotate) });
  });
});

describe('the moving figures', () => {
  test('at rest a zombie point is its place in the slot, scaled towards the feet', () => {
    const box = zombieBox(3, arena);
    const k = ZOMBIE_WIDTH / 56;
    const feet = zombiePointAt(0, 3, arena, { x: 28, y: 80 });
    expect(feet.x).toBeCloseTo(box.left + ZOMBIE_WIDTH / 2);
    expect(feet.y).toBeCloseTo(box.top + ZOMBIE_HEIGHT);
    const h = zombiePointAt(0, 3, arena, head);
    expect(h.x).toBeCloseTo(box.left + ZOMBIE_WIDTH / 2 + (head.x * k - ZOMBIE_WIDTH / 2) * 0.84);
    expect(h.y).toBeCloseTo(box.top + ZOMBIE_HEIGHT - (ZOMBIE_HEIGHT - head.y * k) * 0.84);
  });

  test('a jerking zombie turns around its feet and lifts by the jerk times its depth scale', () => {
    const t = 400; // slot 3 is half a cycle ahead: up 3px, tilted 3°
    const { y, rotate } = puppet(t, 3);
    const feet = { x: zombieBox(3, arena).left + ZOMBIE_WIDTH / 2, y: zombieBox(3, arena).top + ZOMBIE_HEIGHT };
    const rest = zombiePointAt(0, 3, arena, head);
    const now = zombiePointAt(t, 3, arena, head);
    // Distance from the feet: kept by the turn; the lift adds y·s straight up before the turn is seen.
    const lifted = { x: now.x, y: now.y - y * 0.84 };
    expect(Math.hypot(lifted.x - feet.x, lifted.y - feet.y)).toBeCloseTo(Math.hypot(rest.x - feet.x, rest.y - feet.y));
    const angle = (p: { x: number; y: number }) => (Math.atan2(p.y - feet.y, p.x - feet.x) * 180) / Math.PI;
    expect(angle(lifted) - angle(rest)).toBeCloseTo(rotate);
  });

  test('the anchors float and rock with the necromancer, around 50% across and 20% down', () => {
    const at = necromancerAt(arena);
    expect(anchorsAt(0, arena)).toEqual(anchors.map((a) => ({ x: at.x + a.x, y: at.y + a.y })));
    const t = 400; // rocked to −4°
    const turn = (-4 * Math.PI) / 180;
    const o = { x: 75, y: 30 };
    const lift = necroFloat(t);
    anchorsAt(t, arena).forEach((p, i) => {
      const a = anchors[i];
      expect(p.x).toBeCloseTo(at.x + o.x + (a.x - o.x) * Math.cos(turn) - (a.y - o.y) * Math.sin(turn));
      expect(p.y).toBeCloseTo(at.y + lift + o.y + (a.x - o.x) * Math.sin(turn) + (a.y - o.y) * Math.cos(turn));
    });
  });
});

describe('strings', () => {
  test.each([0, 400, 1234, 2650])('at %i ms each zombie has two, from moving anchors to its moving head and front hand', (t) => {
    const from = anchorsAt(t, arena);
    const lines = stringsAt(t, 6, arena);
    expect(lines).toHaveLength(12);
    for (let i = 0; i < 6; i++) {
      const [toHead, toHand] = [lines[2 * i], lines[2 * i + 1]];
      expect({ x: toHead.x2, y: toHead.y2 }).toEqual(zombiePointAt(t, i, arena, head));
      expect({ x: toHand.x2, y: toHand.y2 }).toEqual(zombiePointAt(t, i, arena, hand));
      expect(from).toContainEqual({ x: toHead.x1, y: toHead.y1 });
      expect(from).toContainEqual({ x: toHand.x1, y: toHand.y1 });
      // The head takes the anchor nearest it across; the hand the nearest of the others.
      const nearest = Math.min(...from.map((a) => Math.abs(a.x - toHead.x2)));
      expect(Math.abs(toHead.x1 - toHead.x2)).toBeCloseTo(nearest);
      expect({ x: toHand.x1, y: toHand.y1 }).not.toEqual({ x: toHead.x1, y: toHead.y1 });
      const others = from.filter((a) => a.x !== toHead.x1 || a.y !== toHead.y1);
      expect(Math.abs(toHand.x1 - toHand.x2)).toBeCloseTo(Math.min(...others.map((a) => Math.abs(a.x - toHand.x2))));
    }
  });

  test('the strings move: their ends at 400 ms are not where they were at rest', () => {
    const rest = stringsAt(0, 3, arena);
    const now = stringsAt(400, 3, arena);
    rest.forEach((l, i) => {
      expect(now[i].x1 === l.x1 && now[i].y1 === l.y1).toBe(false);
      expect(now[i].x2 === l.x2 && now[i].y2 === l.y2).toBe(false);
    });
  });

  test('only the six zombies drawn get strings', () => {
    expect(stringsAt(0, 9, arena)).toHaveLength(12);
  });
});
