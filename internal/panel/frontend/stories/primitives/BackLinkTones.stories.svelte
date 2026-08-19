<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import BackLink from '#lib/components/BackLink.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/BackLink tones',
    component: BackLink,
    args: { href: '#rulesets', label: 'Rulesets', tone: 'quiet' },
  });
</script>

<!--
  The way back UP from a drill-down - hierarchy, never history. The browser owns
  Back; this owns Up, so it always targets the parent address. Only pages deeper
  than their section's tab strip carry one.

  Two tones, side by side, because they are the same component: `label` is the
  console's, and `quiet` is the sync detail pages', which sit under a tab strip
  already carrying the emphasis. This was a second component called `Crumb`
  until it turned out to have copied the sentence about keeping the href real
  for a modified click and none of the guard that makes it true.
-->
<Story name="Playground">
  {#snippet template(args)}
    <BackLink {...args} />
  {/snippet}
</Story>

<Story name="Both tones">
  {#snippet template()}
    <div class="page">
      <BackLink href="#repositories" label="Repositories" />
      <BackLink href="#files" label="Files" tone="quiet" />
    </div>
  {/snippet}
</Story>

<Story name="Above a drill-down title">
  {#snippet template()}
    <div class="page">
      <BackLink href="#files" label="Files" tone="quiet" />
      <h1>renovate.json</h1>
      <p>In 24 of 25 repositories · updated 2 days ago</p>
    </div>
  {/snippet}
</Story>

<style>
  .page {
    display: grid;
    gap: var(--space-3);
    justify-items: start;
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
