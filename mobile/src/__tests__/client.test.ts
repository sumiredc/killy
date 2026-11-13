import { ClaimError, createKillyClient, RegisterError } from '../killy/client';
import { FakeServer } from '../testing/fake-server';

const url = 'https://killy.example';

function client(server: FakeServer, token: string | null = 'dev-token') {
  return createKillyClient({ baseURL: url, fetch: server.fetch, deviceToken: () => token, random: () => 0 });
}

describe('registerDevice', () => {
  test('registers without a push token and returns the device token', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/devices', 200, { device_token: 'tok-1' });
    await expect(client(server, null).registerDevice()).resolves.toEqual({ deviceToken: 'tok-1' });
    expect(server.requests).toEqual([
      expect.objectContaining({ method: 'POST', url: `${url}/app/v1/devices`, body: {}, authorization: undefined }),
    ]);
  });

  test('a 429 is RateLimited', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/devices', 429, { error: 'rate_limited' });
    await expect(client(server, null).registerDevice()).rejects.toEqual(new RegisterError('rate_limited'));
  });

  test('no answer is unreachable, and any other answer is other', async () => {
    const down = new FakeServer();
    down.unreachable('POST /app/v1/devices');
    await expect(client(down, null).registerDevice()).rejects.toEqual(new RegisterError('unreachable'));
    const broken = new FakeServer();
    broken.respondText('POST /app/v1/devices', 502, '<html>Bad gateway</html>');
    await expect(client(broken, null).registerDevice()).rejects.toEqual(new RegisterError('other'));
  });
});

describe('claim', () => {
  const added = { id: 'a1', name: 'web-01', hostname: 'web-01', notify: true };

  test('sends the code and the device token, and returns the added server', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/pairings/claim', 200, added);
    await expect(client(server).claim('K7Q4XP', 'my box')).resolves.toEqual(added);
    expect(server.requests[0]).toEqual(
      expect.objectContaining({ body: { code: 'K7Q4XP', name: 'my box' }, authorization: 'Bearer dev-token' }),
    );
  });

  test('without a display name it sends no name, so the server uses the hostname', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/pairings/claim', 200, added);
    await client(server).claim('K7Q4XP', '   ');
    expect(server.requests[0].body).toEqual({ code: 'K7Q4XP' });
  });

  test.each([
    [404, { error: 'code_unknown' }, 'code_unknown'],
    [409, { error: 'code_used' }, 'code_used'],
    [410, { error: 'code_expired' }, 'code_expired'],
    [429, { error: 'rate_limited' }, 'rate_limited'],
    [400, { error: 'invalid_request' }, 'other'],
    [401, { error: 'unauthorized' }, 'other'],
    [503, { error: 'unavailable' }, 'other'],
  ] as const)('a %i %j is %s', async (status, body, kind) => {
    const server = new FakeServer();
    server.respond('POST /app/v1/pairings/claim', status, body);
    await expect(client(server).claim('K7Q4XP')).rejects.toEqual(new ClaimError(kind));
  });

  test('an answer that is not JSON is other', async () => {
    const server = new FakeServer();
    server.respondText('POST /app/v1/pairings/claim', 502, '<html>Bad gateway</html>');
    await expect(client(server).claim('K7Q4XP')).rejects.toEqual(new ClaimError('other'));
  });

  test('no answer is unreachable', async () => {
    const server = new FakeServer();
    server.unreachable('POST /app/v1/pairings/claim');
    await expect(client(server).claim('K7Q4XP')).rejects.toEqual(new ClaimError('unreachable'));
  });

  test('an answer that has not come after 15 seconds is unreachable', async () => {
    jest.useFakeTimers();
    try {
      const server = new FakeServer();
      server.hold('POST /app/v1/pairings/claim');
      const result = client(server).claim('K7Q4XP').catch((e) => e);
      await jest.advanceTimersByTimeAsync(14_999);
      expect(server.lastSignal()?.aborted).toBe(false);
      await jest.advanceTimersByTimeAsync(1);
      await expect(result).resolves.toEqual(new ClaimError('unreachable'));
    } finally {
      jest.useRealTimers();
    }
  });

  test('without a device token nothing is sent', async () => {
    const server = new FakeServer();
    await expect(client(server, null).claim('K7Q4XP')).rejects.toEqual(new ClaimError('not_registered'));
    expect(server.requests).toEqual([]);
  });
});

describe('events', () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  const one = { servers: [{ id: 'a1', name: 'web-01', hostname: 'web-01', notify: true, disconnected: false, parents: [] }] };

  test('opens the stream with the device token and passes on every state', async () => {
    const server = new FakeServer();
    const stream = server.stream();
    const states: unknown[] = [];
    const stop = client(server).events((s) => states.push(s));
    await jest.advanceTimersByTimeAsync(0);
    expect(server.requests[0]).toEqual(
      expect.objectContaining({ method: 'GET', url: `${url}/app/v1/events`, authorization: 'Bearer dev-token' }),
    );
    await stream.send('event: state\ndata: ' + JSON.stringify(one) + '\n\n: hb\n\n');
    await stream.send('event: state\ndata: {"servers":[]}\n\n');
    expect(states).toEqual([one, { servers: [] }]);
    stop();
  });

  test('reconnects after 1, 2 and 4 seconds while the stream keeps failing, then from 1 second after a good connection', async () => {
    const server = new FakeServer();
    server.unreachable('GET /app/v1/events');
    const stop = client(server).events(() => {});
    await jest.advanceTimersByTimeAsync(0);
    expect(server.count('GET /app/v1/events')).toBe(1);
    await jest.advanceTimersByTimeAsync(999);
    expect(server.count('GET /app/v1/events')).toBe(1);
    await jest.advanceTimersByTimeAsync(1);
    expect(server.count('GET /app/v1/events')).toBe(2);
    await jest.advanceTimersByTimeAsync(2000);
    expect(server.count('GET /app/v1/events')).toBe(3);
    await jest.advanceTimersByTimeAsync(3999);
    expect(server.count('GET /app/v1/events')).toBe(3);
    const stream = server.stream();
    await jest.advanceTimersByTimeAsync(1);
    expect(server.count('GET /app/v1/events')).toBe(4);
    await stream.end();
    server.unreachable('GET /app/v1/events');
    await jest.advanceTimersByTimeAsync(999);
    expect(server.count('GET /app/v1/events')).toBe(4);
    await jest.advanceTimersByTimeAsync(1);
    expect(server.count('GET /app/v1/events')).toBe(5);
    stop();
  });

  test('a stream answered with an error is retried like a dropped one', async () => {
    const server = new FakeServer();
    server.respond('GET /app/v1/events', 503, { error: 'unavailable' });
    const stop = client(server).events(() => {});
    await jest.advanceTimersByTimeAsync(1000);
    expect(server.count('GET /app/v1/events')).toBe(2);
    stop();
  });

  test('after stop it does not reconnect and passes on nothing more', async () => {
    const server = new FakeServer();
    const stream = server.stream();
    const states: unknown[] = [];
    const stop = client(server).events((s) => states.push(s));
    await jest.advanceTimersByTimeAsync(0);
    stop();
    await stream.send('event: state\ndata: {"servers":[]}\n\n');
    await jest.advanceTimersByTimeAsync(60_000);
    expect(states).toEqual([]);
    expect(server.count('GET /app/v1/events')).toBe(1);
    expect(stream.cancelled).toBe(true);
  });

  test('a good stream that fails with an error is retried from 1 second', async () => {
    const server = new FakeServer();
    server.unreachable('GET /app/v1/events');
    const stop = client(server).events(() => {});
    await jest.advanceTimersByTimeAsync(0);
    await jest.advanceTimersByTimeAsync(1000);
    await jest.advanceTimersByTimeAsync(2000);
    expect(server.count('GET /app/v1/events')).toBe(3);
    const stream = server.stream();
    await jest.advanceTimersByTimeAsync(4000);
    expect(server.count('GET /app/v1/events')).toBe(4);
    server.unreachable('GET /app/v1/events');
    await stream.fail(new TypeError('Network connection was lost'));
    await jest.advanceTimersByTimeAsync(999);
    expect(server.count('GET /app/v1/events')).toBe(4);
    await jest.advanceTimersByTimeAsync(1);
    expect(server.count('GET /app/v1/events')).toBe(5);
    stop();
  });

  test('a stream that sends nothing for 60 seconds, not even a heartbeat, is dropped and reopened', async () => {
    const server = new FakeServer();
    const stream = server.stream();
    const stop = client(server).events(() => {});
    await jest.advanceTimersByTimeAsync(0);
    await jest.advanceTimersByTimeAsync(59_000);
    await stream.send(': hb\n\n');
    await jest.advanceTimersByTimeAsync(59_000);
    expect(stream.cancelled).toBe(false);
    expect(server.count('GET /app/v1/events')).toBe(1);
    await jest.advanceTimersByTimeAsync(1000);
    expect(stream.cancelled).toBe(true);
    await jest.advanceTimersByTimeAsync(1000);
    expect(server.count('GET /app/v1/events')).toBe(2);
    stop();
  });

  test('a state that is not valid JSON is skipped', async () => {
    const server = new FakeServer();
    const stream = server.stream();
    const states: unknown[] = [];
    const stop = client(server).events((s) => states.push(s));
    await jest.advanceTimersByTimeAsync(0);
    await stream.send('event: state\ndata: {oops\n\nevent: state\ndata: {"servers":[]}\n\n');
    expect(states).toEqual([{ servers: [] }]);
    stop();
  });
});
