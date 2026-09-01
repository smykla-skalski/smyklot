<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import QueueDetailDialog from '#lib/components/QueueDetailDialog.svelte';
  import type { QueueDetail } from '#lib/types.js';

  const detail: QueueDetail = {
    item: {
      id: 'sync-plan:42',
      kind: 'sync_apply',
      lane: 'maintenance',
      target_id: 'github:workspace:42',
      repository_id: 'github:repository:7',
      title: 'Apply organization sync plan',
      state: 'running',
      priority: 'high',
      priority_overridden: false,
      window_mode: 'respect',
      immediate: false,
      profile_id: 'europe-hours',
      profile_name: 'Europe business hours',
      not_before: '2026-08-24T13:00:00Z',
      eligible_at: '2026-08-24T13:00:00Z',
      estimated_start_at: '2026-08-24T13:04:00Z',
      work_ahead: 2,
      progress_current: 4,
      progress_total: 12,
      attempt: 1,
      details: { create: 3, update: 7, delete: 2 },
      revision: 4,
      created_at: '2026-08-24T12:00:00Z',
      updated_at: '2026-08-24T13:02:00Z',
    },
    events: [
      {
        id: 1,
        item_id: 'sync-plan:42',
        actor: 'system',
        kind: 'created',
        state: 'scheduled',
        summary: 'Sync plan approved',
        created_at: '2026-08-24T12:00:00Z',
      },
      {
        id: 2,
        item_id: 'sync-plan:42',
        actor_id: 'account:17',
        actor: 'account:17',
        kind: 'started',
        state: 'running',
        summary: 'Applying organization sync plan',
        created_at: '2026-08-24T13:02:00Z',
      },
    ],
  };

  const { Story } = defineMeta({
    title: 'Views/QueueDetailDialog',
    component: QueueDetailDialog,
    args: { open: true, detail, loading: false, error: '', onClose: fn() },
  });
</script>

<Story name="Running sync" />
<Story name="Loading" args={{ detail: null, loading: true }} />
