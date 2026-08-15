<script lang="ts">
  import haloUrl from '../assets/smyklot-halo.svg';

  const {
    part,
    heading = false,
  }: {
    /** The line under the wordmark, naming the console. Written as it renders. */
    part: string;
    /** Whether this mark is the page's own heading, as it is in the sidebar. */
    heading?: boolean;
  } = $props();
</script>

<svelte:element this={heading ? 'h1' : 'p'} class="mark">
  <img class="mark-icon" src={haloUrl} alt="" width="34" height="34" decoding="async" />
  <span class="mark-copy">
    <span class="mark-name">Smyklot</span>
    <span class="mark-part">{part}</span>
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
</style>
