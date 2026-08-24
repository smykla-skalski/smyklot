import { describe, expect, it } from 'vitest';

import type {
  PanelStreamClock,
  PanelChangeEvent,
  PanelStreamHandlers,
  PanelWebSocket,
} from '../src/lib/events';
import { openPanelStream, panelStreamUrl, readEvent } from '../src/lib/events';

type Listener = (event: never) => void;

class FakeWebSocket implements PanelWebSocket {
  readonly listeners = new Map<string, Listener[]>();
  readonly closes: Array<{ code?: number; reason?: string }> = [];
  readonly sent: string[] = [];

  addEventListener(type: string, listener: Listener): void {
    const listeners = this.listeners.get(type) ?? [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
  }

  close(code?: number, reason?: string): void {
    this.closes.push({ code, reason });
  }

  send(data: string): void {
    this.sent.push(data);
  }

  emit(type: string, event?: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) {
      (listener as (value: unknown) => void)(event);
    }
  }

  deliver(payload: unknown): void {
    this.emit('message', { data: JSON.stringify(payload) });
  }
}

interface ScheduledCall {
  handler: () => void;
  delay: number;
}

interface StreamFixture {
  sockets: FakeWebSocket[];
  urls: string[];
  scheduled: ScheduledCall[];
  cancelled: unknown[];
  resyncs: number;
  revoked: Array<{ code: string; reason: string }>;
  changes: PanelChangeEvent[];
  prefsReady: Array<{ rev: number; sum: string; values?: Record<string, unknown> }>;
  prefsChanged: Array<{ rev: number; changes: Record<string, unknown> }>;
  prefsRejected: string[][];
  handlers: PanelStreamHandlers;
  clock: PanelStreamClock;
  createSocket: (url: string) => PanelWebSocket;
}

function streamFixture(): StreamFixture {
  const state: StreamFixture = {
    sockets: [],
    urls: [],
    scheduled: [],
    cancelled: [],
    resyncs: 0,
    revoked: [],
    changes: [],
    prefsReady: [],
    prefsChanged: [],
    prefsRejected: [],
    handlers: {} as PanelStreamHandlers,
    clock: {} as PanelStreamClock,
    createSocket: (url) => {
      const socket = new FakeWebSocket();
      state.sockets.push(socket);
      state.urls.push(url);
      return socket;
    },
  };
  state.handlers = {
    onResync: () => {
      state.resyncs += 1;
    },
    onChange: (event) => state.changes.push(event),
    onRevoked: (event) => state.revoked.push(event),
    onPrefsReady: (prefs) => state.prefsReady.push(prefs),
    onPrefsChanged: (event) => state.prefsChanged.push(event),
    onPrefsRejected: (keys) => state.prefsRejected.push(keys),
  };
  state.clock = {
    setTimeout: (handler, delay) => {
      const call = { handler, delay };
      state.scheduled.push(call);
      return call;
    },
    clearTimeout: (handle) => state.cancelled.push(handle),
  };
  return state;
}

describe('panelStreamUrl', () => {
  it('keeps the panel mount and uses the matching WebSocket transport', () => {
    expect(panelStreamUrl('/panel', 'https://example.com/panel/')).toBe(
      'wss://example.com/panel/api/v1/events',
    );
    expect(panelStreamUrl('', 'http://127.0.0.1:8080/')).toBe('ws://127.0.0.1:8080/api/v1/events');
  });
});

describe('readEvent', () => {
  it('accepts versioned readiness, scoped changes, and revocation frames', () => {
    expect(readEvent('{"version":1,"type":"ready"}')).toEqual({ version: 1, type: 'ready' });
    expect(
      readEvent(
        '{"version":1,"type":"repository.changed","target_id":"2001","repository_id":"4001"}',
      ),
    ).toEqual({
      version: 1,
      type: 'repository.changed',
      target_id: '2001',
      repository_id: '4001',
    });
    expect(readEvent('{"version":1,"type":"queue.changed"}')).toEqual({
      version: 1,
      type: 'queue.changed',
    });
    expect(
      readEvent('{"version":1,"type":"session.revoked","code":"banned","reason":"policy breach"}'),
    ).toEqual({
      version: 1,
      type: 'session.revoked',
      code: 'banned',
      reason: 'policy breach',
    });
  });

  it('accepts preference frames and drops malformed prefs payloads', () => {
    expect(readEvent('{"version":1,"type":"ready","prefs":{"rev":1,"sum":"abc"}}')).toEqual({
      version: 1,
      type: 'ready',
      prefs: { rev: 1, sum: 'abc' },
    });
    expect(
      readEvent('{"version":1,"type":"ready","prefs":{"rev":1,"sum":"abc","values":{"a":"b"}}}'),
    ).toEqual({
      version: 1,
      type: 'ready',
      prefs: { rev: 1, sum: 'abc', values: { a: 'b' } },
    });
    expect(readEvent('{"version":1,"type":"ready","prefs":{"rev":"broken"}}')).toEqual({
      version: 1,
      type: 'ready',
    });
    expect(
      readEvent('{"version":1,"type":"prefs.changed","rev":2,"changes":{"theme":"dark"}}'),
    ).toEqual({
      version: 1,
      type: 'prefs.changed',
      rev: 2,
      changes: { theme: 'dark' },
    });
    expect(readEvent('{"version":1,"type":"prefs.changed","rev":-1,"changes":{}}')).toBeNull();
    expect(readEvent('{"version":1,"type":"prefs.changed","rev":2}')).toBeNull();
    expect(readEvent('{"version":1,"type":"prefs.rejected","keys":["bogus"]}')).toEqual({
      version: 1,
      type: 'prefs.rejected',
      keys: ['bogus'],
    });
    expect(readEvent('{"version":1,"type":"prefs.rejected","keys":[7]}')).toBeNull();
  });

  it('rejects malformed, unversioned, and future frames without throwing', () => {
    for (const frame of [
      'not json',
      new ArrayBuffer(2),
      '{"type":"resync"}',
      '{"version":2,"type":"resync"}',
      '{"version":1,"type":"repository.changed"}',
      '{"version":1,"type":"later","target_id":"2001"}',
      '{"version":1,"type":"audit.changed","target_id":7}',
    ]) {
      expect(readEvent(frame)).toBeNull();
    }
  });
});

describe('openPanelStream', () => {
  it('resyncs on ready, delivers changes, and reconnects with bounded backoff', () => {
    const state = streamFixture();
    const stream = openPanelStream(
      () => 'wss://example.com/api/v1/events',
      state.handlers,
      state.createSocket,
      state.clock,
    );
    const first = state.sockets[0];
    if (first === undefined) throw new Error('WebSocket was not opened');

    first.emit('open');
    expect(state.resyncs).toBe(0);
    first.deliver({ version: 1, type: 'ready' });
    first.deliver({ version: 1, type: 'failure.changed', target_id: '2001' });
    first.deliver({ version: 1, type: 'queue.changed' });
    first.emit('close');

    expect(state.resyncs).toBe(1);
    expect(state.changes).toEqual([
      { version: 1, type: 'failure.changed', target_id: '2001' },
      { version: 1, type: 'queue.changed' },
    ]);
    expect(state.scheduled).toHaveLength(1);
    expect(state.scheduled[0]?.delay).toBe(1_000);

    state.scheduled[0]?.handler();
    const second = state.sockets[1];
    if (second === undefined) throw new Error('WebSocket was not reconnected');
    second.deliver({ version: 1, type: 'ready' });
    expect(state.resyncs).toBe(2);

    stream.stop();
    expect(second.closes).toEqual([{ code: 1000, reason: 'panel closed' }]);
  });

  it('stops reconnecting after the server revokes the session', () => {
    const state = streamFixture();
    openPanelStream(
      () => 'wss://example.com/api/v1/events',
      state.handlers,
      state.createSocket,
      state.clock,
    );
    const socket = state.sockets[0];
    if (socket === undefined) throw new Error('WebSocket was not opened');

    socket.deliver({
      version: 1,
      type: 'session.revoked',
      code: 'banned',
      reason: 'policy breach',
    });
    socket.emit('close');

    expect(state.revoked).toEqual([{ code: 'banned', reason: 'policy breach' }]);
    expect(socket.closes).toEqual([{ code: 4001, reason: 'session revoked' }]);
    expect(state.scheduled).toHaveLength(0);
  });

  it('rebuilds the dial URL for every connect attempt', () => {
    const state = streamFixture();
    let revision = 0;
    openPanelStream(
      () => `wss://example.com/api/v1/events?prefs_rev=${String(revision)}`,
      state.handlers,
      state.createSocket,
      state.clock,
    );
    revision = 3;
    state.sockets[0]?.emit('close');
    state.scheduled[0]?.handler();

    expect(state.urls).toEqual([
      'wss://example.com/api/v1/events?prefs_rev=0',
      'wss://example.com/api/v1/events?prefs_rev=3',
    ]);
  });

  it('dispatches preference frames to their handlers', () => {
    const state = streamFixture();
    openPanelStream(
      () => 'wss://example.com/api/v1/events',
      state.handlers,
      state.createSocket,
      state.clock,
    );
    const socket = state.sockets[0];
    if (socket === undefined) throw new Error('WebSocket was not opened');

    socket.deliver({ version: 1, type: 'ready', prefs: { rev: 2, sum: 'abc', values: {} } });
    socket.deliver({ version: 1, type: 'prefs.changed', rev: 3, changes: { theme: 'dark' } });
    socket.deliver({ version: 1, type: 'prefs.rejected', keys: ['bogus'] });

    expect(state.prefsReady).toEqual([{ rev: 2, sum: 'abc', values: {} }]);
    expect(state.prefsChanged).toEqual([{ rev: 3, changes: { theme: 'dark' } }]);
    expect(state.prefsRejected).toEqual([['bogus']]);
  });

  it('sends only after the handshake and drops frames while disconnected', () => {
    const state = streamFixture();
    const stream = openPanelStream(
      () => 'wss://example.com/api/v1/events',
      state.handlers,
      state.createSocket,
      state.clock,
    );
    const socket = state.sockets[0];
    if (socket === undefined) throw new Error('WebSocket was not opened');

    expect(stream.send('early')).toBe(false);
    socket.deliver({ version: 1, type: 'ready' });
    expect(stream.send('patch')).toBe(true);
    socket.emit('close');
    expect(stream.send('late')).toBe(false);

    expect(socket.sent).toEqual(['patch']);
  });
});
