import * as Notifications from 'expo-notifications';
import { AndroidImportance, IosAuthorizationStatus } from 'expo-notifications';
import { Linking, Platform } from 'react-native';

import { channelId, foregroundBehavior, makeNotifications, permissionOf } from '../killy/notifications';

jest.mock('expo-constants', () => ({ expoConfig: { extra: { eas: { projectId: 'project-1' } } } }));

const n = jest.mocked(Notifications);

function status(over: Partial<Notifications.NotificationPermissionsStatus>): Notifications.NotificationPermissionsStatus {
  return { status: 'undetermined', granted: false, canAskAgain: true, expires: 'never', ...over } as Notifications.NotificationPermissionsStatus;
}

function ios(s: IosAuthorizationStatus) {
  return status({ ios: { status: s } as Notifications.NotificationPermissionsStatus['ios'] });
}

const tap = (actionIdentifier: string) => ({ actionIdentifier, notification: {} }) as Notifications.NotificationResponse;

test('pushes are not shown, heard or counted while the app is in front, set when the module loads', async () => {
  expect(foregroundBehavior).toEqual({ shouldShowBanner: false, shouldShowList: false, shouldPlaySound: false, shouldSetBadge: false });
  // Recorded at import, before any test clears mocks.
  const handler = n.setNotificationHandler.mock.calls[0]?.[0];
  expect(handler).not.toBeNull();
  await expect(handler!.handleNotification({} as Notifications.Notification)).resolves.toEqual(foregroundBehavior);
});

describe('the permission', () => {
  test.each([
    [IosAuthorizationStatus.AUTHORIZED, 'granted'],
    [IosAuthorizationStatus.PROVISIONAL, 'granted'],
    [IosAuthorizationStatus.EPHEMERAL, 'granted'],
    [IosAuthorizationStatus.DENIED, 'denied'],
    [IosAuthorizationStatus.NOT_DETERMINED, 'undetermined'],
  ])('on iOS, status %i is %s', (s, want) => {
    expect(permissionOf(ios(s))).toBe(want);
  });

  test('on iOS the detailed status decides, not the root one', () => {
    expect(permissionOf({ ...ios(IosAuthorizationStatus.PROVISIONAL), status: 'denied', granted: false } as never)).toBe('granted');
  });

  test.each([
    [status({ status: 'granted' as never, granted: true }), 'granted'],
    // What Android 13 reports before the app has ever asked.
    [status({ status: 'denied' as never, canAskAgain: true }), 'undetermined'],
    [status({ status: 'denied' as never, canAskAgain: false }), 'denied'],
    [status({}), 'undetermined'],
  ])('on Android %o is %s', (s, want) => {
    expect(permissionOf(s)).toBe(want);
  });

  test('asking reads the answer the same way', async () => {
    n.requestPermissionsAsync.mockResolvedValueOnce(ios(IosAuthorizationStatus.DENIED));
    await expect(makeNotifications().requestPermission()).resolves.toBe('denied');
  });
});

test('the push token is the Expo token for the EAS project in app.json', async () => {
  n.getExpoPushTokenAsync.mockResolvedValueOnce({ type: 'expo', data: 'ExponentPushToken[abc]' });
  await expect(makeNotifications().pushToken()).resolves.toBe('ExponentPushToken[abc]');
  expect(n.getExpoPushTokenAsync).toHaveBeenCalledWith({ projectId: 'project-1' });
});

describe('the channel', () => {
  const os = Platform.OS;
  afterEach(() => {
    Platform.OS = os;
    n.setNotificationChannelAsync.mockClear();
  });

  test('on Android it is the default channel, named as given and shown with high importance', async () => {
    Platform.OS = 'android';
    await makeNotifications().prepare('Zombie processes');
    expect(channelId).toBe('default');
    expect(n.setNotificationChannelAsync).toHaveBeenCalledWith('default', { name: 'Zombie processes', importance: AndroidImportance.HIGH });
  });

  test('on iOS there is none', async () => {
    Platform.OS = 'ios';
    await makeNotifications().prepare('Zombie processes');
    expect(n.setNotificationChannelAsync).not.toHaveBeenCalled();
  });
});

describe('taps', () => {
  test('the tap that opened the app counts only when it is on the notification itself', () => {
    const d = makeNotifications();
    n.getLastNotificationResponse.mockReturnValueOnce(tap(Notifications.DEFAULT_ACTION_IDENTIFIER));
    expect(d.openedByTap()).toBe(true);
    n.getLastNotificationResponse.mockReturnValueOnce(tap('other-action'));
    expect(d.openedByTap()).toBe(false);
    n.getLastNotificationResponse.mockReturnValueOnce(null);
    expect(d.openedByTap()).toBe(false);
  });

  test('a tap while running calls back only for the notification itself, until unsubscribed', () => {
    const remove = jest.fn();
    let listener: (r: Notifications.NotificationResponse) => void = () => {};
    n.addNotificationResponseReceivedListener.mockImplementationOnce((l) => {
      listener = l;
      return { remove };
    });
    const cb = jest.fn();
    const stop = makeNotifications().onTap(cb);
    listener(tap('other-action'));
    expect(cb).not.toHaveBeenCalled();
    listener(tap(Notifications.DEFAULT_ACTION_IDENTIFIER));
    expect(cb).toHaveBeenCalledTimes(1);
    stop();
    expect(remove).toHaveBeenCalled();
  });

  test('clearing marks the tap handled', () => {
    makeNotifications().clearTap();
    expect(n.clearLastNotificationResponse).toHaveBeenCalled();
  });
});

test('a new device token calls back until unsubscribed', () => {
  const remove = jest.fn();
  let listener: () => void = () => {};
  n.addPushTokenListener.mockImplementationOnce((l) => {
    listener = l as () => void;
    return { remove };
  });
  const cb = jest.fn();
  const stop = makeNotifications().onPushTokenChange(cb);
  listener();
  expect(cb).toHaveBeenCalledTimes(1);
  stop();
  expect(remove).toHaveBeenCalled();
});

test('the system settings open on Killy’s page', async () => {
  const open = jest.spyOn(Linking, 'openSettings').mockResolvedValue();
  await makeNotifications().openSettings();
  expect(open).toHaveBeenCalled();
});
