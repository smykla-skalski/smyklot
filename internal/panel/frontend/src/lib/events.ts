/**
 * The socket the page holds open to the panel.
 *
 * Nothing is replayed on it, so being connected is not by itself a picture of
 * anything: the page reads the pairing list over HTTP and this only says when
 * that answer has stopped being true. A socket that has just come up says the
 * same thing, which is why `onResync` fires on every open and not only on the
 * first.
 *
 * It restores itself. A page left open overnight goes through a laptop
 * suspending, a wifi network changing, and whatever sits between the browser and
 * the panel forgetting the connection, and a socket that stayed down would leave
 * the page showing a link that was claimed hours ago.
 */

import { panelUrl } from './base';
import type { PanelPairing } from './types';

/** What the panel sends. Anything else is ignored rather than treated as broken. */
export type PanelStreamEvent =
  { type: 'resync' } | { type: 'pairing'; change: string; pairing: PanelPairing };

export interface PanelStreamHandlers {
  /**
   * The socket came up, or the panel said the page has fallen behind. Either
   * way what is on screen may be stale and the list is worth re-reading.
   */
  onResync: () => void;
  /** One pairing became something else. */
  onPairing: (event: { change: string; pairing: PanelPairing }) => void;
}

/**
 * The part of `WebSocket` this uses. Narrower than the real thing so a test can
 * supply one without a browser, and so nothing here reaches for a capability it
 * has not declared.
 */
export interface PanelSocket {
  addEventListener(type: 'open', listener: () => void): void;
  addEventListener(type: 'message', listener: (event: { data: unknown }) => void): void;
  addEventListener(type: 'close', listener: () => void): void;
  addEventListener(type: 'error', listener: () => void): void;
  close(): void;
}

export type PanelSocketFactory = (url: string) => PanelSocket;

/** Where the backoff starts and the ceiling it doubles towards. */
const FIRST_RETRY_MS = 1_000;
const MAX_RETRY_MS = 30_000;

/**
 * How long a connection has to last before it counts as good and the backoff
 * returns to its floor. Without this a panel that accepts the socket and closes
 * it at once would be reconnected to as fast as the browser could dial.
 */
const STABLE_MS = 30_000;

/**
 * Build the socket URL from the mount point.
 *
 * The scheme follows the page's own: a panel served over TLS must not have its
 * socket fall back to plaintext, and one served over plain HTTP — loopback,
 * during development — has no TLS to negotiate.
 */
export function panelStreamUrl(base: string, href: string): string {
  const url = new URL(panelUrl(base, '/api/ws'), href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
}

/**
 * Hold the socket open until the returned function is called.
 *
 * Returns a stop: it closes whatever is open, cancels a pending reconnection,
 * and detaches the visibility listener, so a caller that tears down mid-backoff
 * leaves nothing behind to wake up later.
 */
export function openPanelStream(
  url: string,
  handlers: PanelStreamHandlers,
  createSocket: PanelSocketFactory,
): () => void {
  let socket: PanelSocket | null = null;
  let retryMs = FIRST_RETRY_MS;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let stopped = false;

  const connect = (): void => {
    if (stopped) {
      return;
    }
    timer = null;
    const openedAt = Date.now();
    let current: PanelSocket;
    try {
      current = createSocket(url);
    } catch {
      // Constructing it can throw on a URL the browser will not open at all.
      // That is not worth retrying faster than anything else that failed.
      scheduleRetry(openedAt);
      return;
    }
    socket = current;

    current.addEventListener('open', () => {
      handlers.onResync();
    });
    current.addEventListener('message', (event) => {
      deliver(handlers, event.data);
    });
    // Both end the same way. A browser reports a refused connection as an error
    // followed by a close, and one that only errors would otherwise never be
    // retried at all.
    const ended = (): void => {
      if (socket !== current) {
        return;
      }
      socket = null;
      scheduleRetry(openedAt);
    };
    current.addEventListener('close', ended);
    current.addEventListener('error', ended);
  };

  const scheduleRetry = (openedAt: number): void => {
    if (stopped || timer !== null) {
      return;
    }
    if (Date.now() - openedAt >= STABLE_MS) {
      retryMs = FIRST_RETRY_MS;
    }
    // Spread out, so several tabs woken by the same laptop opening do not all
    // dial in the same millisecond.
    const wait = retryMs / 2 + Math.random() * (retryMs / 2);
    retryMs = Math.min(retryMs * 2, MAX_RETRY_MS);
    timer = setTimeout(connect, wait);
  };

  /**
   * A backgrounded tab has its timers throttled, so the reconnection it
   * scheduled may be minutes late by the time anybody looks at it. Coming back
   * to the page is the moment its picture matters again.
   */
  const wake = (): void => {
    if (stopped || socket !== null || document.visibilityState !== 'visible') {
      return;
    }
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
    retryMs = FIRST_RETRY_MS;
    connect();
  };
  document.addEventListener('visibilitychange', wake);

  connect();

  return () => {
    stopped = true;
    document.removeEventListener('visibilitychange', wake);
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
    socket?.close();
    socket = null;
  };
}

function deliver(handlers: PanelStreamHandlers, data: unknown): void {
  const event = readEvent(data);
  if (event === null) {
    return;
  }
  if (event.type === 'resync') {
    handlers.onResync();
    return;
  }
  handlers.onPairing({ change: event.change, pairing: event.pairing });
}

/**
 * Read one frame, or `null` for anything this build does not recognise.
 *
 * A frame the panel grows later has to reach an older bundle as something it can
 * ignore. Throwing instead would end the page's whole connection over a message
 * it had no need of.
 */
function readEvent(data: unknown): PanelStreamEvent | null {
  if (typeof data !== 'string') {
    return null;
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(data);
  } catch {
    return null;
  }
  if (typeof parsed !== 'object' || parsed === null) {
    return null;
  }
  const frame = parsed as Partial<PanelStreamEvent>;
  if (frame.type === 'resync') {
    return { type: 'resync' };
  }
  if (frame.type !== 'pairing') {
    return null;
  }
  const { change, pairing } = frame as Partial<Extract<PanelStreamEvent, { type: 'pairing' }>>;
  const entry = readPairing(pairing);
  if (typeof change !== 'string' || entry === null) {
    return null;
  }
  return { type: 'pairing', change, pairing: entry };
}

/**
 * Every field `PanelPairing` declares as required.
 *
 * Checking only the id would hand a caller a value typed as a whole pairing that
 * is not one. Nothing reads past the id today — a change means re-reading the
 * list — but the type says otherwise, and the first caller to believe it would
 * be the one that crashes.
 */
const REQUIRED_PAIRING_FIELDS = ['pairing_id', 'state', 'role', 'created_at', 'expires_at'];

/**
 * A pairing, or `null` for anything that is not one.
 *
 * Only the required fields are checked. An optional one that is absent is a
 * pairing in a state that has no value for it, and a field the panel grows later
 * still arrives without this build having to learn about it first.
 */
function readPairing(value: unknown): PanelPairing | null {
  if (typeof value !== 'object' || value === null) {
    return null;
  }
  const fields = value as Record<string, unknown>;
  if (!REQUIRED_PAIRING_FIELDS.every((name) => typeof fields[name] === 'string')) {
    return null;
  }
  return value as PanelPairing;
}
