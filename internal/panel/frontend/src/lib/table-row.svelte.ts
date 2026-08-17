/**
 * A table row you can press.
 *
 * Two tables grew this behaviour independently and a third is about to, and each
 * copy had to rediscover the same two traps:
 *
 *  - `:active` matches a `<tr>` but does not repaint it, so the held-down state
 *    has to be carried as a class the row can be styled by.
 *  - A virtualised row is positioned with a transform, and an INLINE transform
 *    cannot be added to - so a press scale written in CSS silently never lands.
 *    The offset therefore travels as `--row-y` and the transform is composed in
 *    the stylesheet, where the press can extend it.
 *
 * Both are solved once here and in `.data-row` in `app.css`. A table opts a row
 * in with the class and this action; everything else it already draws stays its
 * own business.
 */

/**
 * Everything in a row that is already something to press.
 *
 * `label` is in this list because of what an enablement column is made of: a
 * radio and a label per option, and a click on the WORD has the label as its
 * target. Without it, one press both opened the row and moved the switch. `a` is
 * here so a name in the first cell stays an ordinary link the router handles,
 * which is what makes a modified click open a new tab.
 */
export const ROW_CONTROLS =
  'a, button, input, label, select, summary, textarea, [role="button"], [role="menu"]';

/** Whether an event inside a row landed on something that is already a control. */
export function onRowControl(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(ROW_CONTROLS) !== null;
}

/**
 * A press anywhere else in the row opens it.
 *
 * A modified click is the reader asking for a new tab, and a row that carries a
 * link can give them one, so it is left to the browser.
 */
export function rowOpensOn(event: MouseEvent): boolean {
  if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
    return false;
  }

  return !onRowControl(event.target);
}

/**
 * Marks the row while a pointer holds it down.
 *
 * The class is added here rather than bound through component state because it
 * belongs to the element, not to the list: a virtualised table reuses rows, and
 * an index-keyed `pressedRow` follows whichever record lands in that slot next.
 */
export function pressableRow(node: HTMLElement): { destroy: () => void } {
  const hold = (event: PointerEvent): void => {
    if (onRowControl(event.target)) return;
    node.classList.add('pressing');
  };
  const release = (): void => node.classList.remove('pressing');

  node.addEventListener('pointerdown', hold);
  node.addEventListener('pointerup', release);
  node.addEventListener('pointercancel', release);
  node.addEventListener('pointerleave', release);

  return {
    destroy() {
      node.removeEventListener('pointerdown', hold);
      node.removeEventListener('pointerup', release);
      node.removeEventListener('pointercancel', release);
      node.removeEventListener('pointerleave', release);
    },
  };
}
