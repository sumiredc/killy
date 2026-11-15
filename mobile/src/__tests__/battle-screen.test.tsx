import { webcrypto } from 'node:crypto';

import { act, fireEvent, renderRouter, screen, within } from 'expo-router/testing-library';
import * as Localization from 'expo-localization';

import * as config from '../config';
import RootLayout from '../app/_layout';
import TabsLayout from '../app/(tabs)/_layout';
import BattleScreen from '../app/(tabs)/index';
import SettingsScreen from '../app/(tabs)/settings';
import * as deps from '../killy/deps';
import * as prefs from '../prefs';
import { FakeNotifications, MemoryPrefs } from '../testing/fake-notifications';
import type { ParentState, ServerState } from '../killy/types';
import { FakeServer, type FakeStream, settle } from '../testing/fake-server';

jest.mock('expo-localization', () => ({ getLocales: jest.fn(() => [{ languageCode: 'en' }]) }));

const routes = {
  _layout: RootLayout,
  '(tabs)/_layout': TabsLayout,
  '(tabs)/index': BattleScreen,
  '(tabs)/settings': SettingsScreen,
};

const uuidV4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

beforeEach(() => jest.useFakeTimers());
afterEach(() => {
  jest.useRealTimers();
  jest.restoreAllMocks();
});

async function startApp(server: FakeServer, languageCode = 'en') {
  jest.spyOn(config, 'buildServerURL').mockReturnValue('https://killy.example');
  jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode } as Localization.Locale]);
  jest.spyOn(prefs, 'makePrefs').mockReturnValue(new MemoryPrefs());
  jest.spyOn(deps, 'makeDeps').mockReturnValue({
    fetch: server.fetch,
    tokens: { get: async () => 'dev-token', set: async () => {} },
    random: () => 0,
    randomBytes: (n) => webcrypto.getRandomValues(new Uint8Array(n)),
    notifications: new FakeNotifications(),
  });
  await renderRouter(routes, { initialUrl: '/' });
  await act(settle);
}

function parent(pid: number, zombies: number[], at: string, cmd = `node worker-${pid}.js`): ParentState {
  return { pid, starttime: pid * 10, cmd, zombies, first_detected_at: at };
}

function server(id: string, name: string, parents: ParentState[], over: Partial<ServerState> = {}): ServerState {
  return { id, name, hostname: `host-${id}`, notify: true, disconnected: false, parents, ...over };
}

async function sendState(stream: FakeStream, servers: ServerState[]) {
  await act(() => stream.send(`event: state\ndata: ${JSON.stringify({ servers })}\n\n`));
}

const killButton = () => screen.getByRole('button', { name: 'Hold to kill' });

/** Holds the kill button for ms, then lets go and waits long enough for a late timer to fire. */
async function hold(ms: number) {
  await fireEvent(killButton(), 'pressIn');
  await act(() => jest.advanceTimersByTimeAsync(ms));
  await fireEvent(killButton(), 'pressOut');
  await act(() => jest.advanceTimersByTimeAsync(2000));
}

const t1 = '2026-11-13T00:00:01Z';
const t2 = '2026-11-13T00:00:02Z';
const t3 = '2026-11-13T00:00:03Z';

async function battleWith(servers: ServerState[]) {
  const s = new FakeServer();
  const stream = s.stream();
  await startApp(s);
  await sendState(stream, servers);
  return { s, stream };
}

describe('the current parent', () => {
  test('shows the oldest parent: its server, zombie count, PID, command line and the kill sub-label', async () => {
    await battleWith([
      server('b', 'db-01', [parent(4100, [4101], t2)]),
      server('a', 'web-01', [parent(3801, [3802, 3803, 3804], t1, 'node evil_daemon.js')]),
    ]);
    expect(screen.getByTestId('battle-server')).toHaveTextContent('web-01');
    expect(screen.getByTestId('battle-zombies')).toHaveTextContent('3');
    expect(screen.getByText('parent PID 3801')).toBeOnTheScreen();
    expect(screen.getByText('node evil_daemon.js')).toBeOnTheScreen();
    expect(screen.getByText('SIGKILL → PID 3801 on web-01')).toBeOnTheScreen();
    expect(killButton()).toBeEnabled();
  });

  test('a disconnected server is not fought', async () => {
    await battleWith([server('a', 'web-01', [parent(3801, [3802], t1)], { disconnected: true }), server('b', 'db-01', [parent(4100, [4101], t2)])]);
    expect(screen.getByTestId('battle-server')).toHaveTextContent('db-01');
  });

  test('new zombies of the current parent update its count, and a new parent goes behind it', async () => {
    const { stream } = await battleWith([server('a', 'web-01', [parent(3801, [3802], t1)])]);
    await sendState(stream, [server('a', 'web-01', [parent(3801, [3802, 3803], t1)]), server('b', 'db-01', [parent(4100, [4101], t3)])]);
    expect(screen.getByTestId('battle-server')).toHaveTextContent('web-01');
    expect(screen.getByTestId('battle-zombies')).toHaveTextContent('2');
  });
});

describe('hold to kill', () => {
  test('letting go before 1 second sends nothing', async () => {
    const { s } = await battleWith([server('a', 'web-01', [parent(3801, [3802], t1)])]);
    await hold(999);
    expect(s.count('POST /app/v1/kills')).toBe(0);
  });

  test('holding for 1 second sends one kill for the shown parent with a new lower-case UUID v4 each time', async () => {
    const { s } = await battleWith([server('a', 'web-01', [parent(3801, [3802], t1)])]);
    s.respond('POST /app/v1/kills', 200, { outcome: 'refused', reason: 'protected' });
    await hold(1000);
    await hold(1500);
    const kills = s.requests.filter((r) => r.route === 'POST /app/v1/kills');
    expect(kills.map((k) => ({ ...(k.body as object), id: undefined }))).toEqual([
      { server_id: 'a', ppid: 3801, starttime: 38010, id: undefined },
      { server_id: 'a', ppid: 3801, starttime: 38010, id: undefined },
    ]);
    const ids = kills.map((k) => (k.body as { id: string }).id);
    expect(ids[0]).toMatch(uuidV4);
    expect(ids[1]).toMatch(uuidV4);
    expect(ids[0]).not.toBe(ids[1]);
  });
});

describe('outcomes', () => {
  const two = () => [server('a', 'web-01', [parent(3801, [3802, 3803], t1), parent(4100, [4101], t2)])];

  test('killed shows the PID and the zombies reaped, and the next parent comes up', async () => {
    const { s } = await battleWith(two());
    s.respond('POST /app/v1/kills', 200, { outcome: 'killed', reaped: 2 });
    await hold(1000);
    expect(screen.getByText('PID 3801 killed')).toBeOnTheScreen();
    expect(screen.getByText('2 zombies reaped by init')).toBeOnTheScreen();
    expect(screen.getByText('parent PID 4100')).toBeOnTheScreen();
  });

  test('already gone says so and the next parent comes up', async () => {
    const { s } = await battleWith(two());
    s.respond('POST /app/v1/kills', 200, { outcome: 'already_gone' });
    await hold(1000);
    expect(screen.getByText('PID 3801 had already exited')).toBeOnTheScreen();
    expect(screen.getByText('parent PID 4100')).toBeOnTheScreen();
    expect(screen.queryByText(/killed$/)).toBeNull();
  });

  test.each([
    [{ outcome: 'refused', reason: 'pidfd_unavailable' }, /Not killed: pidfd is not available on the server/],
    [{ outcome: 'refused', reason: 'not_a_zombie_parent' }, /Not killed: it no longer has zombie processes/],
    [{ outcome: 'signal_failed', reason: 'EPERM' }, /Kill failed: the signal could not be sent \(EPERM\)/],
    [{ outcome: 'zombies_remain' }, /Kill failed: zombie processes remained after SIGKILL/],
  ])('%j shows its reason and the parent stays', async (res, text) => {
    const { s } = await battleWith(two());
    s.respond('POST /app/v1/kills', 200, res);
    await hold(1000);
    expect(screen.getByTestId('kill-result')).toHaveTextContent(text);
    expect(screen.getByText('parent PID 3801')).toBeOnTheScreen();
    expect(killButton()).toBeEnabled();
  });

  test('unknown says the result is not known, shows no success, and keeps showing the latest state', async () => {
    const { s, stream } = await battleWith(two());
    s.respond('POST /app/v1/kills', 200, { outcome: 'unknown', reason: 'timeout' });
    await hold(1000);
    expect(screen.getByTestId('kill-result')).toHaveTextContent(/result of the kill is not known/);
    expect(screen.queryByText('PID 3801 killed')).toBeNull();
    expect(screen.queryByText(/reaped by init/)).toBeNull();
    expect(screen.getByText('parent PID 3801')).toBeOnTheScreen();
    await sendState(stream, [server('a', 'web-01', [parent(4100, [4101], t2)])]);
    expect(screen.getByText('parent PID 4100')).toBeOnTheScreen();
  });

  test('a killed parent does not come back with a state taken before the kill, but does as a new episode', async () => {
    const { s, stream } = await battleWith(two());
    s.respond('POST /app/v1/kills', 200, { outcome: 'killed', reaped: 2 });
    await hold(1000);
    await sendState(stream, two());
    expect(screen.getByText('parent PID 4100')).toBeOnTheScreen();
    await sendState(stream, [server('a', 'web-01', [parent(4100, [4101], t2)])]);
    await sendState(stream, two());
    expect(screen.getByText('parent PID 3801')).toBeOnTheScreen();
  });
});

describe('a kill request that fails', () => {
  const one = () => [server('a', 'web-01', [parent(3801, [3802], t1)])];

  test('with no answer it is sent once more with the same ID, then a red line and the button can be held again', async () => {
    const { s } = await battleWith(one());
    s.unreachable('POST /app/v1/kills');
    await hold(1000);
    const kills = s.requests.filter((r) => r.route === 'POST /app/v1/kills');
    expect(kills).toHaveLength(2);
    expect((kills[0].body as { id: string }).id).toBe((kills[1].body as { id: string }).id);
    expect(screen.getByTestId('kill-error')).toHaveTextContent(/Cannot reach the Killy server/);
    expect(killButton()).toBeEnabled();
    await act(() => jest.advanceTimersByTimeAsync(60_000));
    expect(s.count('POST /app/v1/kills')).toBe(2);
  });

  test('a 429 is not sent again and asks to wait', async () => {
    const { s } = await battleWith(one());
    s.respond('POST /app/v1/kills', 429, { error: 'rate_limited' });
    await hold(1000);
    expect(s.count('POST /app/v1/kills')).toBe(1);
    expect(screen.getByTestId('kill-error')).toHaveTextContent(/Too many kills.*Wait/);
    expect(killButton()).toBeEnabled();
  });

  test.each([
    ['400', (s: FakeServer) => s.respond('POST /app/v1/kills', 400, { error: 'invalid_id' })],
    ['401', (s: FakeServer) => s.respond('POST /app/v1/kills', 401, { error: 'unauthorized' })],
    ['404', (s: FakeServer) => s.respond('POST /app/v1/kills', 404, { error: 'server_not_found' })],
    ['503', (s: FakeServer) => s.respond('POST /app/v1/kills', 503, { error: 'unavailable' })],
    ['not JSON', (s: FakeServer) => s.respondText('POST /app/v1/kills', 502, '<html>502</html>')],
  ])('%s is not sent again, shows a red line and the button can be held again', async (_, answer) => {
    const { s } = await battleWith(one());
    answer(s);
    await hold(1000);
    expect(s.count('POST /app/v1/kills')).toBe(1);
    expect(screen.getByTestId('kill-error')).toHaveTextContent(/could not be sent/);
    expect(killButton()).toBeEnabled();
    expect(screen.getByText('parent PID 3801')).toBeOnTheScreen();
  });
});

describe('All clear', () => {
  test('counts the zombies shown when the kill started, and applies the outcome to the state current when it arrives', async () => {
    const { s, stream } = await battleWith([server('a', 'web-01', [parent(3801, [3802, 3803], t1)])]);
    const answer = s.hold('POST /app/v1/kills');
    await fireEvent(killButton(), 'pressIn');
    await act(() => jest.advanceTimersByTimeAsync(1000));
    await fireEvent(killButton(), 'pressOut');
    // While the kill is in flight: the parent shows one zombie more, and a new parent appears.
    await sendState(stream, [server('a', 'web-01', [parent(3801, [3802, 3803, 3804], t1)]), server('b', 'db-01', [parent(5000, [5001], t3)])]);
    await act(() => answer.resolve(200, { outcome: 'killed', reaped: 3 }));
    // The new parent is in the queue, so the battle goes on rather than ending.
    expect(screen.queryByTestId('all-clear')).toBeNull();
    expect(screen.getByText('parent PID 5000')).toBeOnTheScreen();
    s.respond('POST /app/v1/kills', 200, { outcome: 'killed', reaped: 1 });
    await hold(1000);
    const overlay = screen.getByTestId('all-clear');
    expect(within(overlay).getByTestId('stat-parents')).toHaveTextContent('2');
    expect(within(overlay).getByTestId('stat-zombies')).toHaveTextContent('3');
  });

  test('counts killed parents and the zombies shown when each kill started, not already gone ones', async () => {
    const { s, stream } = await battleWith([
      server('a', 'web-01', [parent(3801, [3802, 3803, 3804], t1), parent(4100, [4101, 4102], t2)]),
      server('b', 'db-01', [parent(5000, [5001], t3)]),
    ]);
    s.respond('POST /app/v1/kills', 200, { outcome: 'killed', reaped: 1 });
    await hold(1000);
    s.respond('POST /app/v1/kills', 200, { outcome: 'already_gone' });
    await hold(1000);
    // A zombie more on the last parent before its kill starts.
    await sendState(stream, [
      server('a', 'web-01', [parent(3801, [3802, 3803, 3804], t1), parent(4100, [4101, 4102], t2)]),
      server('b', 'db-01', [parent(5000, [5001, 5002], t3)]),
    ]);
    s.respond('POST /app/v1/kills', 200, { outcome: 'killed', reaped: 2 });
    await hold(1000);
    const overlay = screen.getByTestId('all-clear');
    expect(within(overlay).getByText('All clear')).toBeOnTheScreen();
    expect(within(overlay).getByTestId('stat-parents')).toHaveTextContent('2');
    expect(within(overlay).getByTestId('stat-zombies')).toHaveTextContent('5');
    await fireEvent.press(within(overlay).getByText('Close'));
    expect(screen.queryByTestId('all-clear')).toBeNull();
  });
});

describe('empty states', () => {
  test('without servers it says so and the kill button cannot be pressed', async () => {
    const s = new FakeServer();
    const stream = s.stream();
    await startApp(s);
    await sendState(stream, []);
    expect(screen.getByText('No servers yet')).toBeOnTheScreen();
    expect(screen.getByText('Add a server in Settings to start watching it.')).toBeOnTheScreen();
    expect(killButton()).toBeDisabled();
    await hold(1000);
    expect(s.count('POST /app/v1/kills')).toBe(0);
  });

  test('with servers but no zombies it says so and the kill button cannot be pressed', async () => {
    const { s } = await battleWith([server('a', 'web-01', []), server('b', 'db-01', [parent(4100, [4101], t2)], { disconnected: true })]);
    expect(screen.getByText('No zombie processes')).toBeOnTheScreen();
    expect(screen.getByText('No zombie processes found. You will be notified when one appears.')).toBeOnTheScreen();
    expect(killButton()).toBeDisabled();
    await hold(1000);
    expect(s.count('POST /app/v1/kills')).toBe(0);
  });
});

describe('tabs', () => {
  test('while a kill is in flight the Settings tab cannot be opened', async () => {
    const { s } = await battleWith([server('a', 'web-01', [parent(3801, [3802], t1)])]);
    const answer = s.hold('POST /app/v1/kills');
    await fireEvent(killButton(), 'pressIn');
    await act(() => jest.advanceTimersByTimeAsync(1000));
    await fireEvent.press(screen.getByLabelText('Settings'));
    expect(screen.queryByTestId('settings-screen')).toBeNull();
    await act(() => answer.resolve(200, { outcome: 'refused', reason: 'protected' }));
    await fireEvent(killButton(), 'pressOut');
    await fireEvent.press(screen.getByLabelText('Settings'));
    expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
  });

  test('while Settings is open the Battle tab shows the number of zombie processes waiting', async () => {
    const { s } = await battleWith([
      server('a', 'web-01', [parent(3801, [3802, 3803], t1), parent(4100, [4101, 4102, 4103, 4104], t2)]),
      server('b', 'db-01', [parent(5000, [5001, 5002, 5003], t3)], { disconnected: true }),
    ]);
    // The killed parent's zombies are no longer waiting, nor are the disconnected server's.
    s.respond('POST /app/v1/kills', 200, { outcome: 'killed', reaped: 2 });
    await hold(1000);
    expect(within(screen.getByLabelText('Battle')).queryByText('4')).toBeNull();
    await fireEvent.press(screen.getByLabelText('Settings'));
    expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
    expect(within(screen.getByLabelText('Battle')).getByText('4')).toBeOnTheScreen();
  });

  test('a hold that started on one parent does not kill another that came to the front meanwhile', async () => {
    const { s, stream } = await battleWith([server('a', 'web-01', [parent(3801, [3802], t2)])]);
    await fireEvent(killButton(), 'pressIn');
    await act(() => jest.advanceTimersByTimeAsync(500));
    await sendState(stream, [server('a', 'web-01', [parent(3801, [3802], t2), parent(2000, [2001], t1)])]);
    await act(() => jest.advanceTimersByTimeAsync(1000));
    await fireEvent(killButton(), 'pressOut');
    expect(s.count('POST /app/v1/kills')).toBe(0);
  });

  test('a reason the app does not know is shown as the agent sent it, whatever its name', async () => {
    const { s } = await battleWith([server('a', 'web-01', [parent(3801, [3802], t1)])]);
    s.respond('POST /app/v1/kills', 200, { outcome: 'refused', reason: 'constructor' });
    await hold(1000);
    expect(screen.getByTestId('kill-result')).toHaveTextContent('Not killed: constructor');
  });
});

describe('the arena', () => {
  /** The arena draws its scene once it knows its size; jest has no layout, so the test gives it one. */
  async function laidOut() {
    await fireEvent(screen.getByTestId('arena'), 'layout', { nativeEvent: { layout: { x: 0, y: 0, width: 358, height: 600 } } });
  }
  const pids = (n: number) => Array.from({ length: n }, (_, i) => 3802 + i);

  test('six zombies are all drawn with their PIDs, and no +N', async () => {
    await battleWith([server('a', 'web-01', [parent(3801, pids(6), t1)])]);
    await laidOut();
    expect(screen.getAllByTestId('zombie')).toHaveLength(6);
    expect(screen.getAllByTestId('zombie-pid').map((p) => p.props.children)).toEqual(pids(6));
    expect(screen.queryByTestId('zombies-more')).toBeNull();
    expect(screen.getAllByTestId('string')).toHaveLength(12);
  });

  test.each([
    [7, '+1'],
    [9, '+3'],
  ])('%i zombies: the first six are drawn with %s, and the big count is the real one', async (n, more) => {
    await battleWith([server('a', 'web-01', [parent(3801, pids(n), t1)])]);
    await laidOut();
    expect(screen.getAllByTestId('zombie-pid').map((p) => p.props.children)).toEqual(pids(6));
    expect(within(screen.getByTestId('zombies-more')).getByText(more)).toBeOnTheScreen();
    expect(screen.getByTestId('battle-zombies')).toHaveTextContent(String(n));
  });

  test('with four parents in the battle, one of them put away, there are four hoods and the cleared one is at 22%', async () => {
    const { s } = await battleWith([
      server('a', 'web-01', [parent(3801, [3802], t1), parent(4100, [4101], t2), parent(4200, [4201], t3), parent(4300, [4301], t3)]),
    ]);
    s.respond('POST /app/v1/kills', 200, { outcome: 'already_gone' });
    await hold(1000);
    const c = within(screen.getByTestId('parent-counter'));
    expect(c.getAllByTestId(/^hood-(done|current|waiting)$/).map((h) => h.props.testID)).toEqual([
      'hood-done',
      'hood-current',
      'hood-waiting',
      'hood-waiting',
    ]);
    expect(c.getByTestId('hood-done')).toHaveStyle({ opacity: 0.22 });
    expect(c.queryByTestId('hood-times')).toBeNull();
    expect(screen.getByTestId('parent-counter').props.accessibilityLabel).toBe('parent process 2 of 4');
  });

  test('with five parents in the battle, one of them killed, there is one hood and × the number still in the queue', async () => {
    const { s } = await battleWith([
      server('a', 'web-01', [
        parent(3801, [3802], t1),
        parent(4100, [4101], t2),
        parent(4200, [4201], t3),
        parent(4300, [4301], t3),
        parent(4400, [4401], t3),
      ]),
    ]);
    s.respond('POST /app/v1/kills', 200, { outcome: 'killed', reaped: 1 });
    await hold(1000);
    const c = within(screen.getByTestId('parent-counter'));
    expect(c.getAllByTestId(/^hood-(done|current|waiting)$/).map((h) => h.props.testID)).toEqual(['hood-current']);
    expect(c.getByTestId('hood-times')).toHaveTextContent('× 4');
  });

  test('in an empty state Killy stays, and there is no necromancer, zombie or counter', async () => {
    await battleWith([server('a', 'web-01', [])]);
    await laidOut();
    expect(screen.getByTestId('killy')).toBeOnTheScreen();
    expect(screen.queryByTestId('necromancer')).toBeNull();
    expect(screen.queryByTestId('zombie')).toBeNull();
    expect(screen.queryByTestId('parent-counter')).toBeNull();
  });

  test('the tabs have their icons: a ghost for Battle and a gear for Settings', async () => {
    await battleWith([]);
    // React Navigation draws each icon twice, for the active and the inactive look.
    expect(screen.getAllByTestId('tab-icon-battle').length).toBeGreaterThan(0);
    expect(screen.getAllByTestId('tab-icon-settings').length).toBeGreaterThan(0);
  });
});
