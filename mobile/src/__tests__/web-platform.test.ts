import * as fs from 'node:fs';
import * as path from 'node:path';

import { makeDeps } from '../killy/deps.web';
import { makeNotifications } from '../killy/notifications.web';
import { makePrefs } from '../prefs.web';

// In a Web build deps.web.ts's './notifications' is notifications.web.ts; jest runs as iOS, so say so here.
jest.mock('../killy/notifications', () => jest.requireActual('../killy/notifications.web'));

// The Web versions of what the app keeps on the phone and asks of the OS (Metro picks the .web.ts
// files in a Web build). They are imported by name here, because jest runs as iOS.

/** A localStorage the test holds; `fail` makes every call throw, as a browser that refuses storage does. */
class FakeStorage {
  readonly items = new Map<string, string>();
  fail = false;
  getItem(key: string) {
    if (this.fail) throw new DOMException('denied', 'SecurityError');
    return this.items.get(key) ?? null;
  }
  setItem(key: string, value: string) {
    if (this.fail) throw new DOMException('denied', 'QuotaExceededError');
    this.items.set(key, value);
  }
}

let storage: FakeStorage;
const original = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');

beforeEach(() => {
  storage = new FakeStorage();
  Object.defineProperty(globalThis, 'localStorage', { value: storage, configurable: true, writable: true });
});

afterEach(() => {
  if (original) Object.defineProperty(globalThis, 'localStorage', original);
  else delete (globalThis as { localStorage?: unknown }).localStorage;
});

/** What a browser without localStorage looks like. */
function noStorage() {
  delete (globalThis as { localStorage?: unknown }).localStorage;
}

const staging = 'https://stg-killy.sumire.pink';
const production = 'https://killy.sumire.pink';

describe('the device token', () => {
  test('is kept in localStorage under a key for its Killy server, and read back by the next start', async () => {
    await expect(makeDeps(staging).tokens.get()).resolves.toBeNull();
    await makeDeps(staging).tokens.set('tok-1');
    expect(storage.items.get('device_token.https___stg-killy.sumire.pink')).toBe('tok-1');
    await expect(makeDeps(staging).tokens.get()).resolves.toBe('tok-1');
  });

  test('saved for another Killy server is not read, so it is never sent to this one', async () => {
    await makeDeps(staging).tokens.set('staging-token');
    await expect(makeDeps(production).tokens.get()).resolves.toBeNull();
    await makeDeps(production).tokens.set('production-token');
    await expect(makeDeps(staging).tokens.get()).resolves.toBe('staging-token');
  });

  test('is read as nothing, and saving does nothing, when localStorage refuses every call', async () => {
    await makeDeps(staging).tokens.set('tok-1');
    storage.fail = true;
    await expect(makeDeps(staging).tokens.get()).resolves.toBeNull();
    await expect(makeDeps(staging).tokens.set('tok-2')).resolves.toBeUndefined();
    storage.fail = false;
    expect(storage.items.get('device_token.https___stg-killy.sumire.pink')).toBe('tok-1');
  });

  test('is read as nothing, and saving does nothing, when there is no localStorage', async () => {
    noStorage();
    await expect(makeDeps(staging).tokens.get()).resolves.toBeNull();
    await expect(makeDeps(staging).tokens.set('tok-1')).resolves.toBeUndefined();
  });
});

describe('the rest of the Web deps', () => {
  test('kill IDs come from the browser: n random bytes, different every time', () => {
    const { randomBytes } = makeDeps(staging);
    const a = randomBytes(16);
    expect(a).toHaveLength(16);
    expect(randomBytes(16)).not.toEqual(a);
  });

  test('the notifications are the Web ones: nothing can be shown, so none is asked for', async () => {
    await expect(makeDeps(staging).notifications.permission()).resolves.toBe('denied');
  });
});

describe('prefs', () => {
  test('the language is kept in localStorage; anything but ja or en counts as nothing saved', async () => {
    await expect(makePrefs().language.get()).resolves.toBeNull();
    await makePrefs().language.set('ja');
    expect(storage.items.get('language')).toBe('ja');
    await expect(makePrefs().language.get()).resolves.toBe('ja');
    await makePrefs().language.set('en');
    await expect(makePrefs().language.get()).resolves.toBe('en');
    storage.items.set('language', 'fr');
    await expect(makePrefs().language.get()).resolves.toBeNull();
  });

  test('the app-wide switch is kept as on or off; a value it did not write counts as nothing saved', async () => {
    await expect(makePrefs().notify.get()).resolves.toBeNull();
    await makePrefs().notify.set(false);
    expect(storage.items.get('notify')).toBe('off');
    await expect(makePrefs().notify.get()).resolves.toBe(false);
    await makePrefs().notify.set(true);
    expect(storage.items.get('notify')).toBe('on');
    await expect(makePrefs().notify.get()).resolves.toBe(true);
    storage.items.set('notify', 'true');
    await expect(makePrefs().notify.get()).resolves.toBeNull();
  });

  test('read as nothing, and saving does nothing, without a usable localStorage', async () => {
    await makePrefs().language.set('ja');
    storage.fail = true;
    await expect(makePrefs().language.get()).resolves.toBeNull();
    await expect(makePrefs().notify.get()).resolves.toBeNull();
    await expect(makePrefs().language.set('en')).resolves.toBeUndefined();
    await expect(makePrefs().notify.set(false)).resolves.toBeUndefined();
    noStorage();
    await expect(makePrefs().language.get()).resolves.toBeNull();
    await expect(makePrefs().notify.set(true)).resolves.toBeUndefined();
  });
});

describe('the Web notifications', () => {
  test('say that notifications are blocked, and stay blocked when asked', async () => {
    const n = makeNotifications();
    await expect(n.permission()).resolves.toBe('denied');
    await expect(n.requestPermission()).resolves.toBe('denied');
  });

  test('give no push token, so nothing is sent to the Killy server', async () => {
    await expect(makeNotifications().pushToken()).rejects.toThrow();
  });

  test('know no tap on a notification, and listening or preparing does nothing', async () => {
    const n = makeNotifications();
    expect(n.openedByTap()).toBe(false);
    expect(() => n.clearTap()).not.toThrow();
    const cb = jest.fn();
    n.onTap(cb)();
    n.onPushTokenChange(cb)();
    await expect(n.prepare('Killy')).resolves.toBeUndefined();
    await expect(n.openSettings()).resolves.toBeUndefined();
    expect(cb).not.toHaveBeenCalled();
  });
});

describe('what a Web build bundles', () => {
  // Metro bundles every module a file imports, so a Web file that imports one of these puts it in
  // the Web bundle, where it fails when it is loaded.
  const native = /from '(expo-secure-store|expo-notifications|expo-crypto|expo-camera|expo-constants|expo-localization)'/;
  const files = ['killy/deps.web.ts', 'prefs.web.ts', 'killy/notifications.web.ts', 'killy/web-storage.ts', 'killy/token-key.ts'];

  test.each(files)('%s imports no native module', (file) => {
    const source = fs.readFileSync(path.join(__dirname, '..', file), 'utf8');
    expect(source).not.toMatch(native);
  });

  // `./deps` in deps.web.ts is deps.web.ts itself in a Web build: a value imported from it is a cycle.
  test.each([
    ['killy/deps.web.ts', 'deps'],
    ['prefs.web.ts', 'prefs'],
    ['killy/notifications.web.ts', 'notifications'],
  ])('%s imports no value from %s, the file it replaces', (file, replaced) => {
    const source = fs.readFileSync(path.join(__dirname, '..', file), 'utf8');
    expect(source).not.toMatch(new RegExp(`^import (?!type )[^;]*from '\\./${replaced}';`, 'm'));
  });
});
