import Constants from 'expo-constants';
import * as Notifications from 'expo-notifications';
// Named, not through the namespace: the enums are re-exported with export *, which the import
// lint rule does not follow.
import { AndroidImportance, IosAuthorizationStatus } from 'expo-notifications';
import { Linking, Platform } from 'react-native';

/** Whether the OS lets Killy show notifications. */
export type NotificationPermission = 'granted' | 'denied' | 'undetermined';

/** The OS notifications, as the app uses them. Screen tests replace them. */
export type NotificationsDeps = {
  permission(): Promise<NotificationPermission>;
  /** Makes the channel pushes arrive in (Android; nothing elsewhere), named in the app's language. */
  prepare(channelName: string): Promise<void>;
  requestPermission(): Promise<NotificationPermission>;
  /** The Expo push token. Rejects when there is none (no permission, a simulator, no network). */
  pushToken(): Promise<string>;
  /** Calls cb whenever the OS gives the phone a new push token, until the returned function is called. */
  onPushTokenChange(cb: () => void): () => void;
  /** Opens Killy's page in the system settings. */
  openSettings(): Promise<void>;
  /** Whether a tap on a notification opened the app and has not been handled yet. */
  openedByTap(): boolean;
  /** Marks the tap that opened the app as handled. */
  clearTap(): void;
  /** Calls cb for each tap on a notification while the app runs, until the returned function is called. */
  onTap(cb: () => void): () => void;
};

/** The Android channel pushes arrive in: the plugin's defaultChannel in app.json. Killy server sets no channelId. */
export const channelId = 'default';

/**
 * What a push does while Killy is in front: nothing. The battle screen already shows the latest
 * state, so the banner, the list entry, the sound and the badge would only repeat it (D6 3).
 */
export const foregroundBehavior: Notifications.NotificationBehavior = {
  shouldShowBanner: false,
  shouldShowList: false,
  shouldPlaySound: false,
  shouldSetBadge: false,
};

// Set when this module is loaded, before any push can arrive while the app runs.
Notifications.setNotificationHandler({ handleNotification: async () => foregroundBehavior });

/**
 * The permission as Killy reads it. On iOS, provisional and ephemeral permission also let pushes
 * through.
 */
export function permissionOf(s: Notifications.NotificationPermissionsStatus): NotificationPermission {
  if (s.ios) {
    switch (s.ios.status) {
      case IosAuthorizationStatus.AUTHORIZED:
      case IosAuthorizationStatus.PROVISIONAL:
      case IosAuthorizationStatus.EPHEMERAL:
        return 'granted';
      case IosAuthorizationStatus.DENIED:
        return 'denied';
      default:
        return 'undetermined';
    }
  }
  if (s.granted) return 'granted';
  // Android reports denied before Killy has ever asked; it is blocked only once the OS will not
  // show the prompt again (after the operator refused it twice, or turned it off in the settings).
  return s.canAskAgain ? 'undetermined' : 'denied';
}

/** Only a tap on the notification itself opens the Battle tab, not another action on it. */
function isTap(r: Notifications.NotificationResponse | null): boolean {
  return r !== null && r.actionIdentifier === Notifications.DEFAULT_ACTION_IDENTIFIER;
}

/** The real ones, on expo-notifications. */
export function makeNotifications(): NotificationsDeps {
  return {
    permission: async () => permissionOf(await Notifications.getPermissionsAsync()),
    prepare: async (channelName) => {
      // Android 13 and later shows no permission prompt and gives no token without a channel.
      if (Platform.OS === 'android') {
        await Notifications.setNotificationChannelAsync(channelId, {
          name: channelName,
          importance: AndroidImportance.HIGH,
        });
      }
    },
    requestPermission: async () => permissionOf(await Notifications.requestPermissionsAsync()),
    pushToken: async () => {
      const projectId = Constants.expoConfig?.extra?.eas?.projectId;
      if (typeof projectId !== 'string') {
        throw new Error('no EAS project ID in app.json');
      }
      return (await Notifications.getExpoPushTokenAsync({ projectId })).data;
    },
    // The listener gets the device token; the Expo token is asked for again by the caller.
    onPushTokenChange: (cb) => {
      const sub = Notifications.addPushTokenListener(() => cb());
      return () => sub.remove();
    },
    openSettings: () => Linking.openSettings(),
    openedByTap: () => isTap(Notifications.getLastNotificationResponse()),
    clearTap: () => Notifications.clearLastNotificationResponse(),
    onTap: (cb) => {
      const sub = Notifications.addNotificationResponseReceivedListener((r) => {
        if (isTap(r)) cb();
      });
      return () => sub.remove();
    },
  };
}
