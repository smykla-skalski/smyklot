<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { resolveQueueTimeFixture } from '../support/queue-time.js';
  import { fn } from 'storybook/test';

  import QueueActionDialog from '#lib/components/QueueActionDialog.svelte';
  import type { QueueActionInput, QueueItem } from '#lib/types.js';

  const item: QueueItem = {
    id: 'sync-plan:42',
    kind: 'sync_apply',
    lane: 'maintenance',
    title: 'Apply organization sync plan',
    state: 'scheduled',
    priority: 'high',
    priority_overridden: false,
    window_mode: 'respect',
    immediate: false,
    profile_id: 'europe-hours',
    not_before: '2026-08-24T13:00:00Z',
    eligible_at: '2026-08-24T13:00:00Z',
    work_ahead: 2,
    progress_current: 0,
    progress_total: 3,
    attempt: 0,
    revision: 4,
    created_at: '2026-08-24T12:00:00Z',
    updated_at: '2026-08-24T12:00:00Z',
  };

  const { Story } = defineMeta({
    title: 'Views/QueueActionDialog',
    component: QueueActionDialog,
    args: {
      item,
      action: 'run_now',
      busy: false,
      error: '',
      onClose: fn(),
      onResolveTime: resolveQueueTimeFixture,
      onPreview: fn(async (input: QueueActionInput) => ({
        item_revision: 4,
        requested_at: input.at!,
        eligible_at: input.at!,
        outside_window: input.outside_window ?? false,
        profile_name: 'Always open',
        profile_timezone: 'UTC',
      })),
      onSubmit: fn(),
    },
  });
</script>

<Story name="Run now" />
<Story name="Schedule exact time" args={{ action: 'schedule_at' }} />
