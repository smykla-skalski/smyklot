import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import { mockConfigFileInputs, mockConfigFileStatus } from '../dev/config-file-status';
import { seed } from '../dev/fixtures';
import { describe, expect, it, vi } from 'vitest';
import { QueryClient } from '@tanstack/svelte-query';
import {
  configFileStatusQuery,
  configFileStatusRevision,
  configFileStatusView,
  configFileProposalHref,
} from '../src/lib/config-file-status';
import type { ConfigFileSyncStatus } from '../src/lib/config-file-sync';

const checked: ConfigFileSyncStatus = {
  enabled: true,
  available: true,
  status: 'ready',
  last_check: {
    checked_at: '2026-09-07T12:00:00Z',
    status: 'ready',
    settings_current: true,
    path: '.smyklot.toml',
  },
};

describe('saved configuration file status [Unit]', () => {
  it('distinguishes a current pending operation from waiting for a first check', () => {
    const view = configFileStatusView(
      { ...checked, status: 'pending', last_check: { ...checked.last_check!, status: 'pending' } },
      true,
    );
    expect(view.label).toBe('Sync pending');
    expect(view.description).not.toContain('not been checked');
  });
  it('does not call a previous check current after settings change', () => {
    const view = configFileStatusView(
      {
        ...checked,
        status: 'pending',
        last_check: { ...checked.last_check!, settings_current: false },
      },
      true,
    );
    expect(view.label).toBe('Waiting for check');
    expect(view.description).toContain('saved settings');
    expect(view.proposal).toBeUndefined();
  });
  it('keeps unavailable separate from off', () => {
    expect(
      configFileStatusView({ enabled: true, available: false, status: 'off' }, true).label,
    ).toBe('Unavailable');
    expect(
      configFileStatusView({ enabled: false, available: true, status: 'off' }, false).label,
    ).toBe('Off');
  });
  it('does not reuse ready status while the saved opt-in differs', () => {
    expect(configFileStatusView(checked, false).label).toBe('Off');
    expect(
      configFileStatusView({ enabled: false, available: true, status: 'off' }, true).label,
    ).toBe('Waiting for check');
  });
  it('keeps file use separate and explains why connection cannot run', () => {
    const view = configFileStatusView(checked, true, true);
    expect(view.label).toBe('File settings off');
    expect(view.description).toContain('Use file settings');
  });
  it('shows an outstanding proposal without pretending it is a resolvable conflict', () => {
    const view = configFileStatusView(
      {
        ...checked,
        status: 'blocked',
        last_check: {
          ...checked.last_check!,
          status: 'blocked',
          problem: 'proposal_outstanding',
          proposal: { number: 42, url: 'https://github.com/acme/widget/pull/42' },
        },
      },
      true,
    );
    expect(view.label).toBe('Open pull request');
    expect(view.proposal?.number).toBe(42);
    expect(view.description).not.toContain('conflict');
  });
  it('does not expose stale blockers as the current state', () => {
    const view = configFileStatusView(
      {
        ...checked,
        status: 'pending',
        last_check: {
          ...checked.last_check!,
          status: 'blocked',
          settings_current: false,
          problem: 'invalid_file',
          message: 'Broken TOML',
          proposal: { number: 42, url: 'https://github.com/acme/widget/pull/42' },
        },
      },
      true,
    );
    expect(view.label).toBe('Waiting for check');
    expect(view.description).not.toContain('Broken');
    expect(view.proposal).toBeUndefined();
  });
  it.each([
    'javascript:alert(1)',
    'https://evil.test/acme/widget/pull/42',
    'https://github.com/acme/other/pull/42',
    'https://github.com/acme/widget/pull/42?next=bad',
  ])('rejects a proposal outside the known repository: %s', (url) => {
    expect(configFileProposalHref({ number: 42, url }, 'acme/widget')).toBeNull();
  });
  it('accepts only the actual proposal URL and number', () => {
    expect(
      configFileProposalHref(
        { number: 42, url: 'https://github.com/acme/widget/pull/42' },
        'acme/widget',
      ),
    ).toBe('https://github.com/acme/widget/pull/42');
    expect(
      configFileProposalHref(
        { number: 99, url: 'https://github.com/acme/widget/pull/42' },
        'acme/widget',
      ),
    ).toBeNull();
  });
});

describe('configuration status query identity [Unit]', () => {
  it('changes cache identity when a different saved Sync resource changes', () => {
    const registry = new SettingsDraftRegistry({ storage: null });
    registry.hydrate('viewer');
    const config = { type: 'sync-config', targetId: 'target-a', kind: 'files' } as const;
    registry.adoptBase(config, 1, {});
    const before = configFileStatusRevision(registry, 'target-a', 1);
    registry.adoptBase(config, 2, {});
    expect(configFileStatusRevision(registry, 'target-a', 1)).not.toBe(before);
    const repository = {
      type: 'sync-override',
      targetId: 'target-a',
      repositoryId: 'repo-a',
      kind: 'files',
    } as const;
    registry.adoptBase(repository, 1, {});
    const repoBefore = configFileStatusRevision(registry, 'target-a', 1, 'repo-a');
    registry.adoptBase(repository, 2, {});
    expect(configFileStatusRevision(registry, 'target-a', 1, 'repo-a')).not.toBe(repoBefore);
  });
  it('does not join the migration-reset RepositoryDetail cache family', () => {
    const client = new QueryClient();
    const detail = seed().targets[0]!.repositories[0]!.detail;
    const updated = { ...detail, config_migration: 'none' as const };
    const status = configFileStatusQuery(
      '2001',
      'panel',
      detail.repository.id,
      1,
      async () => checked,
      true,
    );
    client.setQueryData(['repository', '2001', detail.repository.id], detail);
    client.setQueryData(status.queryKey, checked);
    // The actual resetConfigMigration updater is typed to RepositoryDetail.
    // A status DTO must never be matched by this family's prefix mutation.
    expect(() =>
      client.setQueriesData<typeof detail>({ queryKey: ['repository', '2001'] }, (current) =>
        current?.repository.id === detail.repository.id || current === detail ? updated : current,
      ),
    ).not.toThrow();
    expect(client.getQueryData(status.queryKey)).toEqual(checked);
  });
  it('invalidates status across both surfaces without touching another workspace', async () => {
    const client = new QueryClient();
    const first = configFileStatusQuery(
      'target-a',
      'panel',
      undefined,
      1,
      async () => checked,
      true,
    );
    const root = configFileStatusQuery('target-a', 'root', 'repo-a', 1, async () => checked, true);
    const other = configFileStatusQuery(
      'target-b',
      'panel',
      undefined,
      1,
      async () => checked,
      true,
    );
    for (const query of [first, root, other]) client.setQueryData(query.queryKey, checked);
    await client.invalidateQueries({ queryKey: ['config-file-status', 'target-a'] });
    expect(client.getQueryState(first.queryKey)?.isInvalidated).toBe(true);
    expect(client.getQueryState(root.queryKey)?.isInvalidated).toBe(true);
    expect(client.getQueryState(other.queryKey)?.isInvalidated).toBe(false);
  });
  it('keys data by surface, owner and saved revision, without draft state', async () => {
    const read = vi.fn(async () => checked);
    const first = configFileStatusQuery('target-a', 'panel', 'repo-a', 4, read, true);
    const second = configFileStatusQuery('target-b', 'root', 'repo-b', 5, read, true);
    expect(first.queryKey).not.toEqual(second.queryKey);
    await first.queryFn();
    await second.queryFn();
    expect(read.mock.calls).toEqual([
      ['target-a', 'repo-a'],
      ['target-b', 'repo-b'],
    ]);
  });
});

describe('configuration check mock fidelity [Unit]', () => {
  it('keeps the actual check time and makes a saved revision stale without inventing a new check', () => {
    const when = '2026-09-01T12:00:00Z';
    const stale = mockConfigFileStatus('ready', 'acme/widget', true, 2, false, false, when);
    expect(stale.status).toBe('pending');
    expect(stale.last_check?.checked_at).toBe(when);
    expect(stale.last_check?.settings_current).toBe(false);
  });
  it('includes previously absent Sync kinds in the observation identity', () => {
    const state = seed();
    const before = mockConfigFileInputs(state, '2001', '4002', 1);
    state.syncOverrides.set('4002/labels', {
      kind: 'labels',
      unreadable: false,
      enabled: true,
      document: {},
      revision: 1,
    });
    expect(mockConfigFileInputs(state, '2001', '4002', 1)).not.toBe(before);
    const stale = mockConfigFileStatus(
      'ready',
      'acme/widget',
      true,
      1,
      false,
      false,
      undefined,
      false,
    );
    expect(stale.status).toBe('pending');
  });
});
