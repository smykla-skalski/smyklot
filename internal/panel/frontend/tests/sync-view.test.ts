// @vitest-environment jsdom
import { render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncView from '../src/lib/components/SyncView.svelte';
import type { SyncConfig } from '../src/lib/types';

/** The settings form's segmented controls measure themselves; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/**
 * The page an operator opens to keep an organization in step, and the one place
 * that says why nothing is happening.
 *
 * A kind switched on without the permission behind it plans nothing and fails at
 * nothing, so the plan list below reads exactly as it does while waiting for a
 * sweep. Settings sync is the first kind needing a permission no installation
 * has granted, which makes that the ordinary first-use answer rather than a
 * corner of one.
 */
describe('SyncView [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  function config(kind: string, over: Partial<SyncConfig> = {}): SyncConfig {
    return {
      kind,
      enabled: false,
      labels: [],
      allow_removal: false,
      excludes: [],
      revision: 1,
      updated_by: '',
      updated_at: new Date(0).toISOString(),
      digest: '',
      document: {},
      unreadable: false,
      unavailable: '',
      ...over,
    };
  }

  /** The view with both kinds answered, and no plan waiting. */
  function mount(labels: SyncConfig, settings: SyncConfig) {
    return render(SyncView, {
      targetId: 'target-1',
      readOnly: false,
      fetchConfig: (_id: string, kind: string) =>
        Promise.resolve(kind === 'labels' ? labels : settings),
      saveConfig: () => Promise.resolve(labels),
      fetchPlan: () => Promise.resolve({ plan: null }),
      approvePlan: () => Promise.reject(new Error('not in this test')),
    });
  }

  const MISSING = 'Smyklot has not been granted administration access, which settings sync needs';

  /**
   * The whole point of the answer carrying the permission: it has to arrive at
   * the form, not merely at the page. The settings form is a child component,
   * and a prop nobody passes is a notice nobody reads.
   */
  it('carries a missing permission down to the settings form', async () => {
    mount(config('labels'), config('settings', { enabled: true, unavailable: MISSING }));

    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('administration'));
  });

  /** Nobody asked for the kind, so nothing is waiting on the permission. */
  it('says nothing of a permission while the kind is switched off', async () => {
    mount(config('labels'), config('settings', { unavailable: MISSING }));

    await screen.findByRole('heading', { name: 'Repository settings' });

    expect(screen.queryByRole('status')).toBeNull();
  });

  /**
   * And the same rule for labels, which is the kind that reads its notice from
   * this file rather than from the form below it. Labelling is what the bot was
   * let in to do, so this is nearly always empty - which is exactly why nothing
   * would have noticed it being wrong.
   */
  it('says which permission labels are missing', async () => {
    const because = 'Smyklot has not been granted issues access, which labels sync needs';
    mount(config('labels', { enabled: true, unavailable: because }), config('settings'));

    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('issues'));
  });
});
