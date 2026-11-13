import type { FetchLike, ResponseLike } from '../killy/client';

/** One request the app sent, as the Killy server would see it. */
export type SeenRequest = {
  method: string;
  url: string;
  route: string;
  body: unknown;
  authorization: string | undefined;
};

type Responder = () => Promise<ResponseLike>;

/**
 * A stand-in for the Killy server behind fetch. Each route answers with whatever was last set for
 * it; a route with nothing set answers 404 as http.ServeMux does. Every request is recorded, so a
 * test can count what reached the server.
 */
export class FakeServer {
  readonly requests: SeenRequest[] = [];
  private routes = new Map<string, Responder>();

  readonly fetch: FetchLike = async (url, init) => {
    const method = init.method ?? 'GET';
    const route = `${method} ${new URL(url).pathname}`;
    this.signals.push(init.signal);
    this.requests.push({
      method,
      url,
      route,
      body: init.body === undefined ? undefined : JSON.parse(init.body),
      authorization: init.headers?.Authorization,
    });
    const responder = this.routes.get(route);
    if (!responder) {
      return textResponse(404, '404 page not found\n');
    }
    // Like fetch, an aborted request rejects even while the server has not answered.
    return new Promise<ResponseLike>((resolve, reject) => {
      init.signal?.addEventListener('abort', () => reject(new Error('Aborted')));
      responder().then(resolve, reject);
    });
  };

  private signals: (AbortSignal | undefined)[] = [];

  lastSignal(): AbortSignal | undefined {
    return this.signals.at(-1);
  }

  respond(route: string, status: number, body: unknown) {
    this.routes.set(route, async () => textResponse(status, JSON.stringify(body)));
  }

  respondText(route: string, status: number, text: string) {
    this.routes.set(route, async () => textResponse(status, text));
  }

  /** The request fails as fetch does when the network or the server cannot be reached. */
  unreachable(route: string) {
    this.routes.set(route, async () => {
      throw new TypeError('Network request failed');
    });
  }

  /** Requests answer forever until resolve is called with the response. */
  hold(route: string): { resolve: (status: number, body: unknown) => Promise<void> } {
    let release: (r: ResponseLike) => void = () => {};
    const answer = new Promise<ResponseLike>((resolve) => (release = resolve));
    this.routes.set(route, () => answer);
    return {
      resolve: async (status, body) => {
        release(textResponse(status, JSON.stringify(body)));
        await settle();
      },
    };
  }

  /** GET /app/v1/events answers 200 with an event stream the test writes to. */
  stream(): FakeStream {
    const s = new FakeStream();
    this.routes.set('GET /app/v1/events', async () => ({ status: 200, text: async () => '', body: { getReader: () => s.reader } }));
    return s;
  }

  count(route: string): number {
    return this.requests.filter((r) => r.route === route).length;
  }
}

export class FakeStream {
  cancelled = false;
  private chunks: (Uint8Array | null | Error)[] = [];
  private waiting: { resolve: (r: { done: boolean; value?: Uint8Array }) => void; reject: (e: Error) => void } | null = null;

  readonly reader = {
    read: () =>
      new Promise<{ done: boolean; value?: Uint8Array }>((resolve, reject) => {
        const next = this.chunks.shift();
        if (this.cancelled || next === null) {
          resolve({ done: true });
        } else if (next instanceof Error) {
          reject(next);
        } else if (next !== undefined) {
          resolve({ done: false, value: next });
        } else {
          this.waiting = { resolve, reject };
        }
      }),
    cancel: async () => {
      this.cancelled = true;
      this.deliver(null);
    },
  };

  /** Writes text to the stream and lets the app read it. */
  async send(text: string) {
    this.deliver(new TextEncoder().encode(text));
    await settle();
  }

  /** Closes the stream from the server's side. */
  async end() {
    this.deliver(null);
    await settle();
  }

  /** Breaks the stream as a lost network does: the pending read rejects. */
  async fail(error: Error) {
    this.deliver(error);
    await settle();
  }

  private deliver(chunk: Uint8Array | null | Error) {
    const waiting = this.waiting;
    this.waiting = null;
    if (!waiting) {
      this.chunks.push(chunk);
    } else if (chunk instanceof Error) {
      waiting.reject(chunk);
    } else {
      waiting.resolve(chunk === null ? { done: true } : { done: false, value: chunk });
    }
  }
}

function textResponse(status: number, text: string): ResponseLike {
  return { status, text: async () => text, body: null };
}

/** Lets pending promise callbacks run. */
export async function settle() {
  for (let i = 0; i < 20; i++) {
    await Promise.resolve();
  }
}
