import { getRandomBytes } from 'expo-crypto';
import * as SecureStore from 'expo-secure-store';

import type { FetchLike } from './client';

/** Where the device token is kept between starts. */
export type TokenStore = {
  get(): Promise<string | null>;
  set(token: string): Promise<void>;
};

/** What the app uses outside itself: the network, the device token store and randomness. */
export type Deps = {
  fetch: FetchLike;
  tokens: TokenStore;
  random: () => number;
  /** Cryptographically random bytes, for kill IDs. */
  randomBytes: (n: number) => Uint8Array;
};

const tokenKey = 'device_token';

/**
 * The real ones. Screen tests replace this function. Expo installs expo/fetch as the global fetch,
 * which streams response bodies, so the event stream needs no extra library.
 */
export function makeDeps(): Deps {
  return {
    fetch: (url, init) => fetch(url, init),
    tokens: {
      get: () => SecureStore.getItemAsync(tokenKey),
      set: (token) => SecureStore.setItemAsync(tokenKey, token),
    },
    random: Math.random,
    randomBytes: getRandomBytes,
  };
}
