import { SSEParser } from '../killy/sse';

describe('SSEParser', () => {
  test('reads an event with its name and data', () => {
    const p = new SSEParser();
    expect(p.push('event: state\ndata: {"servers":[]}\n\n')).toEqual([{ event: 'state', data: '{"servers":[]}' }]);
  });

  test('keeps an event that arrives in pieces until it is complete', () => {
    const p = new SSEParser();
    expect(p.push('event: sta')).toEqual([]);
    expect(p.push('te\ndata: {"a"')).toEqual([]);
    expect(p.push(':1}\n')).toEqual([]);
    expect(p.push('\n')).toEqual([{ event: 'state', data: '{"a":1}' }]);
  });

  test('ignores comment lines such as the heartbeat', () => {
    const p = new SSEParser();
    expect(p.push(': hb\n\nevent: state\ndata: 1\n\n: hb\n\n')).toEqual([{ event: 'state', data: '1' }]);
  });

  test('reads several events from one piece and accepts CRLF line ends', () => {
    const p = new SSEParser();
    expect(p.push('event: state\r\ndata: 1\r\n\r\nevent: state\ndata: 2\n\n')).toEqual([
      { event: 'state', data: '1' },
      { event: 'state', data: '2' },
    ]);
  });

  test('joins several data lines with a newline, and an event without a name is "message"', () => {
    const p = new SSEParser();
    expect(p.push('data: a\ndata:b\n\n')).toEqual([{ event: 'message', data: 'a\nb' }]);
  });
});
