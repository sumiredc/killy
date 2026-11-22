import { act, fireEvent, renderRouter, screen, waitFor } from 'expo-router/testing-library';
import * as Localization from 'expo-localization';

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

const routes = {
  _layout: RootLayout,
  '(tabs)/_layout': TabsLayout,
  '(tabs)/index': BattleScreen,
  '(tabs)/settings': SettingsScreen,
};

const demoRoute = 'POST /app/v1/demo';
const demo = { id: 'd1', name: 'demo-linux-01', hostname: 'demo-linux-01', notify: true };

afterEach(() => {
  jest.restoreAllMocks();
});

/** Starts the app with savedToken as the device token (null: the phone registers at start). */
async function startApp(server: FakeServer, savedToken: string | null = 'dev-token', languageCode = 'en', demo = true) {
  server.stream();
  jest.spyOn(config, 'buildServerURL').mockReturnValue('https://killy.example');
  jest.spyOn(config, 'buildDemoEnabled').mockReturnValue(demo);
  jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode } as Localization.Locale]);
  jest.spyOn(prefs, 'makePrefs').mockReturnValue(new MemoryPrefs());
  let token = savedToken;
  jest.spyOn(deps, 'makeDeps').mockReturnValue({
    fetch: server.fetch,
    tokens: { get: async () => token, set: async (t) => void (token = t) },
    random: () => 0,
    randomBytes: (n) => new Uint8Array(n),
    notifications: new FakeNotifications(),
  });
  await renderRouter(routes, { initialUrl: '/settings' });
  await act(settle);
  await fireEvent.press(screen.getByText(languageCode === 'ja' ? 'サーバーを追加' : 'Add server'));
  expect(await screen.findByTestId('add-server-sheet')).toBeOnTheScreen();
}

describe('Try the demo server', () => {
  test('the button is below the form', async () => {
    await startApp(new FakeServer());
    expect(screen.getByRole('button', { name: 'Try the demo server' })).toBeOnTheScreen();
  });

  test('in Japanese', async () => {
    await startApp(new FakeServer(), 'dev-token', 'ja');
    expect(screen.getByRole('button', { name: 'デモサーバーを試す' })).toBeOnTheScreen();
  });

  test('it shows Adding…, then Added ‹name›, then closes, with no code', async () => {
    const server = new FakeServer();
    await startApp(server);
    const held = server.hold(demoRoute);
    await fireEvent.press(screen.getByRole('button', { name: 'Try the demo server' }));
    expect(await screen.findByText('Adding…')).toBeOnTheScreen();
    expect(server.requests.at(-1)).toEqual(expect.objectContaining({ route: demoRoute, authorization: 'Bearer dev-token' }));
    await act(() => held.resolve(200, demo));
    expect(await screen.findByText('Added demo-linux-01')).toBeOnTheScreen();
    await waitFor(() => expect(screen.queryByTestId('add-server-sheet')).toBeNull(), { timeout: 3000 });
  });

  test('a saved token the Killy server does not know is replaced, and the demo server is added', async () => {
    const server = new FakeServer();
    server.respondInOrder(demoRoute, [[401, { error: 'unauthorized' }], [200, demo]]);
    server.respond('POST /app/v1/devices', 200, { device_token: 'tok-new' });
    await startApp(server);
    await fireEvent.press(screen.getByRole('button', { name: 'Try the demo server' }));
    expect(await screen.findByText('Added demo-linux-01')).toBeOnTheScreen();
    expect(server.requests.filter((r) => r.route === demoRoute).map((r) => r.authorization)).toEqual(['Bearer dev-token', 'Bearer tok-new']);
    expect(server.count('POST /app/v1/devices')).toBe(1);
  });

  test.each([
    ['demo_not_configured', (s: FakeServer) => s.respond(demoRoute, 404, { error: 'demo_not_configured' }), 'The demo server is not available right now'],
    ['unauthorized', (s: FakeServer) => s.respond(demoRoute, 401, { error: 'unauthorized' }), 'The server could not be added. Try again in a while.'],
    ['unavailable', (s: FakeServer) => s.respond(demoRoute, 503, { error: 'unavailable' }), 'The server could not be added. Try again in a while.'],
    ['an answer that is not JSON', (s: FakeServer) => s.respondText(demoRoute, 502, '<html>502</html>'), 'The server could not be added. Try again in a while.'],
    ['no answer', (s: FakeServer) => s.unreachable(demoRoute), 'Cannot reach the Killy server. Check your network.'],
  ] as const)('%s shows its message and adds nothing', async (_, answer, message) => {
    const server = new FakeServer();
    await startApp(server);
    answer(server);
    await fireEvent.press(screen.getByRole('button', { name: 'Try the demo server' }));
    expect(await screen.findByText(message)).toBeOnTheScreen();
    expect(screen.getByTestId('add-server-sheet')).toBeOnTheScreen();
    expect(screen.queryByText(/^Added /)).toBeNull();
  });

  test('in Japanese, a missing demo server says so', async () => {
    const server = new FakeServer();
    await startApp(server, 'dev-token', 'ja');
    server.respond(demoRoute, 404, { error: 'demo_not_configured' });
    await fireEvent.press(screen.getByRole('button', { name: 'デモサーバーを試す' }));
    expect(await screen.findByText('デモサーバーは今使えません')).toBeOnTheScreen();
  });

  test('while the phone is still registering, the button cannot be pressed and sends nothing', async () => {
    const server = new FakeServer();
    server.hold('POST /app/v1/devices');
    await startApp(server, null);
    const button = screen.getByRole('button', { name: 'Try the demo server' });
    expect(button).toBeDisabled();
    await fireEvent.press(button);
    expect(server.count(demoRoute)).toBe(0);
  });

  test.each([
    ['too many registrations from this network', 429, { error: 'rate_limited' }, /too many registrations from this network/],
    ['the Killy server could not be reached at start', 503, { error: 'unavailable' }, /not registered with the Killy server yet/],
  ] as const)('when %s, nothing is sent and the phone says why', async (_, status, body, message) => {
    const server = new FakeServer();
    server.respond('POST /app/v1/devices', status, body);
    await startApp(server, null);
    await fireEvent.press(screen.getByRole('button', { name: 'Try the demo server' }));
    expect(await screen.findByText(message)).toBeOnTheScreen();
    expect(server.count(demoRoute)).toBe(0);
  });
});

describe('a build without the public demo', () => {
  test('has no Try the demo server button, and adding a server by code still works', async () => {
    await startApp(new FakeServer(), 'dev-token', 'en', false);
    expect(screen.queryByRole('button', { name: 'Try the demo server' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Add' })).toBeOnTheScreen();
  });
});
