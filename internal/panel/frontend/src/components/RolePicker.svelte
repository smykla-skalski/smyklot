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

  const selected = $derived(options.find((option) => option.value === value) ?? options[0]);

  function toggle(): void {
    if (popover === null || disabled) return;
    if (popover.matches(':popover-open')) {
      popover.hidePopover();
      return;
    }
    popover.showPopover();
    placePopover();
    queueMicrotask(() => {
      const selectedOption = popover?.querySelector<HTMLButtonElement>('[aria-selected="true"]');
      selectedOption?.focus();
    });
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
    open = popover?.matches(':popover-open') === true;
  }

  function choose(next: string): void {
    if (next !== value) onSelect(next);
    popover?.hidePopover();
    queueMicrotask(() => trigger?.focus());
  }

  function openFromKeyboard(event: KeyboardEvent): void {
    if (!['ArrowDown', 'ArrowUp'].includes(event.key) || open) return;
    event.preventDefault();
    toggle();
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
    onclick={toggle}
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
    transform: translateY(1px);
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
    gap: var(--space-2);
    grid-template-columns: 1rem minmax(0, 1fr) 1rem;
    min-height: var(--control-height-compact);
    padding: 0 var(--space-2);
    text-align: left;
    width: 100%;
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
