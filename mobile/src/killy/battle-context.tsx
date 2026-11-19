import { createContext, type ReactNode, useContext, useEffect, useMemo, useRef, useState } from 'react';

import { slots } from './arena';
import { applyOutcome, type Battle, buildQueue, closeAllClear, newBattle, onState, parentKey, type QueueItem, type Shown } from './battle';
import { KillError, type KillErrorKind } from './client';
import { useKilly } from './context';
import { newKillId } from './kill-id';
import { BUBBLE_MS, defeatRows, NEXT_IN_MS, type Outcome, recoilEnd, settleAt, strikeEnd } from './strike';
import type { State } from './types';

/**
 * A kill under way, from the end of the hold until the battle moves on (DESIGN: Kill sequence).
 * The parent and its zombies are kept as they were when the kill started, so the screen goes on
 * showing what is being killed whatever the states that arrive meanwhile.
 */
export type Strike = {
  target: QueueItem;
  /** The zombies drawn on the ground, the n of the kill sequence. */
  n: number;
  /** What the sequence goes on to, once the result is in: the defeat (killed, already gone) or the recoil (anything else). */
  outcome: Outcome | null;
  /** When the sequence goes on, in ms from the end of the hold; null until the result is in. */
  at: number | null;
  /** Whether the result is on the info line yet. */
  revealed: boolean;
  /** With reduce motion, the sequence does not play: the battle switches to the result when it arrives. */
  instant: boolean;
  /** How many parents the battle had put away when the kill started. */
  cleared: number;
};

/** Killy's reactions to a kill (DESIGN: Killy's reactions). `id` tells one appearance from the next. */
export type Bubble = { symbol: '……！！' | '……！' | '……？' | '💜'; id: number };

type BattleView = {
  queue: QueueItem[];
  /** The parent on screen: the one being killed while a kill is under way, otherwise the front of the queue. */
  current: QueueItem | undefined;
  strike: Strike | null;
  /** True while a kill is under way: the request, then the sequence. */
  busy: boolean;
  /** What the last kill's outcome says, until the next kill starts (after a defeat, until the battle moves on). */
  shown: Shown | null;
  /** The last kill request's failure, shown under the kill button until the next kill starts. */
  error: KillErrorKind | null;
  allClear: Battle['allClear'];
  /** The parent counter: parents put away (killed or already gone), parents still to fight, and whether the first of them is being fought. */
  counter: { cleared: number; queued: number; fighting: boolean };
  /** Zombies waiting in the queue, for the Battle tab's badge. */
  waiting: number;
  bubble: Bubble | null;
  /** Killy bows from the end of the last parent's sequence until All clear is closed. */
  bowing: boolean;
  /** The last kill ended in a defeat that played: the parent that comes next comes in from above. */
  afterDefeat: boolean;
  /**
   * Kills the parent at the front of the queue, if it is still the one with this key and no kill
   * is under way. `instant` is reduce motion. Gives the number of zombies on the ground if the
   * kill started (the n of the sequence), null otherwise.
   */
  kill: (key: string, instant: boolean) => number | null;
  closeAllClear: () => void;
};

const BattleContext = createContext<BattleView | null>(null);

const empty: State = { servers: [] };

export function BattleProvider({ children }: { children: ReactNode }) {
  const { client, state, randomBytes } = useKilly();
  const [battle, setBattleState] = useState<Battle>(newBattle);
  // Mirrors `battle` so an outcome is applied to the battle as it is when the outcome arrives.
  const current$ = useRef<Battle>(battle);
  const setBattle = (next: Battle) => {
    current$.current = next;
    setBattleState(next);
  };
  const [strike, setStrikeState] = useState<Strike | null>(null);
  const strike$ = useRef<Strike | null>(null);
  const setStrike = (f: (s: Strike | null) => Strike | null) => {
    strike$.current = f(strike$.current);
    setStrikeState(strike$.current);
  };
  const [shown, setShown] = useState<Shown | null>(null);
  const [error, setError] = useState<KillErrorKind | null>(null);
  const [bubble, setBubble] = useState<Bubble | null>(null);
  const [bowing, setBowing] = useState(false);
  const [afterDefeat, setAfterDefeat] = useState(false);
  // The outcome is applied to the state current when it arrives, not the one the kill started from.
  const latest = useRef<State>(empty);

  const timers = useRef(new Set<ReturnType<typeof setTimeout>>());
  const bubbleTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const bubbleId = useRef(0);
  useEffect(() => {
    const all = timers.current;
    return () => {
      all.forEach(clearTimeout);
      if (bubbleTimer.current !== null) {
        clearTimeout(bubbleTimer.current);
      }
    };
  }, []);
  const after = (ms: number, f: () => void) => {
    const id = setTimeout(() => {
      timers.current.delete(id);
      f();
    }, Math.max(0, ms));
    timers.current.add(id);
  };
  const say = (symbol: Bubble['symbol'], ms: number) => {
    if (bubbleTimer.current !== null) {
      clearTimeout(bubbleTimer.current);
    }
    setBubble({ symbol, id: ++bubbleId.current });
    bubbleTimer.current = setTimeout(() => {
      bubbleTimer.current = null;
      setBubble(null);
    }, ms);
  };

  useEffect(() => {
    latest.current = state ?? empty;
    setBattle(onState(current$.current, state ?? empty));
  }, [state]);

  const queue = useMemo(() => buildQueue(state ?? empty, battle.hidden), [state, battle.hidden]);
  const waiting = queue.reduce((sum, i) => sum + i.parent.zombies.length, 0);

  function kill(key: string, instant: boolean): number | null {
    const front = queue[0];
    // The hold was for the parent at the front when it began; if another came to the front, nothing is sent.
    if (strike$.current !== null || battle.allClear !== null || front === undefined || parentKey(front.server.id, front.parent) !== key) {
      return null;
    }
    // The parent and the zombies on screen at this moment are what this kill is about.
    const target = { serverId: front.server.id, parent: front.parent, zombiesShown: front.parent.zombies.length };
    const n = Math.min(front.parent.zombies.length, slots.length);
    const started = Date.now();
    setStrike(() => ({ target: front, n, outcome: null, at: null, revealed: false, instant, cleared: battle.cleared }));
    setShown(null);
    setError(null);
    setAfterDefeat(false);
    say('……！！', BUBBLE_MS.start);

    // Times are from the end of the hold; with reduce motion every step comes when the result does.
    const settle = (outcome: Outcome, reveal: () => void) => {
      const arrival = Date.now() - started;
      const at = instant ? arrival : settleAt(arrival);
      const when = (ms: number) => (instant ? arrival : ms) - (Date.now() - started);
      const show = () => {
        reveal();
        setStrike((s) => s && { ...s, revealed: true });
      };
      setStrike((s) => s && { ...s, outcome, at });
      if (outcome === 'recoil') {
        after(when(at), () => {
          show();
          say('……？', BUBBLE_MS.recoil);
        });
        after(when(recoilEnd(at)), () => setStrike(() => null));
        return;
      }
      const rows = defeatRows(at, n);
      after(when(rows.result), () => {
        show();
        say('……！', BUBBLE_MS.killed);
      });
      // Whether the battle is over is decided when it would move on, from the queue as it is then:
      // a parent reported during the sequence is fought next, and if the last one left meanwhile,
      // the battle is over. What was counted when the result arrived stays.
      after(when(rows.next), () => {
        const b = current$.current;
        const empty = buildQueue(latest.current, b.hidden).length === 0;
        const allClear = empty ? (b.allClear ?? b.killed) : null;
        if (allClear !== b.allClear) {
          setBattle({ ...b, empty, allClear });
        }
        if (allClear !== null) {
          setBowing(true);
          say('💜', BUBBLE_MS.allClear);
        }
        after(when(strikeEnd('defeat', at, n, allClear !== null)), () => {
          setStrike(() => null);
          if (!instant) {
            // The next parent comes in from above; once it has, later ones just appear.
            setAfterDefeat(true);
            after(NEXT_IN_MS, () => setAfterDefeat(false));
            // The result has been seen while the parent was put away; the next parent comes without it.
            // Without the sequence there was no such moment, so it stays until the next kill.
            setShown(null);
          }
        });
      });
    };

    client
      .kill({ id: newKillId(randomBytes), serverId: target.serverId, ppid: target.parent.pid, starttime: target.parent.starttime })
      .then(
        (res) => {
          const r = applyOutcome(current$.current, latest.current, target, res);
          setBattle(r.battle);
          const defeat = r.result.kind === 'killed' || r.result.kind === 'already_gone';
          settle(defeat ? 'defeat' : 'recoil', () => setShown(r.result));
        },
        // A request that failed plays as an unknown outcome; the reason goes under the kill button.
        (e) => settle('recoil', () => setError(e instanceof KillError ? e.kind : 'other')),
      );
    return n;
  }

  const counter = (() => {
    if (strike === null) {
      return { cleared: battle.cleared, queued: queue.length, fighting: true };
    }
    const k = parentKey(strike.target.server.id, strike.target.parent);
    const others = queue.filter((i) => parentKey(i.server.id, i.parent) !== k).length;
    // The parent shows as put away once its result is on screen, not when it arrives; the next
    // one waits until it comes in.
    return strike.outcome === 'defeat' && strike.revealed
      ? { cleared: strike.cleared + 1, queued: others, fighting: false }
      : { cleared: strike.cleared, queued: others + 1, fighting: true };
  })();

  return (
    <BattleContext.Provider
      value={{
        queue,
        current: strike ? strike.target : queue[0],
        strike,
        busy: strike !== null,
        shown,
        error,
        // All clear waits for the end of the sequence.
        allClear: strike ? null : battle.allClear,
        counter,
        waiting,
        bubble,
        bowing,
        afterDefeat,
        kill,
        closeAllClear: () => {
          setBowing(false);
          setAfterDefeat(false);
          setBattle(closeAllClear(current$.current));
        },
      }}>
      {children}
    </BattleContext.Provider>
  );
}

export function useBattle(): BattleView {
  const ctx = useContext(BattleContext);
  if (!ctx) {
    throw new Error('useBattle outside BattleProvider');
  }
  return ctx;
}
