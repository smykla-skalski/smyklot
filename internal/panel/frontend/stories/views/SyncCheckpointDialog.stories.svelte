<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncCheckpointDialog from '#lib/components/SyncCheckpointDialog.svelte';
  import type { SyncConfigCheckpoint } from '#lib/types.js';
  import { ACCOUNT } from '../support/fixtures.js';

  const checkpoint: SyncConfigCheckpoint = {
    id: 'checkpoint-1',
    action: 'sync.config.saved',
    actor: ACCOUNT,
    created_at: '2026-08-23T08:00:00Z',
    affected_kinds: ['labels', 'settings'],
    kinds: [
      {
        kind: 'labels',
        before: {
          enabled: true,
          document: { labels: [], allow_removal: false, excludes: [] },
          digest: 'before-labels',
          revision: 2,
        },
        after: {
          enabled: true,
          document: {
            labels: [{ name: 'ci/green', color: '00ff00' }],
            allow_removal: true,
            excludes: ['manual-*'],
          },
          digest: 'after-labels',
          revision: 3,
        },
        current: {
          enabled: true,
          document: { labels: [], allow_removal: false, excludes: [] },
          digest: 'current-labels',
          revision: 8,
        },
        changed: true,
        differs_from_current: true,
      },
      {
        kind: 'settings',
        before: null,
        after: {
          enabled: true,
          document: { visibility: 'private' },
          digest: 'after-settings',
          revision: 1,
        },
        current: {
          enabled: true,
          document: { visibility: 'private' },
          digest: 'after-settings',
          revision: 1,
        },
        changed: true,
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
        after: null,
        current: null,
        changed: false,
        differs_from_current: false,
      },
    ],
  };

  const base = {
    open: true,
    targetId: 'target-1',
    checkpointId: checkpoint.id,
    readOnly: false,
    hasUnsavedDrafts: false,
    fetchCheckpoint: async () => checkpoint,
    restoreCheckpoint: async () => ({ configs: [], checkpoint_id: 'checkpoint-2' }),
    onRestored: fn(),
    onClose: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/SyncCheckpointDialog',
    component: SyncCheckpointDialog,
    args: base,
  });
</script>

<Story name="Restore available" />
<Story name="Unsaved draft" args={{ hasUnsavedDrafts: true }} />
<Story name="Read only" args={{ readOnly: true }} />
