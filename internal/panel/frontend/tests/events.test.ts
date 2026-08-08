import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { PanelSocket, PanelStreamHandlers } from '../src/lib/events';
import { openPanelStream, panelStreamUrl } from '../src/lib/events';
import type { PanelPairing } from '../src/lib/types';

type Listener = (event: never) => void;

/** A socket a test drives by hand. */
class FakeSocket implements PanelSocket {
  readonly listeners = new Map<string, Listener[]>();
  closed = false;

  constructor(readonly url: string) {}

  addEventListener(type: string, listener: Listener): void {
    const existing = this.listeners.get(type) ?? [];
    existing.push(listener);
    this.listeners.set(type, existing);
  }

  close(): void {
    this.closed = true;
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

interface Harness {
  sockets: FakeSocket[];
  resyncs: number;
  pairings: { change: string; pairing: PanelPairing }[];
  handlers: PanelStreamHandlers;
  createSocket: (url: string) => PanelSocket;
  /** The nth socket the stream opened, or a failure naming how many it opened. */
  socketAt: (index: number) => FakeSocket;
  latest: () => FakeSocket;
}

function harness(): Harness {
  const state: Harness = {
    sockets: [] as FakeSocket[],
    resyncs: 0,
    pairings: [] as { change: string; pairing: PanelPairing }[],
    handlers: {} as PanelStreamHandlers,
    createSocket: (url: string): PanelSocket => {
      const socket = new FakeSocket(url);
      state.sockets.push(socket);
      return socket;
    },
    socketAt: (index: number): FakeSocket => {
      const socket = state.sockets[index];
      if (socket === undefined) {
        throw new Error(`expected socket ${index}, but only ${state.sockets.length} were opened`);
      }
      return socket;
    },
    latest: (): FakeSocket => state.socketAt(state.sockets.length - 1),
  };
  state.handlers = {
    onResync: () => {
      state.resyncs += 1;
    },
    onPairing: (event) => {
      state.pairings.push(event);
    },
  };
  return state;
}

function claimed(pairingId: string): PanelPairing {
  return {
    pairing_id: pairingId,
    state: 'active',
    role: 'operator',
    created_at: '2026-07-26T10:00:00Z',
    expires_at: '2026-07-26T10:10:00Z',
    claimed_at: '2026-07-26T10:01:00Z',
    device: {
      client_id: 'device-1',
      display_name: "Ada's laptop",
      platform: 'macos',
    },
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  // `openPanelStream` watches the page coming back to the foreground, which the
  // node environment has no document for.
  vi.stubGlobal('document', {
    visibilityState: 'visible',
    addEventListener: () => {},
    removeEventListener: () => {},
  });
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('panelStreamUrl', () => {
  /** A panel served over TLS must not have its socket fall back to plaintext. */
  it('follows the scheme of the page that opened it', () => {
    expect(panelStreamUrl('/panel', 'https://iri.example.com/panel/')).toBe(
      'wss://iri.example.com/panel/api/ws',
    );
    expect(panelStreamUrl('', 'https://iri.example.com/')).toBe('wss://iri.example.com/api/ws');
    expect(panelStreamUrl('/panel', 'http://127.0.0.1:8787/panel/')).toBe(
      'ws://127.0.0.1:8787/panel/api/ws',
    );
  });
});

describe('openPanelStream', () => {
  /**
   * Nothing is replayed, so a socket coming up says only that what is on screen
   * may be stale. Firing on every open and not just the first is what makes a
   * reconnection put the page right.
   */
  it('asks for a re-read every time the socket comes up', () => {
    const state = harness();
    const stop = openPanelStream('wss://panel/api/ws', state.handlers, state.createSocket);

    state.socketAt(0).emit('open');
    expect(state.resyncs).toBe(1);

    state.socketAt(0).emit('close');
    vi.advanceTimersByTime(1_000);
    expect(state.sockets).toHaveLength(2);
    state.socketAt(1).emit('open');
    expect(state.resyncs).toBe(2);

    stop();
  });

  it('hands a pairing change on with what the panel said about it', () => {
    const state = harness();
    const stop = openPanelStream('wss://panel/api/ws', state.handlers, state.createSocket);

    state.socketAt(0).emit('open');
    state.socketAt(0).deliver({ type: 'pairing', change: 'claimed', pairing: claimed('pair-1') });

    expect(state.pairings).toEqual([{ change: 'claimed', pairing: claimed('pair-1') }]);
    stop();
  });

  it('treats a resync frame the same as the socket coming up', () => {
    const state = harness();
    const stop = openPanelStream('wss://panel/api/ws', state.handlers, state.createSocket);

    state.socketAt(0).emit('open');
    state.socketAt(0).deliver({ type: 'resync' });

    expect(state.resyncs).toBe(2);
    stop();
  });

  /**
   * The panel will grow frames this build has never heard of, and a page that
   * threw on one would drop its whole connection over a message it had no need
   * of.
   */
  it('ignores anything it does not recognise', () => {
    const state = harness();
    const stop = openPanelStream('wss://panel/api/ws', state.handlers, state.createSocket);
    state.socketAt(0).emit('open');

    state.socketAt(0).emit('message', { data: 'not json' });
    state.socketAt(0).emit('message', { data: new ArrayBuffer(4) });
    state.socketAt(0).deliver({ type: 'something_later' });
    state.socketAt(0).deliver({ type: 'pairing', change: 'claimed' });
    state.socketAt(0).deliver({ type: 'pairing', pairing: claimed('pair-1') });
    // Typed as a whole pairing, so a partial one must not reach a handler that
    // believes the type. Nothing reads past the id today, and the first caller
    // that does would be the one to crash on it.
    const withoutState: Record<string, unknown> = { ...claimed('pair-1') };
    delete withoutState.state;
    state.socketAt(0).deliver({ type: 'pairing', change: 'claimed', pairing: withoutState });
    state.socketAt(0).deliver({ type: 'pairing', change: 'claimed', pairing: 'pair-1' });

    expect(state.pairings).toHaveLength(0);
    expect(state.resyncs).toBe(1);
    stop();
  });

  /**
   * A browser reports a refused connection as an error, sometimes without a
   * close, and a stream that only retried on close would stay down for good.
   */
  it('retries after an error as well as after a close', () => {
    const state = harness();
    const stop = openPanelStream('wss://panel/api/ws', state.handlers, state.createSocket);

    state.socketAt(0).emit('error');
    vi.advanceTimersByTime(1_000);

    expect(state.sockets).toHaveLength(2);
    stop();
  });

  /**
   * One connection ending must schedule one reconnection. A browser that fires
   * both error and close would otherwise leave two timers dialling the panel.
   */
  it('does not dial twice for one connection ending', () => {
    const state = harness();
    const stop = openPanelStream('wss://panel/api/ws', state.handlers, state.createSocket);

    state.socketAt(0).emit('error');
    state.socketAt(0).emit('close');
    vi.advanceTimersByTime(30_000);

    expect(state.sockets).toHaveLength(2);
    stop();
  });

  /**
   * A panel that is down stays down for a while, and a page retrying every
   * second for an hour is a page hammering a service that cannot answer.
   */
  it('backs off while the panel keeps refusing', () => {
    const state = harness();
    const stop = openPanelStream('wss://panel/api/ws', state.handlers, state.createSocket);

    for (let attempt = 1; attempt <= 8; attempt += 1) {
      state.latest().emit('close');
      // The wait carries jitter, so this advances past the ceiling rather than
      // by the exact figure, and the assertion is that one dial happened.
      vi.advanceTimersByTime(30_000);
      expect(state.sockets).toHaveLength(attempt + 1);
    }

    // Nothing has connected, so nothing should have asked for a re-read.
    expect(state.resyncs).toBe(0);
    stop();
  });

  /** Tearing down mid-backoff must leave nothing behind to wake up later. */
  it('stops dialling once it is stopped', () => {
    const state = harness();
    const stop = openPanelStream('wss://panel/api/ws', state.handlers, state.createSocket);
    state.socketAt(0).emit('close');

    stop();
    vi.advanceTimersByTime(60_000);

    expect(state.sockets).toHaveLength(1);
  });

  it('closes the open socket when it is stopped', () => {
    const state = harness();
    const stop = openPanelStream('wss://panel/api/ws', state.handlers, state.createSocket);
    state.socketAt(0).emit('open');

    stop();

    expect(state.socketAt(0).closed).toBe(true);
  });

  /**
   * A page whose socket cannot even be constructed — a URL the browser refuses
   * outright — must still be retried rather than left silently dead.
   */
  it('retries when the socket cannot be constructed', () => {
    const state = harness();
    let refuse = true;
    const stop = openPanelStream('wss://panel/api/ws', state.handlers, (url) => {
      if (refuse) {
        refuse = false;
        throw new Error('refused');
      }
      return state.createSocket(url);
    });

    expect(state.sockets).toHaveLength(0);
    vi.advanceTimersByTime(1_000);
    expect(state.sockets).toHaveLength(1);

    stop();
  });
});
