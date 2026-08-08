import { describe, expect, it } from 'vitest';

import type { PanelEventSource, PanelStreamEvent, PanelStreamHandlers } from '../src/lib/events';
import { openPanelStream, panelStreamUrl, readEvent } from '../src/lib/events';

type Listener = (event: never) => void;

class FakeEventSource implements PanelEventSource {
  readonly listeners = new Map<string, Listener[]>();
  closed = false;

  addEventListener(type: string, listener: Listener): void {
    const listeners = this.listeners.get(type) ?? [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
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

interface StreamFixture {
  sources: FakeEventSource[];
  resyncs: number;
  changes: Array<Exclude<PanelStreamEvent, { type: 'resync' }>>;
  handlers: PanelStreamHandlers;
  createSource: () => PanelEventSource;
}

function streamFixture(): StreamFixture {
  const state: StreamFixture = {
    sources: [],
    resyncs: 0,
    changes: [],
    handlers: {} as PanelStreamHandlers,
    createSource: () => {
      const source = new FakeEventSource();
      state.sources.push(source);
      return source;
    },
  };
  state.handlers = {
    onResync: () => {
      state.resyncs += 1;
    },
    onChange: (event) => state.changes.push(event),
  };
  return state;
}

describe('panelStreamUrl', () => {
  it('keeps the panel mount and follows the page transport', () => {
    expect(panelStreamUrl('/panel', 'https://example.com/panel/')).toBe(
      'https://example.com/panel/api/v1/events',
    );
    expect(panelStreamUrl('', 'http://127.0.0.1:8080/')).toBe(
      'http://127.0.0.1:8080/api/v1/events',
    );
  });
});

describe('readEvent', () => {
  it('accepts resync and scoped change frames', () => {
    expect(readEvent('{"type":"resync"}')).toEqual({ type: 'resync' });
    expect(readEvent('{"type":"repository","target_id":"2001","repository_id":"4001"}')).toEqual({
      type: 'repository',
      target_id: '2001',
      repository_id: '4001',
    });
  });

  it('rejects malformed and future frames without throwing', () => {
    for (const frame of [
      'not json',
      new ArrayBuffer(2),
      '{"type":"repository"}',
      '{"type":"later","target_id":"2001"}',
      '{"type":"audit","target_id":7}',
    ]) {
      expect(readEvent(frame)).toBeNull();
    }
  });
});

describe('openPanelStream', () => {
  it('resyncs on every successful connection and delivers changes', () => {
    const state = streamFixture();
    const stop = openPanelStream(
      'https://example.com/api/v1/events',
      state.handlers,
      state.createSource,
    );
    const first = state.sources[0];
    if (first === undefined) throw new Error('event source was not opened');

    first.emit('open');
    first.deliver({ type: 'failure', target_id: '2001' });

    expect(state.resyncs).toBe(1);
    expect(state.changes).toEqual([{ type: 'failure', target_id: '2001' }]);
    stop();
  });

  it('closes the native event source when stopped', () => {
    const state = streamFixture();
    const stop = openPanelStream(
      'https://example.com/api/v1/events',
      state.handlers,
      state.createSource,
    );
    const first = state.sources[0];
    if (first === undefined) throw new Error('event source was not opened');

    stop();

    expect(first.closed).toBe(true);
  });
});
