<script module lang="ts">
  import type { Attachment } from 'svelte/attachments';

  import type { LayerAlign, LayerSide } from '../lib/anchored-layer';

  /** How wide the layer is relative to what it hangs off. */
  export type PopoverWidth = 'auto' | 'min-trigger' | 'trigger';

  /**
   * `auto` is the platform's light dismiss: a pointerdown anywhere outside closes
   * it, Escape closes it, and opening one auto popover closes the others. `manual`
   * hands all of that to the caller, and is for the one case where light dismiss
   * is wrong - a combobox, where the pointerdown that would dismiss it is somebody
   * clicking into the field they are typing in.
   */
  export type PopoverDismiss = 'auto' | 'manual';

  /** Which surface the layer sits on. The sidebar paints its own. */
  export type PopoverSkin = 'default' | 'sidebar';

  /** Spread onto whatever the caller uses as a trigger. */
  export interface PopoverTriggerAttributes {
    'aria-expanded': boolean;
    'aria-haspopup'?: 'dialog' | 'listbox' | 'menu';
    popovertarget?: string;
    popovertargetaction?: 'toggle';
    [attachment: symbol]: Attachment<HTMLElement>;
  }

  const FOCUSABLE =
    'a[href], button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])';
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { createAttachmentKey } from 'svelte/attachments';

  import { isVertical, placeLayer } from '../lib/anchored-layer';

  /**
   * A layer that hangs off a control: a menu, a picker, a list of suggestions.
   *
   * Six of these were written separately and each one grew its own version of the
   * same four problems. Three rendered into the page, where the first ancestor
   * that scrolls clips them - which is why a filter menu opened inside a pinned
   * table body was cut off at its edge. Three re-implemented dismissal by hand,
   * and the same toggle bug was found and fixed twice: an auto popover
   * light-dismisses on pointerdown, so a second press on the trigger closed it and
   * the click handler, finding it closed, opened it straight back up. Three
   * repeated the same arrow-key walk over their own rows. None of them capped
   * their height, so a long list opened near the bottom of the window ran off it.
   *
   * So the layer goes in the top layer, which no ancestor can clip, the platform
   * owns the toggle, and the arithmetic is in `anchored-layer.ts` where it can be
   * tested. What a caller still owns is its trigger - the markup and the skin of a
   * summary, an icon button and a combobox field have nothing in common - so the
   * trigger stays the caller's and is handed the attributes that wire it up.
   */
  // `let` rather than `const`: the platform's own toggle writes `open` back.
  let {
    align = 'start',
    side = 'below',
    width = 'auto',
    dismiss = 'auto',
    offset,
    role,
    label,
    skin = 'default',
    /** Rows the arrow keys walk. Omit where the layer is not a list. */
    itemSelector,
    /** What takes focus when it opens; the first focusable row by default. */
    focusSelector,
    focusOnOpen = true,
    open = $bindable(false),
    anchor: anchorOverride,
    trigger,
    children,
    onopen,
    onclose,
  }: {
    align?: LayerAlign;
    side?: LayerSide;
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
    /**
     * What to measure against, when that is not the thing that opens it. A
     * combobox has no trigger to speak of - the layer belongs under the field,
     * and the field is not a button that toggles anything.
     */
    anchor?: HTMLElement | null;
    trigger?: Snippet<[PopoverTriggerAttributes]>;
    children: Snippet;
    onopen?: () => void;
    onclose?: () => void;
  } = $props();

  const popoverId = $props.id();

  let panel = $state<HTMLElement | null>(null);
  let capturedTrigger = $state<HTMLElement | null>(null);

  const anchor = $derived(anchorOverride ?? capturedTrigger);

  /* Stable, both of them: a fresh key or a fresh function on every recompute
     would tear the attachment down and set it up again each time `open` changed. */
  const attachmentKey = createAttachmentKey();
  const captureTrigger: Attachment<HTMLElement> = (element) => {
    capturedTrigger = element;
    return () => {
      if (capturedTrigger === element) capturedTrigger = null;
    };
  };

  const triggerAttributes = $derived({
    [attachmentKey]: captureTrigger,
    'aria-expanded': open,
    ...(role === undefined ? {} : { 'aria-haspopup': role }),
    /* Naming the invoker is what makes a second press on the trigger close the
       menu rather than reopen it: the platform knows the dismissal came from
       there. Doing the toggling by hand cannot work, and looked like it did. */
    ...(dismiss === 'auto'
      ? { popovertarget: popoverId, popovertargetaction: 'toggle' as const }
      : {}),
  }) as PopoverTriggerAttributes;

  /** Measured, so it follows its trigger rather than hanging where it opened. */
  function place(): void {
    if (anchor === null || panel === null) return;

    /* Measuring lets the layer back to its natural size for an instant, and a
       box with no overflow has no scroll position to keep - so reading it here
       and putting it back is what stops a capped layer from jumping to its top
       every time it is re-placed. */
    const scrolledTo = { left: panel.scrollLeft, top: panel.scrollTop };
    const rect = anchor.getBoundingClientRect();
    /* Cleared before they are set, because the strategy can change under the
       same layer: a sidebar menu is its trigger's width until the rail
       collapses, and a width left over from before would survive as a hard
       number and quietly win over what the collapsed rule asks for. */
    panel.style.width = '';
    panel.style.minWidth = '';
    if (width === 'trigger') panel.style.width = `${rect.width}px`;
    if (width === 'min-trigger') panel.style.minWidth = `${rect.width}px`;

    /* Measured uncapped first. Capping before knowing which side it lands on
       would size it against the wrong side's room, and a list that fits above
       would be trimmed to what was left below it. */
    panel.style.maxHeight = '';
    panel.style.maxWidth = '';
    const viewport = { height: window.innerHeight, width: window.innerWidth };
    const box = { height: panel.offsetHeight, width: panel.offsetWidth };
    let at = placeLayer(rect, box, viewport, { align, offset, side });

    /* Both axes, not just the one it hangs on. The side it sits on is bounded by
       the room between the trigger and the edge; the other by the window less
       its gutters. Capping only the first left a layer wider than the window
       hanging off it, held at the near edge and running past the far one. */
    const vertical = isVertical(at.side);
    const capped = {
      height: Math.min(box.height, vertical ? at.available : at.crossAvailable),
      width: Math.min(box.width, vertical ? at.crossAvailable : at.available),
    };

    if (capped.height !== box.height || capped.width !== box.width) {
      panel.style.maxHeight = `${capped.height}px`;
      panel.style.maxWidth = `${capped.width}px`;
      at = placeLayer(rect, capped, viewport, { align, offset, side });
    }

    panel.style.left = `${at.left}px`;
    panel.style.top = `${at.top}px`;
    panel.scrollLeft = scrolledTo.left;
    panel.scrollTop = scrolledTo.top;
  }

  function focusInside(): void {
    if (!focusOnOpen || panel === null) return;
    const chosen =
      focusSelector === undefined ? null : panel.querySelector<HTMLElement>(focusSelector);
    (chosen ?? panel.querySelector<HTMLElement>(FOCUSABLE))?.focus();
  }

  /* The DOM is the truth about whether a popover is open - the platform opens and
     closes it without asking - so this syncs one way and `ontoggle` syncs back. */
  $effect(() => {
    const element = panel;
    if (element === null) return;
    const isOpen = element.matches(':popover-open');
    if (open && !isOpen) element.showPopover();
    if (!open && isOpen) element.hidePopover();
  });

  $effect(() => {
    if (!open) return;
    const reposition = (): void => place();
    /* A layer scrolling inside itself has not moved its trigger, so re-placing
       it there is work at best. It was worse than that: measuring resets the
       scroll it was reading, so the layer walked back to its top under whoever
       was scrolling it. */
    const repositionUnlessInside = (event: Event): void => {
      if (event.target instanceof Node && panel?.contains(event.target) === true) return;
      place();
    };
    /* Capture: the scroller that moves a trigger is usually an inner one - a
       pinned table body, a dialog - and a scroll event does not bubble. */
    document.addEventListener('scroll', repositionUnlessInside, true);
    window.addEventListener('resize', reposition);

    /* Contents can change while it is open - a suggestion list narrows to what
       has been typed - and a layer that grew after being placed would be
       measured against the size it used to be.

       Observing delivers once for the size the layer already has, which is not
       a change and was a second full measurement on every open - in every menu,
       and these are built per table row. */
    let sizeKnown = false;
    const watcher = new ResizeObserver(() => {
      if (!sizeKnown) {
        sizeKnown = true;
        return;
      }
      place();
    });
    if (panel !== null) watcher.observe(panel);

    return () => {
      document.removeEventListener('scroll', repositionUnlessInside, true);
      window.removeEventListener('resize', reposition);
      watcher.disconnect();
    };
  });

  function opened(): void {
    place();
    focusInside();
    onopen?.();
  }

  function closed(): void {
    // Only when it is leaving from inside; a press elsewhere means focus has
    // already gone where it was meant to go.
    if (panel?.contains(document.activeElement) === true) anchor?.focus();
    onclose?.();
  }

  /*
   * The one place an open or a close is acted on, whoever started it.
   *
   * Not conditional on `open` having changed here: a caller that opens the layer
   * itself - the picker does, on an arrow key - has already set `open` to true by
   * the time this runs, and treating that as "no change" skipped placing and
   * focusing it. The event fires once per real transition, so acting on it every
   * time is both sufficient and correct.
   */
  function toggled(event: ToggleEvent): void {
    const nowOpen = event.newState === 'open';
    open = nowOpen;
    if (nowOpen) opened();
    else closed();
  }

  function walk(event: KeyboardEvent): void {
    if (itemSelector === undefined) return;
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;

    const items = Array.from(panel?.querySelectorAll<HTMLElement>(itemSelector) ?? []).filter(
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

{#if trigger !== undefined}
  {@render trigger(triggerAttributes)}
{/if}

<!--
  The layer is a surface and a position, and nothing else. Everything inside it -
  padding, grid, widths - belongs to the caller, in an element of the caller's own.

  Not a matter of taste: a class handed to a component does not carry the parent's
  scoping hash, while every element written in the parent's own markup does. So a
  parent styling `.whatever` would compile to `.whatever.svelte-hash`, the layer
  would be given a bare `whatever`, and the rule would silently never match.
-->
<div
  bind:this={panel}
  id={popoverId}
  class={['app-popover', skin]}
  popover={dismiss}
  {role}
  aria-label={label}
  ontoggle={toggled}
  onkeydown={walk}
>
  {@render children()}
</div>

<style>
  /*
   * Fixed and in the top layer. The `left`/`top` written here are placeholders
   * that measurement overwrites - through the CSSOM, never a style attribute,
   * because the panel serves `style-src 'self'` and a parsed style attribute is
   * dropped outright.
   */
  .app-popover {
    background: var(--layer-bg);
    border: 1px solid var(--layer-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    color: var(--text);
    inset: auto;
    margin: 0;
    overflow: auto;
    /* No padding: the caller's own wrapper carries it, for the reason above. */
    padding: 0;
    position: fixed;
  }

  /*
   * Only once it is open. A bare `display` overrides the `display: none` the UA
   * sheet gives a closed popover - author styles win over it - and every list in
   * the tables painted itself under its own row until this was scoped.
   */
  .app-popover:popover-open {
    /* A column, so a caller that pins a header and a footer can let the middle
       shrink and scroll on its own: it sets `min-height: 0` and the cap this
       layer measured for itself does the rest. For a plain list it behaves as a
       block would, and the layer's own `overflow` scrolls it. */
    display: flex;
    flex-direction: column;
  }

  .app-popover::backdrop {
    background: transparent;
  }

  .default {
    --layer-bg: var(--popover-bg);
    --layer-border: var(--popover-border);
  }

  .sidebar {
    --layer-bg: var(--sidebar-popover-bg);
    --layer-border: var(--sidebar-popover-border);
  }
</style>
