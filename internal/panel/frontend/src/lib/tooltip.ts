/**
 * The product's one tooltip.
 *
 * A tooltip laid out beside its trigger is cut off by the first ancestor that
 * clips: a table cell, a pinned table body, a card, a dialog. Worse, a closed
 * one still takes part in layout, and a box that reaches past the right edge of
 * a table wrapper puts a horizontal scrollbar under a table that has nothing to
 * scroll. So the box is built on demand, mounted on the shell, positioned
 * against the viewport, and removed again when it closes.
 *
 * The host is the shell rather than <body> because root mode repaints the
 * palette on `.app-shell.root-mode`; a tooltip on <body> would wear the wrong
 * surface. Styles live in `app.css` under `.app-tooltip` - the node is built
 * here, so it carries no component scope.
 */

import type { Action } from 'svelte/action';

import { placeLayer, type LayerAlign, type LayerRect, type LayerSize } from './anchored-layer';

export interface TooltipOptions {
  /** Matches the trigger's `aria-describedby`. Omit where the trigger already
      carries the same words in an `aria-label`. */
  id?: string;
  text: string;
  /** Which edge of the box lines up with the trigger's. */
  align?: LayerAlign;
}

/**
 * Clearance from the viewport edges, and between trigger and box. Wider than a
 * menu's, because a tooltip is a remark rather than a thing to be used, and one
 * pressed against the window edge reads as a mistake.
 */
const GUTTER = 16;
const OFFSET = 6;

/**
 * Below the trigger when it fits, above when it does not, and never closer to a
 * viewport edge than the gutter.
 *
 * The arithmetic is the shared one - a tooltip is a layer hung off a control
 * like any other, and it had its own copy of this until the copies were counted.
 * What stays here is the tooltip's own choice of gutter and offset.
 */
export function placeTooltip(
  trigger: LayerRect,
  box: LayerSize,
  viewport: LayerSize,
  align: LayerAlign,
): { left: number; top: number } {
  const { left, top } = placeLayer(trigger, box, viewport, {
    align,
    gutter: GUTTER,
    offset: OFFSET,
    side: 'below',
  });

  return { left, top };
}

export const tooltip: Action<HTMLElement, TooltipOptions> = (node, initial) => {
  let options = initial;
  let box: HTMLSpanElement | null = null;

  const place = (): void => {
    if (box === null) return;
    const { left, top } = placeTooltip(
      node.getBoundingClientRect(),
      box.getBoundingClientRect(),
      { height: window.innerHeight, width: window.innerWidth },
      options.align ?? 'end',
    );
    box.style.left = `${left}px`;
    box.style.top = `${top}px`;
  };

  const show = (): void => {
    if (box !== null) return;
    const next = document.createElement('span');
    next.className = 'app-tooltip';
    if (options.id !== undefined) next.id = options.id;
    next.setAttribute('role', 'tooltip');
    next.textContent = options.text;
    // A modal <dialog> paints in the top layer, which no z-index can reach over,
    // so a tooltip explaining something inside one has to live in it. Otherwise
    // the shell, whose class carries the palette root mode repaints.
    (node.closest('dialog') ?? node.closest('.app-shell') ?? document.body).appendChild(next);
    box = next;
    place();
    // A node cannot transition from the style it was born with, so the reveal
    // waits a frame. It is already placed by then, never flashing at 0,0.
    requestAnimationFrame(() => {
      next.classList.add('is-open');
    });
    // Capture: the scroller that moves the trigger is usually an inner one - a
    // pinned table body, a dialog - and those do not bubble a scroll event.
    window.addEventListener('scroll', place, true);
    window.addEventListener('resize', place);
  };

  const hide = (): void => {
    if (box === null) return;
    window.removeEventListener('scroll', place, true);
    window.removeEventListener('resize', place);
    box.remove();
    box = null;
  };

  node.addEventListener('pointerenter', show);
  node.addEventListener('pointerleave', hide);
  node.addEventListener('focus', show);
  node.addEventListener('blur', hide);

  return {
    update: (next: TooltipOptions) => {
      options = next;
      if (box === null) return;
      box.id = next.id ?? '';
      box.textContent = next.text;
      place();
    },
    destroy: () => {
      hide();
      node.removeEventListener('pointerenter', show);
      node.removeEventListener('pointerleave', hide);
      node.removeEventListener('focus', show);
      node.removeEventListener('blur', hide);
    },
  };
};
