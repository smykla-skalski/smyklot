<script lang="ts">
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
        return 'minus-circle';
      case 'invalid':
        return 'failure';
      case 'bypassed':
        return 'shield-slash';
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
  <button type="button" class="symbol" aria-label={label} aria-describedby={id}>
    <Icon name={icon} size={18} />
  </button>
  {#if showLabel}
    <span class="status-label">{status.slice(0, 1).toUpperCase() + status.slice(1)}</span>
  {/if}
  <span class="tooltip" {id} role="tooltip">{message}</span>
</span>

<style>
  .file-indicator {
    align-items: center;
    color: var(--dim);
    display: inline-flex;
    gap: var(--space-2);
    height: var(--control-height-compact);
    position: relative;
  }

  .status-label {
    align-items: center;
    color: currentColor;
    display: inline-flex;
    font-size: var(--font-size-meta);
    font-weight: 500;
    height: 100%;
    line-height: 1;
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
    height: 1.875rem;
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

  .tooltip {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    bottom: calc(100% + 0.4rem);
    box-shadow: var(--shadow-popover);
    color: var(--text);
    font: 500 0.75rem/1.4 var(--sans);
    max-width: calc(100vw - 3rem);
    opacity: 0;
    padding: 0.5rem 0.625rem;
    pointer-events: none;
    position: absolute;
    right: 0;
    transform: translateY(0.2rem);
    transition:
      opacity 120ms ease-out,
      transform 120ms ease-out;
    visibility: hidden;
    white-space: normal;
    width: max-content;
    z-index: var(--layer-popover);
  }

  .file-indicator:hover .tooltip,
  .file-indicator:focus-within .tooltip {
    opacity: 1;
    transform: translateY(0);
    visibility: visible;
  }

  @media (prefers-reduced-motion: reduce) {
    .symbol,
    .tooltip {
      transition: none;
    }
  }
</style>
