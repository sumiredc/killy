import { useEffect, useRef } from 'react';

/** How long the kill button must be held (DESIGN: Hold to kill). */
export const holdFor = 1000;

/**
 * Press handlers that call onFire with the target the press started on, once the press has lasted
 * `holdFor`. Letting go earlier does nothing. onFire decides whether that target is still the one
 * to act on.
 */
export function useHoldToKill<T>(target: T | undefined, onFire: (target: T) => void) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const latest = useRef({ target, onFire });
  useEffect(() => {
    latest.current = { target, onFire };
  });
  useEffect(() => () => cancel(), []);

  function cancel() {
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
  }

  return {
    onPressIn: () => {
      cancel();
      const started = latest.current.target;
      if (started === undefined) {
        return;
      }
      timer.current = setTimeout(() => {
        timer.current = null;
        latest.current.onFire(started);
      }, holdFor);
    },
    onPressOut: cancel,
  };
}
