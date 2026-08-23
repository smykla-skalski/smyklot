// @vitest-environment jsdom
import { QueryClient } from '@tanstack/svelte-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { PanelApi } from '../src/lib/api';
import {
  adoptRepositorySettings,
  repositorySettingsDraftDocument,
  stageRepositorySettingsControl,
} from '../src/lib/repository-settings';
import { SettingsDraftRegistry, settingsDraftStorageKey } from '../src/lib/settings-drafts.svelte';
import {
  adoptTargetDefaults,
  stageTargetDefaultsControl,
  targetDefaultsDraftDocument,
  targetDefaultsResource,
} from '../src/lib/target-defaults-settings';
import type {
  InstallationSettingsBatchInput,
  InstallationSettingsBatchResponse,
  PanelTarget,
  RootElevation,
} from '../src/lib/types';
import { fixtureApi } from '../stories/support/api';
import { REPOSITORY_DETAIL, ROOT_INSTALLATION, ROOT_TARGET } from '../stories/support/fixtures';
import RootInstallationViewHarness from './support/RootInstallationViewHarness.svelte';

class TestResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

class MemoryStorage {
  readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
}

const scope = { type: 'installation', targetId: ROOT_INSTALLATION.id } as const;

function registry(): SettingsDraftRegistry {
  const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
  drafts.hydrate('root-viewer');
  return drafts;
}

function queryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

function writableTarget(revision = ROOT_TARGET.revision): PanelTarget {
  return {
    ...ROOT_TARGET,
    revision,
    access_source: 'elevation',
    capabilities: { ...ROOT_TARGET.capabilities, write: true },
  };
}

function activeElevation(): RootElevation {
  return {
    id: 'root-elevation-1',
    target_id: ROOT_INSTALLATION.id,
    reason: 'Review settings',
    started_at: new Date(Date.now() - 60_000).toISOString(),
    expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
  };
}

function stageTargetDraft(drafts: SettingsDraftRegistry, target: PanelTarget): void {
  adoptTargetDefaults(drafts, target);
  stageTargetDefaultsControl(
    drafts,
    target,
    {
      ...targetDefaultsDraftDocument(drafts, target),
      repository_default_enabled: !target.repository_default_enabled,
    },
    'defaults.repository_default_enabled',
  );
}

function stageRepositoryDraft(drafts: SettingsDraftRegistry): void {
  adoptRepositorySettings(drafts, ROOT_INSTALLATION.id, REPOSITORY_DETAIL);
  stageRepositorySettingsControl(
    drafts,
    ROOT_INSTALLATION.id,
    REPOSITORY_DETAIL,
    {
      ...repositorySettingsDraftDocument(drafts, ROOT_INSTALLATION.id, REPOSITORY_DETAIL),
      enabled_override: false,
    },
    `repositories.${REPOSITORY_DETAIL.repository.id}.enabled_override`,
  );
}

function viewProps(drafts: SettingsDraftRegistry, client: QueryClient, api: PanelApi) {
  return {
    drafts,
    queryClient: client,
    installation: ROOT_INSTALLATION,
    view: 'defaults' as const,
    actorLogin: 'root-user',
    historySection: 'audit' as const,
    api,
    listHref: '/root/installations',
    onList: vi.fn(),
  };
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', TestResizeObserver);
  document.body.innerHTML = '<main class="app-shell root-mode"></main>';
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('Root installation settings composer [Component]', () => {
  it('saves every installation draft once through the Root endpoint and refreshes data', async () => {
    const drafts = registry();
    const target = writableTarget();
    stageTargetDraft(drafts, target);
    stageRepositoryDraft(drafts);
    const savedTarget = targetDefaultsDraftDocument(drafts, target);
    const savedRepository = repositorySettingsDraftDocument(
      drafts,
      ROOT_INSTALLATION.id,
      REPOSITORY_DETAIL,
    );
    const saveRootInstallationSettings = vi.fn(
      async (
        targetId: string,
        input: InstallationSettingsBatchInput,
      ): Promise<InstallationSettingsBatchResponse> => {
        expect(targetId).toBe(ROOT_INSTALLATION.id);
        expect(input.target?.expected_revision).toBe(target.revision);
        expect(input.repositories).toHaveLength(1);
        return {
          checkpoint_id: 'root-checkpoint-1',
          target: { target_id: targetId, ...savedTarget, revision: target.revision + 1 },
          repositories: [
            {
              repository_id: REPOSITORY_DETAIL.repository.id,
              ...savedRepository,
              revision: REPOSITORY_DETAIL.revision + 1,
            },
          ],
        };
      },
    );
    const fetchRootTargetSettings = vi.fn(async () => target);
    const api = fixtureApi({
      fetchRootTargetSettings,
      fetchRootElevation: async () => activeElevation(),
      saveRootInstallationSettings,
    });
    const client = queryClient();
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    render(RootInstallationViewHarness, {
      props: viewProps(drafts, client, api),
    });

    expect(await screen.findByText('2 changed settings')).toBeTruthy();
    const save = screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement;
    await waitFor(() => expect(save.disabled).toBe(false));
    await fireEvent.click(save);

    await waitFor(() => expect(saveRootInstallationSettings).toHaveBeenCalledOnce());
    await waitFor(() => expect(drafts.hasDirty(scope)).toBe(false));
    await waitFor(() => expect(fetchRootTargetSettings).toHaveBeenCalledTimes(2));
    expect(screen.getByText('Settings saved')).toBeTruthy();
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['root-installations'],
    });
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['sync-plan', ROOT_INSTALLATION.id],
    });
  });

  it('keeps a read-only draft visible and discardable without write access', async () => {
    const drafts = registry();
    stageTargetDraft(drafts, ROOT_TARGET);
    const api = fixtureApi({ fetchRootTargetSettings: async () => ROOT_TARGET });
    render(RootInstallationViewHarness, {
      props: viewProps(drafts, queryClient(), api),
    });

    expect(await screen.findByText('1 changed setting')).toBeTruthy();
    const save = screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement;
    await waitFor(() => expect(save.disabled).toBe(true));
    await fireEvent.click(screen.getByRole('button', { name: 'Discard' }));

    await waitFor(() => expect(drafts.hasDirty(scope)).toBe(false));
    expect(screen.queryByText('1 changed setting')).toBeNull();
  });

  it('rebases revision conflicts without losing the draft', async () => {
    const drafts = registry();
    const target = writableTarget();
    stageTargetDraft(drafts, target);
    const wanted = targetDefaultsDraftDocument(drafts, target).repository_default_enabled;
    const concurrent = {
      ...target,
      revision: target.revision + 1,
      pending_ci_quiet_period_seconds_override: 75,
    };
    expect(adoptTargetDefaults(drafts, concurrent)).toBe(false);
    const api = fixtureApi({
      fetchRootTargetSettings: async () => concurrent,
      fetchRootElevation: async () => activeElevation(),
    });
    render(RootInstallationViewHarness, {
      props: viewProps(drafts, queryClient(), api),
    });

    expect(await screen.findByText('Your draft is still safe')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Update draft' }));

    await waitFor(() => expect(drafts.hasConflicts(scope)).toBe(false));
    expect(drafts.resource(targetDefaultsResource(target.id))?.expectedRevision).toBe(
      concurrent.revision,
    );
    expect(targetDefaultsDraftDocument(drafts, concurrent)).toMatchObject({
      repository_default_enabled: wanted,
      pending_ci_quiet_period_seconds_override: 75,
    });
    expect(screen.getByRole('button', { name: 'Save' })).toBeTruthy();
  });

  it('resolves an external-tab conflict while preserving the draft currently shown', async () => {
    const leftStorage = new MemoryStorage();
    const rightStorage = new MemoryStorage();
    const target = writableTarget();
    const drafts = new SettingsDraftRegistry({
      storage: leftStorage,
      now: () => 100,
      writerId: 'left',
    });
    const otherTab = new SettingsDraftRegistry({
      storage: rightStorage,
      now: () => 110,
      writerId: 'right',
    });
    drafts.hydrate('root-viewer');
    otherTab.hydrate('root-viewer');
    stageTargetDraft(drafts, target);
    adoptTargetDefaults(otherTab, target);
    stageTargetDefaultsControl(
      otherTab,
      target,
      {
        ...targetDefaultsDraftDocument(otherTab, target),
        pending_ci_quiet_period_seconds_override: 75,
      },
      'defaults.pending_ci_quiet_period_seconds_override',
    );
    drafts.reconcile(rightStorage.getItem(settingsDraftStorageKey('root-viewer')));
    expect(drafts.hasConflicts(scope)).toBe(true);
    const shown = targetDefaultsDraftDocument(drafts, target);
    const api = fixtureApi({
      fetchRootTargetSettings: async () => target,
      fetchRootElevation: async () => activeElevation(),
    });
    render(RootInstallationViewHarness, {
      props: viewProps(drafts, queryClient(), api),
    });

    expect(await screen.findByText('Your draft is still safe')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Update draft' }));

    await waitFor(() => expect(drafts.hasConflicts(scope)).toBe(false));
    expect(targetDefaultsDraftDocument(drafts, target)).toEqual(shown);
    expect(drafts.hasDirty(scope)).toBe(true);
    expect(screen.getByRole('button', { name: 'Save' })).toBeTruthy();
  });
});
