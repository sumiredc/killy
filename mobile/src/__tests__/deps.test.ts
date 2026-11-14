import * as SecureStore from 'expo-secure-store';

import { makeDeps } from '../killy/deps';
import { makePrefs } from '../prefs';

jest.mock('expo-secure-store', () => {
  const items = new Map<string, string>();
  return {
    getItemAsync: jest.fn(async (key: string) => items.get(key) ?? null),
    setItemAsync: jest.fn(async (key: string, value: string) => void items.set(key, value)),
  };
});

test('the device token is kept in expo-secure-store', async () => {
  const { tokens } = makeDeps();
  await expect(tokens.get()).resolves.toBeNull();
  await tokens.set('tok-1');
  expect(SecureStore.setItemAsync).toHaveBeenCalledWith('device_token', 'tok-1');
  await expect(makeDeps().tokens.get()).resolves.toBe('tok-1');
});

describe('prefs', () => {
  test('the app-wide switch is kept in expo-secure-store and read back', async () => {
    await expect(makePrefs().notify.get()).resolves.toBeNull();
    await makePrefs().notify.set(false);
    expect(SecureStore.setItemAsync).toHaveBeenCalledWith('notify', 'off');
    await expect(makePrefs().notify.get()).resolves.toBe(false);
    await makePrefs().notify.set(true);
    await expect(makePrefs().notify.get()).resolves.toBe(true);
  });

  test('a value it did not write counts as nothing saved', async () => {
    await SecureStore.setItemAsync('notify', 'true');
    await expect(makePrefs().notify.get()).resolves.toBeNull();
  });
});
