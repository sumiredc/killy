import { ClaimError, createKillyClient, KillError, RegisterError, ServerError, UpdateError } from '../killy/client';
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

describe('kill', () => {
  const req = { id: '16fd2706-8baf-433b-82eb-8c7fada847da', serverId: 'a1', ppid: 3801, starttime: 123456 };

  test('sends the kill with the device token and returns the outcome', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/kills', 200, { outcome: 'killed', reaped: 3 });
    await expect(client(server).kill(req)).resolves.toEqual({ outcome: 'killed', reaped: 3 });
    expect(server.requests).toEqual([
      expect.objectContaining({
        url: `${url}/app/v1/kills`,
        body: { id: req.id, server_id: 'a1', ppid: 3801, starttime: 123456 },
        authorization: 'Bearer dev-token',
      }),
    ]);
  });

  test('without an answer it sends the same kill once more, and not a third time', async () => {
    const server = new FakeServer();
    server.unreachable('POST /app/v1/kills');
    await expect(client(server).kill(req)).rejects.toEqual(new KillError('unreachable'));
    expect(server.requests.map((r) => r.body)).toEqual([
      { id: req.id, server_id: 'a1', ppid: 3801, starttime: 123456 },
      { id: req.id, server_id: 'a1', ppid: 3801, starttime: 123456 },
    ]);
  });

  test('an answer to the second try is used', async () => {
    const server = new FakeServer();
    let calls = 0;
    const fetch: typeof server.fetch = async (u, init) => {
      calls++;
      if (calls === 1) {
        throw new TypeError('Network request failed');
      }
      return server.fetch(u, init);
    };
    server.respond('POST /app/v1/kills', 200, { outcome: 'already_gone' });
    const c = createKillyClient({ baseURL: url, fetch, deviceToken: () => 'dev-token', random: () => 0 });
    await expect(c.kill(req)).resolves.toEqual({ outcome: 'already_gone' });
    expect(calls).toBe(2);
  });

  test('a 429 to the second try is rate_limited, with no third try', async () => {
    const server = new FakeServer();
    let calls = 0;
    const fetch: typeof server.fetch = async (u, init) => {
      calls++;
      if (calls === 1) {
        throw new TypeError('Network request failed');
      }
      return server.fetch(u, init);
    };
    server.respond('POST /app/v1/kills', 429, { error: 'rate_limited' });
    const c = createKillyClient({ baseURL: url, fetch, deviceToken: () => 'dev-token', random: () => 0 });
    await expect(c.kill(req)).rejects.toEqual(new KillError('rate_limited'));
    expect(calls).toBe(2);
  });

  test('a 429 is rate_limited and is not sent again', async () => {
    const server = new FakeServer();
    server.respond('POST /app/v1/kills', 429, { error: 'rate_limited' });
    await expect(client(server).kill(req)).rejects.toEqual(new KillError('rate_limited'));
    expect(server.count('POST /app/v1/kills')).toBe(1);
  });

  test.each([
    [400, { error: 'invalid_id' }],
    [401, { error: 'unauthorized' }],
    [404, { error: 'server_not_found' }],
    [503, { error: 'unavailable' }],
    [200, { nothing: true }],
  ])('a %i %j is other and is not sent again', async (status, body) => {
    const server = new FakeServer();
    server.respond('POST /app/v1/kills', status, body);
    await expect(client(server).kill(req)).rejects.toEqual(new KillError('other'));
    expect(server.count('POST /app/v1/kills')).toBe(1);
  });

  test('an answer that is not JSON is other and is not sent again', async () => {
    const server = new FakeServer();
    server.respondText('POST /app/v1/kills', 502, '<html>Bad gateway</html>');
    await expect(client(server).kill(req)).rejects.toEqual(new KillError('other'));
    expect(server.count('POST /app/v1/kills')).toBe(1);
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

describe('updateServer and removeServer', () => {
  test('a rename or mute is a PATCH of that server with only the fields given and the device token', async () => {
    const server = new FakeServer();
    server.respond('PATCH /app/v1/servers/a1', 204, null);
    await expect(client(server).updateServer('a1', { name: 'Renamed' })).resolves.toBeUndefined();
    await client(server).updateServer('a1', { notify: false });
    expect(server.requests).toEqual([
      expect.objectContaining({ method: 'PATCH', url: `${url}/app/v1/servers/a1`, body: { name: 'Renamed' }, authorization: 'Bearer dev-token' }),
      expect.objectContaining({ method: 'PATCH', url: `${url}/app/v1/servers/a1`, body: { notify: false }, authorization: 'Bearer dev-token' }),
    ]);
  });

  test('a removal is a DELETE of that server with the device token', async () => {
    const server = new FakeServer();
    server.respond('DELETE /app/v1/servers/a1', 204, null);
    await expect(client(server).removeServer('a1')).resolves.toBeUndefined();
    expect(server.requests).toEqual([
      expect.objectContaining({ method: 'DELETE', url: `${url}/app/v1/servers/a1`, authorization: 'Bearer dev-token' }),
    ]);
  });

  test('the server id is escaped in the path', async () => {
    const server = new FakeServer();
    await client(server).updateServer('a/../b?c', { notify: true }).catch(() => {});
    await client(server).removeServer('a/../b?c').catch(() => {});
    expect(server.requests.map((r) => r.url)).toEqual([`${url}/app/v1/servers/a%2F..%2Fb%3Fc`, `${url}/app/v1/servers/a%2F..%2Fb%3Fc`]);
  });

  test.each([
    [400, { error: 'invalid_request' }],
    [401, { error: 'unauthorized' }],
    [404, { error: 'server_not_found' }],
    [503, { error: 'unavailable' }],
    [200, {}],
  ] as const)('a %i answer is other and is not sent again', async (status, body) => {
    const server = new FakeServer();
    server.respond('PATCH /app/v1/servers/a1', status, body);
    server.respond('DELETE /app/v1/servers/a1', status, body);
    await expect(client(server).updateServer('a1', { name: 'x' })).rejects.toEqual(new ServerError('other'));
    await expect(client(server).removeServer('a1')).rejects.toEqual(new ServerError('other'));
    expect(server.requests.length).toBe(2);
  });

  test('an answer that is not JSON is other', async () => {
    const server = new FakeServer();
    server.respondText('PATCH /app/v1/servers/a1', 502, '<html>Bad gateway</html>');
    server.respondText('DELETE /app/v1/servers/a1', 502, '<html>Bad gateway</html>');
    await expect(client(server).updateServer('a1', { name: 'x' })).rejects.toEqual(new ServerError('other'));
    await expect(client(server).removeServer('a1')).rejects.toEqual(new ServerError('other'));
  });

  test('no answer is unreachable and is not sent again', async () => {
    const server = new FakeServer();
    server.unreachable('PATCH /app/v1/servers/a1');
    server.unreachable('DELETE /app/v1/servers/a1');
    await expect(client(server).updateServer('a1', { notify: false })).rejects.toEqual(new ServerError('unreachable'));
    await expect(client(server).removeServer('a1')).rejects.toEqual(new ServerError('unreachable'));
    expect(server.requests.length).toBe(2);
  });

  test('an answer that has not come after 15 seconds is unreachable', async () => {
    jest.useFakeTimers();
    try {
      const server = new FakeServer();
      server.hold('PATCH /app/v1/servers/a1');
      const result = client(server).updateServer('a1', { notify: false }).catch((e) => e);
      await jest.advanceTimersByTimeAsync(14_999);
      expect(server.lastSignal()?.aborted).toBe(false);
      await jest.advanceTimersByTimeAsync(1);
      await expect(result).resolves.toEqual(new ServerError('unreachable'));
    } finally {
      jest.useRealTimers();
    }
  });

  test('without a device token nothing is sent', async () => {
    const server = new FakeServer();
    await expect(client(server, null).updateServer('a1', { name: 'x' })).rejects.toEqual(new ServerError('other'));
    await expect(client(server, null).removeServer('a1')).rejects.toEqual(new ServerError('other'));
    expect(server.requests).toEqual([]);
  });
});

describe('check', () => {
  const route = 'POST /app/v1/servers/a1/check';

  test('sends a check of that server with the device token', async () => {
    const server = new FakeServer();
    server.respond(route, 200, { result: 'ok', version: '1.2.3' });
    await client(server).check('a1');
    expect(server.requests).toEqual([
      expect.objectContaining({ method: 'POST', url: `${url}/app/v1/servers/a1/check`, authorization: 'Bearer dev-token' }),
    ]);
  });

  test.each([
    [{ result: 'ok', version: '1.2.3' }, { kind: 'ok', version: '1.2.3' }],
    [{ result: 'agent_disconnected' }, { kind: 'agent_disconnected' }],
    [{ result: 'no_response' }, { kind: 'no_response' }],
    [{ result: 'ok' }, { kind: 'failed' }],
    [{ result: 'rebooting' }, { kind: 'failed' }],
    [{}, { kind: 'failed' }],
  ] as const)('a 200 %j is %j', async (body, want) => {
    const server = new FakeServer();
    server.respond(route, 200, body);
    await expect(client(server).check('a1')).resolves.toEqual(want);
  });

  test.each([
    [401, { error: 'unauthorized' }],
    [404, { error: 'server_not_found' }],
    [503, { error: 'unavailable' }],
  ] as const)('a %i is failed', async (status, body) => {
    const server = new FakeServer();
    server.respond(route, status, body);
    await expect(client(server).check('a1')).resolves.toEqual({ kind: 'failed' });
  });

  test('an answer that is not JSON is failed', async () => {
    const server = new FakeServer();
    server.respondText(route, 502, '<html>Bad gateway</html>');
    await expect(client(server).check('a1')).resolves.toEqual({ kind: 'failed' });
  });

  test('an answer other than 200 is failed even if its body looks like a result', async () => {
    const server = new FakeServer();
    server.respond(route, 502, { result: 'ok', version: '1.2.3' });
    await expect(client(server).check('a1')).resolves.toEqual({ kind: 'failed' });
  });

  test('no answer is unreachable and is not sent again', async () => {
    const server = new FakeServer();
    server.unreachable(route);
    await expect(client(server).check('a1')).resolves.toEqual({ kind: 'unreachable' });
    expect(server.requests.length).toBe(1);
  });

  test('an answer that has not come after 15 seconds is unreachable', async () => {
    jest.useFakeTimers();
    try {
      const server = new FakeServer();
      server.hold(route);
      const result = client(server).check('a1');
      await jest.advanceTimersByTimeAsync(14_999);
      expect(server.lastSignal()?.aborted).toBe(false);
      await jest.advanceTimersByTimeAsync(1);
      await expect(result).resolves.toEqual({ kind: 'unreachable' });
    } finally {
      jest.useRealTimers();
    }
  });

  test('the server id is escaped in the path', async () => {
    const server = new FakeServer();
    await client(server).check('a/../b?c');
    expect(server.requests[0].url).toBe(`${url}/app/v1/servers/a%2F..%2Fb%3Fc/check`);
  });

  test('without a device token nothing is sent and the check failed', async () => {
    const server = new FakeServer();
    await expect(client(server, null).check('a1')).resolves.toEqual({ kind: 'failed' });
    expect(server.requests).toEqual([]);
  });
});

describe('updateDevice', () => {
  test('a PATCH of this device with only the fields given, in the API names, and the device token', async () => {
    const server = new FakeServer();
    server.respond('PATCH /app/v1/devices/me', 204, null);
    await expect(client(server).updateDevice({ pushToken: 'ExponentPushToken[x]' })).resolves.toBeUndefined();
    await client(server).updateDevice({ notify: false });
    expect(server.requests).toEqual([
      expect.objectContaining({ method: 'PATCH', url: `${url}/app/v1/devices/me`, body: { push_token: 'ExponentPushToken[x]' }, authorization: 'Bearer dev-token' }),
      expect.objectContaining({ method: 'PATCH', url: `${url}/app/v1/devices/me`, body: { notify: false }, authorization: 'Bearer dev-token' }),
    ]);
  });

  test.each([
    [400, { error: 'invalid_request' }],
    [401, { error: 'unauthorized' }],
    [503, { error: 'unavailable' }],
    [200, {}],
  ])('a %i answer is an error of kind other', async (status, body) => {
    const server = new FakeServer();
    server.respond('PATCH /app/v1/devices/me', status, body);
    await expect(client(server).updateDevice({ notify: true })).rejects.toEqual(new UpdateError('other'));
  });

  test('no answer is unreachable, and it is not sent again', async () => {
    const server = new FakeServer();
    server.unreachable('PATCH /app/v1/devices/me');
    await expect(client(server).updateDevice({ notify: true })).rejects.toEqual(new UpdateError('unreachable'));
    expect(server.count('PATCH /app/v1/devices/me')).toBe(1);
  });

  test('without a device token nothing is sent', async () => {
    const server = new FakeServer();
    await expect(client(server, null).updateDevice({ notify: true })).rejects.toEqual(new UpdateError('not_registered'));
    expect(server.requests).toEqual([]);
  });
});
