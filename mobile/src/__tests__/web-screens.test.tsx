import { act, fireEvent, renderRouter, screen, within } from 'expo-router/testing-library';
import * as Localization from 'expo-localization';
import { Platform, StyleSheet } from 'react-native';

import * as config from '../config';
import RootLayout from '../app/_layout';
import TabsLayout from '../app/(tabs)/_layout';
import BattleScreen from '../app/(tabs)/index';
import SettingsScreen from '../app/(tabs)/settings';
import * as deps from '../killy/deps';
import * as prefs from '../prefs';
import { FakeNotifications, MemoryPrefs } from '../testing/fake-notifications';
import { FakeServer, settle } from '../testing/fake-server';

jest.mock('expo-localization', () => ({ getLocales: jest.fn(() => [{ languageCode: 'en' }]) }));
jest.mock('expo-camera', () => ({
  useCameraPermissions: () => [null, async () => ({ granted: true })],
  CameraView: () => null,
}));

// In a Web build the './notifications' that deps.web.ts imports is notifications.web.ts.
jest.mock('../killy/notifications', () => jest.requireActual('../killy/notifications.web'));

const routes = {
  _layout: RootLayout,
  '(tabs)/_layout': TabsLayout,
  '(tabs)/index': BattleScreen,
  '(tabs)/settings': SettingsScreen,
};

afterEach(() => jest.restoreAllMocks());

async function startApp(os: 'ios' | 'web') {
  jest.replaceProperty(Platform, 'OS', os);
  const server = new FakeServer();
  server.stream();
  jest.spyOn(config, 'buildServerURL').mockReturnValue('https://killy.example');
  jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode: 'en' } as Localization.Locale]);
  jest.spyOn(prefs, 'makePrefs').mockReturnValue(new MemoryPrefs());
  jest.spyOn(deps, 'makeDeps').mockReturnValue({
    fetch: server.fetch,
    tokens: { get: async () => 'dev-token', set: async () => {} },
    random: () => 0,
    randomBytes: (n) => new Uint8Array(n),
    notifications: new FakeNotifications(),
  });
  await renderRouter(routes, { initialUrl: '/' });
  await act(settle);
}

describe.each([
  ['iOS', 'ios', true],
  ['the Web', 'web', false],
] as const)('on %s', (_name, os, hasNative) => {
  test(`the Notifications switch ${hasNative ? 'is' : 'is not'} in Settings`, async () => {
    await startApp(os);
    await fireEvent.press(screen.getByLabelText('Settings'));
    expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
    // Language is app-wide on every platform; only what a browser cannot do is left out.
    expect(screen.getByText('Language')).toBeOnTheScreen();
    expect(screen.queryByLabelText('Notifications') !== null).toBe(hasNative);
    expect(screen.queryByText('Notifications') !== null).toBe(hasNative);
  });

  test(`Scan QR code ${hasNative ? 'is' : 'is not'} in the Add server sheet`, async () => {
    await startApp(os);
    await fireEvent.press(screen.getByLabelText('Settings'));
    await fireEvent.press(await screen.findByText('Add server'));
    const sheet = within(await screen.findByTestId('add-server-sheet'));
    expect(sheet.getByLabelText('Pairing code')).toBeOnTheScreen();
    expect(sheet.queryByRole('button', { name: 'Scan QR code' }) !== null).toBe(hasNative);
  });
});

describe('a browser that keeps nothing', () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
  afterEach(() => {
    if (original) Object.defineProperty(globalThis, 'localStorage', original);
    else delete (globalThis as { localStorage?: unknown }).localStorage;
  });

  test('registers, keeps its token for the page and opens the event stream, as private browsing would leave it', async () => {
    jest.replaceProperty(Platform, 'OS', 'web');
    // Every call throws, as localStorage does in a browser that refuses storage.
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      value: {
        getItem: () => {
          throw new DOMException('denied', 'SecurityError');
        },
        setItem: () => {
          throw new DOMException('denied', 'QuotaExceededError');
        },
      },
    });
    const server = new FakeServer();
    server.stream();
    server.respond('POST /app/v1/devices', 200, { device_token: 'tok-page' });
    jest.spyOn(globalThis, 'fetch').mockImplementation(server.fetch as never);
    jest.spyOn(config, 'buildServerURL').mockReturnValue('https://killy.example');
    jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode: 'en' } as Localization.Locale]);
    jest.spyOn(deps, 'makeDeps').mockImplementation(jest.requireActual('../killy/deps.web').makeDeps);
    jest.spyOn(prefs, 'makePrefs').mockImplementation(jest.requireActual('../prefs.web').makePrefs);
    await renderRouter(routes, { initialUrl: '/' });
    await act(settle);
    expect(server.count('POST /app/v1/devices')).toBe(1);
    const events = server.requests.find((r) => r.route === 'GET /app/v1/events');
    expect(events?.authorization).toBe('Bearer tok-page');
    expect(screen.getByTestId('battle-screen')).toBeOnTheScreen();
    // Settings still opens, with the language, which cannot be saved either.
    await fireEvent.press(screen.getByLabelText('Settings'));
    expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
  });
});

describe('the kill button, which is held for a second', () => {
  test('on the Web, a long press selects no text and opens no menu', async () => {
    await startApp('web');
    const button = screen.getByRole('button', { name: 'Hold to kill' });
    expect(StyleSheet.flatten(button.props.style)).toMatchObject({ userSelect: 'none', WebkitTouchCallout: 'none' });
    const preventDefault = jest.fn();
    button.props.onContextMenu({ preventDefault });
    expect(preventDefault).toHaveBeenCalled();
  });

  test('on iOS it is as it was', async () => {
    await startApp('ios');
    const button = screen.getByRole('button', { name: 'Hold to kill' });
    expect(StyleSheet.flatten(button.props.style)).not.toHaveProperty('userSelect');
    expect(button.props.onContextMenu).toBeUndefined();
  });
});
