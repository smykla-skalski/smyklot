import { describe, expect, it } from 'vitest';

import type {
  PanelStreamClock,
  PanelStreamEvent,
  PanelStreamHandlers,
  PanelWebSocket,
} from '../src/lib/events';
import { openPanelStream, panelStreamUrl, readEvent } from '../src/lib/events';

type Listener = (event: never) => void;

class FakeWebSocket implements PanelWebSocket {
  readonly listeners = new Map<string, Listener[]>();
  readonly closes: Array<{ code?: number; reason?: string }> = [];

  addEventListener(type: string, listener: Listener): void {
    const listeners = this.listeners.get(type) ?? [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
  }

  close(code?: number, reason?: string): void {
    this.closes.push({ code, reason });
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
  scheduled: ScheduledCall[];
  cancelled: unknown[];
  resyncs: number;
  revoked: Array<{ code: string; reason: string }>;
  changes: Array<Extract<PanelStreamEvent, { target_id: string }>>;
  handlers: PanelStreamHandlers;
  clock: PanelStreamClock;
  createSocket: () => PanelWebSocket;
}

function streamFixture(): StreamFixture {
  const state: StreamFixture = {
    sockets: [],
    scheduled: [],
    cancelled: [],
    resyncs: 0,
    revoked: [],
    changes: [],
    handlers: {} as PanelStreamHandlers,
    clock: {} as PanelStreamClock,
    createSocket: () => {
      const socket = new FakeWebSocket();
      state.sockets.push(socket);
      return socket;
    },
  };
  state.handlers = {
    onResync: () => {
      state.resyncs += 1;
    },
    onChange: (event) => state.changes.push(event),
    onRevoked: (event) => state.revoked.push(event),
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
    expect(
      readEvent('{"version":1,"type":"session.revoked","code":"banned","reason":"policy breach"}'),
    ).toEqual({
      version: 1,
      type: 'session.revoked',
      code: 'banned',
      reason: 'policy breach',
    });
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
    const stop = openPanelStream(
      'wss://example.com/api/v1/events',
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
    first.emit('close');

    expect(state.resyncs).toBe(1);
    expect(state.changes).toEqual([{ version: 1, type: 'failure.changed', target_id: '2001' }]);
    expect(state.scheduled).toHaveLength(1);
    expect(state.scheduled[0]?.delay).toBe(1_000);

    state.scheduled[0]?.handler();
    const second = state.sockets[1];
    if (second === undefined) throw new Error('WebSocket was not reconnected');
    second.deliver({ version: 1, type: 'ready' });
    expect(state.resyncs).toBe(2);

    stop();
    expect(second.closes).toEqual([{ code: 1000, reason: 'panel closed' }]);
  });

  it('stops reconnecting after the server revokes the session', () => {
    const state = streamFixture();
    openPanelStream(
      'wss://example.com/api/v1/events',
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
});
