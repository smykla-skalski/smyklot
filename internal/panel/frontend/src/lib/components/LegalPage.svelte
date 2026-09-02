<script lang="ts">
  import type { PanelBuild } from '../base';
  import NightPage from './NightPage.svelte';

  const {
    build,
    title,
    updated,
    children,
  }: {
    build: PanelBuild;
    title: string;
    /** The day this text last changed, as a policy has to say. */
    updated: string;
    children: import('svelte').Snippet;
  } = $props();
</script>

<!--
@component
The shell the privacy notice and the terms stand on.

The same night page as the sign-in card, an invitation and the error pages,
because these are read by somebody with no session - usually straight off the
front door, which is the only place that links them. The default width rather
than the sign-in card's compact: this is prose to be read rather than one thing
to do, and the card's 26rem column makes a short policy look longer than it is.

One component for both because they are the same page with different words,
and the only thing a reader can do on either is go back.
-->

<NightPage {title} documentTitle={title} {build}>
  <div class="legal">
    <p class="legal-updated">Last updated {updated}</p>
    {@render children()}
  </div>
</NightPage>

<style>
  .legal {
    color: var(--text-secondary);
    display: grid;
    gap: var(--space-4);
  }

  .legal-updated {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: 0;
  }

  .legal :global(h2) {
    color: var(--text-primary);
    font: 650 1.0625rem / var(--leading-body) var(--sans);
    margin: var(--space-2) 0 0;
  }

  .legal :global(p) {
    line-height: var(--leading-body);
    margin: 0;
    text-wrap: pretty;
  }

  .legal :global(ul) {
    display: grid;
    gap: var(--space-2);
    line-height: var(--leading-body);
    margin: 0;
    padding-inline-start: var(--space-5);
  }

  .legal :global(a) {
    color: var(--brand-action-text);
  }
</style>
