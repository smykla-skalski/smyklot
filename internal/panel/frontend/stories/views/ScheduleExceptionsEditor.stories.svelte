<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import ScheduleExceptionsEditor from '#lib/components/ScheduleExceptionsEditor.svelte';
  const { Story } = defineMeta({
    title: 'Views/ScheduleExceptionsEditor',
    component: ScheduleExceptionsEditor,
    args: {
      idPrefix: 'exception-story',
      entries: [{ id: 'holiday', date: '2026-12-25', closed: true, start: '09:00', end: '17:00' }],
      onChange: fn(),
    },
  });
</script>

<Story name="Date exceptions" />
