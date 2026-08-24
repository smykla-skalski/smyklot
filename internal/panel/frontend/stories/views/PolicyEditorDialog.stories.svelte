<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import PolicyEditorDialog from '#lib/components/PolicyEditorDialog.svelte';
  import type { QueuePolicy, ScheduleProfile } from '#lib/types.js';

  const profile: ScheduleProfile = {
    id: 'always-open',
    name: 'Always Open',
    timezone: 'UTC',
    system: true,
    revision: 1,
    windows: Array.from({ length: 7 }, (_, weekday) => ({
      weekday,
      start_minute: 0,
      end_minute: 1440,
    })),
    exceptions: [],
  };
  const policy: QueuePolicy = {
    kind: 'sync_scan',
    enabled: true,
    cadence: 21_600_000_000_000,
    profile_id: profile.id,
    default_priority: 'normal',
    retry_delay: 300_000_000_000,
    approval_ttl: 7_200_000_000_000,
    revision: 3,
    updated_at: '2026-08-24T12:00:00Z',
  };

  const { Story } = defineMeta({
    title: 'Views/PolicyEditorDialog',
    component: PolicyEditorDialog,
    args: {
      policy,
      profiles: [profile],
      busy: false,
      error: '',
      onClose: fn(),
      onSubmit: fn(),
    },
  });
</script>

<Story name="Sync drift policy" />
