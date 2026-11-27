import type { NotificationsDeps } from './notifications';

/**
 * The notifications on the Web: none. The browser gets no push from the Killy server (it never
 * asks for a push token), so the permission is reported as blocked and nothing is registered.
 * Metro picks this file instead of notifications.ts, which loads expo-notifications.
 */
export function makeNotifications(): NotificationsDeps {
  return {
    permission: async () => 'denied',
    prepare: async () => {},
    requestPermission: async () => 'denied',
    pushToken: async () => {
      throw new Error('push notifications are not available on the Web');
    },
    onPushTokenChange: () => () => {},
    openSettings: async () => {},
    openedByTap: () => false,
    clearTap: () => {},
    onTap: () => () => {},
  };
}
