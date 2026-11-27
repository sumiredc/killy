import { getRandomBytes } from 'expo-crypto';
import * as SecureStore from 'expo-secure-store';

import type { FetchLike } from './client';
import { makeNotifications, type NotificationsDeps } from './notifications';
import { tokenKey } from './token-key';

export { tokenKey };

/** Where the device token for this build's Killy server is kept between starts. */
export type TokenStore = {
  get(): Promise<string | null>;
  set(token: string): Promise<void>;
};

/** What the app uses outside itself: the network, the device token store, randomness and the OS notifications. */
export type Deps = {
  fetch: FetchLike;
  tokens: TokenStore;
  random: () => number;
  /** Cryptographically random bytes, for kill IDs. */
  randomBytes: (n: number) => Uint8Array;
  notifications: NotificationsDeps;
};

/**
 * The real ones. Screen tests replace this function. Expo installs expo/fetch as the global fetch,
 * which streams response bodies, so the event stream needs no extra library.
 */
export function makeDeps(baseURL: string): Deps {
  const key = tokenKey(baseURL);
  return {
    fetch: (url, init) => fetch(url, init),
    tokens: {
      get: () => SecureStore.getItemAsync(key),
      set: (token) => SecureStore.setItemAsync(key, token),
    },
    random: Math.random,
    randomBytes: getRandomBytes,
    notifications: makeNotifications(),
  };
}
