<script lang="ts">
  import { tick } from 'svelte';

  import Icon, { type IconName } from './Icon.svelte';

  export interface ActionMenuItem {
    id: string;
    icon: IconName;
    label: string;
    description?: string;
    tone?: 'default' | 'danger';
    disabled?: boolean;
  }

  const {
    label,
    items,
    onSelect,
  }: {
    label: string;
    items: readonly ActionMenuItem[];
    onSelect: (id: string, trigger: HTMLElement | null) => void;
  } = $props();

  let popover = $state<HTMLDivElement | null>(null);
  let trigger = $state<HTMLButtonElement | null>(null);
  let left = $state(0);
  let top = $state(0);

  async function open(): Promise<void> {
    if (trigger === null || popover === null) return;
    if (popover.matches(':popover-open')) {
      popover.hidePopover();
      return;
    }
    const rect = trigger.getBoundingClientRect();
    left = Math.max(8, Math.min(rect.right - 224, window.innerWidth - 232));
    top = rect.bottom + 6;
    popover.showPopover();
    await tick();
    const menuRect = popover.getBoundingClientRect();
    if (menuRect.bottom > window.innerHeight - 8) {
      top = Math.max(8, rect.top - menuRect.height - 6);
      await tick();
    }
    popover.querySelector<HTMLButtonElement>('.action-item:not(:disabled)')?.focus();
  }

  function choose(item: ActionMenuItem): void {
    if (item.disabled === true) return;
    popover?.hidePopover();
    onSelect(item.id, trigger);
  }

  function move(event: KeyboardEvent): void {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    const buttons = Array.from(
      popover?.querySelectorAll<HTMLButtonElement>('.action-item:not(:disabled)') ?? [],
    );
    if (buttons.length === 0) return;
    event.preventDefault();
    const current = buttons.indexOf(event.currentTarget as HTMLButtonElement);
    let next = event.key === 'Home' ? 0 : buttons.length - 1;
    if (event.key === 'ArrowDown') next = (current + 1) % buttons.length;
    if (event.key === 'ArrowUp') next = (current - 1 + buttons.length) % buttons.length;
    buttons[next]?.focus();
  }

  function restoreFocus(): void {
    trigger?.focus();
  }
</script>

<span class="action-menu">
  <button
    class="action-trigger"
    type="button"
    bind:this={trigger}
    aria-label={label}
    title={label}
    onclick={open}
  >
    <Icon name="more" size={22} />
  </button>
</span>

<div
  class="action-popover"
  bind:this={popover}
  popover="auto"
  role="menu"
  aria-label={label}
  style:left={`${left}px`}
  style:top={`${top}px`}
  onbeforetoggle={(event) => {
    if (event.newState === 'closed') restoreFocus();
  }}
>
  {#each items as item (item.id)}
    <button
      class="action-item"
      class:danger={item.tone === 'danger'}
      type="button"
      role="menuitem"
      disabled={item.disabled}
      onclick={() => choose(item)}
      onkeydown={move}
    >
      <span class="action-icon" aria-hidden="true"><Icon name={item.icon} size={16} /></span>
      <span class="action-copy">
        <strong>{item.label}</strong>
        {#if item.description !== undefined}<span>{item.description}</span>{/if}
      </span>
    </button>
  {/each}
</div>

<style>
  .action-menu {
    display: inline-flex;
  }

  .action-trigger {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--radius-control);
    color: var(--text-primary);
    display: flex;
    height: 2.5rem;
    justify-content: center;
    width: 2.5rem;
  }

  .action-trigger:hover {
    background: var(--interactive-hover);
    color: var(--text-primary);
  }

  .action-trigger:active {
    background: var(--interactive-pressed-bg);
  }

  .action-popover {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    margin: 0;
    min-width: 14rem;
    padding: 0.3rem;
    position: fixed;
  }

  .action-item {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: calc(var(--r-ctl) - 2px);
    color: var(--text);
    display: grid;
    gap: var(--space-2);
    grid-template-columns: 1rem minmax(0, 1fr);
    padding: 0.55rem 0.625rem;
    text-align: left;
    width: 100%;
  }

  .action-icon {
    color: var(--text-muted);
    display: grid;
    place-items: center;
  }

  .action-copy {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .action-item:hover:not(:disabled),
  .action-item:focus-visible {
    background: var(--interactive-hover);
  }

  .action-item strong {
    font-size: 0.75rem;
  }

  .action-copy > span {
    color: var(--dim);
    font-size: 0.6875rem;
    margin-top: 0.15rem;
  }

  .action-item.danger strong,
  .action-item.danger .action-icon {
    color: var(--stop);
  }
</style>
