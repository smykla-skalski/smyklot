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

// The wire never carries a timing and a gauge on one point: the service writes
// observations and durations for a statement, a value for a count, and both
// only for a lane, whose oldest wait is a duration. A fixture that fills every
// field passes whichever one the component reads.
function series(
  label: string,
  shape: (step: number) => number,
  kind: 'timing' | 'gauge' | 'lane',
): PerformanceSeries {
  return {
    label,
    points: Array.from({ length: 6 }, (_unused, step) => {
      const at = new Date(START + step * 3_600_000).toISOString();
      if (kind === 'gauge') return { at, value: shape(step) };
      if (kind === 'lane') {
        return { at, observations: 1, mean_ms: 90_000, max_ms: 90_000, value: shape(step) };
      }

      return {
        at,
        observations: 10 + step,
        failures: step === 5 ? 3 : 0,
        mean_ms: shape(step),
        max_ms: shape(step) * 2,
      };
    }),
  };
}

function measured(patch: Partial<Measurements['metrics']> = {}): Measurements {
  return {
    since: new Date(START).toISOString(),
    until: new Date(START + 6 * 3_600_000).toISOString(),
    metrics: {
      query: [series('Store.ListWorkQueue', (step) => 4 + step, 'timing')],
      ledger: [
        series('reaction_scan', (step) => 4000 + step * 10, 'gauge'),
        series('auth_cleanup', () => 0, 'gauge'),
      ],
      lane: [series('maintenance', (step) => step % 3, 'lane')],
      database: [
        series('size_bytes', () => 620_000_000, 'gauge'),
        series('round_trip', (step) => 8 + step, 'timing'),
      ],
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

    expect(await screen.findByText('How long a database read takes')).toBeTruthy();
    expect(screen.getByText('Rows kept after work finishes')).toBeTruthy();
    expect(screen.getByText('Work waiting to run')).toBeTruthy();
    expect(screen.getByText('What the database says about itself')).toBeTruthy();

    expect(screen.getByText('List work queue')).toBeTruthy();
    expect(screen.getByText('Reaction discovery')).toBeTruthy();
    expect(screen.getByText('Background queue')).toBeTruthy();
    expect(screen.getByText('Size on disk')).toBeTruthy();

    expect(screen.getAllByText('620 MB').length).toBeGreaterThan(0);
    expect(screen.getByText('13.0 ms')).toBeTruthy();
    expect(screen.getByText('9.00 ms')).toBeTruthy();
    expect(screen.getByText('75 calls in the window, 3 failed')).toBeTruthy();
    expect(screen.getByText('4,050 rows')).toBeTruthy();
    expect(screen.getByText('2 items waiting')).toBeTruthy();
    expect(screen.queryByText('Sign-in session cleanup')).toBeNull();
    expect(screen.getByText('longest wait 1m 30s')).toBeTruthy();
    expect(screen.getByText('slowest 26.0 ms')).toBeTruthy();
  });

  it('says a section is empty rather than drawing an axis with nothing on it', async () => {
    mount(() => Promise.resolve(measured({ query: [] })));

    expect(await screen.findByText('How long a database read takes')).toBeTruthy();
    expect(screen.getAllByText('Nothing measured in this window.').length).toBeGreaterThan(0);
  });

  it('says why the numbers could not be read, and offers a way back', async () => {
    mount(() => Promise.reject(new Error('the database did not answer')));

    expect(await screen.findByText('These numbers could not be read.')).toBeTruthy();
    expect((await screen.findByRole('alert')).textContent).toContain('the database did not answer');
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy();
  });
});
