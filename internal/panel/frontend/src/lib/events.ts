import { panelUrl } from './base';

export type PanelChangeType = 'target' | 'repository' | 'audit' | 'failure';

export type PanelStreamEvent =
  { type: 'resync' } | { type: PanelChangeType; target_id: string; repository_id?: string };

export interface PanelStreamHandlers {
  onResync: () => void;
  onChange: (event: Exclude<PanelStreamEvent, { type: 'resync' }>) => void;
}

export interface PanelEventSource {
  addEventListener(type: 'open', listener: () => void): void;
  addEventListener(type: 'message', listener: (event: { data: unknown }) => void): void;
  addEventListener(type: 'error', listener: () => void): void;
  close(): void;
}

export type PanelEventSourceFactory = (url: string) => PanelEventSource;

export function panelStreamUrl(base: string, href: string): string {
  return new URL(panelUrl(base, '/api/v1/events'), href).toString();
}

export function openPanelStream(
  url: string,
  handlers: PanelStreamHandlers,
  createSource: PanelEventSourceFactory,
): () => void {
  const source = createSource(url);
  source.addEventListener('open', handlers.onResync);
  source.addEventListener('message', (event) => deliver(handlers, event.data));
  // Native EventSource reconnects with the server-supplied retry policy. An
  // error therefore needs no second client-side retry loop.
  source.addEventListener('error', () => {});

  return () => {
    source.close();
  };
}

function deliver(handlers: PanelStreamHandlers, data: unknown): void {
  const event = readEvent(data);
  if (event === null) return;
  if (event.type === 'resync') {
    handlers.onResync();
    return;
  }
  handlers.onChange(event);
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
  if (frame.type === 'resync') return { type: 'resync' };
  if (!['target', 'repository', 'audit', 'failure'].includes(String(frame.type))) return null;
  if (typeof frame.target_id !== 'string') return null;
  if (frame.repository_id !== undefined && typeof frame.repository_id !== 'string') return null;
  return frame as Exclude<PanelStreamEvent, { type: 'resync' }>;
}
