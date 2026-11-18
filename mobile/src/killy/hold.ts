import { useEffect, useRef } from 'react';
import { cancelAnimation, Easing, ReduceMotion, type SharedValue, useSharedValue, withTiming } from 'react-native-reanimated';

/** How long the kill button must be held (DESIGN: Hold to kill). */
export const holdFor = 1000;

/** How long Killy takes to lean back when a hold starts, and to straighten when it ends (the mock's `.ghost` transition). */
export const leanFor = 250;

/**
 * Press handlers that call onFire with the target the press started on, once the press has lasted
 * `holdFor`. Letting go earlier does nothing, and so does a change of target during the press
 * (another parent came to the front, or the button can no longer be pressed): the hold ends then.
 * onFire decides whether that target is still the one to act on.
 *
 * `charge` goes from 0 to 1 over the same `holdFor` from the same press, so the button is full
 * exactly when the kill starts; it drops back to 0 when the hold ends either way. `windUp` follows
 * it for the scythe, and `lean` is 1 while a hold is on, easing over `leanFor`. With reduce motion,
 * `charge` rises a quarter every quarter of `holdFor` instead of smoothly (the hold must still show),
 * and Killy neither winds up nor leans.
 */
export function useHoldToKill<T>(target: T | undefined, onFire: (target: T) => void, reduceMotion = false) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const started = useRef<T | undefined>(undefined);
  const latest = useRef({ target, onFire });
  const charge = useSharedValue(0);
  const windUp = useSharedValue(0);
  const lean = useSharedValue(0);

  function release() {
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
    started.current = undefined;
    for (const v of [charge, windUp]) {
      cancelAnimation(v);
      v.set(0);
    }
    cancelAnimation(lean);
    lean.set(reduceMotion ? 0 : withTiming(0, { duration: leanFor, reduceMotion: ReduceMotion.Never }));
  }

  useEffect(() => {
    latest.current = { target, onFire };
    if (timer.current !== null && target !== started.current) {
      release();
    }
  });
  useEffect(
    () => () => {
      if (timer.current !== null) {
        clearTimeout(timer.current);
      }
    },
    [],
  );

  return {
    charge: charge as SharedValue<number>,
    windUp: windUp as SharedValue<number>,
    lean: lean as SharedValue<number>,
    onPressIn: () => {
      release();
      const at = latest.current.target;
      if (at === undefined) {
        return;
      }
      started.current = at;
      // Rounding down, so the last quarter, the full button, comes at holdFor with the kill.
      charge.set(
        withTiming(1, {
          duration: holdFor,
          easing: reduceMotion ? Easing.steps(4, false) : Easing.linear,
          reduceMotion: ReduceMotion.Never,
        }),
      );
      if (!reduceMotion) {
        windUp.set(withTiming(1, { duration: holdFor, easing: Easing.linear, reduceMotion: ReduceMotion.Never }));
        lean.set(withTiming(1, { duration: leanFor, reduceMotion: ReduceMotion.Never }));
      }
      timer.current = setTimeout(() => {
        timer.current = null;
        release();
        latest.current.onFire(at);
      }, holdFor);
    },
    onPressOut: release,
  };
}
