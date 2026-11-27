import { readStorage, writeStorage } from './killy/web-storage';
import type { Prefs } from './prefs';

/** The settings of the Web version, in localStorage (Metro picks this file instead of prefs.ts, which loads expo-secure-store). */
export function makePrefs(): Prefs {
  return {
    language: {
      get: async () => {
        const v = readStorage('language');
        return v === 'ja' || v === 'en' ? v : null;
      },
      set: async (v) => writeStorage('language', v),
    },
    notify: {
      get: async () => {
        const v = readStorage('notify');
        return v === 'on' ? true : v === 'off' ? false : null;
      },
      set: async (v) => writeStorage('notify', v ? 'on' : 'off'),
    },
  };
}
