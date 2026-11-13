import { webcrypto } from 'node:crypto';

import { newKillId } from '../killy/kill-id';

// The Killy server's rule (docs/openapi.yaml KillId, #5): lower case, version 4, variant 10.
const uuidV4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const bytes = (n: number) => webcrypto.getRandomValues(new Uint8Array(n));

test('a kill ID is a lower-case UUID v4', () => {
  for (let i = 0; i < 200; i++) {
    expect(newKillId(bytes)).toMatch(uuidV4);
  }
});

test('the version and variant bits are set whatever the random bytes are', () => {
  expect(newKillId((n) => new Uint8Array(n).fill(0xff))).toBe('ffffffff-ffff-4fff-bfff-ffffffffffff');
  expect(newKillId((n) => new Uint8Array(n))).toBe('00000000-0000-4000-8000-000000000000');
});

test('every kill ID is new', () => {
  const ids = new Set(Array.from({ length: 500 }, () => newKillId(bytes)));
  expect(ids.size).toBe(500);
});
