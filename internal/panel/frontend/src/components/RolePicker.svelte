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

  const {
    label,
    value,
    options,
    disabled = false,
    variant = 'compact',
    onSelect,
  }: {
    label: string;
    value: string;
    options: readonly RolePickerOption[];
    disabled?: boolean;
    variant?: 'compact' | 'field';
    onSelect: (value: string) => void;
  } = $props();

  let trigger = $state<HTMLButtonElement | null>(null);
  let popover = $state<HTMLElement | null>(null);
  let open = $state(false);

  /* The browser needs to know which button owns this popover. Doing the toggling
     by hand looks like it works and does not: an auto popover light-dismisses on
     pointerdown, so a second click on the trigger closed it and then the click
     handler, finding it closed, opened it again - the list never shut. Naming the
     invoker hands that sequence to the platform, which knows the dismissal came
     from the trigger and leaves it closed. */
  const popoverId = $props.id();

  const selected = $derived(options.find((option) => option.value === value) ?? options[0]);

  /* Runs once the popover is open, from its own toggle event rather than from the
     click: with the platform owning the toggle, that event is the only place that
     knows an open actually happened. */
  function afterOpen(): void {
    placePopover();
    const selectedOption = popover?.querySelector<HTMLButtonElement>('[aria-selected="true"]');
    selectedOption?.focus();
  }

  function placePopover(): void {
    if (trigger === null || popover === null) return;
    const anchor = trigger.getBoundingClientRect();
    const width = Math.max(156, anchor.width);
    popover.style.width = `${width}px`;
    const height = popover.offsetHeight;
    const left = Math.min(window.innerWidth - width - 8, Math.max(8, anchor.left));
    const below = anchor.bottom + 6;
    const top = below + height <= window.innerHeight - 8 ? below : anchor.top - height - 6;
    popover.style.left = `${left}px`;
    popover.style.top = `${Math.max(8, top)}px`;
  }

  function syncOpen(): void {
    const nowOpen = popover?.matches(':popover-open') === true;
    if (nowOpen && !open) afterOpen();
    open = nowOpen;
  }

  function choose(next: string): void {
    if (next !== value) onSelect(next);
    popover?.hidePopover();
    queueMicrotask(() => trigger?.focus());
  }

  function openFromKeyboard(event: KeyboardEvent): void {
    if (!['ArrowDown', 'ArrowUp'].includes(event.key) || open || disabled) return;
    event.preventDefault();
    popover?.showPopover();
  }

  function moveOptionFocus(event: KeyboardEvent): void {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    const optionButtons = Array.from(
      popover?.querySelectorAll<HTMLButtonElement>('.role-option') ?? [],
    );
    if (optionButtons.length === 0) return;
    event.preventDefault();
    const current = optionButtons.indexOf(event.currentTarget as HTMLButtonElement);
    let next = event.key === 'Home' ? 0 : optionButtons.length - 1;
    if (event.key === 'ArrowDown') next = (current + 1) % optionButtons.length;
    if (event.key === 'ArrowUp') next = (current - 1 + optionButtons.length) % optionButtons.length;
    optionButtons[next]?.focus();
  }
</script>

<div class="role-picker" class:field={variant === 'field'}>
  <button
    class="role-trigger"
    type="button"
    bind:this={trigger}
    {disabled}
    aria-label={label}
    aria-haspopup="listbox"
    aria-expanded={open}
    popovertarget={popoverId}
    popovertargetaction="toggle"
    onkeydown={openFromKeyboard}
  >
    {#if selected !== undefined}
      <span class="role-icon" aria-hidden="true"><Icon name={selected.icon} size={14} /></span>
      <span>{selected.label}</span>
    {/if}
    <span class="role-chevron" aria-hidden="true"><Icon name="chevron-down" size={14} /></span>
  </button>

  <div
    class="role-popover"
    bind:this={popover}
    id={popoverId}
    popover="auto"
    role="listbox"
    aria-label={label}
    ontoggle={syncOpen}
  >
    {#each options as option (option.value)}
      {@const isSelected = option.value === value}
      <button
        class="role-option"
        class:selected={isSelected}
        type="button"
        role="option"
        aria-selected={isSelected}
        onclick={() => choose(option.value)}
        onkeydown={moveOptionFocus}
      >
        <Icon name={option.icon} size={15} />
        <span>{option.label}</span>
        {#if isSelected}<Icon name="success" size={15} />{/if}
      </button>
    {/each}
  </div>
</div>

<style>
  .role-picker {
    display: inline-block;
  }

  .role-trigger {
    align-items: center;
    background: var(--control-surface);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text);
    display: inline-flex;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    gap: var(--space-2);
    height: 1.875rem;
    min-width: 7.25rem;
    padding: 0 0.5rem;
    transition:
      background-color var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out),
      transform var(--duration-press) var(--ease-out);
  }

  .field,
  .field .role-trigger {
    width: 100%;
  }

  .field .role-trigger {
    font-size: var(--font-size-body);
    height: var(--control-height);
    padding-inline: var(--space-3);
  }

  .role-trigger:hover:not(:disabled),
  .role-trigger[aria-expanded='true'] {
    background: var(--control-bg-hover);
    border-color: var(--control-border-hover);
  }

  .role-trigger:active:not(:disabled) {
    background: var(--control-bg-pressed);
    transform: scale(var(--press-scale-compact));
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

  .role-trigger > span:not(.role-chevron, .role-icon) {
    overflow: hidden;
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

  .role-popover {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    color: var(--text);
    inset: auto;
    margin: 0;
    max-height: min(24rem, calc(100dvh - 1rem));
    overflow: auto;
    padding: var(--space-1);
    position: fixed;
  }

  /* Only once it is open. A bare `display` on a popover overrides the `display:
     none` the UA sheet gives a closed one, and every list in the table painted
     itself under its own row - author styles win over the UA sheet, so the
     closed state has to be excluded rather than assumed. */
  .role-popover:popover-open {
    display: grid;
    /* Two pixels, so a hovered row and the selected one below it never meet along
       an edge and read as one taller block. One would do at a whole device ratio
       and round away at a fractional one. */
    gap: 2px;
  }

  .role-popover::backdrop {
    background: transparent;
  }

  .role-option {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    display: grid;
    font: 600 var(--font-size-compact) / 1 var(--sans);
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
