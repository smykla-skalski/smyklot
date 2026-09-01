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
import type { RepositoryDetail, RootWorkspace } from '../src/lib/types';
import { fixtureApi } from '../stories/support/api';
import { REPOSITORY_DETAIL, ROOT_WORKSPACE } from '../stories/support/fixtures';
import RootWorkspacesHarness from './support/RootWorkspacesHarness.svelte';

function workspace(id: string, login: string, displayName: string): RootWorkspace {
  return {
    ...ROOT_WORKSPACE,
    id,
    installation_id: `installation-${id}`,
    account: {
      ...ROOT_WORKSPACE.account,
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

describe('Root workspace draft attention [Component]', () => {
  it('marks only the workspace whose settings are dirty', async () => {
    const dirty = workspace('target-1', 'dirty-workspace', 'Dirty workspace');
    const clean = workspace('target-2', 'clean-workspace', 'Clean workspace');
    const detail = repositoryDetail('repository-1');
    const drafts = dirtyRegistry(dirty.id, detail);

    render(RootWorkspacesHarness, {
      props: {
        drafts,
        queryClient: queryClient(),
        route: { rootView: 'workspaces' },
        api: fixtureApi({ fetchRootWorkspaces: async () => [dirty, clean] }),
        actorLogin: 'root-user',
        listHref: '/root/workspaces',
        hrefFor: (account, view) => `/root/workspaces/${account}/${view}`,
        onList: vi.fn(),
        onNavigate: vi.fn(),
        historySection: 'audit',
      },
    });

    const dirtyRow = (await screen.findByText('Dirty workspace')).closest<HTMLElement>(
      '.object-row',
    );
    const cleanRow = screen.getByText('Clean workspace').closest<HTMLElement>('.object-row');
    expect(dirtyRow).not.toBeNull();
    expect(cleanRow).not.toBeNull();
    expect(within(dirtyRow!).getByText('1 unsaved setting')).toBeTruthy();
    expect(
      within(dirtyRow!)
        .getByRole('link', { name: 'Open as operator - Dirty workspace' })
        .getAttribute('href'),
    ).toBe('/root/workspaces/dirty-workspace/repositories');
    expect(dirtyRow?.getAttribute('data-unsaved')).toBe('true');
    expect(within(cleanRow!).queryByText(/unsaved/)).toBeNull();
    expect(cleanRow?.hasAttribute('data-unsaved')).toBe(false);
  });
});
