export type SSEEvent = { event: string; data: string };

/**
 * Splits a server-sent event stream into events as its text arrives. It covers what the Killy
 * server sends (named events, one or more data lines, comment lines); id and retry fields are
 * ignored because the server sends neither.
 */
export class SSEParser {
  private buffer = '';
  private event = '';
  private data: string[] = [];

  push(text: string): SSEEvent[] {
    this.buffer += text;
    const out: SSEEvent[] = [];
    for (;;) {
      const end = this.buffer.search(/\r\n|\r|\n/);
      // A CR at the very end may be the first half of a CRLF still on its way.
      if (end < 0 || (this.buffer[end] === '\r' && end === this.buffer.length - 1)) {
        break;
      }
      const line = this.buffer.slice(0, end);
      this.buffer = this.buffer.slice(end + (this.buffer.startsWith('\r\n', end) ? 2 : 1));
      this.line(line, out);
    }
    return out;
  }

  private line(line: string, out: SSEEvent[]) {
    if (line === '') {
      if (this.data.length > 0) {
        out.push({ event: this.event || 'message', data: this.data.join('\n') });
      }
      this.event = '';
      this.data = [];
      return;
    }
    if (line.startsWith(':')) {
      return;
    }
    const colon = line.indexOf(':');
    const field = colon < 0 ? line : line.slice(0, colon);
    let value = colon < 0 ? '' : line.slice(colon + 1);
    if (value.startsWith(' ')) {
      value = value.slice(1);
    }
    if (field === 'event') {
      this.event = value;
    } else if (field === 'data') {
      this.data.push(value);
    }
  }
}
