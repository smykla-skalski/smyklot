import { describe, expect, it, vi } from 'vitest';

import { PanelApiError } from '../src/lib/api';
import {
  staysInSyncDraftInstallation,
  SyncDraftScope,
  SyncDraftSet,
} from '../src/lib/sync-drafts.svelte';
import type {
  SyncConfig,
  SyncConfigBatchInput,
  SyncConfigBatchResponse,
  SyncKind,
} from '../src/lib/types';

function config(kind: SyncKind, revision = 1): SyncConfig {
  return {
    kind,
    enabled: false,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision,
    updated_by: '',
    updated_at: new Date(0).toISOString(),
    digest: `${kind}-${revision}`,
    document: {},
    unreadable: false,
    unavailable: '',
  };
}

function complete(revision = 1): SyncConfig[] {
  return (['labels', 'settings', 'rulesets', 'files'] as const).map((kind) =>
    config(kind, revision),
  );
}

describe('SyncDraftSet [Unit]', () => {
  it('saves all dirty kinds in one request with their saved revisions', async () => {
    const drafts = new SyncDraftSet('target-1');
    drafts.adopt(complete());
    drafts.stage('labels', {
      enabled: true,
      labels: [{ name: 'ci/green', color: '00ff00' }],
      allow_removal: true,
      excludes: [],
    });
    drafts.stage('settings', { enabled: true, document: { visibility: 'private' } });
    const save = vi.fn<
      (targetId: string, input: SyncConfigBatchInput) => Promise<SyncConfigBatchResponse>
    >(async () => ({
      configs: complete(2),
      checkpoint_id: 'checkpoint-1',
    }));

    await expect(drafts.save(save)).resolves.toBe(true);

    expect(save).toHaveBeenCalledOnce();
    expect(save.mock.calls[0]?.[1].changes).toEqual([
      {
        kind: 'labels',
        enabled: true,
        labels: [{ name: 'ci/green', color: '00ff00' }],
        allow_removal: true,
        excludes: [],
        expected_revision: 1,
      },
      {
        kind: 'settings',
        enabled: true,
        document: { visibility: 'private' },
        expected_revision: 1,
      },
    ]);
    expect(drafts.dirtyCount).toBe(0);
    expect(drafts.config('labels')?.revision).toBe(2);
    expect(drafts.notice).toContain('Reconciliation creates a plan');
    expect(drafts.refresh).toBe(1);
  });

  it('does not dirty a kind whose draft returns to the saved state', () => {
    const drafts = new SyncDraftSet('target-1');
    drafts.adopt(complete());

    drafts.stage('files', { enabled: true, document: { files: [] } });
    drafts.stage('files', { enabled: false, document: {} });

    expect(drafts.dirty).toBe(false);
  });

  it('discards every kind together', () => {
    const drafts = new SyncDraftSet('target-1');
    drafts.adopt(complete());
    drafts.stage('labels', {
      enabled: true,
      labels: [],
      allow_removal: false,
      excludes: [],
    });
    drafts.stage('rulesets', { enabled: true, document: { rulesets: [] } });

    drafts.discard();

    expect(drafts.dirtyCount).toBe(0);
    expect(drafts.config('labels')?.enabled).toBe(false);
    expect(drafts.config('rulesets')?.enabled).toBe(false);
  });

  it('preserves drafts and identifies a revision conflict', async () => {
    const drafts = new SyncDraftSet('target-1');
    drafts.adopt(complete());
    drafts.stage('settings', { enabled: true, document: { visibility: 'private' } });

    await expect(
      drafts.save(async () => {
        throw new PanelApiError(409, 'conflict', 'settings changed in another session');
      }),
    ).resolves.toBe(false);

    expect(drafts.dirtyKinds).toEqual(['settings']);
    expect(drafts.conflict).toBe(true);
    expect(drafts.problem).toContain('another session');
  });

  it('links validation failures to the kind named by the server', async () => {
    const drafts = new SyncDraftSet('target-1');
    drafts.adopt(complete());
    drafts.stage('labels', {
      enabled: true,
      labels: [{ name: '', color: '00ff00' }],
      allow_removal: false,
      excludes: [],
    });

    await drafts.save(async () => {
      throw new PanelApiError(400, 'invalid_sync_config', 'a label name is required', 'labels');
    });

    expect(drafts.invalidKind).toBe('labels');
    expect(drafts.dirtyKinds).toEqual(['labels']);
  });
});

describe('SyncDraftScope [Unit]', () => {
  it('guards only departures from the current installation route tree', () => {
    expect(staysInSyncDraftInstallation('/i/[account]/sync', 'Acme', 'acme')).toBe(true);
    expect(staysInSyncDraftInstallation('/i/[account]/[view]', 'other', 'acme')).toBe(false);
    expect(staysInSyncDraftInstallation('/root/installations', 'acme', 'acme')).toBe(false);
    expect(staysInSyncDraftInstallation(null, undefined, 'acme')).toBe(false);
  });

  it('keeps one draft while routed pages stay in the same installation', () => {
    const scope = new SyncDraftScope();
    const firstPage = scope.forTarget('target-1');
    firstPage.adopt(complete());
    firstPage.stage('settings', { enabled: true, document: { visibility: 'private' } });

    const nextPage = scope.forTarget('target-1');

    expect(nextPage).toBe(firstPage);
    expect(nextPage.dirtyKinds).toEqual(['settings']);
  });

  it('does not carry a draft into a different installation', () => {
    const scope = new SyncDraftScope();
    const first = scope.forTarget('target-1');
    first.adopt(complete());
    first.stage('settings', { enabled: true, document: { visibility: 'private' } });

    const second = scope.forTarget('target-2');

    expect(second).not.toBe(first);
    expect(second.targetId).toBe('target-2');
    expect(second.dirty).toBe(false);
  });
});
