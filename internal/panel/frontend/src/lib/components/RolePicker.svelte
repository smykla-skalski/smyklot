<script module lang="ts">
  import type { IconName } from './Icon.svelte';

  export interface RolePickerOption {
    value: string;
    label: string;
    icon: IconName;
  }
</script>

<script lang="ts">
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';

  const {
    label,
    value,
    options,
    disabled = false,
    onSelect,
  }: {
    label: string;
    value: string;
    options: readonly RolePickerOption[];
    disabled?: boolean;
    onSelect: (value: string) => void;
  } = $props();

  let triggerButton = $state<HTMLButtonElement | null>(null);
  let open = $state(false);

  const selected = $derived(options.find((option) => option.value === value) ?? options[0]);

  function choose(next: string): void {
    if (next !== value) onSelect(next);
    open = false;
    queueMicrotask(() => triggerButton?.focus());
  }

  function openFromKeyboard(event: KeyboardEvent): void {
    if (!['ArrowDown', 'ArrowUp'].includes(event.key) || open || disabled) return;
    event.preventDefault();
    open = true;
  }
</script>

<!--
@component
What someone may do here, chosen from the roles this workspace defines. A listbox
rather than a menu, because these are values to pick among and not acts to perform -
which is also what a screen reader is told.

The layer is `min-trigger` wide: the trigger is a role's name and the options carry the
sentence that tells each role from its neighbours, so the list is wider than the button
that opens it and the button is the floor rather than the measure.

Focus opens on the current value rather than the top of the list, so the keyboard starts
where the reader already is.
-->

<Popover
  bind:open
  width="min-trigger"
  role="listbox"
  {label}
  itemSelector=".role-option"
  focusSelector="[aria-selected='true']"
>
  {#snippet trigger(attributes)}
    <div class="role-picker">
      <button
        class="role-trigger"
        type="button"
        bind:this={triggerButton}
        {disabled}
        aria-label={label}
        onkeydown={openFromKeyboard}
        {...attributes}
      >
        {#if selected !== undefined}
          <span class="role-icon" aria-hidden="true"><Icon name={selected.icon} size="sm" /></span>
          <span class="band-trim">{selected.label}</span>
        {/if}
        <span class="role-chevron" aria-hidden="true"><Icon name="chevron-down" size="sm" /></span>
      </button>
    </div>
  {/snippet}

  <div class="role-body">
    {#each options as option (option.value)}
      {@const isSelected = option.value === value}
      <button
        class="role-option"
        class:selected={isSelected}
        type="button"
        role="option"
        aria-selected={isSelected}
        onclick={() => choose(option.value)}
      >
        <Icon name={option.icon} size="base" />
        <span>{option.label}</span>
        {#if isSelected}<Icon name="success" size="base" />{/if}
      </button>
    {/each}
  </div>
</Popover>

<style>
  .role-picker {
    display: inline-block;
  }

  .role-trigger {
    align-items: center;
    background: var(--control-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    display: inline-flex;
    font: 600 var(--font-size-compact) / var(--leading-flat) var(--sans);
    gap: var(--space-2);
    /* The panel's compact control height, not a third one of its own: 1.875rem
       is 30px, which lined up with nothing it ever sat beside. */
    height: var(--control-height-compact);
    min-width: 7.25rem;
    padding: 0 0.5rem;
    transition:
      background-color var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out),
      transform var(--duration-press) var(--ease-out);
  }

  .role-trigger:hover:not(:disabled),
  .role-trigger[aria-expanded='true'] {
    background: var(--control-bg-hover);
    border-color: var(--control-border-hover);
  }

  .role-trigger:active:not(:disabled) {
    background: var(--control-bg-pressed);
    box-shadow: var(--pressed-inset);
  }

  .role-trigger:focus-visible {
    border-color: var(--focus);
    box-shadow: inset 0 0 0 1px var(--focus);
    outline: 0;
  }

  .role-icon {
    color: var(--text-muted);
    display: grid;
    flex: 0 0 1.125rem;
    place-items: center;
    width: 1.125rem;
  }

  /* `clip` and not `hidden`, like every other truncating line in the product:
     this span carries `.band-trim`, so its box ends on the cap and the baseline,
     and the ascender of the `d` in Admin stands above the cap by construction.
     `hidden` clips both axes and shaved the top off it. */
  .role-trigger > span:not(.role-chevron, .role-icon) {
    overflow: clip;
    overflow-clip-margin: 0.35em;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .role-chevron {
    color: var(--text-muted);
    display: grid;
    margin-left: auto;
    place-items: center;
    transition: transform var(--duration-fast) var(--ease-out);
  }

  .role-trigger[aria-expanded='true'] .role-chevron {
    transform: rotate(180deg);
  }

  /* Inside the layer. Its height is no longer guessed at 24rem: the layer
     measures the room it actually has and caps itself to that. */
  .role-body {
    display: grid;
    /* Two pixels, so a hovered row and the selected one below it never meet along
       an edge and read as one taller block. One would do at a whole device ratio
       and round away at a fractional one. */
    gap: 2px;
    min-width: 9.75rem;
    padding: var(--space-1);
  }

  .role-option {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    display: grid;
    font: 600 var(--font-size-compact) / var(--leading-flat) var(--sans);
    /* The symbol sits closer to its word than the row does to its edge: a mark and
       the word it belongs to are one object, and at --space-2 they read as two
       columns that happen to be adjacent. */
    gap: var(--space-1);
    grid-template-columns: 1rem minmax(0, 1fr) 1rem;
    min-height: var(--control-height-compact);
    padding: 0 var(--space-2);
    text-align: left;
    width: 100%;
  }

  /* Pulled left by its own bearing, so what the eye measures from is the symbol's
     ink and not the empty space its 24-unit box carries around it - the same
     correction `.btn > svg` makes, and the reason the gaps looked lopsided: the
     left one was padding, the right one was padding plus whatever slack that
     particular glyph happened to have. The column keeps its width, so the text
     stays in the same place down the list. */
  .role-option > :global(svg:first-child) {
    margin-inline-start: calc(-1 * var(--icon-ink-start, 0px));
  }

  .role-option:hover,
  .role-option:focus-visible {
    background: var(--interactive-hover);
    color: var(--text-primary);
    outline: 0;
  }

  .role-option.selected {
    background: var(--brand-action-tint);
    color: var(--brand-action-text);
  }

  @media (prefers-reduced-motion: reduce) {
    .role-trigger,
    .role-chevron {
      transition: none;
    }
  }
</style>
