<script lang="ts">
  import Icon from './Icon.svelte';

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
</script>

<span class="linked-control" role="group" aria-label={label}>
  {#if overridden}
    <button
      type="button"
      class="link-toggle broken"
      data-tip={brokenTip}
      aria-label={brokenTip}
      {disabled}
      onclick={onRestore}
    >
      <Icon name="link-off" size={14} strokeWidth={2} />
    </button>
  {:else}
    <span class="link-toggle" data-tip={linkedTip}>
      <Icon name="link" size={14} strokeWidth={2} />
    </span>
  {/if}
  <div class="value-seg">
    {#each options as option (option.value)}
      <button
        type="button"
        class:on={overridden && value === option.value}
        class:ghost={!overridden && inheritedValue === option.value}
        class:inh-target={overridden && inheritedValue === option.value}
        aria-pressed={overridden && value === option.value}
        {disabled}
        onclick={() => onSelect(option.value)}
      >
        <span class="seg-text">{option.label}</span>
      </button>
    {/each}
  </div>
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

  .value-seg {
    background: var(--segment-track);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: inline-flex;
    height: var(--control-height-compact);
    padding: var(--control-inset);
  }

  .value-seg button {
    align-items: center;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    padding: 0 0.75rem;
    transition: opacity var(--duration-fast) ease;
    white-space: nowrap;
  }

  .value-seg button:hover {
    color: var(--text-primary);
  }

  .value-seg button:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }

  .value-seg button:disabled {
    cursor: default;
    opacity: 0.45;
  }

  .seg-text {
    line-height: 1;
    text-box: trim-both cap alphabetic;
  }

  .value-seg button.on {
    background: var(--segment-thumb);
    box-shadow: var(--segment-shadow);
    color: var(--brand-action-text);
    font-weight: 700;
  }

  /* Dashed outline = the value arrives via inheritance rather than being set here. */
  .value-seg button.ghost {
    border-color: color-mix(in srgb, currentcolor 55%, transparent);
    border-style: dashed;
    color: var(--text-secondary);
    font-weight: 650;
  }

  /* Hovering the broken chain previews the restore: the inherited value takes on
     the exact ghost style it will have after the click, with a soft pulsing ring,
     while the current override dims and loses the corners facing the preview. */
  .link-toggle.broken:hover + .value-seg .inh-target,
  .link-toggle.broken:focus-visible + .value-seg .inh-target {
    animation: preview-pulse 1.6s ease-in-out infinite;
    border-color: color-mix(in srgb, var(--brand-action) 70%, transparent);
    border-style: dashed;
    color: var(--text-primary);
  }

  .link-toggle.broken:hover + .value-seg button.on,
  .link-toggle.broken:focus-visible + .value-seg button.on {
    opacity: 0.4;
  }

  .link-toggle.broken:hover + .value-seg .inh-target + button.on,
  .link-toggle.broken:focus-visible + .value-seg .inh-target + button.on {
    border-bottom-left-radius: 0;
    border-top-left-radius: 0;
  }

  .link-toggle.broken:hover + .value-seg button.on:has(+ .inh-target),
  .link-toggle.broken:focus-visible + .value-seg button.on:has(+ .inh-target) {
    border-bottom-right-radius: 0;
    border-top-right-radius: 0;
  }

  @keyframes preview-pulse {
    0%,
    100% {
      box-shadow: 0 0 0 0 color-mix(in srgb, var(--brand-action) 38%, transparent);
    }

    50% {
      box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand-action) 10%, transparent);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .link-toggle.broken:hover + .value-seg .inh-target,
    .link-toggle.broken:focus-visible + .value-seg .inh-target {
      animation: none;
      box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand-action) 25%, transparent);
    }
  }
</style>
