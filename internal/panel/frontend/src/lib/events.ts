import { panelUrl } from './base';

export const PANEL_STREAM_VERSION = 1;

export type PanelChangeType =
  | 'target.changed'
  | 'repository.changed'
  | 'audit.changed'
  | 'failure.changed'
  | 'users.changed'
  | 'invitation.changed'
  | 'access.changed';

export type PanelStreamEvent =
  | { version: 1; type: 'ready' | 'resync' }
  | {
      version: 1;
      type: PanelChangeType;
      target_id: string;
      repository_id?: string;
    }
  | { version: 1; type: 'session.revoked'; code: string; reason: string };

type PanelChangeEvent = Extract<PanelStreamEvent, { target_id: string }>;
type PanelRevokedEvent = Extract<PanelStreamEvent, { type: 'session.revoked' }>;

export interface PanelStreamHandlers {
  onResync: () => void;
  onChange: (event: PanelChangeEvent) => void;
  onRevoked?: (event: Omit<PanelRevokedEvent, 'version' | 'type'>) => void;
}

export interface PanelWebSocket {
  addEventListener(type: string, listener: (event: unknown) => void): void;
  close(code?: number, reason?: string): void;
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
  url: string,
  handlers: PanelStreamHandlers,
  createSocket: PanelWebSocketFactory,
  clock: PanelStreamClock = browserClock,
): () => void {
  let socket: PanelWebSocket | null = null;
  let retryHandle: unknown;
  let retryAttempt = 0;
  let stopped = false;
  let revoked = false;

  const connect = (): void => {
    if (stopped || revoked) return;
    retryHandle = undefined;
    const opened = createSocket(url);
    socket = opened;
    opened.addEventListener('message', (message) => {
      const data = readMessageData(message);
      const event = readEvent(data);
      if (event === null) return;
      if (event.type === 'ready') {
        retryAttempt = 0;
        handlers.onResync();
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
      if ('target_id' in event) handlers.onChange(event);
    });
    opened.addEventListener('error', () => {});
    opened.addEventListener('close', () => {
      if (socket === opened) socket = null;
      if (stopped || revoked || retryHandle !== undefined) return;
      const delay = Math.min(1_000 * 2 ** retryAttempt, 30_000);
      retryAttempt += 1;
      retryHandle = clock.setTimeout(connect, delay);
    });
  };

  connect();

  return () => {
    stopped = true;
    if (retryHandle !== undefined) clock.clearTimeout(retryHandle);
    socket?.close(1000, 'panel closed');
    socket = null;
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
  if (frame.type === 'ready' || frame.type === 'resync') {
    return { version: PANEL_STREAM_VERSION, type: frame.type };
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
  if (!changeTypes.has(frame.type) || typeof frame.target_id !== 'string') return null;
  if (frame.repository_id !== undefined && typeof frame.repository_id !== 'string') return null;
  return {
    version: PANEL_STREAM_VERSION,
    type: frame.type as PanelChangeType,
    target_id: frame.target_id,
    ...(frame.repository_id === undefined ? {} : { repository_id: frame.repository_id }),
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
