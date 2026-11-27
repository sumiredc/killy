import type { Deps } from './deps';
import { makeNotifications } from './notifications';
import { tokenKey } from './token-key';
import { readStorage, writeStorage } from './web-storage';

/**
 * What the Web version uses outside itself (Metro picks this file instead of deps.ts, which loads
 * expo-secure-store and expo-crypto): the browser's fetch, which streams response bodies, its
 * localStorage for the device token, and its crypto for kill IDs.
 */
export function makeDeps(baseURL: string): Deps {
  const key = tokenKey(baseURL);
  return {
    fetch: (url, init) => fetch(url, init),
    tokens: {
      get: async () => readStorage(key),
      set: async (token) => writeStorage(key, token),
    },
    random: Math.random,
    randomBytes: (n) => globalThis.crypto.getRandomValues(new Uint8Array(n)),
    notifications: makeNotifications(),
  };
}
