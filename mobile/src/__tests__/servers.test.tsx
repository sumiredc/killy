import { act, cleanup, fireEvent, renderRouter, screen, waitFor, within } from 'expo-router/testing-library';
import * as Localization from 'expo-localization';

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

class MemoryTokens {
  saved: string | null = null;
  sets = 0;
  get = async () => this.saved;
  set = async (token: string) => {
    this.sets++;
    this.saved = token;
  };
}

afterEach(() => {
  jest.restoreAllMocks();
});

async function startApp(server: FakeServer, tokens = new MemoryTokens(), languageCode = 'en') {
  jest.spyOn(config, 'buildServerURL').mockReturnValue('https://killy.example');
  jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode } as Localization.Locale]);
  jest.spyOn(prefs, 'makePrefs').mockReturnValue(new MemoryPrefs());
  jest.spyOn(deps, 'makeDeps').mockReturnValue({ fetch: server.fetch, tokens, random: () => 0, randomBytes: (n) => new Uint8Array(n), notifications: new FakeNotifications() });
  const r = await renderRouter(routes, { initialUrl: '/' });
  await act(settle);
  return r;
}

async function openSettings() {
  await fireEvent.press(screen.getByLabelText('Settings'));
  expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
}

async function openAddSheet() {
  await openSettings();
  await fireEvent.press(screen.getByText('Add server'));
  expect(await screen.findByTestId('add-server-sheet')).toBeOnTheScreen();
}

function registered(server: FakeServer): MemoryTokens {
  const tokens = new MemoryTokens();
  tokens.saved = 'dev-token';
  server.stream();
  return tokens;
}

const web = { id: 'a1', name: 'web-01', hostname: 'ip-10-0-0-1', notify: true, disconnected: false };

function parent(pid: number, zombies: number[]) {
  return { pid, starttime: pid * 10, cmd: `node worker-${pid}.js`, zombies, first_detected_at: '2026-11-13T00:00:00Z' };
}

async function sendState(stream: FakeStream, state: unknown) {
  await act(() => stream.send(`event: state\ndata: ${JSON.stringify(state)}\n\n`));
}

describe('device registration', () => {
  test('the first start registers the device without a push token and saves the device token', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/devices', 200, { device_token: 'tok-1' });
    server.stream();
    const tokens = new MemoryTokens();
    await startApp(server, tokens);
    expect(server.requests[0]).toEqual(expect.objectContaining({ route: 'POST /app/v1/devices', body: {} }));
    expect(tokens.saved).toBe('tok-1');
    await waitFor(() => expect(server.count('GET /app/v1/events')).toBe(1));
    expect(server.requests.at(-1)?.authorization).toBe('Bearer tok-1');
  });

  test('a saved token the Killy server does not know is replaced, and Add then works', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/devices', 200, { device_token: 'tok-new' });
    server.respondInOrder('POST /app/v1/pairings/claim', [
      [401, { error: 'unauthorized' }],
      [200, { id: 'a1', name: 'my box', hostname: 'ip-10-0-0-1', notify: true }],
    ]);
    const tokens = registered(server);
    await startApp(server, tokens);
    await openAddSheet();
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'K7Q4XP');
    await fireEvent.press(screen.getByRole('button', { name: 'Add' }));
    expect(await screen.findByText('Added my box')).toBeOnTheScreen();
    expect(tokens.saved).toBe('tok-new');
    expect(server.count('POST /app/v1/devices')).toBe(1);
    const claims = server.requests.filter((r) => r.route === 'POST /app/v1/pairings/claim');
    expect(claims.map((r) => r.authorization)).toEqual(['Bearer dev-token', 'Bearer tok-new']);
  });

  test('a start with a saved device token does not register again', async () => {
    const server = new FakeServer();
    const tokens = registered(server);
    await startApp(server, tokens);
    expect(server.count('POST /app/v1/devices')).toBe(0);
    await waitFor(() => expect(server.count('GET /app/v1/events')).toBe(1));
  });

  test('a start whose saved token cannot be read does not register a new device over it', async () => {
    const server = new FakeServer();
    const tokens = new MemoryTokens();
    tokens.get = async () => {
      throw new Error('User interaction is not allowed');
    };
    await startApp(server, tokens);
    expect(server.count('POST /app/v1/devices')).toBe(0);
    expect(tokens.sets).toBe(0);
    expect(server.count('GET /app/v1/events')).toBe(0);
  });

  test('while registration is still in progress, Add stays disabled and sends nothing', async () => {
    const server = new FakeServer();
    server.hold('POST /app/v1/devices');
    await startApp(server);
    await openAddSheet();
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'K7Q4XP');
    const add = screen.getByRole('button', { name: 'Add' });
    expect(add).toBeDisabled();
    await fireEvent.press(add);
    expect(server.count('POST /app/v1/pairings/claim')).toBe(0);
    expect(screen.queryByText(/not registered with the Killy server/)).toBeNull();
  });

  test('a 429 is shown at start, saves no token, and the next start registers again', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/devices', 429, { error: 'rate_limited' });
    const tokens = new MemoryTokens();
    await startApp(server, tokens);
    expect(await screen.findByText(/too many registrations from this network/)).toBeOnTheScreen();
    expect(tokens.sets).toBe(0);
    expect(server.count('GET /app/v1/events')).toBe(0);
    await cleanup();

    server.respond('POST /app/v1/devices', 200, { device_token: 'tok-2' });
    server.stream();
    await startApp(server, tokens);
    expect(server.count('POST /app/v1/devices')).toBe(2);
    expect(tokens.saved).toBe('tok-2');
    expect(screen.queryByText(/too many registrations from this network/)).toBeNull();
  });

  test('after a 429, Add sends no claim and says the phone could not register', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/devices', 429, { error: 'rate_limited' });
    await startApp(server);
    await openAddSheet();
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'K7Q4XP');
    const before = server.requests.length;
    await fireEvent.press(screen.getByText('Add'));
    await act(settle);
    expect(server.requests.length).toBe(before);
    expect(server.count('POST /app/v1/pairings/claim')).toBe(0);
    expect(within(screen.getByTestId('add-server-sheet')).getByText(/too many registrations from this network/)).toBeOnTheScreen();
  });

  test('when the Killy server could not be reached at start, Add sends no claim and says the phone is not registered', async () => {
    const server = new FakeServer();
    server.unreachable('POST /app/v1/devices');
    await startApp(server);
    await openAddSheet();
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'K7Q4XP');
    await fireEvent.press(screen.getByText('Add'));
    await act(settle);
    expect(server.count('POST /app/v1/pairings/claim')).toBe(0);
    expect(within(screen.getByTestId('add-server-sheet')).getByText(/not registered with the Killy server/)).toBeOnTheScreen();
  });
});

describe('Add server sheet', () => {
  test('the code field keeps only upper-case letters and digits, at most 6', async () => {
    const server = new FakeServer();
    await startApp(server, registered(server));
    await openAddSheet();
    const code = screen.getByLabelText('Pairing code');
    await fireEvent.changeText(code, 'k7-q 4!xpz9');
    expect(code).toHaveDisplayValue('K7Q4XP');
  });

  test('full-width letters and digits from a Japanese keyboard become the code', async () => {
    const server = new FakeServer();
    await startApp(server, registered(server));
    await openAddSheet();
    const code = screen.getByLabelText('Pairing code');
    await fireEvent.changeText(code, 'ａ３ｅＣ４４');
    expect(code).toHaveDisplayValue('A3EC44');
  });

  test('Add is enabled only at 6 characters', async () => {
    const server = new FakeServer();
    await startApp(server, registered(server));
    await openAddSheet();
    const code = screen.getByLabelText('Pairing code');
    const add = screen.getByRole('button', { name: 'Add' });
    expect(add).toBeDisabled();
    await fireEvent.changeText(code, 'K7Q4X');
    expect(add).toBeDisabled();
    await fireEvent.press(add);
    expect(server.count('POST /app/v1/pairings/claim')).toBe(0);
    await fireEvent.changeText(code, 'K7Q4XP');
    expect(add).toBeEnabled();
  });

  test('a claim shows Adding…, then Added ‹name› without waiting for the agent, then closes', async () => {
    const server = new FakeServer();
    await startApp(server, registered(server));
    const claim = server.hold('POST /app/v1/pairings/claim');
    await openAddSheet();
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'K7Q4XP');
    await fireEvent.changeText(screen.getByLabelText('Display name (optional)'), 'my box');
    await fireEvent.press(screen.getByText('Add'));
    expect(await screen.findByText('Adding…')).toBeOnTheScreen();
    expect(server.requests.at(-1)).toEqual(
      expect.objectContaining({ route: 'POST /app/v1/pairings/claim', body: { code: 'K7Q4XP', name: 'my box' }, authorization: 'Bearer dev-token' }),
    );
    await act(() => claim.resolve(200, { id: 'a1', name: 'my box', hostname: 'ip-10-0-0-1', notify: true }));
    expect(await screen.findByText('Added my box')).toBeOnTheScreen();
    await waitFor(() => expect(screen.queryByTestId('add-server-sheet')).toBeNull(), { timeout: 3000 });
  });

  test('once added, Killy shows without the scythe, bowing, with 💜 over the name (DESIGN: Killy\'s reactions, server added)', async () => {
    const server = new FakeServer();
    await startApp(server, registered(server));
    server.respond('POST /app/v1/pairings/claim', 200, { id: 'a1', name: 'my box', hostname: 'ip-10-0-0-1', notify: true });
    await openAddSheet();
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'K7Q4XP');
    await fireEvent.press(screen.getByText('Add'));
    expect(await screen.findByText('Added my box')).toBeOnTheScreen();
    const killy = within(screen.getByTestId('paired-killy'));
    expect(killy.getByTestId('killy-art')).toBeOnTheScreen();
    expect(killy.queryByTestId('killy-scythe')).toBeNull();
    expect(killy.getByText('💜')).toBeOnTheScreen();
    // No bubble and no words from Killy: the name is the system's line below.
    expect(killy.queryByTestId('killy-bubble')).toBeNull();
  });

  test('the display name takes at most 100 characters, as the Killy server accepts', async () => {
    const server = new FakeServer();
    await startApp(server, registered(server));
    await openAddSheet();
    expect(screen.getByLabelText('Display name (optional)')).toHaveProp('maxLength', 100);
  });

  test('without a display name the claim sends none', async () => {
    const server = new FakeServer();
    await startApp(server, registered(server));
    server.respond('POST /app/v1/pairings/claim', 200, { id: 'a1', name: 'ip-10-0-0-1', hostname: 'ip-10-0-0-1', notify: true });
    await openAddSheet();
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'K7Q4XP');
    await fireEvent.press(screen.getByText('Add'));
    expect(await screen.findByText('Added ip-10-0-0-1')).toBeOnTheScreen();
    expect(server.requests.at(-1)?.body).toEqual({ code: 'K7Q4XP' });
  });

  const rerun = /Run killy agent pair on the server again/;
  test.each([
    ['code_unknown', (s: FakeServer) => s.respond('POST /app/v1/pairings/claim', 404, { error: 'code_unknown' }), rerun],
    ['code_used', (s: FakeServer) => s.respond('POST /app/v1/pairings/claim', 409, { error: 'code_used' }), rerun],
    ['code_expired', (s: FakeServer) => s.respond('POST /app/v1/pairings/claim', 410, { error: 'code_expired' }), rerun],
    ['rate_limited', (s: FakeServer) => s.respond('POST /app/v1/pairings/claim', 429, { error: 'rate_limited' }), /Too many attempts/],
    ['invalid_request', (s: FakeServer) => s.respond('POST /app/v1/pairings/claim', 400, { error: 'invalid_request' }), /could not be added/],
    ['unauthorized', (s: FakeServer) => s.respond('POST /app/v1/pairings/claim', 401, { error: 'unauthorized' }), /could not be added/],
    ['unavailable', (s: FakeServer) => s.respond('POST /app/v1/pairings/claim', 503, { error: 'unavailable' }), /could not be added/],
    ['an answer that is not JSON', (s: FakeServer) => s.respondText('POST /app/v1/pairings/claim', 502, '<html>502</html>'), /could not be added/],
    ['no answer', (s: FakeServer) => s.unreachable('POST /app/v1/pairings/claim'), /Cannot reach the Killy server/],
  ] as const)('%s shows its message and adds nothing', async (_, answer, message) => {
    const server = new FakeServer();
    await startApp(server, registered(server));
    answer(server);
    await openAddSheet();
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'K7Q4XP');
    await fireEvent.press(screen.getByText('Add'));
    expect(await screen.findByText(message)).toBeOnTheScreen();
    expect(screen.getByTestId('add-server-sheet')).toBeOnTheScreen();
    expect(screen.queryByText(/^Added /)).toBeNull();
  });

  test('in Japanese, a used code tells the operator to run killy agent pair again', async () => {
    const server = new FakeServer();
    await startApp(server, registered(server), 'ja');
    server.respond('POST /app/v1/pairings/claim', 409, { error: 'code_used' });
    await fireEvent.press(screen.getByLabelText('設定'));
    await fireEvent.press(await screen.findByText('サーバーを追加'));
    await fireEvent.changeText(screen.getByLabelText('ペアリングコード'), 'K7Q4XP');
    await fireEvent.press(screen.getByText('追加する'));
    expect(await screen.findByText(/killy agent pair をもう一度実行してください/)).toBeOnTheScreen();
  });

  test('Cancel closes the sheet without sending anything', async () => {
    const server = new FakeServer();
    await startApp(server, registered(server));
    await openAddSheet();
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'K7Q4XP');
    await fireEvent.press(screen.getByText('Cancel'));
    await waitFor(() => expect(screen.queryByTestId('add-server-sheet')).toBeNull());
    expect(server.count('POST /app/v1/pairings/claim')).toBe(0);
  });
});

describe('Settings servers and the state stream', () => {
  test('rows show each server by its name on this phone with the sub-line for its state, and a badge only with zombies', async () => {
    const server = new FakeServer();
    const tokens = registered(server);
    const stream = server.stream();
    await startApp(server, tokens);
    await openSettings();
    expect(screen.getByText('No servers yet. Add one to start watching for zombie processes.')).toBeOnTheScreen();
    await sendState(stream, {
      servers: [
        { ...web, parents: [parent(100, [101, 102, 103]), parent(200, [201, 202])] },
        { ...web, id: 'a2', name: 'db-01', parents: [] },
        { ...web, id: 'a3', name: 'new-box', disconnected: true, parents: [] },
      ],
    });
    expect(screen.getByText('web-01')).toBeOnTheScreen();
    expect(screen.getByText('5 zombie processes / 2 parents')).toBeOnTheScreen();
    expect(screen.getByText('db-01')).toBeOnTheScreen();
    expect(screen.getByText('No zombie processes')).toBeOnTheScreen();
    expect(screen.getByText('new-box')).toBeOnTheScreen();
    expect(screen.getByText('Latest status unavailable')).toBeOnTheScreen();
    expect(screen.getAllByTestId('server-badge').map((b) => b.props.children)).toEqual([5]);
    expect(screen.queryByText('No servers yet. Add one to start watching for zombie processes.')).toBeNull();
  });

  test('a new state replaces the old one: servers and parents it does not have are gone', async () => {
    const server = new FakeServer();
    const tokens = registered(server);
    const stream = server.stream();
    await startApp(server, tokens);
    await openSettings();
    await sendState(stream, {
      servers: [
        { ...web, parents: [parent(100, [101, 102]), parent(200, [201])] },
        { ...web, id: 'a2', name: 'db-01', parents: [] },
      ],
    });
    expect(screen.getByText('3 zombie processes / 2 parents')).toBeOnTheScreen();
    await sendState(stream, { servers: [{ ...web, parents: [parent(200, [201])] }] });
    expect(screen.getByText('1 zombie processes / 1 parents')).toBeOnTheScreen();
    expect(screen.queryByText('3 zombie processes / 2 parents')).toBeNull();
    expect(screen.queryByText('db-01')).toBeNull();
  });

  test('while the stream is down the last state stays on screen, with no connection indicator', async () => {
    const server = new FakeServer();
    const tokens = registered(server);
    const stream = server.stream();
    await startApp(server, tokens);
    await openSettings();
    await sendState(stream, { servers: [{ ...web, parents: [parent(100, [101, 102])] }] });
    server.unreachable('GET /app/v1/events');
    await act(() => stream.end());
    // The reconnection 1 s later fails too; the screen still shows what it had.
    await waitFor(() => expect(server.count('GET /app/v1/events')).toBe(2), { timeout: 3000 });
    await act(settle);
    expect(screen.getByText('web-01')).toBeOnTheScreen();
    expect(screen.getByText('2 zombie processes / 1 parents')).toBeOnTheScreen();
    expect(screen.queryByText(/offline|disconnected|reconnect/i)).toBeNull();
  });

  test('a server added from the sheet appears when the stream sends the new state', async () => {
    const server = new FakeServer();
    const tokens = registered(server);
    const stream = server.stream();
    await startApp(server, tokens);
    server.respond('POST /app/v1/pairings/claim', 200, { id: 'a3', name: 'new-box', hostname: 'new-box', notify: true });
    await openAddSheet();
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'K7Q4XP');
    await fireEvent.press(screen.getByText('Add'));
    expect(await screen.findByText('Added new-box')).toBeOnTheScreen();
    // The agent has not reported yet, so the Killy server lists it as disconnected.
    await sendState(stream, { servers: [{ ...web, id: 'a3', name: 'new-box', hostname: 'new-box', disconnected: true, parents: [] }] });
    await waitFor(() => expect(screen.queryByTestId('add-server-sheet')).toBeNull(), { timeout: 3000 });
    expect(screen.getByText('new-box')).toBeOnTheScreen();
    expect(screen.getByText('Latest status unavailable')).toBeOnTheScreen();
  });
});
