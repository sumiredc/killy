import type { ParentState, ServerState, State } from './types';

/** One parent to fight, with the server it lives on. */
export type QueueItem = { server: ServerState; parent: ParentState };

/** The answer to a kill (KillResponse). */
export type KillResult = { outcome: string; reaped?: number; reason?: string };

/**
 * Where the battle stands. `hidden` holds parents whose kill ended killed or already_gone, until
 * a state without them arrives (PRD: Consistency). `killed` is what All clear counts.
 */
export type Battle = {
  hidden: Set<string>;
  /** Whether the queue was empty after the last state or outcome. */
  empty: boolean;
  killed: { parents: number; zombies: number };
  /** Parents this battle has put away, killed or already gone, for the parent counter. */
  cleared: number;
  allClear: { parents: number; zombies: number } | null;
};

/** What the battle screen says about a kill's outcome. */
export type Shown =
  | { kind: 'killed'; pid: number; reaped: number }
  | { kind: 'already_gone'; pid: number }
  | { kind: 'refused' | 'signal_failed' | 'zombies_remain' | 'unknown'; reason: string | undefined };

/** A parent is the same parent only on the same server with the same PID and start time. */
export function parentKey(serverId: string, p: { pid: number; starttime: number }): string {
  return `${serverId}/${p.pid}/${p.starttime}`;
}

export function newBattle(): Battle {
  return { hidden: new Set(), empty: true, killed: { parents: 0, zombies: 0 }, cleared: 0, allClear: null };
}

/**
 * The parents of every connected server in one queue, oldest first detection first. Parents found
 * by one snapshot share a time, so ties go by server id, pid and starttime; otherwise the front
 * could change with every state.
 */
export function buildQueue(state: State, hidden: Set<string>): QueueItem[] {
  const items: (QueueItem & { at: number })[] = [];
  for (const server of state.servers) {
    if (server.disconnected) {
      continue;
    }
    for (const parent of server.parents) {
      if (!hidden.has(parentKey(server.id, parent))) {
        items.push({ server, parent, at: Date.parse(parent.first_detected_at) });
      }
    }
  }
  items.sort(
    (x, y) =>
      x.at - y.at ||
      (x.server.id < y.server.id ? -1 : x.server.id > y.server.id ? 1 : 0) ||
      x.parent.pid - y.parent.pid ||
      x.parent.starttime - y.parent.starttime,
  );
  return items.map(({ server, parent }) => ({ server, parent }));
}

/**
 * Takes in a new state. A hidden parent the state no longer reports is shown again the next time
 * it is reported (a new episode). A queue that goes from empty to not empty starts a new battle;
 * one that only becomes empty keeps the record, since a kill in flight may still end it with All
 * clear.
 */
export function onState(b: Battle, state: State): Battle {
  const reported = new Set<string>();
  for (const s of state.servers) {
    for (const p of s.parents) {
      reported.add(parentKey(s.id, p));
    }
  }
  const hidden = new Set([...b.hidden].filter((k) => reported.has(k)));
  const empty = buildQueue(state, hidden).length === 0;
  const starts = b.empty && !empty && b.allClear === null;
  return starts ? { ...b, hidden, empty, killed: { parents: 0, zombies: 0 }, cleared: 0 } : { ...b, hidden, empty };
}

/**
 * Applies a kill's outcome: killed and already_gone hide the parent and move the queue on; only
 * killed is counted, with the zombies shown when the kill started. All clear opens when such an
 * outcome leaves the queue empty.
 */
export function applyOutcome(
  b: Battle,
  state: State,
  target: { serverId: string; parent: ParentState; zombiesShown: number },
  res: KillResult,
): { battle: Battle; result: Shown } {
  const pid = target.parent.pid;
  if (res.outcome !== 'killed' && res.outcome !== 'already_gone') {
    const kind = res.outcome === 'refused' || res.outcome === 'signal_failed' || res.outcome === 'zombies_remain' ? res.outcome : 'unknown';
    return { battle: b, result: { kind, reason: res.reason } };
  }
  const hidden = new Set(b.hidden).add(parentKey(target.serverId, target.parent));
  const killed =
    res.outcome === 'killed' ? { parents: b.killed.parents + 1, zombies: b.killed.zombies + target.zombiesShown } : b.killed;
  const empty = buildQueue(state, hidden).length === 0;
  const allClear = empty ? killed : null;
  const result: Shown = res.outcome === 'killed' ? { kind: 'killed', pid, reaped: res.reaped ?? 0 } : { kind: 'already_gone', pid };
  return { battle: { hidden, empty, killed, cleared: b.cleared + 1, allClear }, result };
}

/** Closing All clear starts a new battle. */
export function closeAllClear(b: Battle): Battle {
  return { ...b, killed: { parents: 0, zombies: 0 }, cleared: 0, allClear: null };
}
