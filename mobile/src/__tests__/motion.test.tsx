import { act, render, screen } from '@testing-library/react-native';
import { AccessibilityInfo } from 'react-native';
import { getAnimatedStyle } from 'react-native-reanimated';

import { Enemies, KillyOnStage } from '../components/Arena';
import { killyBob, necroFloat, necroWork, puppet } from '../killy/idle';
import { useIdleMotion } from '../killy/motion';

// The battle scene's idle loops, driven by jest's fake timers. These tests render the scene with
// the testing library directly: expo-router's renderRouter replaces Reanimated with its mock,
// whose animated styles never move.

const arena = { width: 358, height: 600 };

function Scene({ zombies = [3802, 3803, 3804] }: { zombies?: number[] }) {
  const clock = useIdleMotion();
  return (
    <>
      <Enemies size={arena} zombies={zombies} clock={clock} />
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
  await ms(2800);
  expect(translateY('killy')).toBeCloseTo(killyBob(4400), 0);
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
