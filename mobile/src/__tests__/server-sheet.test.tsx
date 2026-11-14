import { act, fireEvent, renderRouter, screen, waitFor, within } from 'expo-router/testing-library';
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

afterEach(() => {
  jest.restoreAllMocks();
});

const web = { id: 'a1', name: 'Prod web', hostname: 'ip-10-0-0-1', notify: true, disconnected: false, parents: [] };
const db = { ...web, id: 'a2', name: 'db-01', hostname: 'db-01.internal' };

function parent(pid: number, zombies: number[]) {
  return { pid, starttime: pid * 10, cmd: `node worker-${pid}.js`, zombies, first_detected_at: '2026-11-13T00:00:00Z' };
}

/** Starts the app registered, with the given servers in the first state, and opens Settings. */
async function startWith(servers: unknown[], languageCode = 'en'): Promise<{ server: FakeServer; stream: FakeStream }> {
  const server = new FakeServer();
  const stream = server.stream();
  jest.spyOn(config, 'buildServerURL').mockReturnValue('https://killy.example');
  jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode } as Localization.Locale]);
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
  await sendState(stream, servers);
  await fireEvent.press(screen.getByLabelText(languageCode === 'ja' ? '設定' : 'Settings'));
  expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
  return { server, stream };
}

async function sendState(stream: FakeStream, servers: unknown[]) {
  await act(() => stream.send(`event: state\ndata: ${JSON.stringify({ servers })}\n\n`));
}

async function openSheet(name: string) {
  await fireEvent.press(screen.getByRole('button', { name: new RegExp(`^${name}`) }));
  return within(await screen.findByTestId('server-sheet'));
}

const sheetGone = () => waitFor(() => expect(screen.queryByTestId('server-sheet')).toBeNull());

describe('opening the server sheet', () => {
  test('tapping a server row opens the sheet for that server, with its name on this phone', async () => {
    await startWith([web, db]);
    const sheet = await openSheet('db-01');
    expect(sheet.getByText('Server settings')).toBeOnTheScreen();
    expect(sheet.getByLabelText('Display name')).toHaveDisplayValue('db-01');
  });

  test('the Hostname row shows the hostname from the state, not the display name', async () => {
    await startWith([web]);
    const sheet = await openSheet('Prod web');
    expect(sheet.getByText('Hostname')).toBeOnTheScreen();
    expect(sheet.getByTestId('server-hostname')).toHaveTextContent('ip-10-0-0-1');
  });

  test('the sheet has the notification switch, Remove, its hint and Done', async () => {
    await startWith([web]);
    const sheet = await openSheet('Prod web');
    expect(sheet.getByRole('switch', { name: 'Notifications for this server' })).toHaveProp('value', true);
    expect(sheet.getByRole('button', { name: 'Remove this server' })).toBeOnTheScreen();
    expect(sheet.getByText('Removing it does not stop the agent on the server.')).toBeOnTheScreen();
    expect(sheet.getByRole('button', { name: 'Done' })).toBeOnTheScreen();
  });

  test('the display name takes at most 100 characters, as the Killy server accepts', async () => {
    await startWith([web]);
    const sheet = await openSheet('Prod web');
    expect(sheet.getByLabelText('Display name')).toHaveProp('maxLength', 100);
  });

  test('the sheet closes when the server leaves the state', async () => {
    const { stream } = await startWith([web, db]);
    await openSheet('Prod web');
    await sendState(stream, [db]);
    await sheetGone();
  });

  test('a server that leaves the state and comes back does not reopen its sheet', async () => {
    const { stream } = await startWith([web, db]);
    await openSheet('Prod web');
    await sendState(stream, [db]);
    await sheetGone();
    // Linked again, as when the same agent is paired again or the demo server is added again.
    await sendState(stream, [db, web]);
    expect(screen.queryByTestId('server-sheet')).toBeNull();
  });

  test('in Japanese the sheet uses the Japanese labels', async () => {
    await startWith([web], 'ja');
    const sheet = await openSheet('Prod web');
    expect(sheet.getByText('サーバーの設定')).toBeOnTheScreen();
    expect(sheet.getByText('ホスト名')).toBeOnTheScreen();
    expect(sheet.getByRole('button', { name: 'このサーバーを削除' })).toBeOnTheScreen();
    expect(sheet.getByRole('button', { name: '完了' })).toBeOnTheScreen();
    await fireEvent.press(sheet.getByRole('button', { name: 'このサーバーを削除' }));
    expect(sheet.getByRole('button', { name: 'もう一度押すと削除します' })).toBeOnTheScreen();
  });
});

describe('the notification switch', () => {
  test('turning it off sends notify=false at once, before Done', async () => {
    const { server } = await startWith([web]);
    server.respond('PATCH /app/v1/servers/a1', 204, null);
    const sheet = await openSheet('Prod web');
    await fireEvent(sheet.getByRole('switch'), 'valueChange', false);
    await act(settle);
    expect(server.requests.filter((r) => r.route === 'PATCH /app/v1/servers/a1')).toEqual([
      expect.objectContaining({ body: { notify: false }, authorization: 'Bearer dev-token' }),
    ]);
    expect(screen.getByTestId('server-sheet')).toBeOnTheScreen();
  });

  test('while it is being sent the switch cannot be pressed', async () => {
    const { server } = await startWith([web]);
    const patch = server.hold('PATCH /app/v1/servers/a1');
    const sheet = await openSheet('Prod web');
    await fireEvent(sheet.getByRole('switch'), 'valueChange', false);
    expect(sheet.getByRole('switch')).toBeDisabled();
    await act(() => patch.resolve(204, null));
    expect(sheet.getByRole('switch')).toBeEnabled();
  });

  test('the switch follows the state that comes after the change', async () => {
    const { server, stream } = await startWith([web]);
    server.respond('PATCH /app/v1/servers/a1', 204, null);
    const sheet = await openSheet('Prod web');
    await fireEvent(sheet.getByRole('switch'), 'valueChange', false);
    await act(settle);
    await sendState(stream, [{ ...web, notify: false }]);
    expect(sheet.getByRole('switch')).toHaveProp('value', false);
    // A change from elsewhere, such as another start of the app, shows too.
    await sendState(stream, [{ ...web, notify: true }]);
    expect(sheet.getByRole('switch')).toHaveProp('value', true);
  });
});

describe('Done and the display name', () => {
  test('a changed name is sent trimmed when Done is pressed, and the sheet closes on success', async () => {
    const { server } = await startWith([web]);
    const patch = server.hold('PATCH /app/v1/servers/a1');
    const sheet = await openSheet('Prod web');
    await fireEvent.changeText(sheet.getByLabelText('Display name'), '  Renamed  ');
    expect(server.count('PATCH /app/v1/servers/a1')).toBe(0);
    await fireEvent.press(sheet.getByRole('button', { name: 'Done' }));
    expect(server.requests.at(-1)).toEqual(expect.objectContaining({ route: 'PATCH /app/v1/servers/a1', body: { name: 'Renamed' } }));
    expect(screen.getByTestId('server-sheet')).toBeOnTheScreen();
    await act(() => patch.resolve(204, null));
    await sheetGone();
  });

  test.each([
    ['unchanged', 'Prod web'],
    ['unchanged once trimmed', ' Prod web '],
    ['empty', ''],
    ['only spaces', '   '],
  ])('a name that is %s is not sent and Done just closes', async (_, name) => {
    const { server } = await startWith([web]);
    const sheet = await openSheet('Prod web');
    await fireEvent.changeText(sheet.getByLabelText('Display name'), name);
    await fireEvent.press(sheet.getByRole('button', { name: 'Done' }));
    await sheetGone();
    expect(server.count('PATCH /app/v1/servers/a1')).toBe(0);
  });

  test('the name in the field is not reset by a state that arrives while editing', async () => {
    const { stream } = await startWith([web]);
    const sheet = await openSheet('Prod web');
    await fireEvent.changeText(sheet.getByLabelText('Display name'), 'Half typed');
    await sendState(stream, [{ ...web, parents: [parent(100, [101])] }]);
    expect(sheet.getByLabelText('Display name')).toHaveDisplayValue('Half typed');
  });
});

describe('removing the server', () => {
  test('the first press sends nothing and asks for a second press', async () => {
    const { server } = await startWith([web]);
    const sheet = await openSheet('Prod web');
    await fireEvent.press(sheet.getByRole('button', { name: 'Remove this server' }));
    await act(settle);
    expect(server.count('DELETE /app/v1/servers/a1')).toBe(0);
    expect(sheet.getByRole('button', { name: 'Tap again to remove' })).toBeOnTheScreen();
    expect(screen.getByTestId('server-sheet')).toBeOnTheScreen();
  });

  test('the second press sends DELETE and the sheet closes on success', async () => {
    const { server } = await startWith([web]);
    const del = server.hold('DELETE /app/v1/servers/a1');
    const sheet = await openSheet('Prod web');
    await fireEvent.press(sheet.getByRole('button', { name: 'Remove this server' }));
    await fireEvent.press(sheet.getByRole('button', { name: 'Tap again to remove' }));
    expect(server.requests.filter((r) => r.route === 'DELETE /app/v1/servers/a1')).toEqual([
      expect.objectContaining({ authorization: 'Bearer dev-token' }),
    ]);
    await act(() => del.resolve(204, null));
    await sheetGone();
  });
});

describe('failures keep the sheet open with a red line under the control that failed', () => {
  const unreachable = 'Cannot reach the Killy server. Check your network.';
  const failed = 'The change could not be saved. Try again in a while.';
  const answers = [
    ['no answer', (s: FakeServer, route: string) => s.unreachable(route), unreachable],
    ['a 503', (s: FakeServer, route: string) => s.respond(route, 503, { error: 'unavailable' }), failed],
    ['a 404', (s: FakeServer, route: string) => s.respond(route, 404, { error: 'server_not_found' }), failed],
    ['an answer that is not JSON', (s: FakeServer, route: string) => s.respondText(route, 502, '<html>502</html>'), failed],
  ] as const;

  test.each(answers)('the name, after %s', async (_, answer, message) => {
    const { server } = await startWith([web]);
    answer(server, 'PATCH /app/v1/servers/a1');
    const sheet = await openSheet('Prod web');
    await fireEvent.changeText(sheet.getByLabelText('Display name'), 'Renamed');
    await fireEvent.press(sheet.getByRole('button', { name: 'Done' }));
    await act(settle);
    expect(screen.getByTestId('server-sheet')).toBeOnTheScreen();
    expect(sheet.getByTestId('name-error')).toHaveTextContent(message);
    expect(sheet.queryByTestId('notify-error')).toBeNull();
    expect(sheet.queryByTestId('remove-error')).toBeNull();
    expect(sheet.getByLabelText('Display name')).toHaveDisplayValue('Renamed');
  });

  test.each(answers)('the switch, after %s, goes back to the value in the state', async (_, answer, message) => {
    const { server } = await startWith([web]);
    answer(server, 'PATCH /app/v1/servers/a1');
    const sheet = await openSheet('Prod web');
    await fireEvent(sheet.getByRole('switch'), 'valueChange', false);
    await act(settle);
    expect(sheet.getByTestId('notify-error')).toHaveTextContent(message);
    expect(sheet.queryByTestId('name-error')).toBeNull();
    expect(sheet.getByRole('switch')).toHaveProp('value', true);
    expect(sheet.getByRole('switch')).toBeEnabled();
  });

  test.each(answers)('Remove, after %s', async (_, answer, message) => {
    const { server } = await startWith([web]);
    answer(server, 'DELETE /app/v1/servers/a1');
    const sheet = await openSheet('Prod web');
    await fireEvent.press(sheet.getByRole('button', { name: 'Remove this server' }));
    await fireEvent.press(sheet.getByRole('button', { name: 'Tap again to remove' }));
    await act(settle);
    expect(screen.getByTestId('server-sheet')).toBeOnTheScreen();
    expect(sheet.getByTestId('remove-error')).toHaveTextContent(message);
    expect(sheet.queryByTestId('name-error')).toBeNull();
  });

  test('in Japanese a failed save says so in Japanese', async () => {
    const { server } = await startWith([web], 'ja');
    server.respond('PATCH /app/v1/servers/a1', 503, { error: 'unavailable' });
    const sheet = await openSheet('Prod web');
    await fireEvent(sheet.getByRole('switch'), 'valueChange', false);
    await act(settle);
    expect(sheet.getByTestId('notify-error')).toHaveTextContent('変更を保存できませんでした。しばらくしてから、もう一度お試しください。');
  });

  test('the error line is red', async () => {
    const { server } = await startWith([web]);
    server.unreachable('PATCH /app/v1/servers/a1');
    const sheet = await openSheet('Prod web');
    await fireEvent(sheet.getByRole('switch'), 'valueChange', false);
    await act(settle);
    expect(sheet.getByTestId('notify-error')).toHaveStyle({ color: '#F7506A' });
  });
});

describe('the sheet cannot be closed while a change is being sent, so its failure is not lost', () => {
  test.each([
    ['the switch', 'PATCH /app/v1/servers/a1'],
    ['Remove', 'DELETE /app/v1/servers/a1'],
  ] as const)('while %s is being sent, Done and Back do not close it, and the failure shows', async (control, route) => {
    const { server } = await startWith([web]);
    const pending = server.hold(route);
    const sheet = await openSheet('Prod web');
    if (control === 'the switch') {
      await fireEvent(sheet.getByRole('switch'), 'valueChange', false);
    } else {
      await fireEvent.press(sheet.getByRole('button', { name: 'Remove this server' }));
      await fireEvent.press(sheet.getByRole('button', { name: 'Tap again to remove' }));
    }
    expect(sheet.getByRole('button', { name: 'Done' })).toBeDisabled();
    await fireEvent.press(sheet.getByRole('button', { name: 'Done' }));
    await fireEvent(screen.getByTestId('server-sheet-modal'), 'requestClose');
    expect(screen.getByTestId('server-sheet')).toBeOnTheScreen();
    await act(() => pending.resolve(503, { error: 'unavailable' }));
    expect(sheet.getByTestId(control === 'the switch' ? 'notify-error' : 'remove-error')).toHaveTextContent(
      'The change could not be saved. Try again in a while.',
    );
    expect(sheet.getByRole('button', { name: 'Done' })).toBeEnabled();
  });

  test.each([
    ['the name', 'PATCH /app/v1/servers/a1'],
    ['the switch', 'PATCH /app/v1/servers/a1'],
    ['Remove', 'DELETE /app/v1/servers/a1'],
  ] as const)('while %s is being sent, no other change can start, so a save that closes the sheet loses nothing', async (control, route) => {
    const { server } = await startWith([web]);
    server.hold(route);
    const sheet = await openSheet('Prod web');
    if (control === 'the name') {
      await fireEvent.changeText(sheet.getByLabelText('Display name'), 'Renamed');
      await fireEvent.press(sheet.getByRole('button', { name: 'Done' }));
    } else if (control === 'the switch') {
      await fireEvent(sheet.getByRole('switch'), 'valueChange', false);
    } else {
      await fireEvent.press(sheet.getByRole('button', { name: 'Remove this server' }));
      await fireEvent.press(sheet.getByRole('button', { name: 'Tap again to remove' }));
    }
    expect(sheet.getByRole('switch')).toBeDisabled();
    expect(sheet.getByRole('button', { name: /^(Remove this server|Tap again to remove)$/ })).toBeDisabled();
    expect(sheet.getByRole('button', { name: 'Done' })).toBeDisabled();
  });

  test('Back closes the sheet when nothing is being sent', async () => {
    await startWith([web]);
    await openSheet('Prod web');
    await fireEvent(screen.getByTestId('server-sheet-modal'), 'requestClose');
    await sheetGone();
  });
});

test("a muted server's parents stay in the battle queue", async () => {
  await startWith([{ ...web, notify: false, parents: [parent(3801, [3802])] }]);
  await fireEvent.press(screen.getByLabelText('Battle'));
  expect(await screen.findByText('parent PID 3801')).toBeOnTheScreen();
});

describe('the Connection check', () => {
  const checkRoute = 'POST /app/v1/servers/a1/check';
  const failedText = 'The connection could not be checked. Try again in a while.';

  test('before a check the row has Connection and Check, and no result', async () => {
    await startWith([web]);
    const sheet = await openSheet('Prod web');
    expect(sheet.getByText('Connection')).toBeOnTheScreen();
    expect(sheet.getByRole('button', { name: 'Check' })).toBeEnabled();
    expect(sheet.queryByTestId('conn-result')).toBeNull();
  });

  test('while checking, the button cannot be pressed and the row says it is asking the agent', async () => {
    const { server } = await startWith([web]);
    const check = server.hold(checkRoute);
    const sheet = await openSheet('Prod web');
    await fireEvent.press(sheet.getByRole('button', { name: 'Check' }));
    expect(sheet.getByTestId('conn-result')).toHaveTextContent('Asking the agent…');
    expect(sheet.getByTestId('conn-spinner')).toBeOnTheScreen();
    expect(sheet.getByRole('button', { name: 'Check' })).toBeDisabled();
    await act(() => check.resolve(200, { result: 'ok', version: '1.2.3' }));
    expect(sheet.queryByTestId('conn-spinner')).toBeNull();
    expect(server.count(checkRoute)).toBe(1);
  });

  test.each([
    ['ok', (s: FakeServer) => s.respond(checkRoute, 200, { result: 'ok', version: '1.2.3' }), 'Connected (agent 1.2.3)'],
    [
      'agent_disconnected',
      (s: FakeServer) => s.respond(checkRoute, 200, { result: 'agent_disconnected' }),
      'No response from the agent. Check that killy agent is running on the server.',
    ],
    [
      'no_response',
      (s: FakeServer) => s.respond(checkRoute, 200, { result: 'no_response' }),
      'Commands are not reaching the agent. Restart the agent.',
    ],
    ['no answer from the Killy server', (s: FakeServer) => s.unreachable(checkRoute), 'Cannot reach the Killy server. Check your network.'],
    ['a 401', (s: FakeServer) => s.respond(checkRoute, 401, { error: 'unauthorized' }), failedText],
    ['a 404', (s: FakeServer) => s.respond(checkRoute, 404, { error: 'server_not_found' }), failedText],
    ['a 503', (s: FakeServer) => s.respond(checkRoute, 503, { error: 'unavailable' }), failedText],
    ['an answer that is not JSON', (s: FakeServer) => s.respondText(checkRoute, 502, '<html>502</html>'), failedText],
  ] as const)('%s shows its message and the button becomes Check again', async (_, answer, message) => {
    const { server } = await startWith([web]);
    answer(server);
    const sheet = await openSheet('Prod web');
    await fireEvent.press(sheet.getByRole('button', { name: 'Check' }));
    await act(settle);
    expect(sheet.getByTestId('conn-message')).toHaveTextContent(message);
    expect(sheet.getByRole('button', { name: 'Check again' })).toBeEnabled();
  });

  test('success is green with ✓, a failure light red with !', async () => {
    const { server } = await startWith([web]);
    server.respond(checkRoute, 200, { result: 'ok', version: '1.2.3' });
    const sheet = await openSheet('Prod web');
    await fireEvent.press(sheet.getByRole('button', { name: 'Check' }));
    await act(settle);
    expect(sheet.getByTestId('conn-mark')).toHaveTextContent('✓');
    expect(sheet.getByTestId('conn-mark')).toHaveStyle({ color: '#4FDB8F' });
    server.respond(checkRoute, 200, { result: 'no_response' });
    await fireEvent.press(sheet.getByRole('button', { name: 'Check again' }));
    await act(settle);
    expect(sheet.getByTestId('conn-mark')).toHaveTextContent('!');
    expect(sheet.getByTestId('conn-mark')).toHaveStyle({ color: '#FF9AAA' });
  });

  test('Check again sends another check', async () => {
    const { server } = await startWith([web]);
    server.respond(checkRoute, 200, { result: 'ok', version: '1.2.3' });
    const sheet = await openSheet('Prod web');
    await fireEvent.press(sheet.getByRole('button', { name: 'Check' }));
    await act(settle);
    await fireEvent.press(sheet.getByRole('button', { name: 'Check again' }));
    await act(settle);
    expect(server.count(checkRoute)).toBe(2);
  });

  test('the result is not kept once the sheet closes', async () => {
    const { server } = await startWith([web]);
    server.respond(checkRoute, 200, { result: 'ok', version: '1.2.3' });
    let sheet = await openSheet('Prod web');
    await fireEvent.press(sheet.getByRole('button', { name: 'Check' }));
    await act(settle);
    await fireEvent.press(sheet.getByRole('button', { name: 'Done' }));
    await sheetGone();
    sheet = await openSheet('Prod web');
    expect(sheet.queryByTestId('conn-result')).toBeNull();
    expect(sheet.getByRole('button', { name: 'Check' })).toBeOnTheScreen();
  });

  test('a check changes nothing, so the sheet can close while it runs', async () => {
    const { server } = await startWith([web]);
    const check = server.hold(checkRoute);
    const sheet = await openSheet('Prod web');
    await fireEvent.press(sheet.getByRole('button', { name: 'Check' }));
    expect(sheet.getByRole('button', { name: 'Done' })).toBeEnabled();
    await fireEvent.press(sheet.getByRole('button', { name: 'Done' }));
    await sheetGone();
    await act(() => check.resolve(200, { result: 'ok', version: '1.2.3' }));
    expect(server.count('PATCH /app/v1/servers/a1')).toBe(0);
  });

  test('in Japanese the results use the Japanese messages', async () => {
    const { server } = await startWith([web], 'ja');
    server.respond(checkRoute, 200, { result: 'no_response' });
    const sheet = await openSheet('Prod web');
    await fireEvent.press(sheet.getByRole('button', { name: '確認する' }));
    await act(settle);
    expect(sheet.getByTestId('conn-message')).toHaveTextContent('命令がエージェントに届いていません。エージェントを再起動してください。');
    server.respond(checkRoute, 503, { error: 'unavailable' });
    await fireEvent.press(sheet.getByRole('button', { name: 'もう一度確認' }));
    await act(settle);
    expect(sheet.getByTestId('conn-message')).toHaveTextContent('接続を確認できませんでした。しばらくしてから、もう一度お試しください。');
  });
});
