/**
 * The key of the device token for one Killy server. A server registers its own devices, so a token
 * is good only there; one key per server keeps a staging token from being sent to production when
 * both builds are on one phone (iOS keeps Keychain items when an app is replaced). SecureStore keys
 * allow only letters, digits, `.`, `-` and `_`; anything else in the URL becomes `_`. The Web
 * version keeps its token in localStorage under the same key.
 *
 * Apart from deps.ts so that deps.web.ts, which Metro picks on the Web instead of deps.ts, can use it
 * without importing deps.ts itself.
 */
export function tokenKey(baseURL: string): string {
  return 'device_token.' + baseURL.trim().replace(/\/+$/, '').toLowerCase().replace(/[^a-z0-9._-]/g, '_');
}
