<script lang="ts">
  import AppTooltip from './AppTooltip.svelte';
  import Icon from './Icon.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const {
    label,
    source,
    sourcePronoun = 'it',
    inheritedValue = null,
    inheritedLabel,
    value = null,
    options,
    disabled = false,
    fluid = false,
    onSelect,
    onRestore,
  }: {
    label: string;
    source: string;
    sourcePronoun?: 'it' | 'them';
    inheritedValue?: string | null;
    inheritedLabel: string;
    value?: string | null;
    options: readonly { value: string; label: string }[];
    disabled?: boolean;
    /** Shares the supplied row width between the available values. */
    fluid?: boolean;
    onSelect: (value: string) => void;
    onRestore: () => void;
  } = $props();

  const overridden = $derived(value !== null);
  const linkedTip = $derived(
    `Follows ${source} · currently ${inheritedLabel} · pick a value to override`,
  );
  const brokenTip = $derived(
    `Overrides ${source} · press to follow ${sourcePronoun} again · restores ${inheritedLabel}`,
  );

  /**
   * The one segmented control, told what this context adds to it: the inherited value carries a
   * dashed boundary while nothing here has been chosen, and `value` is genuinely null then, because
   * the thumb belongs to a choice made here and there has not been one.
   */
  const segments = $derived(
    options.map((option) => ({
      value: option.value,
      label: option.label,
      outline: !overridden && inheritedValue === option.value,
    })),
  );

  /** Hovering the broken chain offers the restore before it happens. */
  let offering = $state(false);
  const preview = $derived(offering && overridden ? inheritedValue : null);

  const groupName = $derived(`inherit-${label.replaceAll(/[^a-z0-9]+/giu, '-').toLowerCase()}`);
</script>

<span class:fluid class="linked-control">
  {#if overridden}
    <AppTooltip text={brokenTip}>
      {#snippet children(props)}
        <button
          {...props}
          type="button"
          class="link-toggle broken"
          aria-label={brokenTip}
          {disabled}
          onclick={onRestore}
          onpointerenter={() => (offering = true)}
          onpointerleave={() => (offering = false)}
          onfocus={() => (offering = true)}
          onblur={() => (offering = false)}
        >
          <Icon name="link-off" size={14} strokeWidth={2} />
        </button>
      {/snippet}
    </AppTooltip>
  {:else}
    <AppTooltip text={linkedTip}>
      {#snippet children(props)}
        <span {...props} class="link-toggle">
          <Icon name="link" size={14} strokeWidth={2} />
        </span>
      {/snippet}
    </AppTooltip>
  {/if}
  <SegmentedControl
    name={groupName}
    {label}
    options={segments}
    {value}
    {preview}
    {disabled}
    {fluid}
    compact
    onSelect={(selection) => onSelect(selection)}
  />
</span>

<style>
  .linked-control {
    align-items: center;
    display: inline-flex;
    gap: var(--inherit-marker-gap);
  }

  .linked-control.fluid {
    display: grid;
    grid-template-columns: var(--inherit-marker-size) minmax(0, 1fr);
    max-width: 100%;
    min-width: 0;
    width: 100%;
  }

  /* The linked chain is a passive provenance marker; only the broken chain is a
     button (restore inheritance). */
  .link-toggle {
    background: transparent;
    border: 0;
    border-radius: 6px;
    color: var(--text-muted);
    cursor: default;
    display: grid;
    flex: none;
    height: var(--inherit-marker-size);
    place-items: center;
    width: var(--inherit-marker-size);
  }

  .link-toggle.broken {
    color: var(--warning);
    cursor: pointer;
  }

  .link-toggle.broken:hover {
    background: var(--surface-inset);
  }

  .link-toggle.broken:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }

  .link-toggle:disabled {
    cursor: default;
    opacity: 0.45;
  }
</style>
