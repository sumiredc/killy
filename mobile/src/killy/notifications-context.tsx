import { createContext, type ReactNode, useCallback, useContext, useEffect, useEffectEvent, useRef, useState } from 'react';
import { AppState } from 'react-native';

import { useI18n } from '@/i18n/context';
import type { Key } from '@/i18n';
import { makePrefs } from '@/prefs';

import { UpdateError } from './client';
import { useKilly } from './context';
import type { NotificationPermission, NotificationsDeps } from './notifications';

type Notifications = {
  /** Whether the OS lets Killy notify, or null before it was asked. */
  permission: NotificationPermission | null;
  /** The app-wide switch as shown. */
  notify: boolean;
  /** A change of the switch is on its way to the Killy server. */
  sending: boolean;
  /** Why the last change of the switch did not go through, until the next one. */
  error: Key | null;
  setNotify(v: boolean): void;
  openSettings(): void;
  /** How many taps on notifications have opened the Battle tab in this run. */
  taps: number;
  /** For useOpenBattleOnTap. */
  tapSource: NotificationsDeps;
  countTap(): void;
};

const NotificationsContext = createContext<Notifications | null>(null);

const updateErrorText: Record<UpdateError['kind'], Key> = {
  unreachable: 'pairUnreachable',
  not_registered: 'notifNotRegistered',
  other: 'notifFailed',
};

/**
 * Registers the push token once the device is registered (at every start, and whenever the OS
 * gives a new one), keeps the OS permission up to date when the app comes back to the front, and
 * holds the app-wide switch, saved on the phone because the Killy server has no API that returns it.
 */
export function NotificationsProvider({ children }: { children: ReactNode }) {
  const { client, registration, notifications: n } = useKilly();
  const { t } = useI18n();
  const [prefs] = useState(makePrefs);
  const [permission, setPermission] = useState<NotificationPermission | null>(null);
  const [notify, setNotifyValue] = useState(true);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<Key | null>(null);
  const [taps, setTaps] = useState(0);
  const countTap = useCallback(() => setTaps((c) => c + 1), []);

  const registered = registration === 'registered';
  // The token the Killy server has from this start; a failed send leaves it unset, so the same
  // token is sent again when the OS gives it again.
  const sent = useRef<string | null>(null);
  const channelName = t('notifChannel');

  const register = useCallback(async () => {
    let token: string;
    try {
      token = await n.pushToken();
    } catch {
      return;
    }
    if (token === sent.current) {
      return;
    }
    try {
      await client.updateDevice({ pushToken: token });
      sent.current = token;
    } catch {
      // Nothing is shown: battle and adding servers work without it. Sent again on the next start
      // or the next new token.
    }
  }, [client, n]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        let p = await n.permission();
        if (registered) {
          await n.prepare(channelName).catch(() => {});
          if (p === 'undetermined') {
            p = await n.requestPermission();
          }
        }
        if (cancelled) return;
        setPermission(p);
        if (registered && p === 'granted') {
          await register();
        }
      } catch {
        // The OS did not answer; asked again when the app comes back to the front.
      }
    })();
    return () => {
      cancelled = true;
    };
    // The channel name is only for creating it; a language change does not ask again.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [n, registered, register]);

  const onNewToken = useEffectEvent(() => {
    if (registered && permission === 'granted') {
      void register();
    }
  });
  useEffect(() => n.onPushTokenChange(() => onNewToken()), [n]);

  // The operator may have changed the permission in the system settings.
  const onPermission = useEffectEvent((p: NotificationPermission) => {
    const before = permission;
    setPermission(p);
    if (p === 'granted' && before !== 'granted' && registered) {
      void register();
    }
  });
  useEffect(() => {
    const sub = AppState.addEventListener('change', (s) => {
      if (s === 'active') {
        n.permission().then((p) => onPermission(p), () => {});
      }
    });
    return () => sub.remove();
  }, [n]);

  useEffect(() => {
    prefs.notify
      .get()
      .then((v) => {
        if (v !== null) setNotifyValue(v);
      })
      // Unreadable now (iOS while the phone is locked): shown on, and the saved value is kept.
      .catch(() => {});
  }, [prefs]);

  const setNotify = (v: boolean) => {
    if (sending) return;
    setError(null);
    setSending(true);
    setNotifyValue(v);
    client
      .updateDevice({ notify: v })
      .then(
        () => prefs.notify.set(v).catch(() => {}),
        (e: unknown) => {
          setNotifyValue(!v);
          setError(updateErrorText[e instanceof UpdateError ? e.kind : 'other']);
        },
      )
      .finally(() => setSending(false));
  };

  return (
    <NotificationsContext.Provider
      value={{
        permission,
        notify,
        sending,
        error,
        setNotify,
        openSettings: () => void n.openSettings().catch(() => {}),
        taps,
        tapSource: n,
        countTap,
      }}>
      {children}
    </NotificationsContext.Provider>
  );
}

export function useNotifications(): Notifications {
  const ctx = useContext(NotificationsContext);
  if (!ctx) {
    throw new Error('useNotifications outside NotificationsProvider');
  }
  return ctx;
}

/**
 * Calls open for a tap on a notification: the one that started the app, seen on the first draw,
 * and any while the app runs. A handled tap is cleared, so drawing again does not open again.
 */
export function useOpenBattleOnTap(open: () => void) {
  const { tapSource, countTap } = useNotifications();
  const onOpen = useEffectEvent(open);
  useEffect(() => {
    const handle = () => {
      tapSource.clearTap();
      countTap();
      onOpen();
    };
    if (tapSource.openedByTap()) {
      handle();
    }
    return tapSource.onTap(handle);
  }, [tapSource, countTap]);
}
