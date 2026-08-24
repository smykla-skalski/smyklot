<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import ScheduleWindowsEditor from '#lib/components/ScheduleWindowsEditor.svelte';

  const { Story } = defineMeta({
    title: 'Views/ScheduleWindowsEditor',
    component: ScheduleWindowsEditor,
    args: {
      idPrefix: 'schedule-story',
      windows: [
        { id: 'monday', weekday: 1, start: '09:00', end: '17:00' },
        { id: 'friday', weekday: 5, start: '09:00', end: '13:00' },
      ],
      onChange: fn(),
    },
  });
</script>

<Story name="Weekly windows" />
