/** The build has no usable Killy server URL. */
export class ConfigError extends Error {}

/**
 * The Killy server this build talks to, from EXPO_PUBLIC_KILLY_SERVER_URL. The app connects to
 * this server only, so a QR code naming another one cannot make it send the device token there.
 */
export function serverURL(value: string | undefined): string {
  const url = value?.trim() ?? '';
  if (url === '') {
    throw new ConfigError('EXPO_PUBLIC_KILLY_SERVER_URL is not set for this build');
  }
  if (!/^https?:\/\/[^/\s]+/.test(url)) {
    throw new ConfigError(`EXPO_PUBLIC_KILLY_SERVER_URL is not an http(s) URL: ${url}`);
  }
  return url.replace(/\/+$/, '');
}

/** Read where Expo inlines EXPO_PUBLIC_ variables: the name must be written out in full. */
export function buildServerURL(): string {
  return serverURL(process.env.EXPO_PUBLIC_KILLY_SERVER_URL);
}
