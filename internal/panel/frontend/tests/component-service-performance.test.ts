// @vitest-environment jsdom
import { QueryClient } from '@tanstack/svelte-query';
import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.hoisted(() => {
  class HoistedResizeObserver {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  globalThis.ResizeObserver ??= HoistedResizeObserver as unknown as typeof ResizeObserver;
  globalThis.matchMedia ??= ((query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  })) as unknown as typeof matchMedia;
});

import type { PerformanceSeries, ServicePerformance as Measurements } from '../src/lib/types.js';
import ServicePerformanceHarness from './support/ServicePerformanceHarness.svelte';

class TestResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

const START = Date.UTC(2026, 8, 4, 8);

function series(label: string, shape: (step: number) => number): PerformanceSeries {
  return {
    label,
    points: Array.from({ length: 6 }, (_unused, step) => ({
      at: new Date(START + step * 3_600_000).toISOString(),
      observations: 10 + step,
      mean_ms: shape(step),
      value: shape(step),
    })),
  };
}

function measured(patch: Partial<Measurements['metrics']> = {}): Measurements {
  return {
    since: new Date(START).toISOString(),
    until: new Date(START + 6 * 3_600_000).toISOString(),
    metrics: {
      query: [series('Store.ListWorkQueue', (step) => 4 + step)],
      ledger: [series('reaction_scan', (step) => 4000 + step * 10)],
      lane: [series('maintenance', (step) => step % 3)],
      database: [series('size_bytes', () => 620_000_000), series('round_trip', (step) => 8 + step)],
      ...patch,
    },
  };
}

function mount(answer: () => Promise<Measurements>) {
  render(ServicePerformanceHarness, {
    props: {
      queryClient: new QueryClient({
        defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
      }),
      fetchPerformance: answer,
    },
  });
}

describe('ServicePerformance [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
    );
  });

  afterEach(() => vi.unstubAllGlobals());

  it('draws every section it was given, each named rather than coloured', async () => {
    mount(() => Promise.resolve(measured()));

    expect(await screen.findByText('Statements, average')).toBeTruthy();
    expect(screen.getByText('Finished work kept')).toBeTruthy();
    expect(screen.getByText('Work waiting')).toBeTruthy();
    expect(screen.getByText('The database itself')).toBeTruthy();

    expect(screen.getByText('Store.ListWorkQueue')).toBeTruthy();
    expect(screen.getByText('Reaction discovery')).toBeTruthy();
    expect(screen.getByText('Background work')).toBeTruthy();
    expect(screen.getByText('Size on disk')).toBeTruthy();
  });

  it('says a section is empty rather than drawing an axis with nothing on it', async () => {
    mount(() => Promise.resolve(measured({ query: [] })));

    expect(await screen.findByText('Statements, average')).toBeTruthy();
    expect(screen.getAllByText('Nothing measured in this window.').length).toBeGreaterThan(0);
  });

  it('says why the numbers could not be read, and offers a way back', async () => {
    mount(() => Promise.reject(new Error('the database did not answer')));

    expect(await screen.findByText('These numbers could not be read.')).toBeTruthy();
    expect((await screen.findByRole('alert')).textContent).toContain('the database did not answer');
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy();
  });
});
