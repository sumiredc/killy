import { useEffect } from 'react';

import { act, fireEvent, render, screen } from '@testing-library/react-native';
import { getAnimatedStyle, type SharedValue } from 'react-native-reanimated';

import { KillButton } from '../components/KillButton';
import { holdFor, leanFor, useHoldToKill } from '../killy/hold';
import { apply, ghostMatrix, ghostOrigin, rotateAbout, scytheMatrix, scytheOrigin, WIND_UP } from '../killy/pose';

// DESIGN: Hold to kill. The tests render the button with the testing library directly, so
// Reanimated runs (expo-router's renderRouter replaces it with a mock that does not move).

type Probe = { charge: SharedValue<number>; windUp: SharedValue<number>; lean: SharedValue<number> };

function Harness({ target, onFire, reduceMotion = false, onValues }: { target?: string; onFire: (t: string) => void; reduceMotion?: boolean; onValues: (p: Probe) => void }) {
  const press = useHoldToKill(target, onFire, reduceMotion);
  useEffect(() => onValues({ charge: press.charge, windUp: press.windUp, lean: press.lean }));
  return <KillButton label="Hold to kill" enabled={target !== undefined} charge={press.charge} onPressIn={press.onPressIn} onPressOut={press.onPressOut} />;
}

// Under jest it reads the style Reanimated last gave the component; its published type is the web stub's.
const animatedStyle = getAnimatedStyle as unknown as (component: unknown) => { transform?: Record<string, number>[] };
const fillScale = () => animatedStyle(screen.getByTestId('kill-fill')).transform?.[0]?.scaleX ?? 0;
const ms = (n: number) => act(() => jest.advanceTimersByTimeAsync(n));
const button = () => screen.getByRole('button', { name: 'Hold to kill' });

beforeEach(() => jest.useFakeTimers());
afterEach(() => jest.useRealTimers());

async function setUp(opts: { reduceMotion?: boolean; target?: string } = {}) {
  const fired: string[] = [];
  const probe: Probe[] = [];
  await render(
    <Harness
      target={'target' in opts ? opts.target : 'parent-1'}
      onFire={(t) => fired.push(t)}
      reduceMotion={opts.reduceMotion}
      onValues={(p) => void (probe[0] = p)}
    />,
  );
  return { fired, probe };
}

describe('the button fills while held', () => {
  test('in proportion to the time held, full at 1000 ms', async () => {
    expect(holdFor).toBe(1000);
    await setUp();
    await fireEvent(button(), 'pressIn');
    for (const t of [250, 500, 750]) {
      await ms(250);
      // The animation starts on the next frame, so within a frame (16 ms) of t.
      expect(fillScale()).toBeCloseTo(t / holdFor, 1);
    }
  });

  test('letting go before 1000 ms empties it and sends nothing', async () => {
    const { fired, probe } = await setUp();
    await fireEvent(button(), 'pressIn');
    await ms(600);
    expect(fillScale()).toBeGreaterThan(0.5);
    await fireEvent(button(), 'pressOut');
    await ms(50);
    expect(fillScale()).toBe(0);
    expect(probe[0].charge.value).toBe(0);
    await ms(1000);
    expect(fired).toEqual([]);
  });

  test('the kill starts when it is full: not at 990 ms, at 1000 ms', async () => {
    const { fired, probe } = await setUp();
    await fireEvent(button(), 'pressIn');
    await ms(990);
    expect(fired).toEqual([]);
    expect(probe[0].charge.value).toBeLessThan(1);
    expect(probe[0].charge.value).toBeGreaterThan(0.95);
    await ms(10);
    expect(fired).toEqual(['parent-1']);
  });

  test('with reduce motion it fills a quarter every 250 ms, full exactly when the kill starts', async () => {
    const { fired, probe } = await setUp({ reduceMotion: true });
    await fireEvent(button(), 'pressIn');
    // Just before each quarter, then the last moment before the kill.
    for (const [t, want] of [
      [240, 0],
      [490, 0.25],
      [740, 0.5],
      [999, 0.75],
    ] as const) {
      await ms(t - (t === 240 ? 0 : t === 490 ? 240 : t === 740 ? 490 : 740));
      expect(probe[0].charge.value).toBeCloseTo(want, 2);
      expect(fired).toEqual([]);
    }
    await ms(1);
    expect(fired).toEqual(['parent-1']);
  });

  test('with reduce motion the scythe does not wind up', async () => {
    const { probe } = await setUp({ reduceMotion: true });
    await fireEvent(button(), 'pressIn');
    await ms(800);
    expect(probe[0].charge.value).toBeGreaterThan(0);
    expect(probe[0].windUp.value).toBe(0);
  });

  test('the scythe winds up with the fill when motion is allowed', async () => {
    const { probe } = await setUp();
    await fireEvent(button(), 'pressIn');
    await ms(500);
    expect(probe[0].windUp.value).toBeCloseTo(probe[0].charge.value, 1);
    expect(probe[0].windUp.value).toBeGreaterThan(0.4);
  });

  test('when the target goes during a hold (another parent came up, or the button can no longer be pressed), the hold ends', async () => {
    const fired: string[] = [];
    const probe: Probe[] = [];
    const harness = (target: string | undefined) => (
      <Harness target={target} onFire={(t) => fired.push(t)} onValues={(p) => void (probe[0] = p)} />
    );
    await render(harness('parent-1'));
    await fireEvent(button(), 'pressIn');
    await ms(500);
    expect(probe[0].charge.value).toBeGreaterThan(0.4);
    await screen.rerender(harness(undefined));
    await ms(20);
    expect(probe[0].charge.value).toBe(0);
    expect(probe[0].windUp.value).toBe(0);
    await ms(1000);
    expect(fired).toEqual([]);

    await screen.rerender(harness('parent-1'));
    await fireEvent(button(), 'pressIn');
    await ms(500);
    await screen.rerender(harness('parent-2'));
    await ms(1000);
    expect(probe[0].charge.value).toBe(0);
    expect(fired).toEqual([]);
  });
});

describe('Killy winds up', () => {
  test('the scythe turns in proportion to the charge around Killy hand, to −42° when full', () => {
    expect(WIND_UP).toBe(-42);
    for (const c of [0, 0.25, 0.5, 1]) {
      expect(scytheMatrix(c)).toEqual(rotateAbout(-42 * c, scytheOrigin));
    }
    // The hand does not move; the blade tip (199, 46) swings back and up.
    expect(apply(scytheMatrix(1), scytheOrigin).x).toBeCloseTo(scytheOrigin.x);
    expect(apply(scytheMatrix(1), scytheOrigin).y).toBeCloseTo(scytheOrigin.y);
    const tip = apply(scytheMatrix(1), { x: 199, y: 46 });
    expect(tip.y).toBeLessThan(46);
    const angle = (p: { x: number; y: number }) => (Math.atan2(p.y - scytheOrigin.y, p.x - scytheOrigin.x) * 180) / Math.PI;
    expect(angle(tip) - angle({ x: 199, y: 46 })).toBeCloseTo(-42);
  });

  test('the ghost leans back 6° and 4px around its base while a hold is on', () => {
    ghostMatrix(0).forEach((v, i) => expect(v).toBeCloseTo([1, 0, 0, 1, 0, 0][i]));
    const at = apply(ghostMatrix(1), ghostOrigin);
    expect(at.x).toBeCloseTo(ghostOrigin.x - 4 * Math.cos((6 * Math.PI) / 180));
    expect(at.y).toBeCloseTo(ghostOrigin.y + 4 * Math.sin((6 * Math.PI) / 180));
  });

  test('the lean comes in over 250 ms when a hold starts and goes when it ends', async () => {
    expect(leanFor).toBe(250);
    const { probe } = await setUp();
    await fireEvent(button(), 'pressIn');
    await ms(125);
    expect(probe[0].lean.value).toBeGreaterThan(0.2);
    expect(probe[0].lean.value).toBeLessThan(0.8);
    await ms(175);
    expect(probe[0].lean.value).toBeCloseTo(1);
    await fireEvent(button(), 'pressOut');
    await ms(300);
    expect(probe[0].lean.value).toBeCloseTo(0);
  });

  test('with reduce motion Killy does not lean', async () => {
    const { probe } = await setUp({ reduceMotion: true });
    await fireEvent(button(), 'pressIn');
    await ms(300);
    expect(probe[0].lean.value).toBe(0);
  });
});
