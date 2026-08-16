<script lang="ts">
  import type { RepositoryFileStatus } from '../types';
  import AppTooltip from './AppTooltip.svelte';
  import Icon, { type IconName } from './Icon.svelte';

  const {
    id,
    status,
    showLabel = false,
  }: {
    id: string;
    status: RepositoryFileStatus;
    showLabel?: boolean;
  } = $props();

  const label = $derived(`Repository file status: ${status}`);
  const icon = $derived.by<IconName>(() => {
    switch (status) {
      case 'valid':
        return 'success';
      case 'missing':
        return 'circle-dashed';
      case 'invalid':
        return 'failure';
      case 'bypassed':
        return 'circle-slash';
    }
  });
  const message = $derived.by(() => {
    switch (status) {
      case 'valid':
        return '.github/smyklot.yaml is valid';
      case 'missing':
        return '.github/smyklot.yaml is not present';
      case 'invalid':
        return '.github/smyklot.yaml is invalid';
      case 'bypassed':
        return '.github/smyklot.yaml is being ignored';
    }
  });
</script>

<!-- The word is part of the same statement as the mark, so it explains itself on
     hover too. Pointing at "Bypassed" and getting nothing, when the circle beside
     it answers, is the sort of thing a reader reads as the tooltip being broken. -->
<span class="file-indicator status-{status}">
  <AppTooltip {id} text={message} align="start">
    {#snippet children(props)}
      <button {...props} type="button" class="symbol" aria-label={label}>
        <Icon name={icon} size={14} />
        {#if showLabel}
          <span class="status-label">{status.slice(0, 1).toUpperCase() + status.slice(1)}</span>
        {/if}
      </button>
    {/snippet}
  </AppTooltip>
</span>

<style>
  .file-indicator {
    align-items: center;
    color: var(--dim);
    display: inline-flex;
    gap: var(--space-2);
    height: 1.125rem;
  }

  /* Trimmed to the cap band, so the 18px slot centres against the word's ink.
     Untrimmed, "Bypassed" and "Missing" ride visibly high of their glyph. */
  .status-label {
    color: currentColor;
    font: 600 var(--font-size-meta) / 1 var(--sans);
    text-box: trim-both cap alphabetic;
  }

  .status-valid {
    color: var(--clear);
  }

  .status-invalid {
    color: var(--stop);
  }

  .status-bypassed {
    color: var(--warning);
  }

  /* Square when it is only the mark, and as wide as the statement when the word
     is there too - the whole thing is one target, so hovering either half
     answers. */
  .symbol {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--r-ctl);
    color: inherit;
    cursor: help;
    display: inline-flex;
    gap: var(--space-2);
    height: 1.125rem;
    justify-content: center;
    min-width: 1.125rem;
    outline: none;
    padding: 0;
    position: relative;
    transition:
      background-color 120ms ease-out,
      color 120ms ease-out;
  }

  .symbol::before {
    content: '';
    inset: -0.3125rem;
    position: absolute;
  }

  .symbol:hover,
  .symbol:focus-visible {
    background: var(--interactive-hover);
  }

  @media (prefers-reduced-motion: reduce) {
    .symbol {
      transition: none;
    }
  }
</style>
