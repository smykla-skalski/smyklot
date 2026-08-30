<script module lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';

  export type PopoverWidth = 'auto' | 'min-trigger' | 'trigger';
  export type PopoverDismiss = 'auto' | 'manual';
  export type PopoverSkin = 'default' | 'sidebar';
  export type PopoverTriggerAttributes = HTMLButtonAttributes;
</script>

<script lang="ts">
  import { Popover } from 'bits-ui';
  import type { Snippet } from 'svelte';

  let {
    align = 'start',
    side = 'below',
    width = 'auto',
    dismiss = 'auto',
    offset = 6,
    role,
    label,
    skin = 'default',
    itemSelector,
    focusSelector,
    focusOnOpen = true,
    open = $bindable(false),
    anchor,
    trigger,
    children,
    onopen,
    onclose,
  }: {
    align?: 'start' | 'center' | 'end';
    side?: 'above' | 'below' | 'left' | 'right';
    width?: PopoverWidth;
    dismiss?: PopoverDismiss;
    offset?: number;
    role?: 'dialog' | 'listbox' | 'menu';
    label?: string;
    skin?: PopoverSkin;
    itemSelector?: string;
    focusSelector?: string;
    focusOnOpen?: boolean;
    open?: boolean;
    anchor?: HTMLElement | null;
    trigger?: Snippet<[PopoverTriggerAttributes]>;
    children: Snippet;
    onopen?: () => void;
    onclose?: () => void;
  } = $props();

  const sideName = $derived(side === 'above' ? 'top' : side === 'below' ? 'bottom' : side);
  let content = $state<HTMLElement | null>(null);

  function openChanged(next: boolean): void {
    open = next;
    if (next) onopen?.();
    else onclose?.();
  }

  function opening(event: Event): void {
    if (!focusOnOpen || focusSelector !== undefined) event.preventDefault();
    if (focusOnOpen && focusSelector !== undefined) {
      queueMicrotask(() => content?.querySelector<HTMLElement>(focusSelector)?.focus());
    }
  }

  function interactingOutside(event: Event): void {
    if (dismiss === 'manual') event.preventDefault();
  }

  function walk(event: KeyboardEvent): void {
    if (itemSelector === undefined) return;
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;

    const content = event.currentTarget as HTMLElement;
    const items = Array.from(content.querySelectorAll<HTMLElement>(itemSelector)).filter(
      (item) => !item.matches(':disabled'),
    );
    if (items.length === 0) return;

    event.preventDefault();
    const current = items.indexOf(event.target as HTMLElement);
    let next = event.key === 'Home' ? 0 : items.length - 1;
    if (event.key === 'ArrowDown') next = (current + 1) % items.length;
    if (event.key === 'ArrowUp') next = (current - 1 + items.length) % items.length;
    items[next]?.focus();
  }
</script>

<!--
@component
One layer that opens beside its trigger, and the only one. Six of these were
hand-rolled - an account menu, a filter, a table's tools, a display menu, a history
picker, a colour picker - each with its own outside-click handling, its own escape
key and its own idea of how wide to be.

What it is decides how it is announced: `menu` for a list of acts, `listbox` for a
choice among values, `dialog` for anything a reader types into. That is the prop worth
thinking about, because it is the one a screen reader repeats.

`width` is the trap the hand-rolled ones kept falling into. `trigger` matches the
trigger exactly and `min-trigger` treats it as a floor, which is what a filter wants
when its options are longer than the button that opens them; `auto` is the content's
own width. A popover measured once and cached comes back stale the next time its
trigger has changed size.

`dismiss: 'manual'` keeps it open through a click inside, for the one that holds a
form. Bits UI owns the positioning, the focus and the escape key; what is written here
is the skin, the width rules and the roving focus that `itemSelector` drives.
-->

<Popover.Root bind:open onOpenChange={openChanged}>
  {#if trigger !== undefined}
    <Popover.Trigger>
      {#snippet child({ props })}
        {@render trigger(props)}
      {/snippet}
    </Popover.Trigger>
  {/if}

  <Popover.Portal to=".app-shell">
    <Popover.Content
      bind:ref={content}
      class={['app-popover', skin, `width-${width}`]}
      side={sideName}
      sideOffset={offset}
      {align}
      customAnchor={anchor}
      collisionPadding={8}
      {role}
      aria-label={label}
      onOpenAutoFocus={opening}
      onInteractOutside={interactingOutside}
      onkeydown={walk}
    >
      {@render children()}
    </Popover.Content>
  </Popover.Portal>
</Popover.Root>

<style>
  :global(.app-popover) {
    background: var(--layer-bg);
    border: 1px solid var(--layer-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    color: var(--text-primary);
    max-height: var(--bits-floating-available-height);
    max-width: var(--bits-floating-available-width);
    overflow: auto;
    padding: 0;
    z-index: var(--layer-popover);
  }

  :global(.app-popover.width-trigger) {
    width: var(--bits-floating-anchor-width);
  }

  :global(.app-popover.width-min-trigger) {
    min-width: var(--bits-floating-anchor-width);
  }

  :global(.app-popover.default) {
    --layer-bg: var(--popover-bg);
    --layer-border: var(--popover-border);
  }

  :global(.app-popover.sidebar) {
    --layer-bg: var(--sidebar-popover-bg);
    --layer-border: var(--sidebar-popover-border);
  }
</style>
