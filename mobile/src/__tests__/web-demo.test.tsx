import { act, fireEvent, renderRouter, screen } from 'expo-router/testing-library';
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
import { FakeServer, type FakeStream, settle } from '../testing/fake-server';

jest.mock('expo-localization', () => ({ getLocales: jest.fn(() => [{ languageCode: 'en' }]) }));

const routes = {
  _layout: RootLayout,
  '(tabs)/_layout': TabsLayout,
  '(tabs)/index': BattleScreen,
  '(tabs)/settings': SettingsScreen,
};

const demoRoute = 'POST /app/v1/demo';
const devicesRoute = 'POST /app/v1/devices';
const demo = { id: 'd1', name: 'demo-linux-01', hostname: 'demo-linux-01', notify: true };
const parent = { pid: 3801, starttime: 11, cmd: 'node evil_daemon.js', zombies: [3802, 3803], first_detected_at: '2026-11-13T00:00:00Z' };

afterEach(() => jest.restoreAllMocks());

type Options = {
  os?: 'ios' | 'web';
  /** EXPO_PUBLIC_KILLY_WEB_DEMO=1 */
  webDemo?: boolean;
  /** The device token the phone has saved; null registers at start. */
  savedToken?: string | null;
  url?: string;
};

/** Starts the app as the Web build (or another) with the Killy server `server`; returns its event stream. */
async function startApp(server: FakeServer, { os = 'web', webDemo = true, savedToken = 'dev-token', url = '/' }: Options = {}): Promise<FakeStream> {
  jest.replaceProperty(Platform, 'OS', os);
  const stream = server.stream();
  jest.spyOn(config, 'buildServerURL').mockReturnValue('https://killy.example');
  jest.spyOn(config, 'buildWebDemoEnabled').mockReturnValue(webDemo);
  jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode: 'en' } as Localization.Locale]);
  jest.spyOn(prefs, 'makePrefs').mockReturnValue(new MemoryPrefs());
  let token = savedToken;
  jest.spyOn(deps, 'makeDeps').mockReturnValue({
    fetch: server.fetch,
    tokens: { get: async () => token, set: async (t) => void (token = t) },
    random: () => 0,
    randomBytes: (n) => new Uint8Array(n),
    notifications: new FakeNotifications(),
  });
  await renderRouter(routes, { initialUrl: url });
  await act(settle);
  return stream;
}

const stateOf = (servers: object[]) => `event: state\ndata: ${JSON.stringify({ servers })}\n\n`;

describe('the Web version in its demo-only form', () => {
  test('adds the demo server once with the saved token, and its parent is on the battle screen', async () => {
    const server = new FakeServer();
    server.respond(demoRoute, 200, demo);
    const stream = await startApp(server);
    expect(server.count(demoRoute)).toBe(1);
    expect(server.requests.find((r) => r.route === demoRoute)?.authorization).toBe('Bearer dev-token');
    expect(server.count(devicesRoute)).toBe(0);
    await act(() => stream.send(stateOf([{ ...demo, disconnected: false, parents: [parent] }])));
    expect(screen.getByTestId('battle-server')).toHaveTextContent('demo-linux-01');
    expect(screen.getByTestId('battle-zombies')).toHaveTextContent('2');
    expect(screen.getByText('parent PID 3801')).toBeOnTheScreen();
  });

  test('a browser with nothing saved registers first, and adds the demo server with the new token', async () => {
    const server = new FakeServer();
    server.respond(devicesRoute, 200, { device_token: 'tok-new' });
    server.respond(demoRoute, 200, demo);
    await startApp(server, { savedToken: null });
    const sent = server.requests.map((r) => r.route).filter((r) => r !== 'GET /app/v1/events');
    expect(sent).toEqual([devicesRoute, demoRoute]);
    expect(server.requests.find((r) => r.route === demoRoute)?.authorization).toBe('Bearer tok-new');
  });

  test('adds the demo server once per start, however often the screens change', async () => {
    // The Killy server keeps a second call as the one link, but a start is one call, not one per screen.
    const server = new FakeServer();
    server.respond(demoRoute, 200, demo);
    await startApp(server);
    await fireEvent.press(screen.getByLabelText('Settings'));
    await fireEvent.press(screen.getByLabelText('Battle'));
    await act(settle);
    expect(server.count(demoRoute)).toBe(1);
  });

  test('while the browser is still registering it asks for nothing and complains of nothing, then adds the demo server', async () => {
    const server = new FakeServer();
    const held = server.hold(devicesRoute);
    server.respond(demoRoute, 200, demo);
    await startApp(server, { savedToken: null });
    expect(server.count(demoRoute)).toBe(0);
    expect(screen.queryByTestId('web-demo-notice')).toBeNull();
    await act(() => held.resolve(200, { device_token: 'tok-late' }));
    expect(server.count(demoRoute)).toBe(1);
    expect(screen.queryByTestId('web-demo-notice')).toBeNull();
  });

  test('Settings has no Add server, and no pairing code is asked for anywhere', async () => {
    const server = new FakeServer();
    server.respond(demoRoute, 200, demo);
    await startApp(server, { url: '/settings' });
    expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
    expect(screen.queryByText('Add server')).toBeNull();
    expect(screen.queryByLabelText('Pairing code')).toBeNull();
    expect(screen.getByText('Language')).toBeOnTheScreen();
  });

  test('before the demo server is there, the battle screen does not say to add a server in Settings', async () => {
    const server = new FakeServer();
    server.hold(demoRoute);
    await startApp(server);
    expect(screen.getByTestId('battle-screen')).toBeOnTheScreen();
    expect(screen.queryByText('No servers yet')).toBeNull();
    expect(screen.queryByText(/Add a server in Settings/)).toBeNull();
  });

  test.each([
    ['the demo server is not configured', (s: FakeServer) => s.respond(demoRoute, 404, { error: 'demo_not_configured' }), 'The demo server is not available right now'],
    ['the Killy server cannot be reached', (s: FakeServer) => s.unreachable(demoRoute), 'Cannot reach the Killy server. Check your network.'],
    ['the Killy server answers an error', (s: FakeServer) => s.respond(demoRoute, 503, { error: 'unavailable' }), 'The server could not be added. Try again in a while.'],
    ['the answer is not JSON', (s: FakeServer) => s.respondText(demoRoute, 502, '<html>502</html>'), 'The server could not be added. Try again in a while.'],
  ])('when %s, the battle screen says why, and there is nothing to add by hand', async (_name, answer, message) => {
    const server = new FakeServer();
    answer(server);
    await startApp(server);
    expect(screen.getByText(message)).toBeOnTheScreen();
    expect(server.count(demoRoute)).toBe(1);
    expect(screen.queryByText(/Add a server in Settings/)).toBeNull();
  });

  test.each([
    ['too many registrations from this network', 429, { error: 'rate_limited' }, /too many registrations from this network/],
    ['the Killy server could not be reached at start', 503, { error: 'unavailable' }, /not registered with the Killy server yet/],
  ] as const)('when registering fails (%s), the demo server is not asked for and the screen says why', async (_name, status, body, message) => {
    const server = new FakeServer();
    server.respond(devicesRoute, status, body);
    server.respond(demoRoute, 200, demo);
    await startApp(server, { savedToken: null });
    expect(screen.getByText(message)).toBeOnTheScreen();
    expect(server.count(demoRoute)).toBe(0);
  });

  test('says nothing is wrong when the demo server is added', async () => {
    const server = new FakeServer();
    server.respond(demoRoute, 200, demo);
    await startApp(server);
    expect(screen.queryByText(/^The demo server is not available/)).toBeNull();
    expect(screen.queryByText(/Cannot reach the Killy server/)).toBeNull();
    expect(screen.queryByText(/could not be added/)).toBeNull();
  });
});

describe('a build that is not the demo-only Web version', () => {
  test('the Web version without the variable adds nothing by itself, and Settings has Add server', async () => {
    const server = new FakeServer();
    server.respond(demoRoute, 200, demo);
    await startApp(server, { webDemo: false, url: '/settings' });
    expect(server.count(demoRoute)).toBe(0);
    expect(await screen.findByText('Add server')).toBeOnTheScreen();
  });

  test('an iOS or Android build adds nothing by itself, even if the variable is set', async () => {
    const server = new FakeServer();
    server.respond(demoRoute, 200, demo);
    await startApp(server, { os: 'ios', webDemo: true, url: '/settings' });
    expect(server.count(demoRoute)).toBe(0);
    expect(await screen.findByText('Add server')).toBeOnTheScreen();
  });

  test('says "No servers yet" as before when there is none', async () => {
    await startApp(new FakeServer(), { os: 'ios', webDemo: false });
    expect(screen.getByText('No servers yet')).toBeOnTheScreen();
  });
});

describe('the frame on a wide screen', () => {
  test.each([
    ['web', 430],
    ['ios', undefined],
  ] as const)('on %s the server sheet is at most %s wide', async (os, maxWidth) => {
    const server = new FakeServer();
    server.respond(demoRoute, 200, demo);
    const stream = await startApp(server, { os, webDemo: os === 'web', url: '/settings' });
    await act(() => stream.send(stateOf([{ ...demo, disconnected: false, parents: [] }])));
    await fireEvent.press(await screen.findByTestId('server-row'));
    const sheet = await screen.findByTestId('server-sheet');
    const style = StyleSheet.flatten(sheet.props.style);
    if (maxWidth === undefined) expect(style).not.toHaveProperty('maxWidth');
    else expect(style).toMatchObject({ maxWidth, alignSelf: 'center' });
  });

  test('on the Web the whole app is in a frame at most 430px wide, centred', async () => {
    const server = new FakeServer();
    server.respond(demoRoute, 200, demo);
    await startApp(server);
    const frame = screen.getByTestId('phone-frame');
    expect(StyleSheet.flatten(frame.props.style)).toMatchObject({ maxWidth: 430, width: '100%' });
    expect(StyleSheet.flatten(screen.getByTestId('phone-frame-outer').props.style)).toMatchObject({ alignItems: 'center' });
    expect(screen.getByTestId('battle-screen')).toBeOnTheScreen();
  });

  test('on iOS and Android there is no frame', async () => {
    await startApp(new FakeServer(), { os: 'ios', webDemo: false });
    expect(screen.queryByTestId('phone-frame')).toBeNull();
    expect(screen.getByTestId('battle-screen')).toBeOnTheScreen();
  });
});
