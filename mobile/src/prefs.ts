import * as SecureStore from 'expo-secure-store';

import type { Lang } from './i18n';

/** A value kept on the phone between starts. get returns null when nothing usable is saved. */
export type Stored<T> = {
  get(): Promise<T | null>;
  set(value: T): Promise<void>;
};

/** What the app keeps on the phone besides the device token. Screen tests replace makePrefs. */
export type Prefs = {
  /** The language chosen in Settings. */
  language: Stored<Lang>;
  /** The app-wide notification switch. */
  notify: Stored<boolean>;
};

const notifyKey = 'notify';
const languageKey = 'language';

/** The real ones, in expo-secure-store next to the device token. */
export function makePrefs(): Prefs {
  return {
    language: {
      get: async () => {
        const v = await SecureStore.getItemAsync(languageKey);
        return v === 'ja' || v === 'en' ? v : null;
      },
      set: (v) => SecureStore.setItemAsync(languageKey, v),
    },
    notify: {
      get: async () => {
        const v = await SecureStore.getItemAsync(notifyKey);
        return v === 'on' ? true : v === 'off' ? false : null;
      },
      set: (v) => SecureStore.setItemAsync(notifyKey, v ? 'on' : 'off'),
    },
  };
}
