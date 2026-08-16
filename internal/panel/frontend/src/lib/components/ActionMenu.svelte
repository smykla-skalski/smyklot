<script lang="ts">
  import { DropdownMenu } from 'bits-ui';
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

  /* Handed back to the caller on select, which is how a dialog opened from here
     knows what to put focus on once it closes. */
  let triggerButton = $state<HTMLButtonElement | null>(null);
  let open = $state(false);

  function choose(item: ActionMenuItem): void {
    if (item.disabled === true) return;
    open = false;
    onSelect(item.id, triggerButton);
  }
</script>

<!-- Hung under the button and aligned to its right edge, which is where a menu
     opened from a trailing control belongs. Its width is its longest description,
     so it is measured rather than assumed - a guess of 224px once put it a
     hundred pixels off the button it belongs to. -->
<DropdownMenu.Root bind:open>
  <span class="action-menu">
    <DropdownMenu.Trigger
      class="action-trigger"
      bind:ref={triggerButton}
      aria-label={label}
      title={label}
    >
      <Icon name="more" size={22} />
    </DropdownMenu.Trigger>
  </span>

  <DropdownMenu.Portal to=".app-shell">
    <DropdownMenu.Content class="action-body" align="end" sideOffset={6} collisionPadding={8}>
      {#each items as item (item.id)}
        <DropdownMenu.Item
          class={['action-item', item.tone === 'danger' && 'danger']}
          disabled={item.disabled}
          textValue={item.label}
          onSelect={() => choose(item)}
        >
          <span class="action-icon" aria-hidden="true"><Icon name={item.icon} size={16} /></span>
          <span class="action-copy">
            <strong>{item.label}</strong>
            {#if item.description !== undefined}<span>{item.description}</span>{/if}
          </span>
        </DropdownMenu.Item>
      {/each}
    </DropdownMenu.Content>
  </DropdownMenu.Portal>
</DropdownMenu.Root>

<style>
  .action-menu {
    display: inline-flex;
  }

  :global(.action-trigger) {
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

  :global(.action-trigger:hover) {
    background: var(--interactive-hover);
    color: var(--text-primary);
  }

  /* Presses like every other control in the product: it takes the colour and it
     gets smaller. A round target of this size takes the disc scale, the one the
     avatar and the icon buttons use, because the same ratio on a 2.5rem square
     reads as no movement at all. */
  :global(.action-trigger:active) {
    background: var(--interactive-pressed-bg);
    scale: var(--press-scale-disc);
  }

  /* Inside the layer: the surface and where it sits are the layer's, everything
     within it is this component's. */
  :global(.action-body) {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    display: grid;
    min-width: 14rem;
    padding: 0.3rem;
    z-index: var(--layer-popover);
  }

  :global(.action-item) {
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

  :global(.action-icon) {
    color: var(--text-muted);
    display: grid;
    place-items: center;
  }

  :global(.action-copy) {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  :global(.action-item:hover:not(:disabled)),
  :global(.action-item:focus-visible),
  :global(.action-item[data-highlighted]) {
    background: var(--interactive-hover);
  }

  :global(.action-item strong) {
    font-size: 0.75rem;
  }

  :global(.action-copy > span) {
    color: var(--dim);
    font-size: 0.6875rem;
    margin-top: 0.15rem;
  }

  :global(.action-item.danger strong),
  :global(.action-item.danger .action-icon) {
    color: var(--stop);
  }
</style>
