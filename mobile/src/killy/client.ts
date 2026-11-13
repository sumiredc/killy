import { backoff } from './backoff';
import { SSEParser } from './sse';
import type { KillResult } from './battle';
import type { Server, State } from './types';

export type ReaderLike = {
  read(): Promise<{ done: boolean; value?: Uint8Array }>;
  cancel(): Promise<void>;
};

/** The part of a fetch Response the client uses. */
export type ResponseLike = {
  status: number;
  text(): Promise<string>;
  body: { getReader(): ReaderLike } | null;
};

export type FetchLike = (
  url: string,
  init: { method?: string; headers?: Record<string, string>; body?: string; signal?: AbortSignal },
) => Promise<ResponseLike>;

export type RegisterErrorKind = 'rate_limited' | 'unreachable' | 'other';
export type ClaimErrorKind =
  | 'code_unknown'
  | 'code_used'
  | 'code_expired'
  | 'rate_limited'
  | 'unreachable'
  | 'not_registered'
  | 'other';

export class RegisterError extends Error {
  constructor(readonly kind: RegisterErrorKind) {
    super(kind);
  }
}

export type KillErrorKind = 'rate_limited' | 'unreachable' | 'other';

export class KillError extends Error {
  constructor(readonly kind: KillErrorKind) {
    super(kind);
  }
}

export type KillRequest = { id: string; serverId: string; ppid: number; starttime: number };

export class ClaimError extends Error {
  constructor(readonly kind: ClaimErrorKind) {
    super(kind);
  }
}

/** Everything the app asks of the Killy server. Only this talks to it. */
export interface KillyClient {
  registerDevice(): Promise<{ deviceToken: string }>;
  claim(code: string, name?: string): Promise<Server>;
  /** Sends a kill and returns its outcome. Without an answer it sends the same kill once more. */
  kill(req: KillRequest): Promise<KillResult>;
  /** Streams the state to onState, reconnecting on its own, until the returned function is called. */
  events(onState: (s: State) => void): () => void;
}

export type ClientOptions = {
  baseURL: string;
  fetch: FetchLike;
  deviceToken: () => string | null;
  /** Math.random in the app; the reconnection jitter. */
  random: () => number;
};

/** A request that has had no answer by then is treated as unreachable, so the sheet never spins forever. */
export const requestTimeout = 15_000;

/** An event stream with nothing for this long, not even the 20 s heartbeat, is dropped and reopened. */
export const streamIdleTimeout = 60_000;

type Answer = { status: number; body: unknown };

export function createKillyClient({ baseURL, fetch, deviceToken, random }: ClientOptions): KillyClient {
  // Resolves with the status and the parsed JSON body (undefined when it is not JSON), or rejects
  // when no answer came.
  async function request(method: string, path: string, body: unknown, token: string | null): Promise<Answer> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), requestTimeout);
    try {
      const headers: Record<string, string> = { 'Content-Type': 'application/json' };
      if (token !== null) {
        headers.Authorization = `Bearer ${token}`;
      }
      const res = await fetch(baseURL + path, { method, headers, body: JSON.stringify(body), signal: controller.signal });
      const text = await res.text();
      return { status: res.status, body: parseJSON(text) };
    } finally {
      clearTimeout(timer);
    }
  }

  return {
    async registerDevice() {
      let answer: Answer;
      try {
        // No push token: notifications are registered later (#20), and a phone without them can still pair.
        answer = await request('POST', '/app/v1/devices', {}, null);
      } catch {
        throw new RegisterError('unreachable');
      }
      const token = field(answer.body, 'device_token');
      if (answer.status === 200 && token) {
        return { deviceToken: token };
      }
      throw new RegisterError(answer.status === 429 ? 'rate_limited' : 'other');
    },

    async claim(code, name) {
      const token = deviceToken();
      if (token === null) {
        throw new ClaimError('not_registered');
      }
      const trimmed = name?.trim();
      let answer: Answer;
      try {
        answer = await request('POST', '/app/v1/pairings/claim', trimmed ? { code, name: trimmed } : { code }, token);
      } catch {
        throw new ClaimError('unreachable');
      }
      if (answer.status === 200 && isServer(answer.body)) {
        return answer.body;
      }
      const error = field(answer.body, 'error');
      switch (error) {
        case 'code_unknown':
        case 'code_used':
        case 'code_expired':
        case 'rate_limited':
          throw new ClaimError(error);
        default:
          throw new ClaimError('other');
      }
    },

    async kill({ id, serverId, ppid, starttime }) {
      const token = deviceToken();
      if (token === null) {
        throw new KillError('other');
      }
      const body = { id, server_id: serverId, ppid, starttime };
      let answer: Answer;
      try {
        answer = await request('POST', '/app/v1/kills', body, token);
      } catch {
        // No answer at all: the same ID once more is safe, since the agent never signals twice for it.
        try {
          answer = await request('POST', '/app/v1/kills', body, token);
        } catch {
          throw new KillError('unreachable');
        }
      }
      const outcome = field(answer.body, 'outcome');
      if (answer.status === 200 && outcome) {
        const res = answer.body as KillResult;
        return { outcome, ...(res.reaped !== undefined && { reaped: res.reaped }), ...(res.reason !== undefined && { reason: res.reason }) };
      }
      // An answer came, so nothing is sent again.
      throw new KillError(answer.status === 429 ? 'rate_limited' : 'other');
    },

    events(onState) {
      let stopped = false;
      let reader: ReaderLike | null = null;
      let controller: AbortController | null = null;
      let wake: (() => void) | null = null;
      let timer: ReturnType<typeof setTimeout> | null = null;

      const sleep = (ms: number) =>
        new Promise<void>((resolve) => {
          wake = resolve;
          timer = setTimeout(resolve, ms);
        });

      const connect = async (onOpen: () => void): Promise<void> => {
        const token = deviceToken();
        if (token === null) {
          return;
        }
        controller = new AbortController();
        const res = await fetch(baseURL + '/app/v1/events', {
          method: 'GET',
          headers: { Authorization: `Bearer ${token}`, Accept: 'text/event-stream' },
          signal: controller.signal,
        });
        if (res.status !== 200 || res.body === null) {
          return;
        }
        // Reached the server: the next wait starts again from 1 second, however this stream ends.
        onOpen();
        const r = res.body.getReader();
        reader = r;
        const decoder = new TextDecoder();
        const parser = new SSEParser();
        // A connection that died without closing sends nothing; the server's heartbeat every 20 s
        // means 60 s of silence is a dead stream (the agent waits as long on its command stream).
        let idle: ReturnType<typeof setTimeout> | null = null;
        const watch = () => {
          if (idle !== null) clearTimeout(idle);
          idle = setTimeout(() => r.cancel().catch(() => {}), streamIdleTimeout);
        };
        try {
          watch();
          for (;;) {
            const { done, value } = await r.read();
            if (done || stopped) {
              return;
            }
            watch();
            for (const e of parser.push(decoder.decode(value, { stream: true }))) {
              const state = e.event === 'state' ? parseJSON(e.data) : undefined;
              if (isState(state) && !stopped) {
                onState(state);
              }
            }
          }
        } finally {
          if (idle !== null) clearTimeout(idle);
        }
      };

      (async () => {
        let attempt = 0;
        while (!stopped) {
          try {
            await connect(() => (attempt = 0));
          } catch {
            // Dropped or unreachable: wait and try again.
          }
          if (stopped) {
            return;
          }
          await sleep(backoff(attempt, random));
          attempt++;
        }
      })();

      return () => {
        stopped = true;
        if (timer !== null) {
          clearTimeout(timer);
        }
        (wake as (() => void) | null)?.();
        (reader as ReaderLike | null)?.cancel().catch(() => {});
        (controller as AbortController | null)?.abort();
      };
    },
  };
}

function parseJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

function field(body: unknown, name: string): string | undefined {
  if (typeof body === 'object' && body !== null && typeof (body as Record<string, unknown>)[name] === 'string') {
    return (body as Record<string, string>)[name];
  }
  return undefined;
}

function isServer(v: unknown): v is Server {
  return typeof v === 'object' && v !== null && typeof (v as Server).id === 'string' && typeof (v as Server).name === 'string';
}

function isState(v: unknown): v is State {
  return typeof v === 'object' && v !== null && Array.isArray((v as State).servers);
}
