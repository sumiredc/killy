import { createContext, type ReactNode, useContext, useEffect, useMemo, useState } from 'react';
import { Platform } from 'react-native';

import { buildWebDemoEnabled } from '@/config';
import type { Key } from '@/i18n';

import { claimMessages } from './claim-messages';
import { ClaimError } from './client';
import { useKilly } from './context';

type WebDemo = {
  /** This is the Web version in its demo-only form. */
  enabled: boolean;
  /** Why the demo server is not there, as a message key; null while nothing is wrong. */
  notice: Key | null;
};

const off: WebDemo = { enabled: false, notice: null };
const WebDemoContext = createContext<WebDemo>(off);

/**
 * The demo-only Web version (EXPO_PUBLIC_KILLY_WEB_DEMO=1 on the Web): once the device is
 * registered, it adds the public demo server, as "Try the demo server" does, so the battle screen
 * has its parents with no code to type. A start makes one call: the Killy server keeps a second call
 * as the one link, so a demo server removed in Settings is back at the next start. When it cannot be
 * added, `notice` says why. Anywhere else, in a native build or without the variable, it does nothing.
 */
export function WebDemoProvider({ children }: { children: ReactNode }) {
  const enabled = Platform.OS === 'web' && buildWebDemoEnabled();
  const { client, registration, state } = useKilly();
  const [failure, setFailure] = useState<Key | null>(null);

  useEffect(() => {
    if (!enabled || registration !== 'registered') {
      return;
    }
    client.addDemoServer().then(
      () => {},
      (e: unknown) => setFailure(claimMessages[e instanceof ClaimError ? e.kind : 'other']),
    );
  }, [enabled, client, registration]);

  // A registration that is rate limited is said by the battle screen already, for every build.
  // Nor once a server is there, whatever the failed call said: it may have been linked all the same
  // (the answer was lost, or another tab of this browser added it).
  const hasServer = (state?.servers.length ?? 0) > 0;
  // A server that is there settles it: if it is removed later, the old failure is not said again.
  if (hasServer && failure !== null) setFailure(null);
  const notice: Key | null = !enabled || hasServer ? null : registration === 'failed' ? 'deviceNotRegistered' : failure;
  const value = useMemo(() => (enabled ? { enabled, notice } : off), [enabled, notice]);
  return <WebDemoContext.Provider value={value}>{children}</WebDemoContext.Provider>;
}

export function useWebDemo(): WebDemo {
  return useContext(WebDemoContext);
}
