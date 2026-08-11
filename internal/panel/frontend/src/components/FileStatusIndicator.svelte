<script lang="ts">
  import { tooltip } from '../lib/tooltip';
  import type { RepositoryFileStatus } from '../lib/types';
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

<span class="file-indicator status-{status}">
  <button
    use:tooltip={{ id, text: message, align: 'start' }}
    type="button"
    class="symbol"
    aria-label={label}
    aria-describedby={id}
  >
    <Icon name={icon} size={14} />
  </button>
  {#if showLabel}
    <span class="status-label">{status.slice(0, 1).toUpperCase() + status.slice(1)}</span>
  {/if}
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

  .symbol {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--r-ctl);
    color: inherit;
    cursor: help;
    display: inline-flex;
    height: 1.125rem;
    justify-content: center;
    outline: none;
    padding: 0;
    position: relative;
    transition:
      background-color 120ms ease-out,
      color 120ms ease-out;
    width: 1.125rem;
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
