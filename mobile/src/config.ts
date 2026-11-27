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

/**
 * Whether this build offers the public demo server (Add server → Try the demo server). Only the
 * builds for the hackathon's judges do: they connect to the staging Killy server, where the demo
 * agent is. Set EXPO_PUBLIC_KILLY_DEMO=1 for them; any other value, or none, leaves the button out.
 */
export function demoEnabled(value: string | undefined): boolean {
  return value?.trim() === '1';
}

/** Read where Expo inlines EXPO_PUBLIC_ variables: the name must be written out in full. */
export function buildDemoEnabled(): boolean {
  return demoEnabled(process.env.EXPO_PUBLIC_KILLY_DEMO);
}

/**
 * Whether this build is the Web version in its demo-only form: it opens straight onto the public
 * demo server, with no pairing code, no QR code and no notifications. Only the build that is put on
 * the introduction site's /demo/ sets EXPO_PUBLIC_KILLY_WEB_DEMO=1; any other value, or none,
 * leaves the app as it is. It has an effect on the Web only (see WebDemoProvider).
 */
export function webDemoEnabled(value: string | undefined): boolean {
  return value?.trim() === '1';
}

/** Read where Expo inlines EXPO_PUBLIC_ variables: the name must be written out in full. */
export function buildWebDemoEnabled(): boolean {
  return webDemoEnabled(process.env.EXPO_PUBLIC_KILLY_WEB_DEMO);
}
