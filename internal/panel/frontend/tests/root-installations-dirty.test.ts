// @vitest-environment jsdom
import { QueryClient } from '@tanstack/svelte-query';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  adoptRepositorySettings,
  repositorySettingsDraftDocument,
  stageRepositorySettingsControl,
} from '../src/lib/repository-settings';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import type { RepositoryDetail, RootInstallation } from '../src/lib/types';
import { fixtureApi } from '../stories/support/api';
import { REPOSITORY_DETAIL, ROOT_INSTALLATION } from '../stories/support/fixtures';
import RootInstallationsHarness from './support/RootInstallationsHarness.svelte';

function installation(id: string, login: string, displayName: string): RootInstallation {
  return {
    ...ROOT_INSTALLATION,
    id,
    installation_id: `installation-${id}`,
    account: {
      ...ROOT_INSTALLATION.account,
      id,
      subject_id: id,
      login,
      display_name: displayName,
    },
  };
}

function repositoryDetail(id: string): RepositoryDetail {
  return {
    ...REPOSITORY_DETAIL,
    repository: {
      ...REPOSITORY_DETAIL.repository,
      id,
    },
  };
}

function dirtyRegistry(targetId: string, detail: RepositoryDetail): SettingsDraftRegistry {
  const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
  drafts.hydrate('root-user');
  adoptRepositorySettings(drafts, targetId, detail);
  const document = repositorySettingsDraftDocument(drafts, targetId, detail);
  stageRepositorySettingsControl(
    drafts,
    targetId,
    detail,
    { ...document, enabled_override: !document.enabled_override },
    `repositories.${detail.repository.id}.enabled_override`,
  );
  return drafts;
}

function queryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('Root installation draft attention [Component]', () => {
  it('marks only the installation whose settings are dirty', async () => {
    const dirty = installation('target-1', 'dirty-installation', 'Dirty installation');
    const clean = installation('target-2', 'clean-installation', 'Clean installation');
    const detail = repositoryDetail('repository-1');
    const drafts = dirtyRegistry(dirty.id, detail);

    render(RootInstallationsHarness, {
      props: {
        drafts,
        queryClient: queryClient(),
        route: { rootView: 'installations' },
        api: fixtureApi({ fetchRootInstallations: async () => [dirty, clean] }),
        rootRole: 'Root',
        actorLogin: 'root-user',
        listHref: '/root/installations',
        hrefFor: (account, view) => `/root/installations/${account}/${view}`,
        onList: vi.fn(),
        onNavigate: vi.fn(),
        historySection: 'audit',
      },
    });

    const dirtyRow = (await screen.findByText('Dirty installation')).closest('tr');
    const cleanRow = screen.getByText('Clean installation').closest('tr');
    expect(dirtyRow).not.toBeNull();
    expect(cleanRow).not.toBeNull();
    expect(within(dirtyRow!).getByText('Unsaved changes')).toBeTruthy();
    expect(
      within(dirtyRow!).getByRole('link', { name: 'Dirty installation' }).getAttribute('href'),
    ).toBe('/root/installations/dirty-installation/repositories');
    expect(dirtyRow?.getAttribute('data-unsaved')).toBe('true');
    expect(within(cleanRow!).queryByText('Unsaved changes')).toBeNull();
    expect(cleanRow?.hasAttribute('data-unsaved')).toBe(false);
  });
});
