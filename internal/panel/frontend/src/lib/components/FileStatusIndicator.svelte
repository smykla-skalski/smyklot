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

<!--
@component
What became of one file in one repository, as a mark that answers when pointed at.

The word beside it is part of the same statement rather than a separate thing, which is
why it shares the mark's tooltip: pointing at "Bypassed" and getting nothing, when the
circle beside it answers, reads as the tooltip being broken.

The mark is a button and carries the sentence as its accessible name, so the outcome is
readable without a pointer. Colour is never the only channel - the glyph differs per
status as well, because four coloured circles are four circles to somebody who cannot
tell them apart.
-->

<span class="file-indicator status-{status}">
  <AppTooltip {id} text={message} align="start">
    {#snippet children(props)}
      <button {...props} type="button" class="symbol" aria-label={label}>
        <Icon name={icon} size="sm" />
        {#if showLabel}
          <span class="status-label band-trim"
            >{status.slice(0, 1).toUpperCase() + status.slice(1)}</span
          >
        {/if}
      </button>
    {/snippet}
  </AppTooltip>
</span>

<style>
  .file-indicator {
    align-items: center;
    color: var(--text-muted);
    display: inline-flex;
    gap: var(--space-2);
    height: 1.125rem;
  }

  /* Trimmed to the cap band, so the 18px slot centres against the word's ink.
     Untrimmed, "Bypassed" and "Missing" ride visibly high of their glyph. */
  .status-label {
    color: currentColor;
    font: 600 var(--font-size-meta) / var(--leading-flat) var(--sans);
  }

  .status-valid {
    color: var(--success);
  }

  .status-invalid {
    color: var(--danger);
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
      background-color var(--duration-press) var(--ease-standard),
      color var(--duration-press) var(--ease-standard);
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
