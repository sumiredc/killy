import { serverRow } from '../killy/server-row';
import type { ServerState } from '../killy/types';

function server(over: Partial<ServerState> = {}): ServerState {
  return { id: 'a1', name: 'web-01', hostname: 'ip-10-0-0-1', notify: true, disconnected: false, parents: [], ...over };
}

function parent(pid: number, zombies: number[]) {
  return { pid, starttime: pid * 10, cmd: `node worker-${pid}.js`, zombies, first_detected_at: '2026-11-13T00:00:00Z' };
}

describe('serverRow', () => {
  test('shows the display name on this phone, not the hostname', () => {
    expect(serverRow(server({ name: 'my box' })).name).toBe('my box');
  });

  test('a server with zombies shows the zombie and parent counts and a badge with the zombie count', () => {
    const row = serverRow(server({ parents: [parent(100, [101, 102, 103]), parent(200, [201, 202])] }));
    expect(row.sub).toEqual({ key: 'rowZombies', params: { n: 5, p: 2 } });
    expect(row.badge).toBe(5);
  });

  test('a server without zombies says so and has no badge', () => {
    const row = serverRow(server());
    expect(row.sub).toEqual({ key: 'rowClean' });
    expect(row.badge).toBeNull();
  });

  test('a disconnected server says its status is unavailable and has no badge, even with parents', () => {
    const row = serverRow(server({ disconnected: true, parents: [parent(100, [101])] }));
    expect(row.sub).toEqual({ key: 'rowOffline' });
    expect(row.badge).toBeNull();
  });
});
