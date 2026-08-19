// @vitest-environment jsdom
import { render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncView from '../src/lib/components/SyncView.svelte';
import type { SyncPage, SyncSection } from '../src/lib/routes';
import type { RepositorySummary, SyncAction, SyncConfig, SyncPlan } from '../src/lib/types';

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
    section: SyncSection,
    labels: SyncConfig,
    settings: SyncConfig,
    rulesets = config('rulesets'),
    files = config('files'),
  ) {
    const answers: Record<string, SyncConfig> = { labels, settings, rulesets, files };

    return render(SyncView, {
      targetId: 'target-1',
      readOnly: false,
      section,
      sectionHref: (page: SyncPage) => `/i/acme/sync/${page.section}`,
      fetchConfig: (_id: string, kind: string) => Promise.resolve(answers[kind]),
      saveConfig: () => Promise.resolve(labels),
      fetchPlan: () => Promise.resolve({ plan: null }),
      approvePlan: () => Promise.reject(new Error('not in this test')),
    });
  }

  /** One repository as the list endpoint answers it; only three fields matter here. */
  function repository(name: string, available = true): RepositorySummary {
    return {
      id: `repo-${name}`,
      name,
      full_name: `acme/${name}`,
      private: false,
      default_branch: 'main',
      available,
      enabled_override: null,
      effective_enabled: true,
      enabled_source: 'target',
      pending_ci_mode: 'labels',
      pending_ci_mode_source: 'target',
      config_override_count: 0,
      config_file_status: 'valid',
      updated_at: new Date(0).toISOString(),
    };
  }

  function action(repository: string, over: Partial<SyncAction> = {}): SyncAction {
    return {
      repository,
      kind: 'labels',
      operation: 'create',
      subject: 'dependencies',
      state: 'pending',
      ...over,
    };
  }

  function plan(actions: SyncAction[]): SyncPlan {
    return {
      id: 'plan-1',
      trigger: 'sweep',
      state: 'computed',
      digest: 'abc',
      counts: { create: actions.length, update: 0, delete: 0 },
      actions,
      computed_at: new Date(0).toISOString(),
      expires_at: new Date(0).toISOString(),
    };
  }

  /** The overview with a fleet and a plan behind it. */
  function mountOverview(repositories: RepositorySummary[], actions: SyncAction[], total?: number) {
    return render(SyncView, {
      targetId: 'target-1',
      readOnly: false,
      section: 'overview',
      sectionHref: (page: SyncPage) => `/i/acme/sync/${page.section}`,
      repositoryHref: (name: string) => `/i/acme/repositories/${name}`,
      fetchRepositories: () =>
        Promise.resolve({
          items: repositories,
          next_cursor: null,
          total: total ?? repositories.length,
        }),
      fetchConfig: (_id: string, kind: string) => Promise.resolve(config(kind)),
      saveConfig: () => Promise.resolve(config('labels')),
      fetchPlan: () => Promise.resolve({ plan: plan(actions) }),
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
    mount(
      'settings',
      config('labels'),
      config('settings', { enabled: true, unavailable: MISSING }),
    );

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
      readOnly: false,
      section: 'rulesets',
      sectionHref: (page: SyncPage) => `/i/acme/sync/${page.section}`,
      fetchConfig: (_id: string, kind: string) => {
        asked.push(kind);

        return Promise.resolve(config(kind));
      },
      saveConfig: () => Promise.resolve(config('labels')),
      fetchPlan: () => Promise.resolve({ plan: null }),
      approvePlan: () => Promise.reject(new Error('not in this test')),
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
      readOnly: false,
      section: 'files',
      sectionHref: (page: SyncPage) => `/i/acme/sync/${page.section}`,
      fetchConfig: (_id: string, kind: string) => {
        asked.push(kind);

        return Promise.resolve(config(kind));
      },
      saveConfig: () => Promise.resolve(config('labels')),
      fetchPlan: () => Promise.resolve({ plan: null }),
      approvePlan: () => Promise.reject(new Error('not in this test')),
    });

    await screen.findByRole('heading', { name: 'Shared files' });

    expect(asked).toContain('files');
  });

  /**
   * Sync is six subjects, and stacking them made one page nobody could link to
   * a part of. Every section is an address now, so the strip is real links -
   * and the page draws only the one the address names.
   */
  it('draws every section as an address and renders only the one named', async () => {
    mount('labels', config('labels'), config('settings'));

    const strip = await screen.findByRole('navigation', { name: 'Sync sections' });
    const links = within(strip).getAllByRole('link');

    expect(links.map((link) => link.textContent?.trim())).toEqual([
      'Overview',
      'Labels',
      'Settings',
      'Rulesets',
      'Files',
      'Plan',
    ]);
    expect(links[1].getAttribute('aria-current')).toBe('page');
    expect(links[3].getAttribute('href')).toBe('/i/acme/sync/rulesets');

    // Another address entirely, so it is not on this page.
    expect(screen.queryByRole('heading', { name: 'Repository settings' })).toBeNull();
  });

  /**
   * The overview's instrument: one tile per repository, counted from the plan.
   *
   * A repository with no action in a computed plan is in step, which is what a
   * plan means - and the tile says which it is in a word, because colour is one
   * channel and this is read by people who cannot separate all of it.
   */
  it('draws the fleet as a board, with each repository named and stated', async () => {
    mountOverview(
      [repository('smyklot'), repository('platform-infra'), repository('archived', false)],
      [action('smyklot'), action('platform-infra'), action('platform-infra', { kind: 'settings' })],
    );

    const board = await screen.findByRole('group', { name: 'Repositories in this installation' });
    // Links, not buttons. A tile is an address, and it was navigated with
    // `window.location.assign` - which throws the whole application away and
    // loads it again to reach a page the client router could have drawn.
    const tiles = within(board).getAllByRole('link');

    expect(tiles).toHaveLength(3);
    expect(tiles.map((tile) => tile.getAttribute('aria-label'))).toEqual([
      'smyklot - 1 change waiting',
      'platform-infra - 2 changes waiting',
      'archived - not watched here',
    ]);
    expect(tiles.map((tile) => tile.getAttribute('href'))).toEqual([
      '/i/acme/repositories/smyklot',
      '/i/acme/repositories/platform-infra',
      '/i/acme/repositories/archived',
    ]);
  });

  /**
   * The counts come from the plan rather than from the tiles, because a plan can
   * name a repository the board's page does not hold - which is how the page
   * came to say five changes over two repositories beside a list of three.
   */
  it('counts what is out of step from the plan, against the whole population', async () => {
    mountOverview([repository('smyklot')], [action('smyklot'), action('elsewhere')], 29);

    await waitFor(() => expect(screen.getByText(/out of step/u).textContent).toContain('2 of 29'));
    expect(screen.getByText(/2 changes across 2 repositories/u)).toBeTruthy();
  });

  /** A refusal is read on its row, never only in a drill-down. */
  it('says on the row why a repository was refused', async () => {
    const because = 'the app is not an administrator of this repository';
    mountOverview([repository('design-tokens')], [action('design-tokens', { error: because })]);

    await screen.findByText(because);
    expect(screen.getByText('refused')).toBeTruthy();
  });

  /**
   * Without a plan nothing has been compared, so no tile can claim to be in
   * step. The board waits rather than drawing a fleet of ticks it has not
   * checked.
   */
  it('draws no board until a plan has been worked out', async () => {
    render(SyncView, {
      targetId: 'target-1',
      readOnly: false,
      section: 'overview',
      sectionHref: (page: SyncPage) => `/i/acme/sync/${page.section}`,
      fetchRepositories: () =>
        Promise.resolve({ items: [repository('smyklot')], next_cursor: null, total: 1 }),
      fetchConfig: (_id: string, kind: string) => Promise.resolve(config(kind)),
      saveConfig: () => Promise.resolve(config('labels')),
      fetchPlan: () => Promise.resolve({ plan: null }),
      approvePlan: () => Promise.reject(new Error('not in this test')),
    });

    await screen.findByText(/Nothing has been worked out yet/u);

    expect(screen.queryByRole('group', { name: 'Repositories in this installation' })).toBeNull();
  });

  /** A cap the board cannot draw is a cap it says out loud. */
  it('says how many repositories are not on the board', async () => {
    mountOverview([repository('smyklot')], [action('smyklot')], 240);

    await screen.findByText(/239 more repositories are not on the board/u);
  });

  /** Nobody asked for the kind, so nothing is waiting on the permission. */
  it('says nothing of a permission while the kind is switched off', async () => {
    mount('settings', config('labels'), config('settings', { unavailable: MISSING }));

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
    mount('labels', config('labels', { enabled: true, unavailable: because }), config('settings'));

    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('issues'));
  });
});
