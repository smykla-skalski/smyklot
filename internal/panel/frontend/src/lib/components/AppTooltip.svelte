<script lang="ts">
  import { Tooltip } from 'bits-ui';
  import type { Snippet } from 'svelte';

  const {
    id,
    text,
    align = 'center',
    side = 'top',
    mono = false,
    children,
  }: {
    id?: string;
    text: string;
    align?: 'start' | 'center' | 'end';
    side?: 'top' | 'right' | 'bottom' | 'left';
    /**
     * For a tooltip whose whole text is one identifier - a repository name, a
     * ref, a digest. The panel writes those in the mono face everywhere it
     * writes them, and a name that changes face on its way into a tooltip
     * reads as a different kind of thing.
     */
    mono?: boolean;
    children: Snippet<[Record<string, unknown>]>;
  } = $props();
</script>

<Tooltip.Provider delayDuration={250}>
  <Tooltip.Root>
    <Tooltip.Trigger>
      {#snippet child({ props })}
        {@render children(props)}
      {/snippet}
    </Tooltip.Trigger>
    <Tooltip.Portal to=".app-shell">
      <Tooltip.Content
        {id}
        class="app-tooltip-content{mono ? ' is-mono' : ''}"
        {side}
        {align}
        sideOffset={6}
        collisionPadding={8}
      >
        {text}
      </Tooltip.Content>
    </Tooltip.Portal>
  </Tooltip.Root>
</Tooltip.Provider>

<style>
  :global(.app-tooltip-content) {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    color: var(--text-secondary);
    font: 400 var(--font-size-meta) / 1.45 var(--sans);
    letter-spacing: normal;
    max-width: min(17rem, calc(100vw - 3rem));
    padding: 0.625rem 0.75rem;
    pointer-events: none;
    text-align: left;
    text-transform: none;
    white-space: normal;
    width: max-content;
    z-index: var(--layer-tooltip);
  }

  /* An identifier breaks anywhere, because a repository name has no words in
     it to break between and the alternative is a tooltip as wide as the page. */
  :global(.app-tooltip-content.is-mono) {
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    overflow-wrap: anywhere;
  }
</style>
