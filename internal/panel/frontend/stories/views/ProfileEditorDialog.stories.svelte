<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import ProfileEditorDialog from '#lib/components/ProfileEditorDialog.svelte';
  import type { ScheduleProfile } from '#lib/types.js';

  const profile: ScheduleProfile = {
    id: 'europe-hours',
    name: 'Europe business hours',
    timezone: 'Europe/Warsaw',
    system: false,
    revision: 4,
    affected_workspaces: 3,
    affected_items: 8,
    affected_policies: 2,
    windows: [1, 2, 3, 4, 5].map((weekday) => ({
      weekday,
      start_minute: 540,
      end_minute: 1020,
    })),
    exceptions: [{ date: '2026-12-25', closed: true }],
  };

  const { Story } = defineMeta({
    title: 'Views/ProfileEditorDialog',
    component: ProfileEditorDialog,
    args: {
      profile,
      open: true,
      busy: false,
      error: '',
      onClose: fn(),
      onSubmit: fn(),
    },
  });
</script>

<Story name="Edit profile" />
<Story name="New profile" args={{ profile: null }} />
