import { createContext, type ReactNode, useContext, useEffect, useMemo, useRef, useState } from 'react';

import { applyOutcome, type Battle, buildQueue, closeAllClear, newBattle, onState, parentKey, type QueueItem, type Shown } from './battle';
import { KillError, type KillErrorKind } from './client';
import { useKilly } from './context';
import { newKillId } from './kill-id';
import type { State } from './types';

type BattleView = {
  queue: QueueItem[];
  /** True while a kill request is in flight. */
  busy: boolean;
  /** What the last kill's outcome says, until the next kill starts. */
  shown: Shown | null;
  /** The last kill request's failure, shown under the kill button until the next kill starts. */
  error: KillErrorKind | null;
  allClear: Battle['allClear'];
  /** Parents this battle has put away (killed or already gone), for the parent counter. */
  cleared: number;
  /** Zombies waiting in the queue, for the Battle tab's badge. */
  waiting: number;
  /** Kills the parent at the front of the queue, if it is still the one with this key. */
  kill: (key: string) => void;
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
  const [busy, setBusy] = useState(false);
  const [shown, setShown] = useState<Shown | null>(null);
  const [error, setError] = useState<KillErrorKind | null>(null);
  // The outcome is applied to the state current when it arrives, not the one the kill started from.
  const latest = useRef<State>(empty);

  useEffect(() => {
    latest.current = state ?? empty;
    setBattle(onState(current$.current, state ?? empty));
  }, [state]);

  const queue = useMemo(() => buildQueue(state ?? empty, battle.hidden), [state, battle.hidden]);
  const waiting = queue.reduce((sum, i) => sum + i.parent.zombies.length, 0);
  const current = queue[0];

  function kill(key: string) {
    // The hold was for the parent at the front when it began; if another came to the front, nothing is sent.
    if (busy || battle.allClear !== null || current === undefined || parentKey(current.server.id, current.parent) !== key) {
      return;
    }
    // The parent and the zombies on screen at this moment are what this kill is about.
    const target = { serverId: current.server.id, parent: current.parent, zombiesShown: current.parent.zombies.length };
    setBusy(true);
    setShown(null);
    setError(null);
    client
      .kill({ id: newKillId(randomBytes), serverId: target.serverId, ppid: target.parent.pid, starttime: target.parent.starttime })
      .then((res) => {
        const r = applyOutcome(current$.current, latest.current, target, res);
        setBattle(r.battle);
        setShown(r.result);
      })
      .catch((e) => setError(e instanceof KillError ? e.kind : 'other'))
      .finally(() => setBusy(false));
  }

  return (
    <BattleContext.Provider
      value={{ queue, busy, shown, error, allClear: battle.allClear, cleared: battle.cleared, waiting, kill, closeAllClear: () => setBattle(closeAllClear(current$.current)) }}>
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
