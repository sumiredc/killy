import { act, fireEvent, renderRouter, screen, waitFor } from 'expo-router/testing-library';
import * as Localization from 'expo-localization';
import { AppState, type AppStateStatus } from 'react-native';

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

const patchRoute = 'PATCH /app/v1/devices/me';

class MemoryTokens {
  saved: string | null = 'dev-token';
  get = async () => this.saved;
  set = async (token: string) => {
    this.saved = token;
  };
}

/** One phone: what stays on it between starts. */
type Phone = { tokens: MemoryTokens; notifications: FakeNotifications; prefs: MemoryPrefs };

function phone(): Phone {
  return { tokens: new MemoryTokens(), notifications: new FakeNotifications(), prefs: new MemoryPrefs() };
}

/** A phone that allows notifications and has a push token. */
function allowing(token = 'ExponentPushToken[one]'): Phone {
  const p = phone();
  p.notifications.current = 'granted';
  p.notifications.answer = 'granted';
  p.notifications.token = token;
  return p;
}

let appStateListeners: ((s: AppStateStatus) => void)[] = [];

beforeEach(() => {
  appStateListeners = [];
  jest.spyOn(AppState, 'addEventListener').mockImplementation((type, listener) => {
    if (type === 'change') appStateListeners.push(listener as (s: AppStateStatus) => void);
    return { remove: () => (appStateListeners = appStateListeners.filter((l) => l !== listener)) };
  });
});

afterEach(() => {
  jest.restoreAllMocks();
});

async function startApp(server: FakeServer, p: Phone, { languageCode = 'en', initialUrl = '/' } = {}): Promise<FakeStream> {
  const stream = server.stream();
  jest.spyOn(config, 'buildServerURL').mockReturnValue('https://killy.example');
  jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode } as Localization.Locale]);
  jest.spyOn(prefs, 'makePrefs').mockReturnValue(p.prefs);
  jest.spyOn(deps, 'makeDeps').mockReturnValue({
    fetch: server.fetch,
    tokens: p.tokens,
    random: () => 0,
    randomBytes: (n) => new Uint8Array(n),
    notifications: p.notifications,
  });
  await renderRouter(routes, { initialUrl });
  await act(settle);
  return stream;
}

function patches(server: FakeServer): unknown[] {
  return server.requests.filter((r) => r.route === patchRoute).map((r) => r.body);
}

async function openSettings() {
  await fireEvent.press(screen.getByLabelText('Settings'));
  expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
}

/** The operator leaves for the system settings, changes something there, and comes back. */
async function viaSystemSettings(change: () => void = () => {}) {
  await act(async () => {
    for (const l of appStateListeners) l('background');
    await settle();
    change();
    for (const l of appStateListeners) l('active');
    await settle();
  });
}

const web = { id: 'a1', name: 'web-01', hostname: 'ip-10-0-0-1', notify: true, disconnected: false };

describe('push token registration', () => {
  test('a registered phone that allows notifications sends its push token at start', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 204, {});
    const p = allowing();
    await startApp(server, p);
    expect(patches(server)).toEqual([{ push_token: 'ExponentPushToken[one]' }]);
    expect(server.requests.find((r) => r.route === patchRoute)?.authorization).toBe('Bearer dev-token');
    expect(p.notifications.calls).not.toContain('requestPermission');
  });

  test('when the OS has not decided yet, the app makes the channel, asks, and sends the token once allowed', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 204, {});
    const p = allowing();
    p.notifications.current = 'undetermined';
    await startApp(server, p);
    const calls = p.notifications.calls;
    expect(calls).toContain('prepare Zombie processes');
    expect(calls.indexOf('prepare Zombie processes')).toBeLessThan(calls.indexOf('requestPermission'));
    expect(patches(server)).toEqual([{ push_token: 'ExponentPushToken[one]' }]);
  });

  test('the channel is named in the app language', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 204, {});
    const p = allowing();
    p.notifications.current = 'undetermined';
    await startApp(server, p, { languageCode: 'ja' });
    expect(p.notifications.calls).toContain('prepare ゾンビプロセス');
  });

  test('the same token is sent again on every start', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 204, {});
    const p = allowing();
    await startApp(server, p);
    await act(async () => screen.unmount());
    await startApp(server, p);
    expect(patches(server)).toEqual([{ push_token: 'ExponentPushToken[one]' }, { push_token: 'ExponentPushToken[one]' }]);
  });

  test('a new token from the OS is sent; the same token again is not', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 204, {});
    const p = allowing();
    await startApp(server, p);
    await act(async () => {
      p.notifications.changeToken('ExponentPushToken[two]');
      await settle();
    });
    await act(async () => {
      p.notifications.changeToken('ExponentPushToken[two]');
      await settle();
    });
    expect(patches(server)).toEqual([{ push_token: 'ExponentPushToken[one]' }, { push_token: 'ExponentPushToken[two]' }]);
  });

  test('a failed registration shows nothing and is sent again on the next start', async () => {
    const server = new FakeServer();
    server.unreachable(patchRoute);
    const p = allowing();
    await startApp(server, p);
    await openSettings();
    expect(screen.queryByTestId('notify-app-error')).toBeNull();
    expect(screen.queryByText('Cannot reach the Killy server. Check your network.')).toBeNull();
    await act(async () => screen.unmount());
    server.respond(patchRoute, 204, {});
    await startApp(server, p);
    expect(patches(server)).toEqual([{ push_token: 'ExponentPushToken[one]' }, { push_token: 'ExponentPushToken[one]' }]);
  });

  test('a token that failed to register is sent when the OS gives the same token again', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 500, { error: 'unavailable' });
    const p = allowing();
    await startApp(server, p);
    server.respond(patchRoute, 204, {});
    await act(async () => {
      p.notifications.changeToken('ExponentPushToken[one]');
      await settle();
    });
    expect(patches(server)).toHaveLength(2);
  });

  test('without a push token nothing is sent, and servers can still be added and fought', async () => {
    const server = new FakeServer();
    const p = phone(); // blocks notifications, no push token
    server.respond('POST /app/v1/pairings/claim', 200, web);
    const stream = await startApp(server, p);
    await act(() =>
      stream.send(
        `event: state\ndata: ${JSON.stringify({ servers: [{ ...web, parents: [{ pid: 3801, starttime: 1, cmd: 'node evil.js', zombies: [3802], first_detected_at: '2026-11-13T00:00:00Z' }] }] })}\n\n`,
      ),
    );
    expect(await screen.findByText('SIGKILL → PID 3801 on web-01')).toBeOnTheScreen();
    await openSettings();
    await fireEvent.press(screen.getByText('Add server'));
    await fireEvent.changeText(screen.getByLabelText('Pairing code'), 'ABC123');
    await fireEvent.press(screen.getByText('Add'));
    expect(await screen.findByText('Added web-01')).toBeOnTheScreen();
    expect(server.count(patchRoute)).toBe(0);
    expect(p.notifications.calls).not.toContain('pushToken');
  });

  test('registrations that come at once send the token once', async () => {
    const server = new FakeServer();
    const held = server.hold(patchRoute);
    const p = allowing();
    await startApp(server, p);
    await act(async () => {
      p.notifications.changeToken('ExponentPushToken[one]');
      await settle();
    });
    await act(() => held.resolve(204, {}));
    await act(settle);
    expect(patches(server)).toEqual([{ push_token: 'ExponentPushToken[one]' }]);
  });

  test('a phone that could not register with the Killy server asks for nothing and sends nothing', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/devices', 500, { error: 'unavailable' });
    const p = allowing();
    p.tokens.saved = null;
    p.notifications.current = 'undetermined';
    await startApp(server, p);
    expect(p.notifications.calls).not.toContain('requestPermission');
    expect(server.count(patchRoute)).toBe(0);
  });
});

describe('the app-wide notification switch', () => {
  function appSwitch() {
    return screen.getByLabelText('Notifications');
  }

  test('the App group has a Notifications row with its hint, on when nothing is saved', async () => {
    const server = new FakeServer();
    await startApp(server, allowing());
    await openSettings();
    expect(screen.getByText('App')).toBeOnTheScreen();
    expect(screen.getByText('Alert me when zombie processes appear')).toBeOnTheScreen();
    expect(appSwitch()).toHaveProp('value', true);
  });

  test('the row is in Japanese on a Japanese phone', async () => {
    const server = new FakeServer();
    await startApp(server, allowing(), { languageCode: 'ja' });
    await fireEvent.press(screen.getByLabelText('設定'));
    expect(await screen.findByText('アプリ')).toBeOnTheScreen();
    expect(screen.getByText('ゾンビプロセスが見つかったら知らせる')).toBeOnTheScreen();
  });

  test('turning it off sends notify false, saves it, and the next start shows it off', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 204, {});
    const p = allowing();
    await startApp(server, p);
    await openSettings();
    await fireEvent(appSwitch(), 'valueChange', false);
    await act(settle);
    expect(patches(server)).toContainEqual({ notify: false });
    expect(p.prefs.saved.notify).toBe(false);
    expect(appSwitch()).toHaveProp('value', false);
    await act(async () => screen.unmount());
    await startApp(server, p);
    await openSettings();
    expect(appSwitch()).toHaveProp('value', false);
  });

  test('the switch cannot be pressed while its change is being sent', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 204, {});
    const p = allowing();
    await startApp(server, p);
    await openSettings();
    const held = server.hold(patchRoute);
    await fireEvent(appSwitch(), 'valueChange', false);
    expect(appSwitch()).toHaveProp('value', false);
    expect(appSwitch()).toBeDisabled();
    await act(() => held.resolve(204, {}));
    expect(appSwitch()).toBeEnabled();
  });

  test.each([
    ['an error answer', (s: FakeServer) => s.respond(patchRoute, 503, { error: 'unavailable' }), 'The setting could not be saved. Try again in a while.'],
    ['no answer', (s: FakeServer) => s.unreachable(patchRoute), 'Cannot reach the Killy server. Check your network.'],
  ])('after %s the switch goes back, the reason shows under it, and nothing is saved', async (_, fail, text) => {
    const server = new FakeServer();
    server.respond(patchRoute, 204, {});
    const p = allowing();
    await startApp(server, p);
    await openSettings();
    fail(server);
    await fireEvent(appSwitch(), 'valueChange', false);
    await act(settle);
    expect(appSwitch()).toHaveProp('value', true);
    expect(screen.getByTestId('notify-app-error')).toHaveTextContent(text);
    expect(p.prefs.saved.notify).toBeUndefined();
  });

  test('a phone not registered with the Killy server keeps the switch and says why', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/devices', 500, { error: 'unavailable' });
    const p = allowing();
    p.tokens.saved = null;
    await startApp(server, p);
    await openSettings();
    await fireEvent(appSwitch(), 'valueChange', false);
    await act(settle);
    expect(appSwitch()).toHaveProp('value', true);
    expect(screen.getByTestId('notify-app-error')).toHaveTextContent(
      'This phone is not registered with the Killy server yet. Check your network and restart the app.',
    );
    expect(server.count(patchRoute)).toBe(0);
  });

  test('the next press clears the reason', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 503, { error: 'unavailable' });
    await startApp(server, allowing());
    await openSettings();
    await fireEvent(appSwitch(), 'valueChange', false);
    await act(settle);
    expect(screen.getByTestId('notify-app-error')).toBeOnTheScreen();
    server.hold(patchRoute);
    await fireEvent(appSwitch(), 'valueChange', false);
    expect(screen.queryByTestId('notify-app-error')).toBeNull();
  });

  test('a saved value that cannot be read shows the switch on and is not overwritten', async () => {
    const server = new FakeServer();
    const p = allowing();
    p.prefs.saved.notify = false;
    p.prefs.failRead = true;
    await startApp(server, p);
    await openSettings();
    expect(appSwitch()).toHaveProp('value', true);
    expect(p.prefs.saved.notify).toBe(false);
  });
});

describe('notifications blocked by the OS', () => {
  test('the row says so with a link to the system settings, and the switch cannot be pressed', async () => {
    const server = new FakeServer();
    const p = phone();
    await startApp(server, p);
    await openSettings();
    expect(screen.getByText('Notifications are blocked by the OS')).toBeOnTheScreen();
    expect(screen.getByLabelText('Notifications')).toBeDisabled();
    await fireEvent.press(screen.getByText('Open Settings'));
    expect(p.notifications.calls).toContain('openSettings');
  });

  test('in Japanese', async () => {
    const server = new FakeServer();
    await startApp(server, phone(), { languageCode: 'ja' });
    await fireEvent.press(screen.getByLabelText('設定'));
    expect(await screen.findByText('OS で通知がブロックされています')).toBeOnTheScreen();
    expect(screen.getByText('設定を開く')).toBeOnTheScreen();
  });

  test('coming back from the system settings with notifications allowed clears the block and registers the token', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 204, {});
    const p = phone();
    await startApp(server, p);
    await openSettings();
    expect(server.count(patchRoute)).toBe(0);
    await viaSystemSettings(() => {
      p.notifications.current = 'granted';
      p.notifications.token = 'ExponentPushToken[later]';
    });
    expect(screen.queryByText('Notifications are blocked by the OS')).toBeNull();
    expect(screen.getByLabelText('Notifications')).toBeEnabled();
    expect(patches(server)).toEqual([{ push_token: 'ExponentPushToken[later]' }]);
  });

  test('a refusal that the OS would let Killy ask again counts as blocked until the next start', async () => {
    const server = new FakeServer();
    const p = phone();
    p.notifications.current = 'undetermined';
    p.notifications.answer = 'undetermined'; // Android after the first refusal: it may ask once more
    await startApp(server, p);
    await openSettings();
    expect(screen.getByText('Notifications are blocked by the OS')).toBeOnTheScreen();
    expect(screen.getByLabelText('Notifications')).toBeDisabled();
    await viaSystemSettings();
    expect(screen.getByText('Notifications are blocked by the OS')).toBeOnTheScreen();
  });

  test('coming back still blocked keeps the block and sends nothing', async () => {
    const server = new FakeServer();
    const p = phone();
    await startApp(server, p);
    await openSettings();
    await viaSystemSettings();
    expect(screen.getByText('Notifications are blocked by the OS')).toBeOnTheScreen();
    expect(server.count(patchRoute)).toBe(0);
  });

  test('turning notifications off in the system settings shows the block on return', async () => {
    const server = new FakeServer();
    server.respond(patchRoute, 204, {});
    const p = allowing();
    await startApp(server, p);
    await openSettings();
    expect(screen.queryByText('Notifications are blocked by the OS')).toBeNull();
    await viaSystemSettings(() => {
      p.notifications.current = 'denied';
    });
    expect(screen.getByText('Notifications are blocked by the OS')).toBeOnTheScreen();
  });
});

describe('tapping a notification', () => {
  test('a tap that started the app opens the Battle tab, once', async () => {
    const server = new FakeServer();
    const p = phone();
    p.notifications.tapped = true;
    await startApp(server, p, { initialUrl: '/settings' });
    expect(await screen.findByTestId('battle-screen')).toBeOnTheScreen();
    expect(screen.queryByTestId('settings-screen')).toBeNull();
    expect(p.notifications.calls).toContain('clearTap');
    await openSettings();
    await act(settle);
    expect(screen.getByTestId('settings-screen')).toBeOnTheScreen();
  });

  test('a tap while the app is in the background opens the Battle tab', async () => {
    const server = new FakeServer();
    const p = phone();
    await startApp(server, p, { initialUrl: '/settings' });
    expect(screen.getByTestId('settings-screen')).toBeOnTheScreen();
    await act(async () => {
      p.notifications.tap();
      await settle();
    });
    expect(await screen.findByTestId('battle-screen')).toBeOnTheScreen();
    expect(screen.queryByTestId('settings-screen')).toBeNull();
  });

  test('a tap closes an open server sheet too', async () => {
    const server = new FakeServer();
    const p = phone();
    const stream = await startApp(server, p, { initialUrl: '/settings' });
    await act(() => stream.send(`event: state\ndata: ${JSON.stringify({ servers: [{ ...web, parents: [] }] })}\n\n`));
    await fireEvent.press(await screen.findByTestId('server-row'));
    expect(await screen.findByTestId('server-sheet')).toBeOnTheScreen();
    await act(async () => {
      p.notifications.tap();
      await settle();
    });
    await waitFor(() => expect(screen.queryByTestId('server-sheet', { includeHiddenElements: true })).toBeNull());
  });

  test('a tap closes an open sheet so the Battle tab is not hidden behind it', async () => {
    const server = new FakeServer();
    const p = phone();
    await startApp(server, p, { initialUrl: '/settings' });
    await fireEvent.press(screen.getByText('Add server'));
    expect(await screen.findByTestId('add-server-sheet')).toBeOnTheScreen();
    await act(async () => {
      p.notifications.tap();
      await settle();
    });
    // Gone, not only hidden with the Settings tab: a Modal would stay over the Battle tab.
    await waitFor(() => expect(screen.queryByTestId('add-server-sheet', { includeHiddenElements: true })).toBeNull());
  });

  test('without a tap the app stays where it opened', async () => {
    const server = new FakeServer();
    await startApp(server, phone(), { initialUrl: '/settings' });
    expect(screen.getByTestId('settings-screen')).toBeOnTheScreen();
  });
});
