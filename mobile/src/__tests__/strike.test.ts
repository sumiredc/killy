import { apply, BOW, ghostMatrix, ghostOrigin, rotateAbout } from '../killy/pose';
import {
  defeatRows,
  flashOpacity,
  killyDash,
  killyHop,
  necromancer,
  recoilEnd,
  scytheSwing,
  settleAt,
  sigkill,
  strikeEnd,
  stringsOpacity,
  zombie,
} from '../killy/strike';

// DESIGN: Kill sequence and Kill outcomes other than killed, with times from the end of the hold
// given by the tests. `at` is when the sequence goes on: 670 ms, or the result's arrival if later.

describe('the timeline', () => {
  test('the sequence goes on at 670 ms, or when the result arrives if that is later', () => {
    expect(settleAt(0)).toBe(670);
    expect(settleAt(669)).toBe(670);
    expect(settleAt(2400)).toBe(2400);
  });

  test.each([1, 2, 3, 6])('with %i zombies the rows add up as DESIGN\'s table: result at 3850 + 200n, next at 5150 + 200n', (n) => {
    const r = defeatRows(670, n);
    expect(r.sag).toBe(670 + 380 + 70 * n);
    expect(r.release).toBe(r.sag + 600);
    expect(r.gone).toBe(r.release + 1300 + 130 * n);
    expect(r.result).toBe(3850 + 200 * n);
    expect(r.next).toBe(5150 + 200 * n);
  });

  test('a late result moves every later row by as much', () => {
    expect(defeatRows(2000, 2).result - defeatRows(670, 2).result).toBe(1330);
  });

  test('a defeat ends when the battle moves on, 600 ms later for All clear; a recoil once the strings are back', () => {
    expect(strikeEnd('defeat', 670, 2, false)).toBe(5550);
    expect(strikeEnd('defeat', 670, 2, true)).toBe(6150);
    expect(recoilEnd(670)).toBe(670 + 300 + 250);
    expect(strikeEnd('recoil', 670, 2, false)).toBe(1220);
  });
});

describe('up to 670 ms, played at once', () => {
  test('Killy dashes 70px forward and back over 700 ms', () => {
    expect(killyDash(0)).toBe(0);
    expect(killyDash(0.45 * 700)).toBeCloseTo(70);
    expect(killyDash(699)).toBeCloseTo(0, 0);
    expect(killyDash(700)).toBe(0);
  });

  test('the scythe swings from the wound-up −42° to 75° and back to 0, then leaves the scythe to the hold', () => {
    expect(scytheSwing(0)).toBe(-42);
    expect(scytheSwing(0.45 * 700)).toBeCloseTo(75);
    expect(scytheSwing(699)).toBeCloseTo(0, 0);
    expect(scytheSwing(700)).toBeNull();
    expect(scytheSwing(-1)).toBeNull();
  });

  test('at 320 ms the arena flashes and "SIGKILL" floats up 34px as it fades over 1 s', () => {
    expect(flashOpacity(319)).toBe(0);
    expect(flashOpacity(320)).toBe(1);
    expect(flashOpacity(320 + 350)).toBe(0);
    expect(sigkill(319).opacity).toBe(0);
    expect(sigkill(320)).toEqual({ opacity: 1, y: -0 });
    expect(sigkill(1319).y).toBeCloseTo(-34, 0);
    expect(sigkill(1320).opacity).toBe(0);
  });

  test('the strings fade out from 60% over 250 ms at 320 ms', () => {
    expect(stringsOpacity(319, null, 670)).toBeCloseTo(0.6);
    expect(stringsOpacity(570, null, 670)).toBeCloseTo(0);
    expect(stringsOpacity(5000, 'defeat', 670)).toBeCloseTo(0);
  });

  test('the necromancer shakes at 320 ms', () => {
    // The mock's `shake`: −6px a quarter of the way through its 300 ms.
    expect(necromancer(320 + 75, null, 670, 1).x).toBeCloseTo(-6, 0);
    expect(necromancer(319, null, 670, 1).x).toBe(0);
  });
});

describe('until the result arrives', () => {
  // Quarter-way points of each 300 ms tremble from 670 ms, where the shake is at −6px.
  const trembles = (from: number, to: number) => {
    const times: number[] = [];
    for (let t = 670 + 75; t < to; t += 300) {
      if (t >= from) {
        times.push(t);
      }
    }
    return times;
  };

  test('the necromancer trembles from 670 ms for as long as the result is awaited', () => {
    for (const t of trembles(670, 9000)) {
      expect(necromancer(t, null, 670, 2).x).toBeCloseTo(-6, 0);
    }
  });

  test('a result that arrives at 2400 ms: the trembling goes on until then, and the defeat starts only then', () => {
    for (const t of trembles(670, 2400)) {
      expect(necromancer(t, 'defeat', 2400, 2).x).toBeCloseTo(-6, 0);
    }
    expect(necromancer(2400 + 75, 'defeat', 2400, 2).x).toBe(0);
    for (const t of [670, 1000, 2399]) {
      expect(zombie(t, 0, 2, 'defeat', 2400).alive).toBe(true);
    }
    expect(zombie(2400, 0, 2, 'defeat', 2400).alive).toBe(false);
  });

  test('no zombie falls while the result is awaited', () => {
    for (const t of [0, 670, 1500, 5000, 60_000]) {
      for (let i = 0; i < 3; i++) {
        const z = zombie(t, i, 3, null, 670);
        expect(z.alive).toBe(true);
        expect(z.arm).toBe(0);
        expect(z.body).toEqual({ x: 0, rotate: 0, scaleY: 1, opacity: 1 });
        expect(z.soul.opacity).toBe(0);
      }
    }
    expect(necromancer(60_000, null, 670, 3).opacity).toBe(1);
  });
});

describe('the defeat (killed, already gone)', () => {
  const n = 3;
  const r = defeatRows(670, n);

  test('each zombie is hit 70 ms after the one before: face-dead and arms down 65° over 200 ms', () => {
    expect(zombie(670, 0, n, 'defeat', 670).alive).toBe(false);
    expect(zombie(739, 1, n, 'defeat', 670).alive).toBe(true);
    expect(zombie(740, 1, n, 'defeat', 670).alive).toBe(false);
    expect(zombie(670 + 200, 0, n, 'defeat', 670).arm).toBeCloseTo(-65);
  });

  test('they sag back together, to 22° and squashed to 90%, over 500 ms', () => {
    expect(zombie(r.sag - 1, 0, n, 'defeat', 670).body.rotate).toBe(0);
    const down = zombie(r.sag + 500, 2, n, 'defeat', 670).body;
    expect(down.rotate).toBeCloseTo(22);
    expect(down.scaleY).toBeCloseTo(0.9);
    expect(down.x).toBeCloseTo(4);
  });

  test('each, 130 ms apart, fades over 700 ms while its soul rises about 130px over 1.3 s', () => {
    expect(zombie(r.release, 0, n, 'defeat', 670).body.opacity).toBe(1);
    expect(zombie(r.release + 700, 0, n, 'defeat', 670).body.opacity).toBeCloseTo(0);
    expect(zombie(r.release + 700, 1, n, 'defeat', 670).body.opacity).toBeGreaterThan(0);
    expect(zombie(r.release + 130 + 700, 1, n, 'defeat', 670).body.opacity).toBeCloseTo(0);
    expect(zombie(r.release + 1300 - 1, 0, n, 'defeat', 670).soul.y).toBeCloseTo(-130, -1);
    expect(zombie(r.release + 1300, 0, n, 'defeat', 670).soul.opacity).toBe(0);
  });

  test('the necromancer fades upward only after all zombies are gone, over 900 ms', () => {
    const last = r.release + 130 * (n - 1) + 700;
    expect(last).toBeLessThanOrEqual(r.gone);
    expect(necromancer(r.gone, 'defeat', 670, n).opacity).toBe(1);
    const gone = necromancer(r.gone + 900, 'defeat', 670, n);
    expect(gone.opacity).toBeCloseTo(0);
    expect(gone.y).toBeCloseTo(-30);
    expect(gone.scale).toBeCloseTo(0.7);
  });

  test('Killy hops twice, 14px up, 450 ms each, from when the result shows', () => {
    expect(killyHop(r.result - 1, r.result)).toBeNull();
    expect(killyHop(r.result + 225, r.result)).toBeCloseTo(-14);
    expect(killyHop(r.result + 450 + 225, r.result)).toBeCloseTo(-14);
    expect(killyHop(r.result + 900, r.result)).toBeNull();
  });
});

describe('the recoil (refused, failed, unknown)', () => {
  test('the necromancer shakes once more at the result and stays', () => {
    expect(necromancer(670 + 75, 'recoil', 670, 2).x).toBeCloseTo(-6, 0);
    for (const t of [670, 1000, 5000, 60_000]) {
      const p = necromancer(t, 'recoil', 670, 2);
      expect(p.opacity).toBe(1);
      expect(p.y).toBe(0);
      expect(p.scale).toBe(1);
    }
    expect(necromancer(1300, 'recoil', 670, 2).x).toBe(0);
  });

  test('no zombie falls', () => {
    for (const t of [670, 2000, 6000]) {
      expect(zombie(t, 0, 2, 'recoil', 670)).toEqual(zombie(t, 0, 2, null, 670));
    }
  });

  test('the strings come back to 60% once the necromancer has recoiled', () => {
    expect(stringsOpacity(670 + 299, 'recoil', 670)).toBeCloseTo(0);
    expect(stringsOpacity(670 + 300 + 250, 'recoil', 670)).toBeCloseTo(0.6);
    expect(stringsOpacity(60_000, 'recoil', 670)).toBeCloseTo(0.6);
  });
});

test('Killy bows by turning 14° forward and sinking 4px (the mock\'s `.killy.bow .ghost`)', () => {
  expect(BOW).toEqual({ rotate: 14, y: 4 });
  const p = { x: 100, y: 40 };
  expect(apply(ghostMatrix(0, 1), p)).toEqual(apply(rotateAbout(14, ghostOrigin, 0, 4), p));
  // Turned clockwise about the ghost's origin, so its head goes forward (to the right) and down.
  const head = apply(ghostMatrix(0, 1), { x: 80, y: 16 });
  expect(head.x).toBeGreaterThan(80);
  expect(apply(ghostMatrix(0, 0), p)).toEqual(p);
});
