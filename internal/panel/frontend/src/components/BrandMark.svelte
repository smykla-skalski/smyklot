<script lang="ts">
  import haloUrl from '../assets/smyklot-halo.svg';

  const {
    part,
    heading = false,
    stacked = false,
    size = 36,
  }: {
    /**
     * The line under the wordmark, naming the console. Written as it renders.
     * Omit it where the page says the same thing elsewhere - the invitation page
     * has it in the footer, and twice on one screen is once too many.
     */
    part?: string;
    /** Whether this mark is the page's own heading, as it is in the sidebar. */
    heading?: boolean;
    /** Icon over wordmark rather than beside it, for a mark standing on its own. */
    stacked?: boolean;
    /**
     * The icon's edge in pixels, glow included. The halo fills 89.25% of it - the
     * rest is the canvas the ring's bloom spills onto - so a mark that has to
     * measure the ring rather than the box multiplies by that. The wordmark
     * scales with this when stacked.
     */
    size?: number;
  } = $props();
</script>

<svelte:element this={heading ? 'h1' : 'p'} class={['mark', stacked && 'stacked']}>
  <img class="mark-icon" src={haloUrl} alt="" width={size} height={size} decoding="async" />
  <span class="mark-copy">
    <span class="mark-name">Smyklot</span>
    {#if part !== undefined}
      <span class="mark-part">{part}</span>
    {/if}
  </span>
</svelte:element>

<style>
  .mark {
    align-items: center;
    display: flex;
    gap: 0.625rem;
    margin: 0;
    min-width: 0;
  }

  .mark-icon {
    flex: none;
    object-fit: contain;
  }

  .mark-copy {
    display: grid;
    gap: 0.3rem;
    min-width: 0;
  }

  /* The sidebar tokens are the mark's own: they are declared per theme at the
     document root, so a page with no sidebar - the invitation page - paints the
     mark in the same ink. They are not interchangeable with `--text-primary`,
     which stays dark under the Root light theme while the mark does not. */
  .mark-name {
    color: var(--sidebar-text);
    font: 700 0.8125rem / 1 var(--sans);
    letter-spacing: 0.11em;
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .mark-part {
    color: var(--sidebar-text-muted);
    font: 700 0.65625rem / 1 var(--sans);
    letter-spacing: 0.12em;
    text-box: trim-both cap alphabetic;
  }

  /* Standing on its own rather than heading a rail: the icon carries the mark and
     the words sit under it, so the type steps up to match the larger disc. */
  .mark.stacked {
    flex-direction: column;
    gap: var(--space-3);
    text-align: center;
  }

  .mark.stacked .mark-copy {
    gap: 0.45rem;
    justify-items: center;
  }

  .mark.stacked .mark-name {
    font-size: 1rem;
    letter-spacing: 0.14em;
  }

  .mark.stacked .mark-part {
    font-size: 0.75rem;
    letter-spacing: 0.15em;
  }
</style>
