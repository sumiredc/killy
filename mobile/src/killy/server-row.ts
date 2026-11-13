import type { Key } from '@/i18n';

import type { ServerState } from './types';

export type ServerRow = {
  name: string;
  sub: { key: Key; params?: Record<string, number> };
  badge: number | null;
};

/**
 * What the Settings row of a server shows (DESIGN: Settings tab). A disconnected server's parents
 * are its last report, so it shows neither them nor a badge.
 */
export function serverRow(server: ServerState): ServerRow {
  if (server.disconnected) {
    return { name: server.name, sub: { key: 'rowOffline' }, badge: null };
  }
  const zombies = server.parents.reduce((sum, p) => sum + p.zombies.length, 0);
  if (zombies === 0) {
    return { name: server.name, sub: { key: 'rowClean' }, badge: null };
  }
  return { name: server.name, sub: { key: 'rowZombies', params: { n: zombies, p: server.parents.length } }, badge: zombies };
}
