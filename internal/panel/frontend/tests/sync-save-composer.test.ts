// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

import { PanelApiError } from '../src/lib/api';
import SyncSaveComposer from '../src/lib/components/SyncSaveComposer.svelte';
import { SyncDraftSet } from '../src/lib/sync-drafts.svelte';
import type { SyncConfig, SyncKind } from '../src/lib/types';

function config(kind: SyncKind): SyncConfig {
  return {
    kind,
    enabled: false,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 1,
    updated_by: '',
    updated_at: new Date(0).toISOString(),
    digest: kind,
    document: {},
    unreadable: false,
    unavailable: '',
  };
}

function dirtyDrafts(): SyncDraftSet {
  const drafts = new SyncDraftSet('target-1');
  drafts.adopt((['labels', 'settings', 'rulesets', 'files'] as const).map(config));
  drafts.stage('labels', {
    enabled: true,
    labels: [],
    allow_removal: false,
    excludes: [],
  });
  drafts.stage('settings', { enabled: true, document: { visibility: 'private' } });
  return drafts;
}

function mount(drafts: SyncDraftSet, onSave = vi.fn(), onReload = vi.fn()) {
  return render(SyncSaveComposer, {
    drafts,
    readOnly: false,
    onSave,
    onReload,
    sectionHref: (kind: SyncKind) => `/sync/${kind}`,
    onOpenSection: vi.fn(),
  });
}

describe('SyncSaveComposer [Component]', () => {
  it('shows one installation-wide count and discards every changed kind', async () => {
    const drafts = dirtyDrafts();
    mount(drafts);

    expect(screen.getByText('2 changed Sync sections')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Discard' }));

    expect(drafts.dirtyCount).toBe(0);
    expect(screen.queryByRole('complementary', { name: 'Sync configuration draft' })).toBeNull();
  });

  it('offers one Save action for the whole draft', async () => {
    const onSave = vi.fn();
    mount(dirtyDrafts(), onSave);

    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(onSave).toHaveBeenCalledOnce();
  });

  it('links a validation failure to the section named by the server', async () => {
    const drafts = dirtyDrafts();
    await drafts.save(async () => {
      throw new PanelApiError(400, 'invalid_sync_config', 'a label name is required', 'labels');
    });
    const open = vi.fn();
    render(SyncSaveComposer, {
      drafts,
      readOnly: false,
      onSave: vi.fn(),
      onReload: vi.fn(),
      sectionHref: (kind: SyncKind) => `/sync/${kind}`,
      onOpenSection: open,
    });

    const link = screen.getByRole('link', { name: 'Open labels' });
    expect(link.getAttribute('href')).toBe('/sync/labels');
    await fireEvent.click(link);
    expect(open).toHaveBeenCalledWith('labels');
    expect(drafts.dirtyCount).toBe(2);
  });

  it('loads current revisions before retrying a conflicted draft', async () => {
    const drafts = dirtyDrafts();
    await drafts.save(async () => {
      throw new PanelApiError(409, 'conflict', 'settings changed in another session');
    });
    const save = vi.fn();
    const reload = vi.fn();
    mount(drafts, save, reload);

    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Load latest' }));

    expect(reload).toHaveBeenCalledOnce();
    expect(save).not.toHaveBeenCalled();
    expect(drafts.dirtyCount).toBe(2);
  });
});
