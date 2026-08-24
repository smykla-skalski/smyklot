<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import GeneralQueueView from '#lib/components/GeneralQueueView.svelte';
  import type { QueueItem } from '#lib/types.js';
  import { stubApi } from '../support/api.js';

  const now = '2026-08-24T12:00:00Z';
  const items: QueueItem[] = [
    {
      id: 'sync-plan:42',
      kind: 'sync_apply',
      lane: 'maintenance',
      target_id: 'github:installation:42',
      repository_id: 'repository:7',
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
    {
      id: 'delivery:91',
      kind: 'webhook_delivery',
      lane: 'webhook',
      title: 'Webhook: pull_request',
      state: 'running',
      priority: 'urgent',
      priority_overridden: false,
      window_mode: 'bypass',
      immediate: false,
      not_before: now,
      eligible_at: now,
      work_ahead: 0,
      progress_current: 0,
      progress_total: 0,
      attempt: 1,
      revision: 2,
      created_at: now,
      updated_at: now,
      started_at: now,
    },
  ];

  const api = stubApi({
    fetchRootQueue: async () => ({
      items,
      next_offset: 0,
      total: items.length,
      facets: {
        targets: ['github:installation:42'],
        repositories: ['repository:7'],
        profiles: ['europe-hours', 'immediate'],
        states: ['running', 'scheduled'],
        workloads: ['sync_apply', 'webhook_delivery'],
        priorities: ['high', 'urgent'],
      },
    }),
  });
  const { Story } = defineMeta({
    title: 'Views/GeneralQueueView',
    component: GeneralQueueView,
    args: { api, rootRole: 'Super Root', canControl: true },
  });
</script>

<Story name="Active queue" />
<Story
  name="Empty"
  args={{
    api: stubApi({
      fetchRootQueue: async () => ({
        items: [],
        next_offset: 0,
        total: 0,
        facets: {
          targets: [],
          repositories: [],
          profiles: [],
          states: [],
          workloads: [],
          priorities: [],
        },
      }),
    }),
  }}
/>
