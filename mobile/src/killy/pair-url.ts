// Checks a pairing link from a QR code or a killy://pair deep link before anything is sent. The
// link's server is never connected to: it only has to be the server this build talks to, so a
// QR code naming another server cannot make the app send its device token elsewhere.

export type PairCheck = { ok: true; code: string } | { ok: false; reason: 'invalid' | 'other_server' };

/** The form of a pairing code (ClaimRequest.code in docs/openapi.yaml). */
const codePattern = /^[A-Z0-9]{6}$/;

const prefix = 'killy://pair?';

/**
 * Takes the code out of the text of a QR code. Only `killy://pair?` with exactly one `code` of 6
 * upper-case letters or digits and one `server` is accepted; the URL is split by hand rather than
 * with `URL`, so jest and the phone read it the same way.
 */
export function parsePairURL(text: string, appServer: string): PairCheck {
  if (!text.startsWith(prefix) || text.includes('#')) {
    return invalid;
  }
  // A Map, not an object: a key such as constructor or __proto__ must be just another key.
  const params = new Map<string, string[]>();
  for (const pair of text.slice(prefix.length).split('&')) {
    const eq = pair.indexOf('=');
    const key = decode(eq < 0 ? pair : pair.slice(0, eq));
    const value = decode(eq < 0 ? '' : pair.slice(eq + 1));
    if (key === null || value === null) {
      return invalid;
    }
    params.set(key, [...(params.get(key) ?? []), value]);
  }
  return checkPairParams({ code: params.get('code'), server: params.get('server') }, appServer);
}

/** The same checks for the params expo-router hands to the pair route, already decoded. */
export function checkPairParams(params: Record<string, string | string[] | undefined>, appServer: string): PairCheck {
  const code = single(params.code);
  const server = single(params.server);
  if (code === null || server === null || !codePattern.test(code)) {
    return invalid;
  }
  if (!sameServer(server, appServer)) {
    return { ok: false, reason: 'other_server' };
  }
  return { ok: true, code };
}

/**
 * Compares two Killy server URLs as the operator may type them: the scheme and host in any case,
 * with or without the default port and a trailing `/`.
 */
export function sameServer(a: string, b: string): boolean {
  const x = canonical(a);
  return x !== null && x === canonical(b);
}

const invalid: PairCheck = { ok: false, reason: 'invalid' };

function single(v: string | string[] | undefined): string | null {
  if (typeof v === 'string') return v;
  if (Array.isArray(v) && v.length === 1) return v[0];
  return null;
}

// url.Values.Encode on the agent writes spaces as +.
function decode(s: string): string | null {
  try {
    return decodeURIComponent(s.replace(/\+/g, ' '));
  } catch {
    return null;
  }
}

const defaultPorts: Record<string, string> = { http: '80', https: '443' };

function canonical(url: string): string | null {
  const m = /^([A-Za-z][A-Za-z0-9+.-]*):\/\/([^/?#]+)([^?#]*)$/.exec(url.trim());
  if (!m) return null;
  const scheme = m[1].toLowerCase();
  if (!(scheme in defaultPorts)) return null;
  const hostPort = /^([^:]+)(?::(\d+))?$/.exec(m[2]);
  if (!hostPort) return null;
  const host = hostPort[1].toLowerCase();
  const port = hostPort[2] === undefined || hostPort[2] === defaultPorts[scheme] ? '' : `:${hostPort[2]}`;
  return `${scheme}://${host}${port}${m[3].replace(/\/+$/, '')}`;
}
