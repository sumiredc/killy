import * as SecureStore from 'expo-secure-store';

import { makeDeps } from '../killy/deps';

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
