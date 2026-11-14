import { act, fireEvent, renderRouter, screen, waitFor, within } from 'expo-router/testing-library';
import { router } from 'expo-router';
import * as Localization from 'expo-localization';

import * as config from '../config';
import RootLayout from '../app/_layout';
import TabsLayout from '../app/(tabs)/_layout';
import BattleScreen from '../app/(tabs)/index';
import SettingsScreen from '../app/(tabs)/settings';
import PairRoute from '../app/pair';
import * as deps from '../killy/deps';
import * as prefs from '../prefs';
import { FakeNotifications, MemoryPrefs } from '../testing/fake-notifications';
import { FakeServer, type FakeStream, settle } from '../testing/fake-server';

jest.mock('expo-localization', () => ({ getLocales: jest.fn(() => [{ languageCode: 'en' }]) }));

// The camera the tests hold: whether permission is granted, and the scan callback of the open view.
const mockCamera: {
  granted: boolean;
  requests: number;
  scan: ((r: { type: string; data: string }) => void) | null;
  barcodeTypes: string[] | undefined;
} = { granted: true, requests: 0, scan: null, barcodeTypes: undefined };

jest.mock('expo-camera', () => {
  const { View } = jest.requireActual('react-native');
  return {
    useCameraPermissions: () => [
      null,
      async () => {
        mockCamera.requests++;
        return { granted: mockCamera.granted, status: mockCamera.granted ? 'granted' : 'denied', canAskAgain: false, expires: 'never' };
      },
    ],
    CameraView: (props: { onBarcodeScanned?: (r: { type: string; data: string }) => void; barcodeScannerSettings?: { barcodeTypes: string[] } }) => {
      mockCamera.scan = props.onBarcodeScanned ?? null;
      mockCamera.barcodeTypes = props.barcodeScannerSettings?.barcodeTypes;
      return <View testID="qr-camera" />;
    },
  };
});

const appServer = 'https://killy.example';
const routes = {
  _layout: RootLayout,
  '(tabs)/_layout': TabsLayout,
  '(tabs)/index': BattleScreen,
  '(tabs)/settings': SettingsScreen,
  pair: PairRoute,
};

beforeEach(() => {
  mockCamera.granted = true;
  mockCamera.requests = 0;
  mockCamera.scan = null;
  mockCamera.barcodeTypes = undefined;
});

afterEach(() => {
  jest.restoreAllMocks();
});

function qr(code: string, server = appServer) {
  return `killy://pair?code=${code}&server=${encodeURIComponent(server)}`;
}

// The event stream of the app the last startApp started.
let appStream: FakeStream;

async function startApp(initialUrl = '/', languageCode = 'en'): Promise<FakeServer> {
  const server = new FakeServer();
  appStream = server.stream();
  jest.spyOn(config, 'buildServerURL').mockReturnValue(appServer);
  jest.mocked(Localization.getLocales).mockReturnValue([{ languageCode } as Localization.Locale]);
  jest.spyOn(prefs, 'makePrefs').mockReturnValue(new MemoryPrefs());
  jest.spyOn(deps, 'makeDeps').mockReturnValue({
    fetch: server.fetch,
    tokens: { get: async () => 'dev-token', set: async () => {} },
    random: () => 0,
    randomBytes: (n) => new Uint8Array(n),
    notifications: new FakeNotifications(),
  });
  await renderRouter(routes, { initialUrl });
  await act(settle);
  return server;
}

async function openAddSheet() {
  await fireEvent.press(screen.getByLabelText('Settings'));
  await fireEvent.press(await screen.findByText('Add server'));
  return within(await screen.findByTestId('add-server-sheet'));
}

/** Requests that went to the Killy server other than the event stream. */
function sentToServer(server: FakeServer) {
  return server.requests.filter((r) => r.route !== 'GET /app/v1/events');
}

async function scan(data: string) {
  await act(async () => {
    mockCamera.scan?.({ type: 'qr', data });
    await settle();
  });
}

describe('scanning a QR code in the Add server sheet', () => {
  test('Scan QR code asks for the camera and opens it for QR codes only', async () => {
    await startApp();
    const sheet = await openAddSheet();
    await fireEvent.press(sheet.getByRole('button', { name: 'Scan QR code' }));
    expect(mockCamera.requests).toBe(1);
    expect(screen.getByTestId('qr-camera')).toBeOnTheScreen();
    expect(mockCamera.barcodeTypes).toEqual(['qr']);
  });

  test('the code of a killy://pair QR goes into the code field, the camera closes and nothing is sent', async () => {
    const server = await startApp();
    const sheet = await openAddSheet();
    await fireEvent.changeText(sheet.getByLabelText('Display name (optional)'), 'my box');
    await fireEvent.press(sheet.getByRole('button', { name: 'Scan QR code' }));
    await scan(qr('K7Q4XP'));
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('K7Q4XP');
    expect(sheet.getByLabelText('Display name (optional)')).toHaveDisplayValue('my box');
    expect(screen.queryByTestId('qr-camera')).toBeNull();
    expect(sentToServer(server)).toEqual([]);
    expect(sheet.getByRole('button', { name: 'Add' })).toBeEnabled();
  });

  test('the server in the QR may differ only in how it is written', async () => {
    await startApp();
    const sheet = await openAddSheet();
    await fireEvent.press(sheet.getByRole('button', { name: 'Scan QR code' }));
    await scan(qr('K7Q4XP', 'HTTPS://Killy.Example:443/'));
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('K7Q4XP');
  });

  test('only the first QR read is used', async () => {
    await startApp();
    const sheet = await openAddSheet();
    await fireEvent.press(sheet.getByRole('button', { name: 'Scan QR code' }));
    await act(async () => {
      const onScan = mockCamera.scan;
      onScan?.({ type: 'qr', data: qr('K7Q4XP') });
      onScan?.({ type: 'qr', data: qr('A1B2C3') });
      await settle();
    });
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('K7Q4XP');
  });

  test('a QR for another Killy server is an error, keeps the field and sends nothing', async () => {
    const server = await startApp();
    const sheet = await openAddSheet();
    await fireEvent.changeText(sheet.getByLabelText('Pairing code'), 'ABC');
    await fireEvent.press(sheet.getByRole('button', { name: 'Scan QR code' }));
    await scan(qr('K7Q4XP', 'https://evil.example'));
    expect(sheet.getByText('This QR code is for another Killy server. Run killy agent pair against https://killy.example.')).toBeOnTheScreen();
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('ABC');
    expect(screen.queryByTestId('qr-camera')).toBeNull();
    expect(sentToServer(server)).toEqual([]);
  });

  test.each([
    ['another URL', 'https://example.com/pair?code=K7Q4XP'],
    ['a killy://pair URL without a code', `killy://pair?server=${encodeURIComponent(appServer)}`],
    ['a code that is not 6 characters', qr('K7Q4X')],
    ['plain text', 'hello'],
  ])('%s is not a Killy pairing QR and keeps the field', async (_, data) => {
    const server = await startApp();
    const sheet = await openAddSheet();
    await fireEvent.changeText(sheet.getByLabelText('Pairing code'), 'ABC');
    await fireEvent.press(sheet.getByRole('button', { name: 'Scan QR code' }));
    await scan(data);
    expect(sheet.getByText('This is not a Killy pairing QR code.')).toBeOnTheScreen();
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('ABC');
    expect(sentToServer(server)).toEqual([]);
  });

  test('without camera permission the sheet says the code can be typed, and typing still works', async () => {
    mockCamera.granted = false;
    await startApp();
    const sheet = await openAddSheet();
    await fireEvent.press(sheet.getByRole('button', { name: 'Scan QR code' }));
    expect(screen.queryByTestId('qr-camera')).toBeNull();
    expect(sheet.getByText('The camera is not available. Type the 6-character code instead.')).toBeOnTheScreen();
    await fireEvent.changeText(sheet.getByLabelText('Pairing code'), 'K7Q4XP');
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('K7Q4XP');
    expect(sheet.getByRole('button', { name: 'Add' })).toBeEnabled();
  });

  test('the camera can be closed without reading anything', async () => {
    await startApp();
    const sheet = await openAddSheet();
    await fireEvent.press(sheet.getByRole('button', { name: 'Scan QR code' }));
    await fireEvent.press(screen.getByRole('button', { name: 'Close the camera' }));
    expect(screen.queryByTestId('qr-camera')).toBeNull();
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('');
  });

  test('in Japanese the messages are in Japanese', async () => {
    await startApp('/', 'ja');
    await fireEvent.press(screen.getByLabelText('設定'));
    await fireEvent.press(await screen.findByText('サーバーを追加'));
    const sheet = within(await screen.findByTestId('add-server-sheet'));
    await fireEvent.press(sheet.getByRole('button', { name: 'QR コードを読み取る' }));
    await scan(qr('K7Q4XP', 'https://evil.example'));
    expect(sheet.getByText('別の Killy server の QR コードです。https://killy.example に向けて killy agent pair を実行してください。')).toBeOnTheScreen();
  });
});

describe('opening the app from a killy://pair link', () => {
  test('the Add server sheet opens with the code in the field and nothing is sent', async () => {
    const server = await startApp(`/pair?code=K7Q4XP&server=${encodeURIComponent(appServer)}`);
    const sheet = within(await screen.findByTestId('add-server-sheet'));
    expect(screen.getByTestId('settings-screen')).toBeOnTheScreen();
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('K7Q4XP');
    expect(sentToServer(server)).toEqual([]);
  });

  test('a link for another Killy server opens the sheet with the error and sends nothing', async () => {
    const server = await startApp(`/pair?code=K7Q4XP&server=${encodeURIComponent('https://evil.example')}`);
    const sheet = within(await screen.findByTestId('add-server-sheet'));
    expect(sheet.getByText(/This QR code is for another Killy server/)).toBeOnTheScreen();
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('');
    expect(sentToServer(server)).toEqual([]);
  });

  test('a link whose code is not 6 characters opens the sheet with the error', async () => {
    await startApp(`/pair?code=K7Q4X&server=${encodeURIComponent(appServer)}`);
    const sheet = within(await screen.findByTestId('add-server-sheet'));
    expect(sheet.getByText('This is not a Killy pairing QR code.')).toBeOnTheScreen();
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('');
  });

  test('a second link while the sheet is open updates it in place, so iOS never presents a second sheet', async () => {
    await startApp(`/pair?code=K7Q4XP&server=${encodeURIComponent('https://evil.example')}`);
    const first = await screen.findByTestId('add-server-sheet');
    expect(within(first).getByText(/This QR code is for another Killy server/)).toBeOnTheScreen();
    await act(async () => {
      router.push(`/pair?code=A1B2C3&server=${encodeURIComponent(appServer)}`);
      await settle();
    });
    const sheet = await screen.findByTestId('add-server-sheet');
    expect(screen.getAllByTestId('settings-screen', { includeHiddenElements: true })).toHaveLength(1);
    expect(sheet === first).toBe(true);
    expect(within(sheet).getByLabelText('Pairing code')).toHaveDisplayValue('A1B2C3');
    expect(within(sheet).queryByText(/another Killy server/)).toBeNull();
    expect(screen.getAllByTestId('add-server-sheet')).toHaveLength(1);
    await fireEvent.press(within(sheet).getByText('Cancel'));
    await waitFor(() => expect(screen.queryByTestId('add-server-sheet')).toBeNull());
  });

  test('a link straight to Settings with a code in it opens nothing: only killy://pair is checked and taken', async () => {
    const server = await startApp('/settings?pairCode=K7Q4XP');
    expect(await screen.findByTestId('settings-screen')).toBeOnTheScreen();
    await act(settle);
    expect(screen.queryByTestId('add-server-sheet')).toBeNull();
    expect(sentToServer(server)).toEqual([]);
  });

  test('a refused link keeps what is in the code field and shows why', async () => {
    await startApp();
    const sheet = await openAddSheet();
    await fireEvent.changeText(sheet.getByLabelText('Pairing code'), 'ABC');
    await act(async () => {
      router.push(`/pair?code=K7Q4XP&server=${encodeURIComponent('https://evil.example')}`);
      await settle();
    });
    const open = within(await screen.findByTestId('add-server-sheet'));
    expect(open.getByLabelText('Pairing code')).toHaveDisplayValue('ABC');
    expect(open.getByText(/This QR code is for another Killy server/)).toBeOnTheScreen();
  });

  test('a link that arrives while a server is being added is taken once that claim fails', async () => {
    const server = await startApp();
    const claim = server.hold('POST /app/v1/pairings/claim');
    const sheet = await openAddSheet();
    await fireEvent.changeText(sheet.getByLabelText('Pairing code'), 'ZZZZZZ');
    await fireEvent.press(sheet.getByRole('button', { name: 'Add' }));
    expect(await screen.findByText('Adding…')).toBeOnTheScreen();
    await act(async () => {
      router.push(`/pair?code=A1B2C3&server=${encodeURIComponent(appServer)}`);
      await settle();
    });
    // The running claim is not interrupted.
    expect(screen.getByText('Adding…')).toBeOnTheScreen();
    await act(() => claim.resolve(404, { error: 'code_unknown' }));
    const open = within(await screen.findByTestId('add-server-sheet'));
    expect(open.getByLabelText('Pairing code')).toHaveDisplayValue('A1B2C3');
  });

  test('a link while the Server sheet is open waits until it closes: only one sheet is ever open', async () => {
    await startApp();
    await act(() => appStream.send(`event: state\ndata: ${JSON.stringify({ servers: [{ id: 'a1', name: 'web-01', hostname: 'web-01', notify: true, disconnected: false, parents: [] }] })}\n\n`));
    await fireEvent.press(screen.getByLabelText('Settings'));
    await fireEvent.press(await screen.findByRole('button', { name: /^web-01/ }));
    expect(await screen.findByTestId('server-sheet')).toBeOnTheScreen();
    await act(async () => {
      router.push(`/pair?code=A1B2C3&server=${encodeURIComponent(appServer)}`);
      await settle();
    });
    expect(screen.getByTestId('server-sheet')).toBeOnTheScreen();
    expect(screen.queryByTestId('add-server-sheet')).toBeNull();
    await fireEvent.press(screen.getByRole('button', { name: 'Done' }));
    const sheet = within(await screen.findByTestId('add-server-sheet'));
    expect(screen.queryByTestId('server-sheet')).toBeNull();
    expect(sheet.getByLabelText('Pairing code')).toHaveDisplayValue('A1B2C3');
  });

  test('the sheet does not open again by itself once it was closed', async () => {
    await startApp(`/pair?code=K7Q4XP&server=${encodeURIComponent(appServer)}`);
    const sheet = within(await screen.findByTestId('add-server-sheet'));
    await fireEvent.press(sheet.getByText('Cancel'));
    await waitFor(() => expect(screen.queryByTestId('add-server-sheet')).toBeNull());
    await fireEvent.press(screen.getByLabelText('Battle'));
    await fireEvent.press(screen.getByLabelText('Settings'));
    await act(settle);
    expect(screen.queryByTestId('add-server-sheet')).toBeNull();
  });

  test('Add sends the claim with the code from the link', async () => {
    const server = await startApp(`/pair?code=K7Q4XP&server=${encodeURIComponent(appServer)}`);
    server.respond('POST /app/v1/pairings/claim', 200, { id: 'a1', name: 'web-01', hostname: 'web-01', notify: true });
    const sheet = within(await screen.findByTestId('add-server-sheet'));
    await fireEvent.press(sheet.getByRole('button', { name: 'Add' }));
    expect(await screen.findByText('Added web-01')).toBeOnTheScreen();
    expect(server.requests.filter((r) => r.route === 'POST /app/v1/pairings/claim')).toEqual([
      expect.objectContaining({ body: { code: 'K7Q4XP' } }),
    ]);
  });
});
