<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ScheduleDatePreview from '#lib/components/ScheduleDatePreview.svelte';
  import { stubApi } from '../support/api.js';
  const { Story } = defineMeta({
    title: 'Views/ScheduleDatePreview',
    component: ScheduleDatePreview,
    args: {
      id: 'preview-date-story',
      timezoneValid: true,
      profile: {
        name: 'Closed date',
        timezone: 'UTC',
        windows: [],
        exceptions: [{ date: '2026-12-25', closed: true }],
      },
      api: stubApi({
        previewScheduleDate: async (input) => ({
          date: input.date,
          timezone: input.profile.timezone,
          windows: [],
        }),
      }),
    },
  });
</script>

<Story name="Preview a closed date" />
