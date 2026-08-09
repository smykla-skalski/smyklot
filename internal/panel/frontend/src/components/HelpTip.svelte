<script lang="ts">
  import Icon from './Icon.svelte';

  const {
    id,
    label,
    text,
    compact = false,
    align = 'end',
  }: {
    id: string;
    label: string;
    text: string;
    compact?: boolean;
    align?: 'start' | 'end';
  } = $props();

  let trigger: HTMLButtonElement;
  let tooltip: HTMLSpanElement;
  let tooltipLeft = $state(0);
  let tooltipTop = $state(0);

  function placeTooltip(): void {
    if (!compact) return;
    const bounds = trigger.getBoundingClientRect();
    const width = Math.min(320, window.innerWidth - 32);
    const height = tooltip.getBoundingClientRect().height;
    const desiredLeft = align === 'start' ? bounds.left : bounds.right - width;
    tooltipLeft = Math.max(16, Math.min(desiredLeft, window.innerWidth - width - 16));
    const below = bounds.bottom + 6;
    tooltipTop =
      below + height <= window.innerHeight - 16 ? below : Math.max(16, bounds.top - height - 6);
  }
</script>

<span class="help-tip" class:compact class:align-start={align === 'start'}>
  <button
    bind:this={trigger}
    type="button"
    aria-label={label}
    aria-describedby={id}
    onpointerenter={placeTooltip}
    onfocus={placeTooltip}
  >
    <Icon name="info" size={17} />
  </button>
  <span
    bind:this={tooltip}
    class="tooltip"
    {id}
    role="tooltip"
    style:left={compact ? `${tooltipLeft}px` : undefined}
    style:top={compact ? `${tooltipTop}px` : undefined}>{text}</span
  >
</span>

<style>
  .help-tip {
    display: inline-flex;
    position: relative;
  }

  button {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--dim);
    cursor: help;
    display: inline-flex;
    height: var(--control-height);
    justify-content: flex-end;
    padding: 0;
    width: var(--control-height);
  }

  button:hover,
  button:focus-visible {
    color: var(--signal);
  }

  .compact button {
    height: 1.25rem;
    justify-content: center;
    width: 1.5rem;
  }

  .compact .tooltip {
    position: fixed;
    right: auto;
  }

  .tooltip {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    color: var(--text);
    font: 500 0.75rem/1.4 var(--sans);
    max-width: calc(100vw - 3rem);
    opacity: 0;
    padding: 0.5rem 0.625rem;
    pointer-events: none;
    position: absolute;
    right: 0;
    top: calc(100% + 0.4rem);
    transform: translateY(-0.2rem);
    transition:
      opacity 120ms ease-out,
      transform 120ms ease-out;
    visibility: hidden;
    white-space: normal;
    width: 20rem;
    z-index: var(--layer-popover);
  }

  .align-start .tooltip {
    left: 0;
    right: auto;
  }

  .help-tip:hover .tooltip,
  .help-tip:focus-within .tooltip {
    opacity: 1;
    transform: translateY(0);
    visibility: visible;
  }

  @media (prefers-reduced-motion: reduce) {
    .tooltip {
      transition: none;
    }
  }
</style>
