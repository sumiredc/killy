import { useEffect } from 'react';

import { act, fireEvent, render, screen } from '@testing-library/react-native';
import { AccessibilityInfo, Animated, StyleSheet, View } from 'react-native';
import { getAnimatedStyle } from 'react-native-reanimated';

import { Enemies, KillyOnStage, StrikeEffects } from '../components/Arena';
import { BottomSheet } from '../components/BottomSheet';
import { ServerChip } from '../components/ServerChip';
import { useFidget } from '../killy/fidget';
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

// DESIGN: Killy's reactions and Motion. A new `id` starts the fidget; with reduce motion Killy
// stands still and the bubble comes at once.
describe("Killy's fidget and bubble", () => {
  const symbol = '……！' as const;
  function Reacting({ id, bubble, reduce }: { id: number; bubble: { symbol: typeof symbol; id: number } | null; reduce: boolean }) {
    const clock = useIdleClock(false);
    const fidget = useFidget(id, reduce);
    return <KillyOnStage clock={clock} bubble={bubble} fidget={fidget} instant={reduce} />;
  }
  const dashX = () => Number(transform('killy-dash').find((x) => 'translateX' in x)?.translateX ?? 0);
  const bubbleStyle = () => animatedStyle(screen.getByTestId('killy-bubble', { includeHiddenElements: true })) as { opacity?: number; transform?: { scale?: number }[] };
  const scale = () => bubbleStyle().transform?.find((x) => 'scale' in x)?.scale ?? 0;

  test('Killy shuffles 6px left and right three times, then stands still', async () => {
    await render(<Reacting id={1} bubble={{ symbol, id: 1 }} reduce={false} />);
    await ms(112.5);
    expect(dashX()).toBeCloseTo(-6, 0);
    await ms(225);
    expect(dashX()).toBeCloseTo(6, 0);
    await ms(450 * 3);
    expect(dashX()).toBe(0);
  });

  test('a new fidget starts again from the beginning', async () => {
    await render(<Reacting id={1} bubble={null} reduce={false} />);
    await ms(1400);
    expect(dashX()).toBe(0);
    await screen.rerender(<Reacting id={2} bubble={null} reduce={false} />);
    await ms(113);
    expect(dashX()).toBeCloseTo(-6, 0);
  });

  test('the bubble pops in over 180 ms', async () => {
    await render(<Reacting id={0} bubble={null} reduce={false} />);
    await screen.rerender(<Reacting id={0} bubble={{ symbol, id: 1 }} reduce={false} />);
    expect(bubbleStyle().opacity ?? 0).toBeLessThan(0.5);
    await ms(200);
    expect(bubbleStyle().opacity).toBe(1);
    expect(scale()).toBeCloseTo(1, 1);
  });

  test('with reduce motion Killy does not fidget and the bubble is there at once, and goes at once', async () => {
    await render(<Reacting id={1} bubble={null} reduce />);
    await screen.rerender(<Reacting id={1} bubble={{ symbol, id: 1 }} reduce />);
    // One frame: there is no 180 ms pop to wait for.
    await ms(16);
    expect(bubbleStyle().opacity).toBe(1);
    expect(scale()).toBe(1);
    await ms(112);
    expect(dashX()).toBe(0);
    await screen.rerender(<Reacting id={1} bubble={null} reduce />);
    expect(screen.queryByTestId('killy-bubble', { includeHiddenElements: true })).toBeNull();
  });
});

// DESIGN: Other motion. With reduce motion these are instant switches.
describe('a new zombie and the server chip', () => {
  function Party({ zombies, dropIn }: { zombies: number[]; dropIn: boolean }) {
    const clock = useIdleClock(false);
    const motion = useStrikeMotion(null, false, false);
    return <Enemies size={arena} zombies={zombies} clock={clock} motion={motion} dropIn={dropIn} />;
  }
  const zombieY = (i: number) => Number(transform('zombie', i).find((x) => 'translateY' in x)?.translateY ?? 0);
  const zombieOpacity = (i: number) => (animatedStyle(screen.getAllByTestId('zombie')[i]) as { opacity?: number }).opacity;

  test('a zombie that comes after the first ones drops in from 60px above in 600 ms, the others staying put', async () => {
    await render(<Party zombies={[3802, 3803]} dropIn />);
    await ms(16);
    expect([zombieY(0), zombieOpacity(0)]).toEqual([0, 1]);
    await screen.rerender(<Party zombies={[3802, 3803, 3804]} dropIn />);
    await ms(16);
    // Scaled with the zombie: 60px above, less the first frames.
    expect(zombieY(2)).toBeLessThan(-30);
    expect(zombieOpacity(2) ?? 0).toBeLessThan(0.5);
    expect([zombieY(0), zombieOpacity(0)]).toEqual([0, 1]);
    await ms(700);
    expect(zombieY(2)).toBeCloseTo(0, 0);
    expect(zombieOpacity(2)).toBe(1);
  });

  test('the zombies a parent starts with are there at once', async () => {
    await render(<Party zombies={[3802, 3803, 3804]} dropIn />);
    await ms(16);
    for (const i of [0, 1, 2]) {
      expect([zombieY(i), zombieOpacity(i)]).toEqual([0, 1]);
    }
  });

  test('with reduce motion a new zombie is simply there', async () => {
    await render(<Party zombies={[3802]} dropIn={false} />);
    await screen.rerender(<Party zombies={[3802, 3803]} dropIn={false} />);
    await ms(16);
    expect([zombieY(1), zombieOpacity(1)]).toEqual([0, 1]);
  });

  const scale = () => Number(transform('server-chip').find((x) => 'scale' in x)?.scale ?? 1);

  test('the chip pops to 1.08 for 500 ms when the parent is from another server', async () => {
    await render(<ServerChip serverId="a" name="web-01" instant={false} />);
    await ms(16);
    expect(scale()).toBe(1);
    await screen.rerender(<ServerChip serverId="b" name="db-01" instant={false} />);
    await ms(200);
    expect(scale()).toBeCloseTo(1.08, 1);
    await ms(400);
    expect(scale()).toBe(1);
  });

  test('the chip does not pop for the same server, even renamed', async () => {
    await render(<ServerChip serverId="a" name="web-01" instant={false} />);
    await screen.rerender(<ServerChip serverId="a" name="web-02" instant={false} />);
    await ms(200);
    expect(scale()).toBe(1);
  });

  test('with reduce motion the chip changes at once and does not pop', async () => {
    await render(<ServerChip serverId="a" name="web-01" instant />);
    await screen.rerender(<ServerChip serverId="b" name="db-01" instant />);
    await ms(200);
    expect(scale()).toBe(1);
    expect(screen.getByTestId('battle-server')).toHaveTextContent('db-01');
  });
});

// DESIGN: Other motion. The sheet rises over 300 ms, the backdrop fades over 200 ms; with reduce motion they are there at once.
// BottomSheet uses React Native's Animated, which starts once the Modal says it is shown and the sheet has been laid out.
describe('a sheet coming in', () => {
  const style = (testID: string) => StyleSheet.flatten(screen.getByTestId(testID).props.style) as { opacity?: number; transform?: { translateY?: number }[] };
  const lift = () => Number(style('sheet-rise').transform?.find((x) => 'translateY' in x)?.translateY ?? 0);
  const backdrop = () => style('sheet-backdrop').opacity;
  const Sheet = () => (
    <BottomSheet onRequestClose={() => {}} modalTestID="sheet-modal">
      <View testID="sheet-content" />
    </BottomSheet>
  );
  // The sheet is moved by pixels, from its height once it has been laid out; it starts when the Modal is shown.
  const ready = async () => {
    await fireEvent(screen.getByTestId('sheet-rise'), 'layout', { nativeEvent: { layout: { height: 400, width: 358, x: 0, y: 0 } } });
    await fireEvent(screen.getByTestId('sheet-modal'), 'show');
  };

  // On the phone the motion runs on the native side, so what a test can see is what it is asked to do.
  const timings = () => jest.spyOn(Animated, 'timing');

  test('it rises over 300 ms with the mock\'s easing while the backdrop fades over 200 ms', async () => {
    const timing = timings();
    await render(<Sheet />);
    await ready();
    expect(timing).toHaveBeenCalledTimes(2);
    const [rise, fade] = timing.mock.calls.map(([, config]) => config);
    expect(rise).toEqual(expect.objectContaining({ toValue: 1, duration: 300, useNativeDriver: true }));
    expect(fade).toEqual(expect.objectContaining({ toValue: 1, duration: 200, useNativeDriver: true }));
    // Before it starts, the sheet is 40% of its height below its place and the backdrop is not there.
    expect(lift()).toBe(160);
    expect(backdrop()).toBe(0);
  });

  test('it waits for the Modal to be shown, and starts anyway after half a second', async () => {
    const timing = timings();
    await render(<Sheet />);
    await fireEvent(screen.getByTestId('sheet-rise'), 'layout', { nativeEvent: { layout: { height: 400, width: 358, x: 0, y: 0 } } });
    await ms(400);
    expect(timing).not.toHaveBeenCalled();
    await ms(200);
    expect(timing).toHaveBeenCalledTimes(2);
  });

  test('with reduce motion the sheet and the backdrop are there at once', async () => {
    reduceMotion(true);
    const timing = timings();
    await render(<Sheet />);
    await ready();
    await ms(32);
    expect(timing).not.toHaveBeenCalled();
    expect(lift()).toBe(0);
    expect(backdrop()).toBe(1);
  });
});
