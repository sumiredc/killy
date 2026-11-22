import { createContext, type ReactNode, useContext, useEffect, useState } from 'react';

import { useServerURL } from '@/config-context';

import { createKillyClient, type KillyClient, RegisterError } from './client';
import { makeDeps } from './deps';
import type { NotificationsDeps } from './notifications';
import type { State } from './types';

/**
 * Where device registration stands. `rate_limited` and `failed` last until the next start, when
 * registration is tried again.
 */
export type Registration = 'pending' | 'registered' | 'rate_limited' | 'failed';

type Killy = {
  client: KillyClient;
  registration: Registration;
  /** The latest state from the Killy server, or null before the first one arrives. */
  state: State | null;
  randomBytes: (n: number) => Uint8Array;
  notifications: NotificationsDeps;
};

const KillyContext = createContext<Killy | null>(null);

/** Registers the device, keeps the event stream open while it has a token, and holds the latest state. */
export function KillyProvider({ children }: { children: ReactNode }) {
  const baseURL = useServerURL();
  // Made once: the URL is fixed for the build, and the token is set once registration is done.
  const [{ client, deps, setToken }] = useState(() => {
    const deps = makeDeps(baseURL);
    let token: string | null = null;
    const client = createKillyClient({
      baseURL,
      fetch: deps.fetch,
      random: deps.random,
      deviceToken: () => token,
      // A token the server does not know (its device record is gone) is replaced by a new registration.
      renewToken: async (t) => {
        await deps.tokens.set(t);
        token = t;
      },
    });
    return { client, deps, setToken: (t: string) => (token = t) };
  });
  const [registration, setRegistration] = useState<Registration>('pending');
  const [state, setState] = useState<State | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      let saved: string | null;
      try {
        saved = await deps.tokens.get();
      } catch {
        // A token that exists but cannot be read now (iOS keeps it locked while the phone is) must
        // not be replaced: the servers are linked to it. Try again on the next start.
        if (!cancelled) setRegistration('failed');
        return;
      }
      if (saved !== null) {
        setToken(saved);
        if (!cancelled) setRegistration('registered');
        return;
      }
      try {
        const { deviceToken } = await client.registerDevice();
        await deps.tokens.set(deviceToken);
        setToken(deviceToken);
        if (!cancelled) setRegistration('registered');
      } catch (e) {
        if (!cancelled) setRegistration(e instanceof RegisterError && e.kind === 'rate_limited' ? 'rate_limited' : 'failed');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [client, deps, setToken]);

  useEffect(() => {
    if (registration !== 'registered') {
      return;
    }
    // Each state replaces the previous one whole, so nothing from before a reconnection stays.
    return client.events(setState);
  }, [client, registration]);

  return <KillyContext.Provider value={{ client, registration, state, randomBytes: deps.randomBytes, notifications: deps.notifications }}>{children}</KillyContext.Provider>;
}

export function useKilly(): Killy {
  const ctx = useContext(KillyContext);
  if (!ctx) {
    throw new Error('useKilly outside KillyProvider');
  }
  return ctx;
}
