<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import QueueActionDialog from '#lib/components/QueueActionDialog.svelte';
  import type { QueueItem } from '#lib/types.js';

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
      onPreview: fn(async () => ({
        item_revision: 4,
        requested_at: '2026-08-24T13:00:00Z',
        eligible_at: '2026-08-24T13:00:00Z',
        outside_window: false,
        profile_name: 'Europe hours',
        profile_timezone: 'Europe/Warsaw',
      })),
      onSubmit: fn(),
    },
  });
</script>

<Story name="Run now" />
<Story name="Schedule outside window" args={{ action: 'schedule_at' }} />
