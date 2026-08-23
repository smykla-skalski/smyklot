// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import SyncCheckpointDialog, {
  syncCheckpointSummary,
} from '../src/lib/components/SyncCheckpointDialog.svelte';
import type {
  SyncConfigCheckpoint,
  SyncConfigCheckpointState,
  SyncConfigRestoreInput,
} from '../src/lib/types';

const actor = {
  id: 'account-1',
  provider: 'github',
  subject_id: '1',
  login: 'bart',
  display_name: 'Bart Smykla',
  avatar_url: null,
};

function state(
  document: Record<string, unknown>,
  revision: number,
  enabled = true,
): SyncConfigCheckpointState {
  return { enabled, document, revision, digest: `digest-${revision}` };
}

function checkpoint(): SyncConfigCheckpoint {
  return {
    id: 'checkpoint-1',
    action: 'sync.config.saved',
    actor,
    created_at: '2026-08-23T08:00:00Z',
    affected_kinds: ['labels', 'files'],
    kinds: [
      {
        kind: 'labels',
        before: state({ labels: [], allow_removal: false, excludes: [] }, 2),
        after: state(
          {
            labels: [{ name: 'ci/green', color: '00ff00' }],
            allow_removal: true,
            excludes: ['manual-*'],
          },
          3,
        ),
        current: state({ labels: [], allow_removal: false, excludes: [] }, 8),
        changed: true,
        differs_from_current: true,
      },
      {
        kind: 'settings',
        before: state({ visibility: 'private' }, 2),
        after: state({ visibility: 'private' }, 2),
        current: state({ visibility: 'private' }, 5),
        changed: false,
        differs_from_current: false,
      },
      {
        kind: 'rulesets',
        before: null,
        after: null,
        current: null,
        changed: false,
        differs_from_current: false,
      },
      {
        kind: 'files',
        before: null,
        after: state({ files: [{ path: 'renovate.json', content: '{}' }], retired: [] }, 1),
        current: null,
        changed: true,
        differs_from_current: true,
      },
    ],
  };
}

function mount(
  over: {
    readOnly?: boolean;
    hasUnsavedDrafts?: boolean;
    restore?: (input: SyncConfigRestoreInput) => Promise<{ configs: []; checkpoint_id: string }>;
  } = {},
) {
  const restore = vi.fn(
    async (_targetId: string, _checkpointId: string, input: SyncConfigRestoreInput) =>
      over.restore?.(input) ?? { configs: [] as [], checkpoint_id: 'checkpoint-2' },
  );
  const onRestored = vi.fn();
  const onClose = vi.fn();
  render(SyncCheckpointDialog, {
    target: document.querySelector('.app-shell') as HTMLElement,
    props: {
      open: true,
      targetId: 'target-1',
      checkpointId: 'checkpoint-1',
      readOnly: over.readOnly ?? false,
      hasUnsavedDrafts: over.hasUnsavedDrafts ?? false,
      returnFocus: null,
      fetchCheckpoint: async () => checkpoint(),
      restoreCheckpoint: restore,
      onRestored,
      onClose,
    },
  });
  return { restore, onRestored, onClose };
}

describe('SyncCheckpointDialog [Component]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  it('summarises each kind without a generic diff engine', () => {
    expect(
      syncCheckpointSummary(
        'labels',
        state(
          {
            labels: [{ name: 'one' }, { name: 'two' }],
            allow_removal: true,
            excludes: ['manual-*'],
          },
          1,
        ),
      ),
    ).toBe('On · 2 labels · removal allowed · 1 exclusion');
    expect(syncCheckpointSummary('settings', state({ visibility: 'private' }, 1))).toBe(
      'On · 1 managed setting',
    );
    expect(syncCheckpointSummary('rulesets', null)).toBe('Not configured');
    expect(
      syncCheckpointSummary(
        'files',
        state({ files: [{ path: 'one' }], retired: ['old', 'older'] }, 1, false),
      ),
    ).toBe('Off · 1 shared file · 2 retired');
  });

  it('preselects only differing kinds and exposes formatted raw state', async () => {
    mount();

    await screen.findByRole('dialog', { name: 'Sync configuration history' });
    await waitFor(() => expect(screen.getByText('Saved by Bart Smykla')).toBeTruthy());
    expect(screen.getByRole('checkbox', { name: /Labels/ })).toHaveProperty('checked', true);
    expect(screen.getByRole('checkbox', { name: /Files/ })).toHaveProperty('checked', true);
    expect(screen.getByRole('checkbox', { name: /Settings/ })).toHaveProperty('checked', false);
    expect(screen.getByRole('checkbox', { name: /Settings/ })).toHaveProperty('disabled', true);
    const rawStateButtons = screen.getAllByRole('button', { name: 'View stored state' });
    expect(rawStateButtons).toHaveLength(4);
    await fireEvent.click(rawStateButtons[0] as HTMLElement);
    expect(screen.getByText(/"before":/).textContent).toContain('"current"');
  });

  it('restores the selected kinds only after confirmation', async () => {
    const { restore, onRestored, onClose } = mount();
    await screen.findByText('Saved by Bart Smykla');

    await fireEvent.click(screen.getByRole('button', { name: 'Restore selected' }));
    expect(restore).not.toHaveBeenCalled();
    expect(screen.getByRole('alert').textContent).toContain('creates new active revisions');

    await fireEvent.click(screen.getByRole('button', { name: 'Confirm restore' }));
    await waitFor(() => expect(restore).toHaveBeenCalledOnce());

    expect(restore.mock.calls[0]?.[2]).toEqual({
      kinds: [
        { kind: 'labels', expected_revision: 8 },
        { kind: 'files', expected_revision: 0 },
      ],
    });
    expect(onRestored).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('allows inspection but blocks restore while a Sync draft exists', async () => {
    mount({ hasUnsavedDrafts: true });
    await screen.findByText('Saved by Bart Smykla');

    expect(
      screen.getByText('Save or discard the current Sync draft before restoring history.'),
    ).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Restore selected' })).toHaveProperty(
      'disabled',
      true,
    );
  });

  it('keeps inspection available without rendering Restore for read-only users', async () => {
    mount({ readOnly: true });
    await screen.findByText('Saved by Bart Smykla');

    expect(screen.getByText('You can inspect this snapshot, but cannot restore it.')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Restore selected' })).toBeNull();
  });
});
