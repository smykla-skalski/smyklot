<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import ScheduleTimezoneField from '#lib/components/ScheduleTimezoneField.svelte';
  import { stubApi, previewScheduleTimezoneFixture } from '../support/api.js';
  const { Story } = defineMeta({
    title: 'Views/ScheduleTimezoneField',
    component: ScheduleTimezoneField,
    args: {
      id: 'timezone-story',
      value: 'UTC',
      api: stubApi({ previewScheduleTimezone: previewScheduleTimezoneFixture }),
      onValidityChange: fn(),
    },
  });
</script>

<Story name="Timezone" />
