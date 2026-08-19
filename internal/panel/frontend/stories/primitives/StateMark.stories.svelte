<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import StateMark from '#lib/components/StateMark.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/StateMark',
    component: StateMark,
  });
</script>

<!--
  The same four words the board's tiles use, drawn for a row. In step is the
  quietest thing on it - no ground, no words - because a settled repository is
  the answer nobody came to read.
-->
<Story name="In step" args={{ state: 'settled' }} />

<Story name="Would change" args={{ state: 'change', label: '2 repositories differ' }} />

<!-- The one a reader must not miss, so it carries a glyph as well as a ground. -->
<Story name="Refused" args={{ state: 'refused', label: 'refused' }} />

<Story name="Not watched" args={{ state: 'off' }} />
