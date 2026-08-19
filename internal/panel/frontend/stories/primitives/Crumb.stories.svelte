<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Crumb from '#lib/components/Crumb.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Crumb',
    component: Crumb,
    args: { href: '#rulesets', label: 'Rulesets' },
  });
</script>

<!--
  The way back UP from a drill-down - hierarchy, never history. The browser
  owns Back; the crumb owns Up, so it always targets the parent address. Only
  pages deeper than their section's tab strip carry one.
-->
<Story name="Playground">
  {#snippet template(args)}
    <Crumb {...args} />
  {/snippet}
</Story>

<Story name="Above a drill-down title">
  {#snippet template()}
    <div class="page">
      <Crumb href="#files" label="Files" />
      <h1>renovate.json</h1>
      <p>In 24 of 25 repositories · updated 2 days ago</p>
    </div>
  {/snippet}
</Story>

<style>
  .page {
    display: grid;
    gap: var(--space-3);
  }

  h1 {
    font-family: var(--mono);
    font-size: 1.4rem;
    letter-spacing: -0.01em;
    margin: 0;
    text-box: trim-both cap alphabetic;
  }

  p {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: 0;
  }
</style>
