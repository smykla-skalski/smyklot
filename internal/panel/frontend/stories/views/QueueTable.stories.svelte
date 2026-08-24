<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import QueueTable from '#lib/components/QueueTable.svelte';
  import type { QueueItem } from '#lib/types.js';

  const now = '2026-08-24T12:00:00Z';
  const items: QueueItem[] = [
    {
      id: 'sync-plan:42',
      kind: 'sync_apply',
      lane: 'maintenance',
      target_id: 'github:installation:42',
      title: 'Apply organization sync plan',
      summary: '3 changes approved',
      state: 'scheduled',
      priority: 'high',
      priority_overridden: false,
      window_mode: 'respect',
      immediate: false,
      profile_id: 'europe-hours',
      profile_name: 'Europe business hours',
      profile_timezone: 'Europe/Warsaw',
      not_before: '2026-08-24T13:00:00Z',
      eligible_at: '2026-08-24T13:00:00Z',
      estimated_start_at: '2026-08-24T13:04:00Z',
      work_ahead: 2,
      progress_current: 0,
      progress_total: 3,
      attempt: 0,
      revision: 4,
      created_at: now,
      updated_at: now,
      actions: ['run_now', 'next_window', 'schedule_at', 'set_priority'],
    },
  ];

  const { Story } = defineMeta({
    title: 'Views/QueueTable',
    component: QueueTable,
    args: { items, clock: () => Date.parse(now), onOpen: fn(), onAction: fn() },
  });
</script>

<Story name="Scheduled work" />
<Story name="Empty" args={{ items: [] }} />
