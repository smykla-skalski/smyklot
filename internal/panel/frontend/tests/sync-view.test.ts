// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import type { SettingsDraftStorage } from '../src/lib/settings-draft-storage';
import type {
  SyncCell,
  SyncConfig,
  SyncFilesContext,
  SyncOverride,
  SyncPlan,
  SyncStatus,
} from '../src/lib/types';
import SyncViewHarness from './support/SyncViewHarness.svelte';

/** The settings form's segmented controls measure themselves; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

class MemoryStorage implements SettingsDraftStorage {
  readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
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

  function registry(storage: SettingsDraftStorage | null = null): SettingsDraftRegistry {
    const drafts = new SettingsDraftRegistry({ storage, now: () => 1, writerId: 'test' });
    drafts.hydrate('viewer-1');
    return drafts;
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
    state: {
      clock?: () => number;
      plan?: SyncPlan | null;
      status?: SyncStatus;
      drafts?: SettingsDraftRegistry;
      fileName?: string;
      filesContext?: SyncFilesContext;
      fetchOverride?: (
        targetId: string,
        repositoryId: string,
        kind: string,
      ) => Promise<SyncOverride>;
    } = {},
  ) {
    const answers: Record<string, SyncConfig> = { labels, settings, rulesets, files };
    const drafts = state.drafts ?? registry();

    const page = render(SyncViewHarness, {
      targetId: 'target-1',
      section,
      fileName: state.fileName,
      readOnly: false,
      drafts,
      fetchConfig: (_id: string, kind: string) => Promise.resolve(answers[kind]),
      fetchPlan: () => Promise.resolve({ plan: state.plan ?? null }),
      approvePlan: () => Promise.reject(new Error('not in this test')),
      discardPlan: () => Promise.reject(new Error('not in this test')),
      fetchStatus: () =>
        Promise.resolve(
          state.status ?? { checked_at: new Date(0).toISOString(), repositories: [] },
        ),
      sectionHref: (section: string) => `#/sync/${section}`,
      onOpenSection: () => {},
      rulesetHref: (name: string) => `#/sync/rulesets/${name}`,
      onOpenRuleset: () => {},
      fileHref: (path: string) => `#/sync/files/${path}`,
      onOpenFile: () => {},
      fetchFilesContext: () =>
        Promise.resolve(
          state.filesContext ?? { repositories: 0, covered: 0, known_paths: [], merges: [] },
        ),
      fetchOverride: state.fetchOverride ?? (() => Promise.reject(new Error('not in this test'))),
      clock: state.clock,
    });
    return { page, drafts };
  }

  const MISSING = 'Smyklot has not been granted administration access, which settings sync needs';

  it('stages labels under exact controls and marks the changed surfaces', async () => {
    const { drafts } = mount(
      config('labels', { labels: [{ name: 'bug', color: 'd73a4a' }] }),
      config('settings'),
      config('rulesets'),
      config('files'),
      'labels',
    );

    await screen.findByRole('heading', { name: 'Labels' });
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Label sync' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Remove bug' }));

    expect(drafts.dirtyControls().map(({ id }) => id)).toEqual([
      'sync.labels.enabled',
      'sync.labels.labels',
    ]);
    expect(document.querySelector('.kind-head')?.getAttribute('data-unsaved')).toBe('true');
    expect(document.querySelector('.label-card')?.getAttribute('data-unsaved')).toBe('true');

    expect(drafts.discardScope({ type: 'installation', targetId: 'target-1' })).toBe(1);
    await waitFor(() =>
      expect(
        (screen.getByRole('checkbox', { name: 'Label sync' }) as HTMLInputElement).checked,
      ).toBe(false),
    );
    expect(screen.getByRole('button', { name: 'Remove bug' })).toBeTruthy();
  });

  it('stages a per-kind switch and document without a network write', async () => {
    const { drafts } = mount(
      config('labels'),
      config('settings', { enabled: false, document: { has_wiki: false } }),
      config('rulesets'),
      config('files'),
      'settings',
    );

    await screen.findByRole('heading', { name: 'Repository settings' });
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Settings sync' }));
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Wiki' }));

    expect(drafts.dirtyControls().map(({ id }) => id)).toEqual([
      'sync.settings.enabled',
      'sync.settings.document',
    ]);
    expect(
      [...document.querySelectorAll<HTMLElement>('.policy-row')]
        .find((row) => (row.textContent ?? '').includes('Wiki'))
        ?.getAttribute('data-unsaved'),
    ).toBe('true');
  });

  it('stages a repository file adjustment without a write request', async () => {
    const merge = {
      path: 'renovate.json',
      strategy: 'deep-merge',
      overrides: { timezone: 'Europe/Warsaw' },
    };
    const fetchOverride = vi.fn(() =>
      Promise.resolve<SyncOverride>({
        kind: 'files',
        enabled: null,
        document: { merges: [merge] },
        revision: 3,
        updated_by: 'bart',
        updated_at: new Date(0).toISOString(),
        unreadable: false,
      }),
    );
    const { drafts } = mount(
      config('labels'),
      config('settings'),
      config('rulesets'),
      config('files', {
        document: {
          files: [{ path: 'renovate.json', content: '{ "timezone": "UTC" }' }],
        },
      }),
      'files',
      {
        fileName: 'renovate.json',
        filesContext: {
          repositories: 1,
          covered: 1,
          known_paths: [],
          merges: [
            {
              repository: 'repo-a',
              repository_id: 'repo-1',
              path: 'renovate.json',
              merge,
            },
          ],
        },
        fetchOverride,
      },
    );

    await screen.findByRole('heading', { name: 'renovate.json' });
    await fireEvent.click(screen.getByRole('button', { name: /repo-a/ }));
    const remove = await screen.findByRole('button', { name: 'Stop changing timezone' });
    await waitFor(() => expect((remove as HTMLButtonElement).disabled).toBe(false));
    await fireEvent.click(remove);

    expect(fetchOverride).toHaveBeenCalledTimes(1);
    expect(fetchOverride).toHaveBeenCalledWith('target-1', 'repo-1', 'files');
    expect(drafts.dirtyControls().map(({ id }) => id)).toEqual([
      'repositories.repo-1.sync.files.document',
    ]);
    expect(drafts.dirtyControls()[0]?.location).toEqual({
      section: 'repositories',
      path: ['repo-1', 'sync', 'files', 'document'],
    });
  });

  it('overlays a persisted draft after the registry restarts', async () => {
    const storage = new MemoryStorage();
    const first = mount(
      config('labels'),
      config('settings'),
      config('rulesets'),
      config('files'),
      'labels',
      { drafts: registry(storage) },
    );
    await screen.findByRole('heading', { name: 'Labels' });
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Label sync' }));
    first.page.unmount();

    const restarted = registry(storage);
    expect(restarted.dirtyControlCount).toBe(1);
    mount(config('labels'), config('settings'), config('rulesets'), config('files'), 'labels', {
      drafts: restarted,
    });

    await waitFor(() =>
      expect(
        (screen.getByRole('checkbox', { name: 'Label sync' }) as HTMLInputElement).checked,
      ).toBe(true),
    );
  });

  function fleet(...repositories: Array<[string, SyncCell['state']]>): SyncStatus {
    return {
      checked_at: new Date(0).toISOString(),
      repositories: repositories.map(([repository, state]) => ({
        repository,
        cells: {
          labels: { state },
          settings: { state },
          rulesets: { state },
          files: { state },
        },
      })),
    };
  }

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

    render(SyncViewHarness, {
      targetId: 'target-1',
      section: 'rulesets',
      readOnly: false,
      drafts: registry(),
      fetchConfig: (_id: string, kind: string) => {
        asked.push(kind);

        return Promise.resolve(config(kind));
      },
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

    render(SyncViewHarness, {
      targetId: 'target-1',
      section: 'files',
      readOnly: false,
      drafts: registry(),
      fetchConfig: (_id: string, kind: string) => {
        asked.push(kind);

        return Promise.resolve(config(kind));
      },
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

  it('calls a fully disabled fleet switched off instead of in step', async () => {
    mount(config('labels'), config('settings'), config('rulesets'), config('files'), 'overview', {
      status: fleet(['one', 'off'], ['two', 'off']),
    });

    await screen.findByRole('heading', { name: 'All 2 are switched off here' });
  });

  it('separates active and switched-off repositories in a settled fleet', async () => {
    mount(config('labels'), config('settings'), config('rulesets'), config('files'), 'overview', {
      status: fleet(['active', 'in_step'], ['disabled', 'off']),
    });

    await screen.findByRole('heading', { name: '1 active in step · 1 switched off' });
  });

  it('gives an empty fleet an honest verdict', async () => {
    mount(config('labels'), config('settings'), config('rulesets'), config('files'), 'overview');

    await screen.findByRole('heading', { name: 'No repositories to check' });
  });

  it('renders relative plan times against an injected catalogue clock', async () => {
    const now = Date.UTC(2026, 7, 18, 12, 0, 0);
    const plan: SyncPlan = {
      id: 'plan-1',
      trigger: 'reconcile',
      state: 'computed',
      digest: 'digest',
      counts: { create: 1, update: 0, delete: 0 },
      actions: [
        {
          repository: 'smyklot',
          kind: 'labels',
          operation: 'create',
          subject: 'bug',
          state: 'pending',
        },
      ],
      computed_at: new Date(now - 12 * 60_000).toISOString(),
      expires_at: new Date(now + 6 * 60 * 60_000 + 5 * 60_000).toISOString(),
    };
    const status: SyncStatus = {
      checked_at: new Date(now - 5 * 60_000).toISOString(),
      repositories: [
        {
          repository: 'smyklot',
          cells: {
            labels: { state: 'pending', changes: 1 },
            settings: { state: 'in_step' },
            rulesets: { state: 'in_step' },
            files: { state: 'in_step' },
          },
        },
      ],
    };

    mount(config('labels'), config('settings'), config('rulesets'), config('files'), 'overview', {
      clock: () => now,
      plan,
      status,
    });

    await screen.findByRole('heading', { name: '1 of 1 are out of step' });
    expect(screen.getByText('5 minutes ago')).toBeTruthy();
    expect(screen.getByText(/Expires in 6 hours/u)).toBeTruthy();
  });
});
