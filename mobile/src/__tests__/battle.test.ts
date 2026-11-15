import { applyOutcome, buildQueue, closeAllClear, newBattle, onState, parentKey, type QueueItem } from '../killy/battle';
import type { ParentState, ServerState, State } from '../killy/types';

function parent(pid: number, zombies: number[], at: string, starttime = pid * 10): ParentState {
  return { pid, starttime, cmd: `cmd-${pid}`, zombies, first_detected_at: at };
}

function server(id: string, parents: ParentState[], over: Partial<ServerState> = {}): ServerState {
  return { id, name: `name-${id}`, hostname: `host-${id}`, notify: true, disconnected: false, parents, ...over };
}

const pids = (q: QueueItem[]) => q.map((i) => `${i.server.id}:${i.parent.pid}`);

describe('buildQueue', () => {
  test('parents of every connected server form one queue, oldest detection first', () => {
    const state: State = {
      servers: [
        server('b', [parent(20, [21], '2026-11-13T00:00:03Z'), parent(10, [11], '2026-11-13T00:00:01Z')]),
        server('a', [parent(30, [31], '2026-11-13T00:00:02Z')]),
      ],
    };
    expect(pids(buildQueue(state, new Set()))).toEqual(['b:10', 'a:30', 'b:20']);
  });

  test('parents found at the same time are ordered by server id, pid and starttime', () => {
    const at = '2026-11-13T00:00:00Z';
    const state: State = {
      servers: [
        server('b', [parent(5, [6], at)]),
        server('a', [parent(7, [8], at, 2), parent(7, [9], at, 1), parent(3, [4], at)]),
      ],
    };
    expect(buildQueue(state, new Set()).map((i) => `${i.server.id}:${i.parent.pid}:${i.parent.starttime}`)).toEqual([
      'a:3:30',
      'a:7:1',
      'a:7:2',
      'b:5:50',
    ]);
  });

  test('a disconnected server has no parents in the queue', () => {
    const state: State = {
      servers: [server('a', [parent(10, [11], '2026-11-13T00:00:01Z')], { disconnected: true }), server('b', [parent(20, [21], '2026-11-13T00:00:02Z')])],
    };
    expect(pids(buildQueue(state, new Set()))).toEqual(['b:20']);
  });

  test('a hidden parent is left out, but the same pid and starttime on another server is not', () => {
    const p = parent(10, [11], '2026-11-13T00:00:01Z');
    const state: State = { servers: [server('a', [p]), server('b', [p])] };
    expect(pids(buildQueue(state, new Set([parentKey('a', p)])))).toEqual(['b:10']);
  });
});

describe('a battle', () => {
  const t1 = '2026-11-13T00:00:01Z';
  const t2 = '2026-11-13T00:00:02Z';
  const t3 = '2026-11-13T00:00:03Z';

  test('a killed parent is counted with the zombies shown when its kill started and the queue moves on', () => {
    const p1 = parent(10, [11, 12, 13], t1);
    const p2 = parent(20, [21], t2);
    const state: State = { servers: [server('a', [p1, p2])] };
    let b = onState(newBattle(), state);
    const r = applyOutcome(b, state, { serverId: 'a', parent: p1, zombiesShown: 3 }, { outcome: 'killed', reaped: 2 });
    b = r.battle;
    expect(r.result).toEqual({ kind: 'killed', pid: 10, reaped: 2 });
    expect(pids(buildQueue(state, b.hidden))).toEqual(['a:20']);
    expect(b.allClear).toBeNull();
    b = applyOutcome(b, state, { serverId: 'a', parent: p2, zombiesShown: 1 }, { outcome: 'killed', reaped: 1 }).battle;
    expect(b.allClear).toEqual({ parents: 2, zombies: 4 });
  });

  test('an already gone parent moves the queue on but is not counted', () => {
    const p1 = parent(10, [11, 12], t1);
    const p2 = parent(20, [21, 22, 23], t2);
    const state: State = { servers: [server('a', [p1, p2])] };
    let b = onState(newBattle(), state);
    const r = applyOutcome(b, state, { serverId: 'a', parent: p1, zombiesShown: 2 }, { outcome: 'already_gone' });
    expect(r.result).toEqual({ kind: 'already_gone', pid: 10 });
    b = r.battle;
    expect(pids(buildQueue(state, b.hidden))).toEqual(['a:20']);
    b = applyOutcome(b, state, { serverId: 'a', parent: p2, zombiesShown: 3 }, { outcome: 'killed', reaped: 3 }).battle;
    expect(b.allClear).toEqual({ parents: 1, zombies: 3 });
  });

  test.each([
    ['refused', { outcome: 'refused', reason: 'pidfd_unavailable' }, { kind: 'refused', reason: 'pidfd_unavailable' }],
    ['refused', { outcome: 'refused', reason: 'protected' }, { kind: 'refused', reason: 'protected' }],
    ['signal_failed', { outcome: 'signal_failed', reason: 'EPERM' }, { kind: 'signal_failed', reason: 'EPERM' }],
    ['zombies_remain', { outcome: 'zombies_remain' }, { kind: 'zombies_remain', reason: undefined }],
    ['unknown', { outcome: 'unknown', reason: 'timeout' }, { kind: 'unknown', reason: 'timeout' }],
  ] as const)('%s shows its reason, hides nothing and counts nothing', (_, res, result) => {
    const p1 = parent(10, [11], t1);
    const state: State = { servers: [server('a', [p1])] };
    const b = onState(newBattle(), state);
    const r = applyOutcome(b, state, { serverId: 'a', parent: p1, zombiesShown: 1 }, res);
    expect(r.result).toEqual(result);
    expect(pids(buildQueue(state, r.battle.hidden))).toEqual(['a:10']);
    expect(r.battle.allClear).toBeNull();
    expect(r.battle.killed).toEqual({ parents: 0, zombies: 0 });
  });

  test('a killed parent stays out while states still report it, and comes back as a new episode after a state without it', () => {
    const p1 = parent(10, [11], t1);
    const p2 = parent(20, [21], t2);
    const both: State = { servers: [server('a', [p1, p2])] };
    let b = onState(newBattle(), both);
    b = applyOutcome(b, both, { serverId: 'a', parent: p1, zombiesShown: 1 }, { outcome: 'killed', reaped: 1 }).battle;
    b = onState(b, both);
    expect(pids(buildQueue(both, b.hidden))).toEqual(['a:20']);
    const without: State = { servers: [server('a', [p2])] };
    b = onState(b, without);
    b = onState(b, both);
    expect(pids(buildQueue(both, b.hidden))).toEqual(['a:10', 'a:20']);
  });

  test('an already gone parent stays out the same way', () => {
    const p1 = parent(10, [11], t1);
    const p2 = parent(20, [21], t2);
    const both: State = { servers: [server('a', [p1, p2])] };
    let b = onState(newBattle(), both);
    b = applyOutcome(b, both, { serverId: 'a', parent: p1, zombiesShown: 1 }, { outcome: 'already_gone' }).battle;
    b = onState(b, both);
    expect(pids(buildQueue(both, b.hidden))).toEqual(['a:20']);
  });

  test('a parent hidden on one server stays hidden while only another server reports the same pid and starttime', () => {
    const p = parent(10, [11], t1);
    const stateA: State = { servers: [server('a', [p]), server('b', [parent(30, [31], t3)])] };
    let b = onState(newBattle(), stateA);
    b = applyOutcome(b, stateA, { serverId: 'a', parent: p, zombiesShown: 1 }, { outcome: 'killed', reaped: 1 }).battle;
    const both: State = { servers: [server('a', [p]), server('b', [p])] };
    b = onState(b, both);
    expect(pids(buildQueue(both, b.hidden))).toEqual(['b:10']);
  });

  test('a new parent during the battle goes to the end, and the current parent shows its new zombies', () => {
    const p1 = parent(10, [11], t1);
    let state: State = { servers: [server('a', [p1])] };
    let b = onState(newBattle(), state);
    state = { servers: [server('a', [parent(10, [11, 12], t1)]), server('b', [parent(20, [21], t3)])] };
    b = onState(b, state);
    const q = buildQueue(state, b.hidden);
    expect(pids(q)).toEqual(['a:10', 'b:20']);
    expect(q[0].parent.zombies).toEqual([11, 12]);
  });

  test('a queue emptied by something other than a kill shows no All clear and forgets the battle', () => {
    const p1 = parent(10, [11], t1);
    const p2 = parent(20, [21, 22], t2);
    const state: State = { servers: [server('a', [p1, p2])] };
    let b = onState(newBattle(), state);
    b = applyOutcome(b, state, { serverId: 'a', parent: p1, zombiesShown: 1 }, { outcome: 'killed', reaped: 1 }).battle;
    b = onState(b, { servers: [server('a', [p1])] });
    expect(b.allClear).toBeNull();
    b = onState(b, { servers: [server('a', [])] });
    b = onState(b, { servers: [server('a', [parent(40, [41], t3)])] });
    b = applyOutcome(b, { servers: [server('a', [parent(40, [41], t3)])] }, { serverId: 'a', parent: parent(40, [41], t3), zombiesShown: 1 }, { outcome: 'killed', reaped: 1 }).battle;
    expect(b.allClear).toEqual({ parents: 1, zombies: 1 });
  });

  test('a state without the parent arriving while its kill is in flight does not lose the battle so far', () => {
    const p1 = parent(10, [11], t1);
    const p2 = parent(20, [21, 22], t2);
    const both: State = { servers: [server('a', [p1, p2])] };
    let b = onState(newBattle(), both);
    b = applyOutcome(b, both, { serverId: 'a', parent: p1, zombiesShown: 1 }, { outcome: 'killed', reaped: 1 }).battle;
    // P2's kill is in flight; the agent's next scan already misses P2.
    const none: State = { servers: [server('a', [])] };
    b = onState(b, none);
    b = applyOutcome(b, none, { serverId: 'a', parent: p2, zombiesShown: 2 }, { outcome: 'killed', reaped: 2 }).battle;
    expect(b.allClear).toEqual({ parents: 2, zombies: 3 });
  });

  test('the parents put away, killed or already gone, are counted for the parent counter', () => {
    const p1 = parent(10, [11], t1);
    const p2 = parent(20, [21], t2);
    const p3 = parent(30, [31], t3);
    const state: State = { servers: [server('a', [p1, p2, p3])] };
    let b = onState(newBattle(), state);
    expect(b.cleared).toBe(0);
    b = applyOutcome(b, state, { serverId: 'a', parent: p1, zombiesShown: 1 }, { outcome: 'already_gone' }).battle;
    b = applyOutcome(b, state, { serverId: 'a', parent: p2, zombiesShown: 1 }, { outcome: 'refused', reason: 'protected' }).battle;
    b = applyOutcome(b, state, { serverId: 'a', parent: p2, zombiesShown: 1 }, { outcome: 'killed', reaped: 1 }).battle;
    expect(b.cleared).toBe(2);
  });

  test('a battle that starts after the queue emptied counts its put-away parents from zero', () => {
    const p1 = parent(10, [11], t1);
    const p2 = parent(20, [21], t2);
    const both: State = { servers: [server('a', [p1, p2])] };
    let b = onState(newBattle(), both);
    b = applyOutcome(b, both, { serverId: 'a', parent: p1, zombiesShown: 1 }, { outcome: 'killed', reaped: 1 }).battle;
    b = onState(b, { servers: [server('a', [])] });
    b = onState(b, { servers: [server('a', [parent(40, [41], t3)])] });
    expect(b.cleared).toBe(0);
  });

  test('closing All clear counts the put-away parents from zero', () => {
    const p1 = parent(10, [11], t1);
    const state: State = { servers: [server('a', [p1])] };
    let b = onState(newBattle(), state);
    b = applyOutcome(b, state, { serverId: 'a', parent: p1, zombiesShown: 1 }, { outcome: 'killed', reaped: 1 }).battle;
    expect(b.cleared).toBe(1);
    expect(closeAllClear(b).cleared).toBe(0);
  });

  test('All clear keeps its numbers while it is shown, and closing it starts a new battle', () => {
    const p1 = parent(10, [11, 12], t1);
    const state: State = { servers: [server('a', [p1])] };
    let b = onState(newBattle(), state);
    b = applyOutcome(b, state, { serverId: 'a', parent: p1, zombiesShown: 2 }, { outcome: 'killed', reaped: 2 }).battle;
    expect(b.allClear).toEqual({ parents: 1, zombies: 2 });
    const p2 = parent(20, [21], t2);
    const next: State = { servers: [server('a', [p1, p2])] };
    b = onState(b, next);
    expect(b.allClear).toEqual({ parents: 1, zombies: 2 });
    b = closeAllClear(b);
    expect(b.allClear).toBeNull();
    b = applyOutcome(b, next, { serverId: 'a', parent: p2, zombiesShown: 1 }, { outcome: 'killed', reaped: 1 }).battle;
    expect(b.allClear).toEqual({ parents: 1, zombies: 1 });
  });
});
