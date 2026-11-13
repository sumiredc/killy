import { createContext, type ReactNode, useContext } from 'react';

const ServerURLContext = createContext<string | null>(null);

/** Gives the screens the Killy server URL. Only rendered when the build has one. */
export function ServerURLProvider({ url, children }: { url: string; children: ReactNode }) {
  return <ServerURLContext.Provider value={url}>{children}</ServerURLContext.Provider>;
}

export function useServerURL(): string {
  const url = useContext(ServerURLContext);
  if (url === null) {
    throw new Error('useServerURL outside ServerURLProvider');
  }
  return url;
}
