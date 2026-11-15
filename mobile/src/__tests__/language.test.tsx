import { act, fireEvent, renderRouter, screen } from 'expo-router/testing-library';
import * as Localization from 'expo-localization';
import * as SplashScreen from 'expo-splash-screen';

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
jest.mock('expo-splash-screen', () => ({ preventAutoHideAsync: jest.fn(async () => {}), hideAsync: jest.fn(async () => {}) }));

const routes = {
  _layout: RootLayout,
  '(tabs)/_layout': TabsLayout,
  '(tabs)/index': BattleScreen,
  '(tabs)/settings': SettingsScreen,
};

afterEach(() => {
  jest.restoreAllMocks();
});

/** Starts the app on a phone set to languageCode, with what p holds, without waiting for it to draw. */
async function launch(p: MemoryPrefs, languageCode: string, initialUrl = '/settings') {
  const server = new FakeServer();
  server.stream();
  jest.spyOn(config, 'buildServerURL').mockReturnValue('https://killy.example');
  jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode } as Localization.Locale]);
  jest.spyOn(prefs, 'makePrefs').mockReturnValue(p);
  jest.spyOn(deps, 'makeDeps').mockReturnValue({
    fetch: server.fetch,
    tokens: { get: async () => 'dev-token', set: async () => {} },
    random: () => 0,
    randomBytes: (n) => new Uint8Array(n),
    notifications: new FakeNotifications(),
  });
  await renderRouter(routes, { initialUrl });
}

async function startApp(p: MemoryPrefs, languageCode = 'en') {
  await launch(p, languageCode);
  await act(settle);
  expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
}

function option(name: '日本語' | 'English') {
  return screen.getByRole('button', { name });
}

describe('the Language row', () => {
  test('the App group has a Language row with 日本語 and English, the current one selected', async () => {
    await startApp(new MemoryPrefs());
    expect(screen.getByText('Language')).toBeOnTheScreen();
    expect(option('English')).toBeSelected();
    expect(option('日本語')).not.toBeSelected();
  });

  test('switching changes the open screen and the tab names at once', async () => {
    await startApp(new MemoryPrefs());
    await fireEvent.press(option('日本語'));
    expect(screen.getByText('言語')).toBeOnTheScreen();
    expect(screen.getByText('アプリ')).toBeOnTheScreen();
    expect(screen.getByLabelText('撃退')).toBeOnTheScreen();
    expect(screen.getByLabelText('設定')).toBeOnTheScreen();
    expect(option('日本語')).toBeSelected();
    await fireEvent.press(option('English'));
    expect(screen.getByText('Language')).toBeOnTheScreen();
    expect(screen.getByLabelText('Battle')).toBeOnTheScreen();
  });

  test('the two options keep their own names in either language', async () => {
    await startApp(new MemoryPrefs(), 'ja');
    expect(option('日本語')).toBeOnTheScreen();
    expect(option('English')).toBeOnTheScreen();
  });

  test('the chosen language is saved and wins over the phone language at the next start', async () => {
    const p = new MemoryPrefs();
    await startApp(p, 'en');
    await fireEvent.press(option('日本語'));
    await act(settle);
    expect(p.saved.language).toBe('ja');
    await act(async () => screen.unmount());
    await startApp(p, 'en');
    expect(screen.getByText('言語')).toBeOnTheScreen();
    expect(screen.getByLabelText('撃退')).toBeOnTheScreen();
  });

  test('English saved on a Japanese phone starts in English', async () => {
    const p = new MemoryPrefs();
    p.saved.language = 'en';
    await startApp(p, 'ja');
    expect(screen.getByText('Language')).toBeOnTheScreen();
  });

  test.each([
    ['ja', '言語'],
    ['fr', 'Language'],
  ])('with nothing saved, a phone set to %s starts with %s', async (code, text) => {
    await startApp(new MemoryPrefs(), code);
    expect(screen.getByText(text)).toBeOnTheScreen();
  });

  test('a saved language that cannot be read starts with the phone language and is not overwritten', async () => {
    const p = new MemoryPrefs();
    p.saved.language = 'en';
    p.failRead = true;
    await startApp(p, 'ja');
    expect(screen.getByText('言語')).toBeOnTheScreen();
    expect(p.saved.language).toBe('en');
  });

  test('a language that cannot be saved is still shown, with nothing said about it', async () => {
    const p = new MemoryPrefs();
    p.failWrite = true;
    await startApp(p, 'en');
    await fireEvent.press(option('日本語'));
    await act(settle);
    expect(screen.getByText('言語')).toBeOnTheScreen();
    expect(screen.queryByTestId('notify-app-error')).toBeNull();
    expect(p.saved.language).toBeUndefined();
  });
});

describe('starting', () => {
  test('nothing is drawn and the splash stays until the saved language is read', async () => {
    const hide = jest.mocked(SplashScreen.hideAsync);
    hide.mockClear();
    const p = new MemoryPrefs();
    p.saved.language = 'ja';
    const release = p.holdLanguage();
    await launch(p, 'en', '/');
    await act(settle);
    expect(screen.queryByTestId('battle-screen')).toBeNull();
    expect(screen.queryByText('No servers yet')).toBeNull();
    expect(hide).not.toHaveBeenCalled();
    await act(async () => {
      release();
      await settle();
    });
    expect(await screen.findByTestId('battle-screen')).toBeOnTheScreen();
    expect(screen.getByText('サーバーがありません')).toBeOnTheScreen();
    expect(hide).toHaveBeenCalled();
  });
});
