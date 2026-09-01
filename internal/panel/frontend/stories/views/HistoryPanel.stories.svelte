<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import HistoryPanel from '#lib/components/HistoryPanel.svelte';
  import { AUDIT, FAILURES, TARGET } from '../support/fixtures.js';

  /* Both fetchers are ordinary props, so no cache is seeded - the queries resolve
     against these. Seed a cache only where the data arrives through `api`. */
  const base = {
    targetId: TARGET.id,
    fetchAudit: () => Promise.resolve({ items: AUDIT, next_cursor: null, total: AUDIT.length }),
    fetchFailures: () =>
      Promise.resolve({ items: FAILURES, next_cursor: null, total: FAILURES.length }),
    section: 'audit' as const,
    onSection: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/HistoryPanel',
    component: HistoryPanel,
    args: base,
  });
</script>

<!--
  What changed, who changed it and when. Both of this panel's tables are virtualised,
  so their rows carry an offset in `--row-y` rather than an inline transform - written
  to `style` it would be a transform nothing can extend, and the press scale would be
  silently dropped.
-->
<Story name="Audit">
  {#snippet template(args)}<HistoryPanel {...args} />{/snippet}
</Story>

<!-- The deliveries that did not arrive. Same shell, different columns. -->
<Story name="Failures">
  {#snippet template(args)}<HistoryPanel {...args} section="failures" />{/snippet}
</Story>

<!--
  The Root console's reading of the same history: the Target column names the
  workspace an entry belongs to, where a workspace's own panel already knows.
-->
<Story name="Root console">
  {#snippet template(args)}<HistoryPanel {...args} context="root" />{/snippet}
</Story>

<!-- Nothing recorded yet. -->
<Story name="Empty">
  {#snippet template(args)}
    <HistoryPanel
      {...args}
      fetchAudit={() => Promise.resolve({ items: [], next_cursor: null, total: 0 })}
    />
  {/snippet}
</Story>
