<script lang="ts">
  import type { TimeDisplay } from '../preferences';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';
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

  function select(value: string): void {
    if (value === 'relative' || value === 'absolute') onSelect(value);
  }
</script>

<Popover align="end" role="dialog" label="Display options">
  {#snippet trigger(attributes)}
    <button
      class="display-trigger"
      type="button"
      aria-label="Display options"
      title="Display options"
      {...attributes}
    >
      <span class="display-icon" aria-hidden="true"
        ><Icon name="sliders" size={14} strokeWidth={2} /></span
      >
      <span class="menu-chevron" aria-hidden="true"
        ><Icon name="chevron-down" size={14} strokeWidth={2} /></span
      >
    </button>
  {/snippet}

  <div class="display-body">
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
</Popover>

<style>
  .display-trigger {
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
    justify-self: end;
    padding: 0 0.9rem;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
    user-select: none;
  }

  .display-trigger:hover,
  .display-trigger[aria-expanded='true'] {
    background: var(--control-bg-hover);
  }

  /* The ink follows the ground down. Muted on the pressed fill reads 3.93:1, under AA; secondary
     holds 5.49:1, and the same pair the segmented control uses for the same reason. */
  .display-trigger:hover .display-icon,
  .display-trigger:hover .menu-chevron,
  .display-trigger:active .display-icon,
  .display-trigger:active .menu-chevron,
  .display-trigger[aria-expanded='true'] .display-icon,
  .display-trigger[aria-expanded='true'] .menu-chevron {
    color: var(--text-secondary);
  }

  .display-trigger:active {
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

  .display-trigger[aria-expanded='true'] .menu-chevron {
    transform: rotate(180deg);
  }

  /* Inside the layer, so the layer's own surface carries none of this. */
  .display-body {
    align-items: center;
    display: grid;
    gap: 0.75rem;
    grid-template-columns: minmax(0, 1fr) auto;
    padding: 0.75rem;
    width: min(22rem, calc(100vw - 2rem));
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
    .display-body {
      grid-template-columns: 1fr;
    }
  }
</style>
