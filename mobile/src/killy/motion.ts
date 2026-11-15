import { useEffect, useState } from 'react';
import { AccessibilityInfo } from 'react-native';
import { cancelAnimation, Easing, ReduceMotion, type SharedValue, useSharedValue, withRepeat, withTiming } from 'react-native-reanimated';

import { CLOCK_WRAP } from './idle';

/**
 * Whether the OS asks for less motion: iOS "Reduce Motion", Android "Remove animations". The
 * battle's loops, kill sequence and overlays all ask this (DESIGN: Motion). It is null until the
 * OS has answered, so nothing starts moving before the answer is known, and it follows changes
 * made while the app is open.
 */
export function useReduceMotion(): boolean | null {
  const [reduce, setReduce] = useState<boolean | null>(null);
  useEffect(() => {
    let live = true;
    const sub = AccessibilityInfo.addEventListener('reduceMotionChanged', (on) => setReduce(on));
    AccessibilityInfo.isReduceMotionEnabled().then(
      (on) => live && setReduce((now) => now ?? on),
      () => live && setReduce((now) => now ?? false),
    );
    return () => {
      live = false;
      sub.remove();
    };
  }, []);
  return reduce;
}

/**
 * The clock of the idle loops, in ms. It runs only while `running`; otherwise it rests at 0, where
 * every loop is in its resting pose. Whether to run is decided here, from useReduceMotion, so
 * Reanimated's own reduce-motion handling is turned off for it.
 */
export function useIdleClock(running: boolean): SharedValue<number> {
  const clock = useSharedValue(0);
  useEffect(() => {
    if (!running) {
      cancelAnimation(clock);
      clock.value = 0;
      return;
    }
    clock.value = 0;
    clock.value = withRepeat(withTiming(CLOCK_WRAP, { duration: CLOCK_WRAP, easing: Easing.linear, reduceMotion: ReduceMotion.Never }), -1, false);
    return () => cancelAnimation(clock);
  }, [running, clock]);
  return clock;
}

/** The idle clock of the battle scene: it runs unless the OS asks for less motion, and not before it has said. */
export function useIdleMotion(): SharedValue<number> {
  return useIdleClock(useReduceMotion() === false);
}
