<script lang="ts">
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

  let menu = $state<HTMLDetailsElement | null>(null);
  let trigger = $state<HTMLElement | null>(null);

  $effect(() => {
    function outside(event: PointerEvent): void {
      if (menu?.open === true && event.target instanceof Node && !menu.contains(event.target)) {
        close(false);
      }
    }
    function escape(event: KeyboardEvent): void {
      if (event.key !== 'Escape' || menu?.open !== true) return;
      event.preventDefault();
      close(true);
    }
    document.addEventListener('pointerdown', outside);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('keydown', escape);
    };
  });

  function choose(item: ActionMenuItem): void {
    if (item.disabled === true) return;
    close(true);
    onSelect(item.id, trigger);
  }

  function close(restoreFocus: boolean): void {
    if (menu !== null) menu.open = false;
    if (restoreFocus) trigger?.focus();
  }

  function move(event: KeyboardEvent): void {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    const buttons = Array.from(
      menu?.querySelectorAll<HTMLButtonElement>('.action-item:not(:disabled)') ?? [],
    );
    if (buttons.length === 0) return;
    event.preventDefault();
    const current = buttons.indexOf(event.currentTarget as HTMLButtonElement);
    let next = event.key === 'Home' ? 0 : buttons.length - 1;
    if (event.key === 'ArrowDown') next = (current + 1) % buttons.length;
    if (event.key === 'ArrowUp') next = (current - 1 + buttons.length) % buttons.length;
    buttons[next]?.focus();
  }
</script>

<details class="action-menu" bind:this={menu}>
  <summary bind:this={trigger} aria-label={label} title={label}>
    <Icon name="more" size={18} />
  </summary>
  <div class="action-popover" role="menu" aria-label={label}>
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
</details>

<style>
  .action-menu {
    flex: none;
    position: relative;
  }

  .action-menu[open] {
    z-index: var(--layer-popover);
  }

  summary {
    align-items: center;
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--r-ctl);
    display: flex;
    height: 1.875rem;
    justify-content: center;
    position: relative;
    width: 1.875rem;
  }

  summary::-webkit-details-marker {
    display: none;
  }

  summary::marker {
    content: '';
  }

  summary::before {
    content: '';
    inset: -0.3125rem;
    position: absolute;
  }

  summary:hover,
  .action-menu[open] summary {
    background: var(--strip-lift);
    border-color: var(--control-border);
  }

  .action-popover {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    min-width: 14rem;
    padding: 0.3rem;
    position: absolute;
    right: 0;
    top: calc(100% + 0.3rem);
    z-index: var(--layer-popover);
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

  .action-item.danger strong {
    color: var(--stop);
  }

  .action-item.danger .action-icon {
    color: var(--stop);
  }
</style>
