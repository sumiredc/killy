import { useEffect, useRef } from 'react';
import { cancelAnimation, Easing, ReduceMotion, type SharedValue, useSharedValue, withTiming } from 'react-native-reanimated';

/** How long the kill button must be held (DESIGN: Hold to kill). */
export const holdFor = 1000;

/** How long Killy takes to lean back when a hold starts, and to straighten when it ends (the mock's `.ghost` transition). */
export const leanFor = 250;

/**
 * Press handlers that call onFire with the target the press started on, once the press has lasted
 * `holdFor`. Letting go earlier does nothing. onFire decides whether that target is still the one
 * to act on.
 *
 * `charge` goes from 0 to 1 over the same `holdFor` from the same press, so the button is full
 * exactly when the kill starts; it drops back to 0 when the press ends either way. `lean` is 1
 * while a hold is on and 0 otherwise, easing over `leanFor`. With reduce motion, `charge` rises in
 * quarters instead of smoothly (the hold must still show) and Killy does not lean.
 */
export function useHoldToKill<T>(target: T | undefined, onFire: (target: T) => void, reduceMotion = false) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const latest = useRef({ target, onFire });
  const charge = useSharedValue(0);
  const lean = useSharedValue(0);
  useEffect(() => {
    latest.current = { target, onFire };
  });
  useEffect(
    () => () => {
      if (timer.current !== null) {
        clearTimeout(timer.current);
      }
    },
    [],
  );

  function release() {
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
    cancelAnimation(charge);
    charge.value = 0;
    cancelAnimation(lean);
    lean.value = reduceMotion ? 0 : withTiming(0, { duration: leanFor, reduceMotion: ReduceMotion.Never });
  }

  return {
    charge: charge as SharedValue<number>,
    lean: lean as SharedValue<number>,
    onPressIn: () => {
      release();
      const started = latest.current.target;
      if (started === undefined) {
        return;
      }
      charge.value = withTiming(1, {
        duration: holdFor,
        easing: reduceMotion ? Easing.steps(4, true) : Easing.linear,
        reduceMotion: ReduceMotion.Never,
      });
      if (!reduceMotion) {
        lean.value = withTiming(1, { duration: leanFor, reduceMotion: ReduceMotion.Never });
      }
      timer.current = setTimeout(() => {
        timer.current = null;
        release();
        latest.current.onFire(started);
      }, holdFor);
    },
    onPressOut: release,
  };
}
