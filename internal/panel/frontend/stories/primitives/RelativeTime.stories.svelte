<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import RelativeTime from '#lib/components/RelativeTime.svelte';
  import { NOW } from '../support/fixtures.js';

  const ago = (ms: number): string => new Date(NOW - ms).toISOString();
  const ahead = (ms: number): string => new Date(NOW + ms).toISOString();

  const { Story } = defineMeta({
    title: 'Primitives/RelativeTime',
    component: RelativeTime,
    argTypes: {
      exact: { control: 'boolean' },
      future: { control: 'boolean' },
    },
    args: {
      value: ago(2 * 3_600_000),
      nowMs: NOW,
      exact: false,
      future: false,
    },
  });
</script>

<!--
  How long ago, with the whole instant one press away. Never a `title`: a native
  tooltip is a hover, so it does not exist on a phone, cannot be reached from a
  keyboard, and cannot be copied - and the stamp is the thing somebody wants to
  paste into a message. The dotted underline is the affordance; the hit area is
  a pseudo-element, so the words keep their exact box inside any pill or row.
-->
<Story name="Relative" />

<!-- Already showing the whole instant, for a table whose reader chose that. -->
<Story name="Exact" args={{ exact: true }} />

<!-- An instant still ahead reads forwards. -->
<Story name="Ahead" args={{ value: ahead(4 * 60_000), future: true }} />

<!-- Old enough that the coarse reading is a date rather than a count. -->
<Story name="Long ago" args={{ value: ago(40 * 86_400_000) }} />

<!--
  A surface that has already decided the resting words - a decision reads as a
  date - and still hands the whole instant to whoever presses it.
-->
<Story name="Worded" args={{ label: '18 Aug 2026' }} />
