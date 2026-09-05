import { describe, expect, it } from 'vitest';
import { queueSeeds, syncPlanSeed } from '../dev/fixtures';
import { repositorySyncHealth, syncIssues, syncPermissionsHref } from '../src/lib/sync-health';
import type { SyncRepositoryStatus, SyncStatus } from '../src/lib/types';

const repository = (name: string): SyncRepositoryStatus => ({
  repository: name,
  cells: {
    labels: { state: 'in_step' },
    settings: { state: 'in_step' },
    rulesets: { state: 'off' },
    files: { state: 'pending', changes: 2 },
  },
});
const plan = () => syncPlanSeed((offset) => new Date(Date.UTC(2026, 8, 5) + offset).toISOString());

describe('automatic sync health [Unit]', () => {
  it('keeps queued work and automatic retries out of human attention', () => {
    const status: SyncStatus = { checked_at: '', repositories: [repository('api')] };
    for (const state of ['approved', 'applying'] as const) {
      expect(syncIssues(status, { ...plan(), state })).toEqual([]);
    }
    expect(repositorySyncHealth(status.repositories[0]!)).toBe('syncing');
  });

  it('shows an installation permission once, even when many repositories are affected', () => {
    const rows = ['api', 'web'].map(repository);
    for (const row of rows) row.cells.files = { state: 'refused' };
    const issues = syncIssues(
      { checked_at: '', repositories: rows, unavailable: { files: 'Contents write is required' } },
      null,
    );
    expect(issues).toEqual([
      {
        id: 'kind:files',
        kind: 'files',
        permission: true,
        title: 'GitHub permission needed',
        detail: 'Contents write is required',
      },
    ]);
  });

  it('keeps issue keys unique when repository names match other issue categories', () => {
    const rows = ['files', 'queue-blocked', 'legacy-approval', 'action-api'].map(repository);
    for (const row of rows) row.cells.labels = { state: 'refused', reason: 'Invalid labels' };
    const queue = queueSeeds((offset) => new Date(offset).toISOString())[0]!;
    const pending = plan();
    const issues = syncIssues(
      { checked_at: '', repositories: rows, unavailable: { files: 'Contents write is required' } },
      {
        ...pending,
        state: 'computed',
        queue_item: { ...queue, state: 'blocked' },
        actions: [{ ...pending.actions[0]!, repository: 'api', kind: 'labels', state: 'failed' }],
      },
    );
    expect(issues).toHaveLength(8);
    expect(new Set(issues.map((issue) => issue.id)).size).toBe(issues.length);
  });

  it('keeps a finished failure visible independently of a live plan', () => {
    const row = repository('api');
    row.cells.files = { state: 'refused' };
    row.reason = 'docs is not a directory';
    expect(repositorySyncHealth(row)).toBe('blocked');
    expect(syncIssues({ checked_at: '', repositories: [row] }, null)).toMatchObject([
      { repository: 'api', kind: 'files', detail: row.reason },
    ]);
  });

  it('surfaces an unreadable configuration without requiring its editor to load', () => {
    expect(
      syncIssues(
        { checked_at: '', repositories: [], invalid: { labels: 'Invalid stored configuration' } },
        null,
      ),
    ).toMatchObject([{ kind: 'labels', title: 'Configuration needs a fix', permission: false }]);
  });

  it('keeps a repository problem distinct from a workspace permission problem', () => {
    const row = repository('api');
    row.cells.labels = { state: 'refused', reason: 'Labels permission is missing' };
    row.cells.files = { state: 'refused', reason: 'docs is not a directory' };
    row.reason = 'Labels permission is missing';
    const issues = syncIssues(
      { checked_at: '', repositories: [row], unavailable: { labels: row.reason } },
      null,
    );
    expect(issues).toHaveLength(2);
    expect(issues[1]).toMatchObject({
      repository: 'api',
      kind: 'files',
      detail: 'docs is not a directory',
    });
  });

  it('surfaces a paused queue policy with its recovery destination', () => {
    const queue = queueSeeds((offset) => new Date(offset).toISOString())[0]!;
    expect(
      syncIssues(null, {
        ...plan(),
        state: 'approved',
        queue_item: { ...queue, state: 'blocked', blocked_reason: 'Workload disabled by policy' },
      }),
    ).toMatchObject([
      {
        id: 'system:queue-blocked',
        queue: true,
        title: 'Automatic sync is paused',
        detail: 'Workload disabled by policy',
      },
    ]);
  });

  it('preserves a one-time decision for a plan created under manual approval', () => {
    expect(syncIssues(null, plan())).toMatchObject([{ id: 'system:legacy-approval' }]);
  });

  it('sends permission recovery to the installed app for the account type', () => {
    const target = {
      type: 'Organization' as const,
      installation_id: '100',
      account: { login: 'team' },
    };
    expect(syncPermissionsHref(target)).toBe(
      'https://github.com/organizations/team/settings/installations/100',
    );
    expect(syncPermissionsHref({ ...target, type: 'User' })).toBe(
      'https://github.com/settings/installations/100',
    );
  });
});
