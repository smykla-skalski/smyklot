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
    onOpenChange,
  }: {
    label: string;
    items: readonly ActionMenuItem[];
    onSelect: (id: string, trigger: HTMLElement | null) => void;
    /**
     * Whether the layer is showing. The menu lives in a portal, so a list that has to hold still
     * while one of its rows is being operated cannot tell from focus or from the pointer - both
     * have left the row by then.
     */
    onOpenChange?: (open: boolean) => void;
  } = $props();

  /* Handed back to the caller on select, which is how a dialog opened from here
     knows what to put focus on once it closes. */
  let triggerButton = $state<HTMLButtonElement | null>(null);
  let open = $state(false);

  $effect(() => {
    onOpenChange?.(open);
  });

  function choose(item: ActionMenuItem): void {
    if (item.disabled === true) return;
    open = false;
    onSelect(item.id, triggerButton);
  }
</script>

<!--
@component
Acts, not navigation. A menu holds things that HAPPEN; a place a reader can go is a
link on the page, where it can be opened in a new tab and seen before it is pressed. A
destructive entry sits last and wears the danger ink, so the thing that cannot be taken
back is never the thing under the pointer by accident.

`onOpenChange` is the prop worth knowing about. The menu lives in a portal, so a list
that has to hold still while one of its rows is being operated cannot work that out
from focus or from the pointer - both have left the row by the time the layer opens.

Its width is its longest description, measured rather than assumed: a guess of 224px
once put it a hundred pixels off the button it belongs to.
-->

<!-- Hung under the button and aligned to its right edge, which is where a menu
     opened from a trailing control belongs. Its width is its longest description,
     so it is measured rather than assumed - a guess of 224px once put it a
     hundred pixels off the button it belongs to. -->
<DropdownMenu.Root bind:open>
  <span class="action-menu">
    <DropdownMenu.Trigger
      class="icon-button action-trigger"
      bind:ref={triggerButton}
      aria-label={label}
      title={label}
    >
      <Icon name="more" size="sm" strokeWidth={2} />
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
          <span class="action-icon" aria-hidden="true"><Icon name={item.icon} size="base" /></span>
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

  /* The trigger's shape, ink and states are `.icon-button` in app.css, shared
     with the quick actions beside it and the filter triggers above it. The class
     that remains names the control for whoever goes looking for it. */

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
    color: var(--text-primary);
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
    color: var(--text-muted);
    font-size: 0.6875rem;
    margin-top: 0.15rem;
  }

  :global(.action-item.danger strong),
  :global(.action-item.danger .action-icon) {
    color: var(--danger);
  }
</style>
