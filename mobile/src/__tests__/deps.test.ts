import * as SecureStore from 'expo-secure-store';

import { makeDeps, tokenKey } from '../killy/deps';
import { makePrefs } from '../prefs';

jest.mock('expo-secure-store', () => {
  const items = new Map<string, string>();
  return {
    getItemAsync: jest.fn(async (key: string) => items.get(key) ?? null),
    setItemAsync: jest.fn(async (key: string, value: string) => void items.set(key, value)),
    clear: () => items.clear(),
  };
});

beforeEach(() => (SecureStore as unknown as { clear(): void }).clear());

const staging = 'https://stg-killy.sumire.pink';
const production = 'https://killy.sumire.pink';

test('the device token is kept in expo-secure-store, under a key for its Killy server', async () => {
  const { tokens } = makeDeps(production);
  await expect(tokens.get()).resolves.toBeNull();
  await tokens.set('tok-1');
  expect(SecureStore.setItemAsync).toHaveBeenCalledWith('device_token.https___killy.sumire.pink', 'tok-1');
  await expect(makeDeps(production).tokens.get()).resolves.toBe('tok-1');
});

test('a token saved for another Killy server is not read, so it is never sent to this one', async () => {
  await makeDeps(staging).tokens.set('staging-token');
  await expect(makeDeps(production).tokens.get()).resolves.toBeNull();
  await makeDeps(production).tokens.set('production-token');
  await expect(makeDeps(staging).tokens.get()).resolves.toBe('staging-token');
  await expect(makeDeps(production).tokens.get()).resolves.toBe('production-token');
});

test('the token an earlier version saved under the one key for all servers is not read', async () => {
  await SecureStore.setItemAsync('device_token', 'old-token');
  await expect(makeDeps(staging).tokens.get()).resolves.toBeNull();
  await expect(makeDeps(production).tokens.get()).resolves.toBeNull();
});

describe('tokenKey', () => {
  test('uses only characters SecureStore allows, and ignores case and a trailing slash', () => {
    expect(tokenKey('https://stg-killy.sumire.pink:8443/')).toMatch(/^[A-Za-z0-9._-]+$/);
    expect(tokenKey('HTTPS://Killy.Sumire.Pink/')).toBe(tokenKey(production));
  });

  test('differs between servers', () => {
    expect(tokenKey(staging)).not.toBe(tokenKey(production));
  });
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

test('the language is kept in expo-secure-store, and anything but ja or en counts as nothing saved', async () => {
  await expect(makePrefs().language.get()).resolves.toBeNull();
  await makePrefs().language.set('ja');
  expect(SecureStore.setItemAsync).toHaveBeenCalledWith('language', 'ja');
  await expect(makePrefs().language.get()).resolves.toBe('ja');
  await makePrefs().language.set('en');
  await expect(makePrefs().language.get()).resolves.toBe('en');
  await SecureStore.setItemAsync('language', 'fr');
  await expect(makePrefs().language.get()).resolves.toBeNull();
});
