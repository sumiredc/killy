import { checkPairParams, parsePairURL, sameServer } from '../killy/pair-url';

const app = 'https://killy.sumire.pink';
const enc = encodeURIComponent;

function qr(code: string, server: string) {
  return `killy://pair?code=${code}&server=${enc(server)}`;
}

describe('parsePairURL', () => {
  test.each([
    ['the QR of killy agent pair', qr('K7Q4XP', app)],
    ['the server and code in the other order', `killy://pair?server=${enc(app)}&code=K7Q4XP`],
    ['another parameter next to them', `${qr('K7Q4XP', app)}&v=2`],
    ['a parameter named like a key every object has (constructor)', `${qr('K7Q4XP', app)}&constructor=x`],
    ['a parameter named __proto__', `${qr('K7Q4XP', app)}&__proto__=x`],
  ])('accepts %s', (_, text) => {
    expect(parsePairURL(text, app)).toEqual({ ok: true, code: 'K7Q4XP' });
  });

  test('a + in the query is a space, as url.Values.Encode writes it', () => {
    const spaced = 'http://localhost:18080/a b';
    expect(parsePairURL('killy://pair?code=K7Q4XP&server=http%3A%2F%2Flocalhost%3A18080%2Fa+b', spaced)).toEqual({ ok: true, code: 'K7Q4XP' });
  });

  test.each([
    ['an https URL', `https://killy.sumire.pink/pair?code=K7Q4XP&server=${enc(app)}`],
    ['another killy:// host', `killy://claim?code=K7Q4XP&server=${enc(app)}`],
    ['a path after pair', `killy://pair/x?code=K7Q4XP&server=${enc(app)}`],
    ['plain text', 'K7Q4XP'],
    ['no query', 'killy://pair'],
    ['no code', `killy://pair?server=${enc(app)}`],
    ['an empty code', qr('', app)],
    ['a code of 5 characters', qr('K7Q4X', app)],
    ['a code of 7 characters', qr('K7Q4XPZ', app)],
    ['a lower-case code', qr('k7q4xp', app)],
    ['a code with a symbol', qr('K7Q4X-', app)],
    ['two codes', `${qr('K7Q4XP', app)}&code=A1B2C3`],
    ['no server', 'killy://pair?code=K7Q4XP'],
    ['two servers', `${qr('K7Q4XP', app)}&server=${enc(app)}`],
    ['a broken percent escape', 'killy://pair?code=K7Q4XP&server=%E0%A4%A'],
    ['a broken percent escape in another parameter', `${qr('K7Q4XP', app)}&v=%E0%A4%A`],
    ['a fragment', `${qr('K7Q4XP', app)}#x`],
    ['only a key every object has', `killy://pair?toString=x`],
  ])('refuses %s as invalid', (_, text) => {
    expect(parsePairURL(text, app)).toEqual({ ok: false, reason: 'invalid' });
  });

  test.each([
    ['another host', 'https://evil.example'],
    ['http instead of https', 'http://killy.sumire.pink'],
    ['another port', 'https://killy.sumire.pink:8443'],
    ['a path', 'https://killy.sumire.pink/other'],
    ['a look-alike host', 'https://killy.sumire.pink.evil.example'],
  ])('refuses %s as another Killy server', (_, server) => {
    expect(parsePairURL(qr('K7Q4XP', server), app)).toEqual({ ok: false, reason: 'other_server' });
  });

  test('an invalid URL is invalid even when its server also differs', () => {
    expect(parsePairURL(qr('K7Q4X', 'https://evil.example'), app)).toEqual({ ok: false, reason: 'invalid' });
  });
});

describe('sameServer', () => {
  test.each([
    ['the same URL', app, app],
    ['a trailing /', `${app}/`, app],
    ['upper-case host', 'https://KILLY.Sumire.PINK', app],
    ['upper-case scheme', 'HTTPS://killy.sumire.pink', app],
    ['the default https port', 'https://killy.sumire.pink:443', app],
    ['the default http port', 'http://localhost:80', 'http://localhost'],
    ['a non-default port on both', 'http://localhost:18080/', 'http://localhost:18080'],
  ])('%s is the same server', (_, a, b) => {
    expect(sameServer(a, b)).toBe(true);
    expect(sameServer(b, a)).toBe(true);
  });

  test.each([
    ['another host', 'https://evil.example', app],
    ['another scheme', 'http://killy.sumire.pink', app],
    ['port 443 on http', 'http://killy.sumire.pink:443', 'http://killy.sumire.pink'],
    ['another port', 'http://localhost:18081', 'http://localhost:18080'],
    ['a path that differs in case', 'https://killy.example/A', 'https://killy.example/a'],
    ['user info', 'https://user@killy.sumire.pink', app],
    ['another host hidden behind user info', 'https://killy.sumire.pink@evil.example', app],
    ['not a URL', 'killy.sumire.pink', app],
    ['empty', '', app],
  ])('%s is another server', (_, a, b) => {
    expect(sameServer(a, b)).toBe(false);
  });
});

describe('checkPairParams', () => {
  test('accepts the params of killy://pair from the router', () => {
    expect(checkPairParams({ code: 'K7Q4XP', server: app }, app)).toEqual({ ok: true, code: 'K7Q4XP' });
  });

  test.each([
    ['no code', { server: app }],
    ['a 5-character code', { code: 'K7Q4X', server: app }],
    ['a lower-case code', { code: 'k7q4xp', server: app }],
    ['two codes', { code: ['K7Q4XP', 'A1B2C3'], server: app }],
    ['no server', { code: 'K7Q4XP' }],
    ['two servers', { code: 'K7Q4XP', server: [app, app] }],
  ])('refuses %s as invalid', (_, params) => {
    expect(checkPairParams(params, app)).toEqual({ ok: false, reason: 'invalid' });
  });

  test('refuses another Killy server', () => {
    expect(checkPairParams({ code: 'K7Q4XP', server: 'https://evil.example' }, app)).toEqual({ ok: false, reason: 'other_server' });
  });

  test('accepts the app server written differently', () => {
    expect(checkPairParams({ code: 'K7Q4XP', server: 'HTTPS://killy.sumire.pink:443/' }, app)).toEqual({ ok: true, code: 'K7Q4XP' });
  });
});
