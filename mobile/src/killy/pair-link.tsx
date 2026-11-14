import { createContext, type ReactNode, useCallback, useContext, useMemo, useRef, useState } from 'react';

import type { PairCheck } from './pair-url';

/** A killy://pair link that passed or failed its checks, waiting for the Add server sheet. */
export type PairLink = { id: number } & PairCheck;

type PairLinks = {
  link: PairLink | null;
  /** Hands a checked link to Settings, replacing one not taken yet. */
  offer: (check: PairCheck) => void;
  /** Forgets the link, taken or not: closing the sheet drops it. */
  clear: () => void;
};

const PairLinkContext = createContext<PairLinks | null>(null);

/**
 * Carries a checked killy://pair link from its route to the Add server sheet. It is app state, not
 * a URL parameter of Settings, so only the pair route, after its checks, can put a code in the
 * field: a killy://settings?… link opens no sheet.
 */
export function PairLinkProvider({ children }: { children: ReactNode }) {
  const [link, setLink] = useState<PairLink | null>(null);
  const next = useRef(1);
  const offer = useCallback((check: PairCheck) => setLink({ id: next.current++, ...check }), []);
  const clear = useCallback(() => setLink(null), []);
  const value = useMemo(() => ({ link, offer, clear }), [link, offer, clear]);
  return <PairLinkContext.Provider value={value}>{children}</PairLinkContext.Provider>;
}

export function usePairLinks(): PairLinks {
  const ctx = useContext(PairLinkContext);
  if (!ctx) {
    throw new Error('usePairLinks outside PairLinkProvider');
  }
  return ctx;
}
