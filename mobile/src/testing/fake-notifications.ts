import type { NotificationPermission, NotificationsDeps } from '../killy/notifications';
import type { Prefs } from '../prefs';

/**
 * The OS notifications for screen tests. The permission, the push token and a pending tap are set
 * by the test; every call is recorded. As made, it is a phone that blocks notifications and has no
 * push token, so a test that is not about notifications sends nothing for them.
 */
export class FakeNotifications implements NotificationsDeps {
  /** What permission() answers; requestPermission() changes it to `answer`. */
  current: NotificationPermission = 'denied';
  /** What the OS answers when asked for permission. */
  answer: NotificationPermission = 'denied';
  /** The Expo push token, or null when there is none. */
  token: string | null = null;
  /** A tap that opened the app. */
  tapped = false;
  readonly calls: string[] = [];
  private tokenListeners = new Set<() => void>();
  private tapListeners = new Set<() => void>();

  permission = async () => {
    this.calls.push('permission');
    return this.current;
  };
  prepare = async (channelName: string) => {
    this.calls.push(`prepare ${channelName}`);
  };
  requestPermission = async () => {
    this.calls.push('requestPermission');
    this.current = this.answer;
    return this.current;
  };
  pushToken = async () => {
    this.calls.push('pushToken');
    if (this.token === null) {
      throw new Error('no push token');
    }
    return this.token;
  };
  onPushTokenChange = (cb: () => void) => {
    this.tokenListeners.add(cb);
    return () => void this.tokenListeners.delete(cb);
  };
  openSettings = async () => {
    this.calls.push('openSettings');
  };
  openedByTap = () => this.tapped;
  clearTap = () => {
    this.calls.push('clearTap');
    this.tapped = false;
  };
  onTap = (cb: () => void) => {
    this.tapListeners.add(cb);
    return () => void this.tapListeners.delete(cb);
  };

  /** The OS gives the phone a new push token. */
  changeToken(token: string) {
    this.token = token;
    for (const cb of this.tokenListeners) cb();
  }

  /** The operator taps a notification while the app runs. */
  tap() {
    for (const cb of this.tapListeners) cb();
  }
}

/** Prefs in memory. `failRead` makes every get reject, as when iOS keeps the store locked. */
export class MemoryPrefs implements Prefs {
  saved: { notify?: boolean } = {};
  failRead = false;
  notify = {
    get: async () => {
      if (this.failRead) throw new Error('locked');
      return this.saved.notify ?? null;
    },
    set: async (v: boolean) => {
      this.saved.notify = v;
    },
  };
}
