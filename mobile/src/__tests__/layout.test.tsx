import { fireEvent, renderRouter, screen } from 'expo-router/testing-library';
import * as Localization from 'expo-localization';

import * as config from '../config';
import RootLayout from '../app/_layout';
import TabsLayout from '../app/(tabs)/_layout';
import BattleScreen from '../app/(tabs)/index';
import SettingsScreen from '../app/(tabs)/settings';

jest.mock('expo-localization', () => ({ getLocales: jest.fn(() => [{ languageCode: 'en' }]) }));

const routes = {
  _layout: RootLayout,
  '(tabs)/_layout': TabsLayout,
  '(tabs)/index': BattleScreen,
  '(tabs)/settings': SettingsScreen,
};

// Expo inlines EXPO_PUBLIC_KILLY_SERVER_URL when it builds the app, so the tests replace the
// function that reads it; serverURL itself is tested in config.test.ts.
afterEach(() => {
  jest.restoreAllMocks();
});

async function startApp(languageCode = 'en', url: string | null = 'https://killy.example') {
  jest.spyOn(config, 'buildServerURL').mockImplementation(() => config.serverURL(url ?? undefined));
  jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode } as Localization.Locale]);
  await renderRouter(routes, { initialUrl: '/' });
}

test('the app opens on the Battle tab and switches between the two tabs', async () => {
  await startApp();
  expect(await screen.findByTestId('battle-screen')).toBeOnTheScreen();
  await fireEvent.press(screen.getByLabelText('Settings'));
  expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
  await fireEvent.press(screen.getByLabelText('Battle'));
  expect(await screen.findByTestId('battle-screen')).toBeOnTheScreen();
});

test('a phone set to Japanese shows the app in Japanese', async () => {
  await startApp('ja');
  expect(await screen.findByLabelText('撃退')).toBeOnTheScreen();
  expect(screen.getByLabelText('設定')).toBeOnTheScreen();
  expect(screen.getByText('サーバーがありません')).toBeOnTheScreen();
});

test('a phone set to French shows the app in English', async () => {
  await startApp('fr');
  expect(await screen.findByLabelText('Battle')).toBeOnTheScreen();
  expect(screen.getByText('No servers yet')).toBeOnTheScreen();
});

test('without a Killy server for the build the app shows an error and no tabs', async () => {
  await startApp('en', null);
  expect(await screen.findByTestId('config-error')).toBeOnTheScreen();
  expect(screen.getByText(/EXPO_PUBLIC_KILLY_SERVER_URL/)).toBeOnTheScreen();
  expect(screen.queryByTestId('battle-screen')).toBeNull();
  expect(screen.queryByLabelText('Battle')).toBeNull();
});

test('Japanese text is drawn in the Japanese font', async () => {
  await startApp('ja');
  expect(await screen.findByText('サーバーがありません')).toHaveStyle({ fontFamily: 'ZenKakuGothicNew_700Bold' });
});

test('English text is drawn in Geist', async () => {
  await startApp('en');
  expect(await screen.findByText('No servers yet')).toHaveStyle({ fontFamily: 'GeistMono_600SemiBold' });
});
