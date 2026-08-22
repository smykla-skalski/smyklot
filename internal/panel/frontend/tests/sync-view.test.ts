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

  /**
   * The view with every kind answered, and no plan waiting.
   *
   * Answered by name rather than by "labels or the other one", because the page
   * asks for three now and a helper that lumped two together would hand the
   * rulesets form the settings answer - which renders a second notice and makes
   * the assertions below ambiguous rather than wrong.
   */
  function mount(
    labels: SyncConfig,
    settings: SyncConfig,
    rulesets = config('rulesets'),
    files = config('files'),
    section: 'overview' | 'labels' | 'settings' | 'rulesets' | 'files' | 'plan' = 'settings',
  ) {
    const answers: Record<string, SyncConfig> = { labels, settings, rulesets, files };

    return render(SyncView, {
      targetId: 'target-1',
      section,
      readOnly: false,
      fetchConfig: (_id: string, kind: string) => Promise.resolve(answers[kind]),
      saveConfig: () => Promise.resolve(labels),
      fetchPlan: () => Promise.resolve({ plan: null }),
      approvePlan: () => Promise.reject(new Error('not in this test')),
      discardPlan: () => Promise.reject(new Error('not in this test')),
      fetchStatus: () =>
        Promise.resolve({ checked_at: new Date(0).toISOString(), repositories: [] }),
      sectionHref: (section: string) => `#/sync/${section}`,
      onOpenSection: () => {},
      rulesetHref: (name: string) => `#/sync/rulesets/${name}`,
      onOpenRuleset: () => {},
      fileHref: (path: string) => `#/sync/files/${path}`,
      onOpenFile: () => {},
      fetchFilesContext: () =>
        Promise.resolve({ repositories: 0, covered: 0, known_paths: [], merges: [] }),
      fetchOverride: () => Promise.reject(new Error('not in this test')),
      saveOverride: () => Promise.reject(new Error('not in this test')),
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

  /**
   * The rulesets form is a third child reading a third kind, and the page fetches
   * by name. A kind the page never asks for is a form nobody mounts, which is
   * how chunk 3's whole sync UI came to pass every test written about it while
   * being unreachable.
   */
  it('asks for the rulesets kind and mounts its form', async () => {
    const asked: string[] = [];

    render(SyncView, {
      targetId: 'target-1',
      section: 'rulesets',
      readOnly: false,
      fetchConfig: (_id: string, kind: string) => {
        asked.push(kind);

        return Promise.resolve(config(kind));
      },
      saveConfig: () => Promise.resolve(config('labels')),
      fetchPlan: () => Promise.resolve({ plan: null }),
      approvePlan: () => Promise.reject(new Error('not in this test')),
      discardPlan: () => Promise.reject(new Error('not in this test')),
      fetchStatus: () =>
        Promise.resolve({ checked_at: new Date(0).toISOString(), repositories: [] }),
      sectionHref: (section: string) => `#/sync/${section}`,
      onOpenSection: () => {},
      rulesetHref: (name: string) => `#/sync/rulesets/${name}`,
      onOpenRuleset: () => {},
      fileHref: (path: string) => `#/sync/files/${path}`,
      onOpenFile: () => {},
      fetchFilesContext: () =>
        Promise.resolve({ repositories: 0, covered: 0, known_paths: [], merges: [] }),
      fetchOverride: () => Promise.reject(new Error('not in this test')),
      saveOverride: () => Promise.reject(new Error('not in this test')),
    });

    await screen.findByRole('heading', { name: 'Rulesets' });

    expect(asked).toContain('rulesets');
  });

  /**
   * And a fourth. The page fetches by name, so a kind it never asks for is a
   * form nobody mounts however complete the form is.
   */
  it('asks for the files kind and mounts its form', async () => {
    const asked: string[] = [];

    render(SyncView, {
      targetId: 'target-1',
      section: 'files',
      readOnly: false,
      fetchConfig: (_id: string, kind: string) => {
        asked.push(kind);

        return Promise.resolve(config(kind));
      },
      saveConfig: () => Promise.resolve(config('labels')),
      fetchPlan: () => Promise.resolve({ plan: null }),
      approvePlan: () => Promise.reject(new Error('not in this test')),
      discardPlan: () => Promise.reject(new Error('not in this test')),
      fetchStatus: () =>
        Promise.resolve({ checked_at: new Date(0).toISOString(), repositories: [] }),
      sectionHref: (section: string) => `#/sync/${section}`,
      onOpenSection: () => {},
      rulesetHref: (name: string) => `#/sync/rulesets/${name}`,
      onOpenRuleset: () => {},
      fileHref: (path: string) => `#/sync/files/${path}`,
      onOpenFile: () => {},
      fetchFilesContext: () =>
        Promise.resolve({ repositories: 0, covered: 0, known_paths: [], merges: [] }),
      fetchOverride: () => Promise.reject(new Error('not in this test')),
      saveOverride: () => Promise.reject(new Error('not in this test')),
    });

    await screen.findByRole('heading', { name: 'Shared files' });

    expect(asked).toContain('files');
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
    mount(
      config('labels', { enabled: true, unavailable: because }),
      config('settings'),
      config('rulesets'),
      config('files'),
      'labels',
    );

    /* By name, not the only status: the labels page also carries the save
       whisper, which is a status of its own. */
    await waitFor(() =>
      expect(
        screen.getAllByRole('status').some((node) => (node.textContent ?? '').includes('issues')),
      ).toBe(true),
    );
  });
});
