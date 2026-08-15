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

  /* The browser needs to know which button owns this menu. Toggling it by hand
     looks right and cannot work: an auto popover light-dismisses on pointerdown,
     so a second press on the trigger closed it and the handler, finding it
     closed, opened it again - the menu could only be dismissed by clicking
     somewhere else. */
  const popoverId = $props.id();

  /* Placed once it is open, from its own toggle event: that is the only moment
     the menu has been laid out and can be measured. */
  async function place(): Promise<void> {
    if (trigger === null || popover === null) return;
    const rect = trigger.getBoundingClientRect();
    await tick();
    const menu = popover.getBoundingClientRect();

    /* Hung under the button and aligned to its right edge, which is where a menu
       opened from a trailing control belongs. The width used to be assumed to be
       224px; the menu is as wide as its longest description, so a wrong guess put
       it a hundred pixels off the button it belongs to. */
    left = Math.min(window.innerWidth - menu.width - 8, Math.max(8, rect.right - menu.width));
    top = rect.bottom + 6;
    await tick();

    if (popover.getBoundingClientRect().bottom > window.innerHeight - 8) {
      top = Math.max(8, rect.top - menu.height - 6);
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
    popovertarget={popoverId}
    popovertargetaction="toggle"
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
  id={popoverId}
  onbeforetoggle={(event) => {
    if (event.newState === 'closed') restoreFocus();
  }}
  ontoggle={(event) => {
    if (event.newState === 'open') void place();
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

  /* Presses like every other control in the product: it takes the colour and it
     gets smaller. A round target of this size takes the disc scale, the one the
     avatar and the icon buttons use, because the same ratio on a 2.5rem square
     reads as no movement at all. */
  .action-trigger:active {
    background: var(--interactive-pressed-bg);
    scale: var(--press-scale-disc);
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
