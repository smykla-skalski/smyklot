<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SegmentedControl from '#lib/components/SegmentedControl.svelte';

  const VIEWS = [
    { value: 'audit', label: 'Audit' },
    { value: 'failures', label: 'Failures' },
  ];

  const { Story } = defineMeta({
    title: 'Primitives/SegmentedControl',
    component: SegmentedControl,
    argTypes: {
      compact: { control: 'boolean' },
      disabled: { control: 'boolean' },
      align: { control: 'inline-radio', options: ['start', 'end'] },
      variant: { control: 'inline-radio', options: ['default', 'navigation'] },
      surface: { control: 'inline-radio', options: ['panel', 'sidebar', 'night'] },
    },
    args: {
      name: 'story-history',
      label: 'History type',
      options: VIEWS,
      value: 'audit',
      compact: false,
      disabled: false,
      onSelect: fn(),
    },
  });
</script>

<Story name="Playground" />

<!-- `null` selects nothing, and the thumb does not render. -->
<Story name="Nothing selected" args={{ value: null }} />

<Story name="Compact" args={{ compact: true }} />

<!--
  A badge counts what the option leads to; a detail says where the value lands. Both
  render after the label, and the label stays the accessible name.
-->
<Story
  name="Badges and details"
  args={{
    name: 'story-badges',
    label: 'Repository state',
    value: 'enabled',
    options: [
      { value: 'enabled', label: 'Enabled', tone: 'on', badge: 12 },
      { value: 'disabled', label: 'Disabled', tone: 'off', badge: 3 },
      {
        value: 'inherit',
        label: 'Inherit',
        outline: true,
        detail: { text: 'Enabled', tone: 'on' },
      },
    ],
  }}
/>

<!--
  A dashed boundary says the value is where the control lands without a choice being
  made here, rather than something chosen.
-->
<Story
  name="Inherited option"
  args={{
    name: 'story-inherit',
    label: 'Enablement',
    value: 'inherit',
    options: [
      { value: 'inherit', label: 'Inherit', outline: true },
      { value: 'enabled', label: 'Enabled', tone: 'on' },
      { value: 'disabled', label: 'Disabled', tone: 'off' },
    ],
  }}
/>

<Story name="Disabled" args={{ disabled: true }} />
