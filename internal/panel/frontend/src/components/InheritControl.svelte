<script lang="ts">
  import { tooltip } from '../lib/tooltip';
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

<span class="linked-control">
  {#if overridden}
    <button
      type="button"
      class="link-toggle broken"
      use:tooltip={{ text: brokenTip, align: 'center' }}
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
  {:else}
    <span class="link-toggle" use:tooltip={{ text: linkedTip, align: 'center' }}>
      <Icon name="link" size={14} strokeWidth={2} />
    </span>
  {/if}
  <SegmentedControl
    name={groupName}
    {label}
    options={segments}
    {value}
    {preview}
    {disabled}
    compact
    onSelect={(selection) => onSelect(selection)}
  />
</span>

<style>
  .linked-control {
    align-items: center;
    display: inline-flex;
    gap: 0.35rem;
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
    height: 26px;
    place-items: center;
    width: 26px;
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
