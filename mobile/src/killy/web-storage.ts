/**
 * localStorage for the Web version, which keeps the device token and the settings there (no
 * expo-secure-store on the Web). A browser may have none, or refuse every call (private browsing,
 * storage turned off): then nothing is read and nothing is saved, and the app runs on what it
 * holds in memory until the page is closed. A missing localStorage fails like a refusing one.
 */
export function readStorage(key: string): string | null {
  try {
    return globalThis.localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function writeStorage(key: string, value: string): void {
  try {
    globalThis.localStorage.setItem(key, value);
  } catch {
    // Not saved; the next start starts as a new one.
  }
}
