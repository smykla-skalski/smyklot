<script lang="ts">
  import type { TimeDisplay } from '../lib/preferences';
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
    <span class="display-icon" aria-hidden="true">
      <span></span>
      <span></span>
      <span></span>
    </span>
    <span class="menu-chevron" aria-hidden="true"></span>
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
    align-items: center;
    background: var(--control-surface);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    cursor: pointer;
    display: flex;
    gap: 0.4rem;
    height: var(--history-control-height, var(--control-height));
    justify-content: center;
    padding: 0 0.5rem;
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
    background: var(--strip-lift);
  }

  .display-icon {
    display: grid;
    gap: 0.18rem;
    width: 0.8rem;
  }

  .display-icon > span {
    background: var(--dim);
    height: 1px;
    position: relative;
  }

  .display-icon > span::after {
    background: var(--control-surface);
    border: 1px solid var(--dim);
    border-radius: 50%;
    content: '';
    height: 0.22rem;
    position: absolute;
    top: 50%;
    transform: translateY(-50%);
    width: 0.22rem;
  }

  .display-icon > span:nth-child(1)::after,
  .display-icon > span:nth-child(3)::after {
    left: 0.12rem;
  }

  .display-icon > span:nth-child(2)::after {
    right: 0.12rem;
  }

  .menu-chevron {
    border-bottom: 1.5px solid var(--dim);
    border-right: 1.5px solid var(--dim);
    height: 0.38rem;
    margin: 0 0.1rem 0.2rem 0.15rem;
    transform: rotate(45deg);
    transition: transform 120ms ease-out;
    width: 0.38rem;
  }

  .display-menu[open] .menu-chevron {
    margin-bottom: -0.2rem;
    transform: rotate(225deg);
  }

  .display-popover {
    align-items: center;
    background: var(--strip);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    box-shadow: 0 8px 24px var(--shadow);
    display: grid;
    gap: 0.75rem;
    grid-template-columns: minmax(0, 1fr) auto;
    padding: 0.75rem;
    position: absolute;
    right: 0;
    top: calc(100% + 0.35rem);
    width: min(22rem, calc(100vw - 2rem));
    z-index: 15;
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
