import { useEffect } from 'react';

import { act, render, screen } from '@testing-library/react-native';
import { AccessibilityInfo } from 'react-native';
import { getAnimatedStyle } from 'react-native-reanimated';

import { Enemies, KillyOnStage, StrikeEffects } from '../components/Arena';
import type { Strike } from '../killy/battle-context';
import { killyBob, necroFloat, necroWork, puppet } from '../killy/idle';
import { useIdleClock, useIdleMotion } from '../killy/motion';
import { type StrikeMotion, useStrikeMotion } from '../killy/strike-motion';

// The battle scene's idle loops, driven by jest's fake timers. These tests render the scene with
// the testing library directly: expo-router's renderRouter replaces Reanimated with its mock,
// whose animated styles never move.

const arena = { width: 358, height: 600 };

function Scene({ zombies = [3802, 3803, 3804] }: { zombies?: number[] }) {
  const clock = useIdleMotion();
  const motion = useStrikeMotion(null, false, false);
  return (
    <>
      <Enemies size={arena} zombies={zombies} clock={clock} motion={motion} />
      <KillyOnStage clock={clock} />
    </>
  );
}

/** Makes the OS answer `on` for reduce motion, and returns how to change it later as the OS would. */
function reduceMotion(on: boolean) {
  let changed: ((on: boolean) => void) | undefined;
  jest.spyOn(AccessibilityInfo, 'isReduceMotionEnabled').mockResolvedValue(on);
  jest.spyOn(AccessibilityInfo, 'addEventListener').mockImplementation(((event: string, cb: (on: boolean) => void) => {
    if (event === 'reduceMotionChanged') {
      changed = cb;
    }
    return { remove: () => {} };
  }) as typeof AccessibilityInfo.addEventListener);
  return (next: boolean) => act(() => changed?.(next));
}

type Transform = Record<string, number | string>[];
// Under jest it reads the style Reanimated last gave the component; its published type is the web stub's.
const animatedStyle = getAnimatedStyle as unknown as (component: unknown) => { transform?: Transform };
const transform = (testID: string, i = 0) => animatedStyle(screen.getAllByTestId(testID)[i]).transform ?? [];
const translateY = (testID: string, i = 0) => Number(transform(testID, i).find((x) => 'translateY' in x)?.translateY ?? 0);
const rotate = (testID: string, i = 0) => parseFloat(String(transform(testID, i).find((x) => 'rotate' in x)?.rotate ?? '0'));
const ms = (n: number) => act(() => jest.advanceTimersByTimeAsync(n));

beforeEach(() => jest.useFakeTimers());
afterEach(() => {
  jest.useRealTimers();
  jest.restoreAllMocks();
});

test('the loops start when the OS allows motion, and move as DESIGN says from that moment', async () => {
  const set = reduceMotion(true);
  await render(<Scene />);
  await ms(500);
  await set(false);
  await ms(1400);
  // The clock starts on the next frame, so these are within a frame (16 ms) of 1400 ms.
  expect(translateY('killy')).toBeCloseTo(killyBob(1400), 0);
  expect(translateY('killy')).toBeLessThan(-4.9);
  expect(translateY('necromancer')).toBeCloseTo(necroFloat(1400), 0);
  expect(rotate('necromancer-work')).toBeCloseTo(necroWork(1400), 0);
  expect(translateY('zombie', 0)).toBeCloseTo(puppet(1400, 0).y, 0);
  expect(rotate('zombie', 1)).toBeCloseTo(puppet(1400, 1).rotate, 0);
  await ms(200);
  expect(translateY('necromancer')).toBeLessThan(-7.9);
});

test('every zombie keeps its depth scale while it jerks', async () => {
  reduceMotion(false);
  await render(<Scene />);
  await ms(400);
  expect(transform('zombie', 0)[0]).toEqual({ scale: 1 });
  expect(rotate('zombie', 0)).toBeLessThan(-3.5);
});

test('with reduce motion on, nothing loops', async () => {
  reduceMotion(true);
  await render(<Scene />);
  for (const step of [400, 1000, 2600]) {
    await ms(step);
    expect(translateY('killy')).toBe(0);
    expect(translateY('necromancer')).toBe(0);
    expect(rotate('necromancer-work')).toBe(0);
    for (let i = 0; i < 3; i++) {
      expect(translateY('zombie', i)).toBeCloseTo(0);
      expect(rotate('zombie', i)).toBeCloseTo(0);
    }
  }
});

test('nothing moves before the OS has said whether to reduce motion', async () => {
  jest.spyOn(AccessibilityInfo, 'isReduceMotionEnabled').mockReturnValue(new Promise(() => {}));
  await render(<Scene />);
  await ms(1400);
  expect(translateY('killy')).toBe(0);
  expect(rotate('necromancer-work')).toBe(0);
});

test('turning reduce motion on while the loops run puts everything back at rest and keeps it there', async () => {
  const set = reduceMotion(false);
  await render(<Scene />);
  await ms(1400);
  expect(translateY('killy')).toBeLessThan(-4);
  await set(true);
  await ms(700);
  expect(translateY('killy')).toBe(0);
  expect(translateY('necromancer')).toBe(0);
  expect(rotate('zombie', 0)).toBeCloseTo(0);
});

test('the answer to a change made before the first answer arrives is the change', async () => {
  let answer: (on: boolean) => void = () => {};
  let changed: ((on: boolean) => void) | undefined;
  jest.spyOn(AccessibilityInfo, 'isReduceMotionEnabled').mockReturnValue(new Promise((resolve) => (answer = resolve)));
  jest.spyOn(AccessibilityInfo, 'addEventListener').mockImplementation(((event: string, cb: (on: boolean) => void) => {
    if (event === 'reduceMotionChanged') {
      changed = cb;
    }
    return { remove: () => {} };
  }) as typeof AccessibilityInfo.addEventListener);
  await render(<Scene />);
  await act(() => changed?.(false));
  await act(async () => answer(true));
  await ms(1400);
  expect(translateY('killy')).toBeLessThan(-4);
});

test('if the OS cannot be asked, the loops run', async () => {
  jest.spyOn(AccessibilityInfo, 'isReduceMotionEnabled').mockRejectedValue(new Error('no answer'));
  await render(<Scene />);
  await ms(1400);
  expect(translateY('killy')).toBeLessThan(-4);
});

test('zombies added while the scene is moving get their strings without breaking it', async () => {
  reduceMotion(false);
  await render(<Scene zombies={[3802]} />);
  await ms(300);
  await screen.rerender(<Scene zombies={[3802, 3803, 3804, 3805]} />);
  await ms(300);
  expect(screen.getAllByTestId('zombie')).toHaveLength(4);
  expect(screen.getAllByTestId('string')).toHaveLength(8);
});

describe('the kill sequence in the arena', () => {
  // DESIGN: Kill sequence. The arena is drawn from the strike as the battle gives it; the clock
  // starts with `start`, as the battle screen calls it when the hold ends.
  const target = { server: { id: 'a', name: 'web-01', hostname: 'web-01', notify: true, disconnected: false, parents: [] }, parent: { pid: 3801, starttime: 1, cmd: 'x', zombies: [3802, 3803], first_detected_at: '' } };
  const pending: Strike = { target, n: 2, outcome: null, at: null, revealed: false, instant: false, cleared: 0 };
  const probe: { motion?: StrikeMotion } = {};

  function StrikeScene({
    strike,
    bowing = false,
    idle = false,
    entering = false,
  }: {
    strike: Strike | null;
    bowing?: boolean;
    idle?: boolean;
    entering?: boolean;
  }) {
    const clock = useIdleClock(idle);
    const m = useStrikeMotion(strike, bowing, false);
    useEffect(() => void (probe.motion = m));
    return (
      <>
        <Enemies size={arena} zombies={[3802, 3803]} clock={clock} motion={m} striking={strike !== null} entering={entering} />
        <KillyOnStage clock={clock} motion={m} />
        <StrikeEffects size={arena} motion={m} />
      </>
    );
  }

  const opacity = (testID: string, i = 0) => (animatedStyle(screen.getAllByTestId(testID, { includeHiddenElements: true })[i]) as { opacity?: number }).opacity;
  const translateX = (testID: string, i = 0) => Number(transform(testID, i).find((x) => 'translateX' in x)?.translateX ?? 0);

  async function started() {
    await render(<StrikeScene strike={pending} />);
    await act(() => probe.motion?.start(2));
  }

  test('Killy dashes and the arena flashes on the sequence clock', async () => {
    await started();
    await ms(0.45 * 700);
    expect(translateX('killy-dash')).toBeCloseTo(70, -1);
    await ms(320 - 315 + 16);
    expect(opacity('strike-flash')).toBeGreaterThan(0.8);
    await ms(400);
    expect(opacity('strike-flash')).toBe(0);
  });

  test('while the result is awaited the necromancer trembles and the zombies stand; the defeat plays once it is in', async () => {
    await started();
    await ms(670 + 75 + 300 * 3);
    expect(Math.abs(translateX('necromancer-work'))).toBeGreaterThan(3);
    expect(opacity('zombie-body', 0)).toBe(1);
    // In at 2000 ms: the zombies go at 2000 + (380 + 140) + 600 + 700 for the first.
    await screen.rerender(<StrikeScene strike={{ ...pending, outcome: 'defeat', at: 2000 }} />);
    await ms(2000 + 520 + 600 + 700 - 1645 + 16);
    expect(opacity('zombie-body', 0)).toBeLessThan(0.05);
    expect(opacity('zombie-soul', 0)).toBeGreaterThan(0);
  });

  test('the strings are cut at 320 ms and, after a recoil, come back once the necromancer has recoiled', async () => {
    await started();
    expect(opacity('strings-cut')).toBe(1);
    await ms(320 + 250 + 16);
    expect(opacity('strings-cut')).toBeCloseTo(0);
    await screen.rerender(<StrikeScene strike={{ ...pending, outcome: 'recoil', at: 670 }} />);
    await ms(1220 - 586 + 16);
    expect(opacity('strings-cut')).toBeCloseTo(1);
  });

  test('a knocked-out zombie stops its puppet jerk', async () => {
    reduceMotion(false);
    await render(<StrikeScene strike={{ ...pending, outcome: 'defeat', at: 670 }} idle />);
    await act(() => probe.motion?.start(2));
    await ms(400);
    expect(translateY('zombie', 0)).toBeLessThan(-1);
    // At 2000 ms the jerk would be at its highest (a quarter into its 1.6 s cycle).
    await ms(2000 - 400);
    expect(translateY('zombie', 0)).toBe(0);
    expect(rotate('zombie', 0)).toBe(0);
  });

  test('Killy hops twice from when the result shows, in place of its bob', async () => {
    await render(<StrikeScene strike={{ ...pending, outcome: 'defeat', at: 670 }} />);
    await act(() => probe.motion?.start(2));
    // n = 2: the result at 4250.
    await ms(4250 - 100);
    expect(translateY('killy')).toBe(0);
    await ms(100 + 225 + 16);
    expect(translateY('killy')).toBeLessThan(-12);
    await ms(450);
    expect(translateY('killy')).toBeLessThan(-12);
    await ms(450);
    expect(translateY('killy')).toBe(0);
  });

  test('once the kill is over, the arena is at rest whatever the sequence\'s clock still says', async () => {
    await started();
    await screen.rerender(<StrikeScene strike={{ ...pending, outcome: 'defeat', at: 670 }} />);
    await ms(5550);
    expect(opacity('zombie-body', 0)).toBe(0);
    // The battle has moved on: drawn without the strike, the zombies stand from the first frame.
    await screen.rerender(<StrikeScene strike={null} />);
    expect(opacity('zombie-body', 0)).toBe(1);
    expect(opacity('necromancer')).toBe(1);
    expect(opacity('strings-cut')).toBe(1);
  });

  test('after a recoil the zombies stay, and the necromancer too', async () => {
    await started();
    await screen.rerender(<StrikeScene strike={{ ...pending, outcome: 'recoil', at: 670 }} />);
    await ms(6000);
    expect(opacity('zombie-body', 0)).toBe(1);
    expect(opacity('necromancer')).toBe(1);
  });

  test('Killy bows over 250 ms while the battle says so, and straightens when it stops', async () => {
    await render(<StrikeScene strike={null} />);
    expect(probe.motion?.bow.value).toBe(0);
    await screen.rerender(<StrikeScene strike={null} bowing />);
    await ms(125);
    expect(probe.motion?.bow.value).toBeGreaterThan(0.2);
    expect(probe.motion?.bow.value).toBeLessThan(1);
    await ms(150);
    expect(probe.motion?.bow.value).toBe(1);
    await screen.rerender(<StrikeScene strike={null} />);
    expect(probe.motion?.bow.value).toBe(0);
  });

  test('a parent drawn as entering comes in from 40px above over 600 ms, hidden from its first frame', async () => {
    await render(<StrikeScene strike={null} entering />);
    expect(translateY('necromancer')).toBeCloseTo(-40);
    expect(opacity('necromancer')).toBe(0);
    await ms(16);
    expect(translateY('necromancer')).toBeLessThan(-30);
    expect(opacity('necromancer')).toBeLessThan(0.3);
    await ms(600);
    expect(translateY('necromancer')).toBeCloseTo(0);
    expect(opacity('necromancer')).toBe(1);
  });
});
