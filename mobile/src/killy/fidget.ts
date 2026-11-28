import { useEffect } from 'react';
import { cancelAnimation, Easing, ReduceMotion, type SharedValue, useSharedValue, withTiming } from 'react-native-reanimated';

import { FIDGET_TOTAL, REST } from './strike';

/**
 * The clock of Killy's fidget, in ms since it began, or REST when Killy stands still. A new `id`
 * (the battle's count of fidgets) starts it again from the beginning. With reduce motion Killy does
 * not fidget; only the bubble shows (DESIGN: Motion).
 */
export function useFidget(id: number, reduceMotion: boolean): SharedValue<number> {
  const elapsed = useSharedValue(REST);
  useEffect(() => {
    cancelAnimation(elapsed);
    if (id === 0 || reduceMotion) {
      elapsed.set(REST);
      return;
    }
    elapsed.set(0);
    elapsed.set(withTiming(FIDGET_TOTAL, { duration: FIDGET_TOTAL, easing: Easing.linear, reduceMotion: ReduceMotion.Never }, (done) => {
      'worklet';
      if (done) {
        elapsed.set(REST);
      }
    }));
    return () => cancelAnimation(elapsed);
  }, [id, reduceMotion, elapsed]);
  return elapsed;
}
