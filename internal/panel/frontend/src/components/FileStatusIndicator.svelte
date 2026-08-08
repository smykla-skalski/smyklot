<script lang="ts">
  import type { RepositoryFileStatus } from '../lib/types';

  const {
    id,
    status,
  }: {
    id: string;
    status: RepositoryFileStatus;
  } = $props();

  const label = $derived(`Repository file status: ${status}`);
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
    {#if status === 'valid'}
      <svg viewBox="0 0 20 20" aria-hidden="true">
        <circle cx="10" cy="10" r="7.25"></circle>
        <path d="m6.5 10.1 2.2 2.2 4.9-4.9"></path>
      </svg>
    {:else if status === 'missing'}
      <svg viewBox="0 0 20 20" aria-hidden="true">
        <path d="M6 2.75h5l3 3v11.5H6z"></path>
        <path d="M11 2.75v3h3M8 11h4"></path>
      </svg>
    {:else if status === 'invalid'}
      <svg viewBox="0 0 20 20" aria-hidden="true">
        <path d="M10 2.5 18 17H2z"></path>
        <path d="M10 7v4.25"></path>
        <circle class="solid-dot" cx="10" cy="14" r="0.8"></circle>
      </svg>
    {:else}
      <svg viewBox="0 0 20 20" aria-hidden="true">
        <path d="M2.5 10s2.8-4.5 7.5-4.5 7.5 4.5 7.5 4.5-2.8 4.5-7.5 4.5S2.5 10 2.5 10Z"></path>
        <path d="m3.5 3.5 13 13"></path>
      </svg>
    {/if}
  </button>
  <span class="tooltip" {id} role="tooltip">{message}</span>
</span>

<style>
  .file-indicator {
    color: var(--dim);
    display: inline-flex;
    position: relative;
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
    height: var(--control-height);
    justify-content: center;
    outline: none;
    padding: 0;
    transition:
      background-color 120ms ease-out,
      color 120ms ease-out;
    width: 1.75rem;
  }

  .symbol:hover,
  .symbol:focus-visible {
    background: var(--well);
    filter: brightness(1.15);
  }

  .symbol:focus-visible {
    box-shadow: inset 0 0 0 2px var(--brand);
  }

  svg {
    fill: none;
    height: 1rem;
    stroke: currentColor;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 1.5;
    width: 1rem;
  }

  .solid-dot {
    fill: currentColor;
    stroke: none;
  }

  .tooltip {
    background: var(--strip-lift);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    bottom: calc(100% + 0.4rem);
    box-shadow: 0 8px 24px var(--shadow);
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
    z-index: 30;
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
