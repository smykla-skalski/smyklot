<script lang="ts">
  import type { TimeDisplay } from '../lib/preferences';
  import Icon from './Icon.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const TIME_OPTIONS = [
    { value: 'relative', label: 'Relative' },
    { value: 'absolute', label: 'Date & time' },
  ] as const;

  const {
    value,
    onSelect,
  }: {
    value: TimeDisplay;
    onSelect: (value: TimeDisplay) => void;
  } = $props();

  let menu = $state<HTMLDetailsElement | null>(null);
  let trigger = $state<HTMLElement | null>(null);

  $effect(() => {
    function closeFromOutside(event: PointerEvent): void {
      if (menu?.open === true && event.target instanceof Node && !menu.contains(event.target)) {
        menu.open = false;
      }
    }

    function closeFromKeyboard(event: KeyboardEvent): void {
      if (event.key !== 'Escape' || menu?.open !== true) return;
      event.preventDefault();
      menu.open = false;
      trigger?.focus();
    }

    document.addEventListener('pointerdown', closeFromOutside);
    document.addEventListener('keydown', closeFromKeyboard);
    return () => {
      document.removeEventListener('pointerdown', closeFromOutside);
      document.removeEventListener('keydown', closeFromKeyboard);
    };
  });

  function select(value: string): void {
    if (value === 'relative' || value === 'absolute') onSelect(value);
  }
</script>

<details class="display-menu" bind:this={menu}>
  <summary bind:this={trigger} aria-label="Display options" title="Display options">
    <span class="display-icon" aria-hidden="true"
      ><Icon name="sliders" size={14} strokeWidth={2} /></span
    >
    <span class="menu-chevron" aria-hidden="true"
      ><Icon name="chevron-down" size={14} strokeWidth={2} /></span
    >
  </summary>

  <div class="display-popover">
    <div class="option-copy">
      <strong>Time display</strong>
      <span>Choose how event times appear</span>
    </div>
    <SegmentedControl
      name="history-time-display"
      label="Time display"
      options={TIME_OPTIONS}
      {value}
      onSelect={select}
    />
  </div>
</details>

<style>
  .display-menu {
    justify-self: end;
    position: relative;
  }

  summary {
    /* 66px wide: the ordinary figure would move its edge two thirds of a pixel, which is not a
       press. See --press-scale-compact in app.css for the bands. */
    --press-scale: var(--press-scale-compact);
    align-items: center;
    background: var(--control-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    cursor: pointer;
    display: flex;
    gap: 0.45rem;
    height: var(--local-control-height, var(--control-height));
    justify-content: center;
    padding: 0 0.9rem;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
    user-select: none;
  }

  summary::-webkit-details-marker {
    display: none;
  }

  summary::marker {
    content: '';
  }

  summary:hover,
  .display-menu[open] summary {
    background: var(--control-bg-hover);
  }

  /* The ink follows the ground down. Muted on the pressed fill reads 3.93:1, under AA; secondary
     holds 5.49:1, and the same pair the segmented control uses for the same reason. */
  summary:hover .display-icon,
  summary:hover .menu-chevron,
  summary:active .display-icon,
  summary:active .menu-chevron,
  .display-menu[open] .display-icon,
  .display-menu[open] .menu-chevron {
    color: var(--text-secondary);
  }

  summary:active {
    background: var(--control-bg-pressed);
    border-color: var(--control-border-hover);
    transform: scale(var(--press-scale));
  }

  .display-icon {
    color: var(--text-muted);
    display: grid;
    place-items: center;
  }

  .menu-chevron {
    color: var(--text-muted);
    display: grid;
    place-items: center;
    transition: transform var(--duration-fast) var(--ease-out);
  }

  .display-menu[open] .menu-chevron {
    transform: rotate(180deg);
  }

  .display-popover {
    align-items: center;
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    display: grid;
    gap: 0.75rem;
    grid-template-columns: minmax(0, 1fr) auto;
    padding: 0.75rem;
    position: absolute;
    right: 0;
    top: calc(100% + 0.35rem);
    width: min(22rem, calc(100vw - 2rem));
    z-index: var(--layer-menu);
  }

  .option-copy {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .option-copy strong {
    font-size: 0.75rem;
  }

  .option-copy span {
    color: var(--dim);
    font-size: 0.6875rem;
  }

  @media (max-width: 26rem) {
    .display-popover {
      grid-template-columns: 1fr;
    }
  }
</style>
