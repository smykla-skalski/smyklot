import { panelUrl } from './base';

export const PANEL_STREAM_VERSION = 1;

export type PanelChangeType =
  | 'target.changed'
  | 'repository.changed'
  | 'audit.changed'
  | 'failure.changed'
  | 'users.changed'
  | 'invitation.changed'
  | 'access.changed'
  | 'queue.changed';

type ScopedPanelChangeType = Exclude<PanelChangeType, 'queue.changed'>;

export type PanelChangeEvent =
  | {
      version: 1;
      type: ScopedPanelChangeType;
      target_id: string;
      repository_id?: string;
    }
  | { version: 1; type: 'queue.changed'; target_id?: string };

// PanelPrefsInfo rides the ready frame: the stored preference revision and
// checksum always, the full document only when the client's dial parameters
// did not match the server state.
export interface PanelPrefsInfo {
  rev: number;
  sum: string;
  values?: Record<string, unknown>;
}

export type PanelStreamEvent =
  | { version: 1; type: 'ready'; prefs?: PanelPrefsInfo }
  | { version: 1; type: 'resync' }
  | PanelChangeEvent
  | { version: 1; type: 'session.revoked'; code: string; reason: string }
  | { version: 1; type: 'prefs.changed'; rev: number; changes: Record<string, unknown> }
  | { version: 1; type: 'prefs.rejected'; keys: string[] };

type PanelRevokedEvent = Extract<PanelStreamEvent, { type: 'session.revoked' }>;
type PanelPrefsChangedEvent = Extract<PanelStreamEvent, { type: 'prefs.changed' }>;

export interface PanelStreamHandlers {
  /**
   * Whether changes are currently arriving as they happen.
   *
   * `true` on the handshake reply, which is the first proof the socket is open
   * and the server is talking; `false` when it closes, however it closed. What
   * reads it decides how much to trust data with nothing correcting it - see
   * `query-client`.
   */
  onLive?: (live: boolean) => void;
  onResync: () => void;
  onChange: (event: PanelChangeEvent) => void;
  onRevoked?: (event: Omit<PanelRevokedEvent, 'version' | 'type'>) => void;
  onPrefsReady?: (prefs: PanelPrefsInfo) => void;
  onPrefsChanged?: (event: Omit<PanelPrefsChangedEvent, 'version' | 'type'>) => void;
  onPrefsRejected?: (keys: string[]) => void;
}

export interface PanelWebSocket {
  addEventListener(type: string, listener: (event: unknown) => void): void;
  close(code?: number, reason?: string): void;
  send(data: string): void;
}

// PanelStreamHandle controls one open stream: stop tears it down, send ships
// a frame when the stream has completed its handshake and reports whether it
// was accepted.
export interface PanelStreamHandle {
  stop(): void;
  send(data: string): boolean;
}

export type PanelWebSocketFactory = (url: string) => PanelWebSocket;

export interface PanelStreamClock {
  setTimeout(handler: () => void, delay: number): unknown;
  clearTimeout(handle: unknown): void;
}

const browserClock: PanelStreamClock = {
  setTimeout: (handler, delay) => window.setTimeout(handler, delay),
  clearTimeout: (handle) => window.clearTimeout(handle as number),
};

export function panelStreamUrl(base: string, href: string): string {
  const url = new URL(panelUrl(base, '/api/v1/events'), href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
}

export function openPanelStream(
  url: () => string,
  handlers: PanelStreamHandlers,
  createSocket: PanelWebSocketFactory,
  clock: PanelStreamClock = browserClock,
): PanelStreamHandle {
  let socket: PanelWebSocket | null = null;
  let retryHandle: unknown;
  let retryAttempt = 0;
  let stopped = false;
  let revoked = false;
  // The stream is sendable once the current socket has delivered its ready
  // frame — a received frame proves the connection is open, and the server
  // ignores nothing sent before its handshake reply anyway.
  let sendable = false;

  const connect = (): void => {
    if (stopped || revoked) return;
    retryHandle = undefined;
    const opened = createSocket(url());
    socket = opened;
    opened.addEventListener('message', (message) => {
      const data = readMessageData(message);
      const event = readEvent(data);
      if (event === null) return;
      if (event.type === 'ready') {
        retryAttempt = 0;
        sendable = true;
        handlers.onLive?.(true);
        handlers.onResync();
        if (event.prefs !== undefined) handlers.onPrefsReady?.(event.prefs);
        return;
      }
      if (event.type === 'resync') {
        handlers.onResync();
        return;
      }
      if (event.type === 'session.revoked') {
        revoked = true;
        handlers.onRevoked?.({ code: event.code, reason: event.reason });
        opened.close(4001, 'session revoked');
        return;
      }
      if (event.type === 'prefs.changed') {
        handlers.onPrefsChanged?.({ rev: event.rev, changes: event.changes });
        return;
      }
      if (event.type === 'prefs.rejected') {
        handlers.onPrefsRejected?.(event.keys);
        return;
      }
      if (event.type === 'queue.changed' || 'target_id' in event) handlers.onChange(event);
    });
    opened.addEventListener('error', () => {});
    opened.addEventListener('close', () => {
      if (socket === opened) {
        socket = null;
        sendable = false;
        handlers.onLive?.(false);
      }
      if (stopped || revoked || retryHandle !== undefined) return;
      const delay = Math.min(1_000 * 2 ** retryAttempt, 30_000);
      retryAttempt += 1;
      retryHandle = clock.setTimeout(connect, delay);
    });
  };

  connect();

  return {
    stop: () => {
      stopped = true;
      if (retryHandle !== undefined) clock.clearTimeout(retryHandle);
      socket?.close(1000, 'panel closed');
      socket = null;
      sendable = false;
      handlers.onLive?.(false);
    },
    send: (data: string): boolean => {
      if (socket === null || !sendable) return false;
      try {
        socket.send(data);
        return true;
      } catch {
        return false;
      }
    },
  };
}

function readMessageData(message: unknown): unknown {
  if (typeof message !== 'object' || message === null || !('data' in message)) return null;
  return (message as { data: unknown }).data;
}

export function readEvent(data: unknown): PanelStreamEvent | null {
  if (typeof data !== 'string') return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(data);
  } catch {
    return null;
  }
  if (typeof parsed !== 'object' || parsed === null) return null;
  const frame = parsed as Record<string, unknown>;
  if (frame.version !== PANEL_STREAM_VERSION || typeof frame.type !== 'string') return null;
  if (frame.type === 'ready') {
    const prefs = readPrefsInfo(frame.prefs);
    return {
      version: PANEL_STREAM_VERSION,
      type: frame.type,
      ...(prefs === null ? {} : { prefs }),
    };
  }
  if (frame.type === 'resync') {
    return { version: PANEL_STREAM_VERSION, type: frame.type };
  }
  if (frame.type === 'prefs.changed') {
    if (!isRevision(frame.rev) || !isPlainObject(frame.changes)) return null;
    return {
      version: PANEL_STREAM_VERSION,
      type: frame.type,
      rev: frame.rev,
      changes: frame.changes,
    };
  }
  if (frame.type === 'prefs.rejected') {
    if (!Array.isArray(frame.keys) || !frame.keys.every((key) => typeof key === 'string')) {
      return null;
    }
    return { version: PANEL_STREAM_VERSION, type: frame.type, keys: frame.keys };
  }
  if (frame.type === 'session.revoked') {
    if (typeof frame.code !== 'string' || typeof frame.reason !== 'string') return null;
    return {
      version: PANEL_STREAM_VERSION,
      type: frame.type,
      code: frame.code,
      reason: frame.reason,
    };
  }
  if (frame.type === 'queue.changed') {
    if (frame.target_id !== undefined && typeof frame.target_id !== 'string') return null;
    return {
      version: PANEL_STREAM_VERSION,
      type: frame.type,
      ...(frame.target_id === undefined ? {} : { target_id: frame.target_id }),
    };
  }
  if (!changeTypes.has(frame.type) || typeof frame.target_id !== 'string') return null;
  if (frame.repository_id !== undefined && typeof frame.repository_id !== 'string') return null;
  return {
    version: PANEL_STREAM_VERSION,
    type: frame.type as ScopedPanelChangeType,
    target_id: frame.target_id,
    ...(frame.repository_id === undefined ? {} : { repository_id: frame.repository_id }),
  };
}

function isRevision(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

// readPrefsInfo validates the optional prefs payload on a ready frame; a
// malformed payload is dropped rather than failing the whole frame.
function readPrefsInfo(value: unknown): PanelPrefsInfo | null {
  if (!isPlainObject(value)) return null;
  if (!isRevision(value.rev) || typeof value.sum !== 'string') return null;
  if (value.values !== undefined && !isPlainObject(value.values)) return null;

  return {
    rev: value.rev,
    sum: value.sum,
    ...(value.values === undefined ? {} : { values: value.values }),
  };
}

const changeTypes = new Set<string>([
  'target.changed',
  'repository.changed',
  'audit.changed',
  'failure.changed',
  'users.changed',
  'invitation.changed',
  'access.changed',
]);
