/**
 * Optical alignment between a mark (a dot, an icon slot, a help glyph) and the
 * word beside it.
 *
 * A label trimmed to `cap alphabetic` has a box that runs cap-top to baseline,
 * so flex centring puts the mark on the middle of the CAP band. That is exact
 * for a word whose ink stops at the baseline - "Fresh", "1 failure",
 * "Enablement" - and 1.25px out at 12px for one with a descender - "Healthy",
 * "Bypassed", "Sync now" - because the descender pulls the word's visual centre
 * down. Measured on this face: cap 0.745em, descender 0.22em, so the ink centre
 * of a descender word sits (0.745 - 0.22) / 2 = 0.11em below the cap centre.
 *
 * CSS cannot know which word it is about to render, so this does: it reads the
 * text and sets `--ink-nudge` for that element's subtree only. Every mark in the
 * product translates by that variable, so all of them follow the same rule.
 */

/** Glyphs that put ink below the baseline in this product's faces. */
const DESCENDERS = /[gjpqy]/;

export function inkDescends(text: string): boolean {
  return DESCENDERS.test(text);
}

/**
 * Svelte action. Put it on the element that holds BOTH the mark and its label -
 * the chip, the label row, the button - not on the mark itself.
 */
export function inkAlign(node: HTMLElement): { destroy: () => void } {
  const apply = (): void => {
    node.classList.toggle('ink-descends', inkDescends(node.textContent ?? ''));
  };
  apply();
  const observer = new MutationObserver(apply);
  observer.observe(node, { characterData: true, childList: true, subtree: true });
  return {
    destroy: () => {
      observer.disconnect();
    },
  };
}
