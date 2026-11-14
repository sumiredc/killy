import * as SecureStore from 'expo-secure-store';

/** A value kept on the phone between starts. get returns null when nothing usable is saved. */
export type Stored<T> = {
  get(): Promise<T | null>;
  set(value: T): Promise<void>;
};

/** What the app keeps on the phone besides the device token. Screen tests replace makePrefs. */
export type Prefs = {
  /** The app-wide notification switch. */
  notify: Stored<boolean>;
};

const notifyKey = 'notify';

/** The real ones, in expo-secure-store next to the device token. */
export function makePrefs(): Prefs {
  return {
    notify: {
      get: async () => {
        const v = await SecureStore.getItemAsync(notifyKey);
        return v === 'on' ? true : v === 'off' ? false : null;
      },
      set: (v) => SecureStore.setItemAsync(notifyKey, v ? 'on' : 'off'),
    },
  };
}
