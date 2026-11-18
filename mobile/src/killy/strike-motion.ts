import { useEffect } from 'react';
import { cancelAnimation, Easing, ReduceMotion, type SharedValue, useSharedValue, withTiming } from 'react-native-reanimated';

import type { Strike } from './battle-context';
import { BOW_FOR, CLOCK_MS, DEFEAT_FROM, outcomeCode, REST } from './strike';

/** What the arena's kill sequence is drawn from, on the UI thread. */
export type StrikeMotion = {
  /** Ms since the hold completed, or REST when no kill is under way. */
  elapsed: SharedValue<number>;
  /** outcomeCode of the result: 0 while it is awaited. */
  outcome: SharedValue<number>;
  /** When the sequence goes on (settleAt of the result's arrival). */
  at: SharedValue<number>;
  /** The zombies on the ground. */
  n: SharedValue<number>;
  /** Killy's bow, 0–1. */
  bow: SharedValue<number>;
  /** Starts the sequence's clock; called in the same moment as the hold ends, so the swing follows the wind-up without a frame between. */
  start: (n: number) => void;
};

const never = { reduceMotion: ReduceMotion.Never };
// CSS `ease`: Reanimated's Easing.ease is a different curve.
const cssEase = Easing.bezier(0.25, 0.1, 0.25, 1);

/**
 * Drives the arena from the battle's strike. Whether to move at all is decided by the caller from
 * useReduceMotion, so Reanimated's own reduce-motion handling is turned off here: with reduce motion
 * the clock never starts and Killy is bowed at once.
 */
export function useStrikeMotion(strike: Strike | null, bowing: boolean, reduceMotion: boolean): StrikeMotion {
  const elapsed = useSharedValue(REST);
  const outcome = useSharedValue(0);
  const at = useSharedValue(DEFEAT_FROM);
  const n = useSharedValue(0);
  const bow = useSharedValue(0);

  useEffect(() => {
    if (strike !== null) {
      outcome.set(outcomeCode(strike.outcome));
      at.set(strike.at ?? DEFEAT_FROM);
      return;
    }
    cancelAnimation(elapsed);
    elapsed.set(REST);
    outcome.set(0);
  }, [strike, elapsed, outcome, at]);

  useEffect(() => {
    cancelAnimation(bow);
    bow.set(bowing && !reduceMotion ? withTiming(1, { duration: BOW_FOR, easing: cssEase, ...never }) : bowing ? 1 : 0);
  }, [bowing, reduceMotion, bow]);

  return {
    elapsed,
    outcome,
    at,
    n,
    bow,
    start: (count) => {
      // The outcome and its time come from the strike (the effect above); here only the clock starts.
      n.set(count);
      elapsed.set(0);
      elapsed.set(withTiming(CLOCK_MS, { duration: CLOCK_MS, easing: Easing.linear, ...never }));
    },
  };
}
