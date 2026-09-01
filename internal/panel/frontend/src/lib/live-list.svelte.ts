import { MediaQuery } from 'svelte/reactivity';
import { untrack } from 'svelte';
import type { TransitionConfig } from 'svelte/transition';

/**
 * A list that changes while somebody is reading it.
 *
 * Two overview cards draw the queue as it moves: the workspace's Active work and the
 * console's Queue. The rows in them are the only thing on either page that rearranges
 * itself - a deadline runs out, the service acts, the stream says so - and without
 * these two rules that is a page that shifts under the reader's hands.
 *
 * The rules come from the queue's own table, which learned them first. What is here is
 * the pair of them, said once:
 *
 *  - **Nothing moves while it is being read.** A pointer or focus inside the list holds
 *    the arrangement as it stood the moment reading began. Rows still say something
 *    true; they just say it without moving.
 *  - **Nothing teleports.** A row that arrives fades in, and a row that leaves gives its
 *    height back over the length of the fade, so the rows below it follow rather than
 *    snap up. A card whose rows vanish outright drops its own height in one frame, and
 *    every card below it jumps - which is how a reader loses the line they were on in a
 *    card they were not even looking at.
 *
 * Deliberately WITHOUT `animate:flip`, which the queue's own table can afford and these
 * cannot: Svelte takes an outroing row out of flow so its siblings can be flipped into
 * place, and out of flow the row hands back its whole height at once - the jump this
 * exists to prevent, delivered by the thing meant to smooth it.
 */

/* Built on first read rather than at import. `MediaQuery` asks the window for
   `matchMedia` the moment it exists, and this module also carries `collapse`, which is a
   pure function of an element's own box - importing it to measure that should not
   require a window that can answer media queries. */
let stillness: MediaQuery | undefined;

function still(): boolean {
  stillness ??= new MediaQuery('prefers-reduced-motion: reduce');

  return stillness.current;
}

/**
 * How long each part of the movement takes.
 *
 * Leaving is quicker than arriving and quicker than the slide that follows it: a row
 * that has finished should be out of the way before the rows below close the gap, or
 * the two movements read as one confused one.
 *
 * Under `prefers-reduced-motion` every duration goes to zero rather than the directives
 * coming off. A row still lands where it belongs; it just gets there at once.
 */
export const rowMotion = {
  get arriving() {
    return { duration: still() ? 0 : 260, delay: still() ? 0 : 80 };
  },
  get leaving() {
    return { duration: still() ? 0 : 140 };
  },
};

/**
 * A row leaving: it fades, and it gives back the space it was holding as it goes.
 *
 * A fade alone is what a table can afford, because the rows below it are held in place
 * by the table until the row is gone. A list of blocks cannot: the row keeps its full
 * height until the last frame and then the whole card snaps shut. So every box property
 * that contributes to the row's height comes down with the opacity - `overflow: hidden`
 * because the words inside do not shrink with the box that holds them.
 */
export function collapse(node: Element, { duration = 140, delay = 0 } = {}): TransitionConfig {
  const style = getComputedStyle(node);
  /* A length that does not parse becomes zero rather than `NaN`. `NaN` renders as
     `block-size: NaNpx`, which the browser drops - so the row would hold its full height
     for the whole transition and then snap, which is the fault this exists to prevent,
     arriving silently. */
  const px = (value: string): number => {
    const parsed = Number.parseFloat(value);

    return Number.isFinite(parsed) ? parsed : 0;
  };
  const height = px(style.blockSize);
  const paddingTop = px(style.paddingBlockStart);
  const paddingBottom = px(style.paddingBlockEnd);

  return {
    duration,
    delay,
    css: (t: number) =>
      `overflow: hidden;` +
      /* A grid item's floor is its own content, so `block-size` alone animates a box
         whose track refuses to follow it down. */
      `min-block-size: 0;` +
      `opacity: ${t};` +
      `block-size: ${t * height}px;` +
      `padding-block: ${t * paddingTop}px ${t * paddingBottom}px;`,
  };
}

/**
 * The arrangement a card shows, which is the live one except while it is being read.
 *
 * Constructed with a getter rather than a value, so the caller keeps ownership of where
 * the rows come from and this stays a rule about *when* they may change.
 */
export class LiveList<T> {
  #live: () => readonly T[];
  #key: (item: T) => string;

  /* Three ways a row can be in use. The third cannot be inferred from the first two: a
     menu opens in a portal, so by the time it is showing, neither the pointer nor focus
     is anywhere near the row it belongs to. */
  pointerInside = $state(false);
  focusInside = $state(false);
  menuOpen = $state(false);

  constructor(live: () => readonly T[], key: (item: T) => string) {
    this.#live = live;
    this.#key = key;
  }

  get reading(): boolean {
    return this.pointerInside || this.focusInside || this.menuOpen;
  }

  /**
   * The order as it stood the moment reading began, and `null` the rest of the time.
   *
   * `untrack` on the list, because what is wanted is a snapshot rather than a
   * subscription: read plainly this would recompute on every tick of the page's clock
   * and the card would never be held at all.
   */
  readonly #held = $derived(
    this.reading ? untrack(() => this.#live().map((item) => this.#key(item))) : null,
  );

  get rows(): readonly T[] {
    const live = this.#live();
    const held = this.#held;
    if (held === null) return live;

    /* Scanned rather than indexed: an overview card holds a handful of rows, and a Map
       and a Set built per read would each be a fresh collection on every tick of the
       page's clock. */
    const kept = held
      .map((key) => live.find((item) => this.#key(item) === key))
      .filter((item): item is T => item !== undefined);
    /* What arrived while the card was held goes on the END, in the order it would have
       sorted into. Held back entirely it would be invisible for as long as somebody kept
       reading, and a list that hides new work is worse than one that moves; sorted into
       place it would push the row under the pointer down, which is the whole thing being
       prevented. Appended, it is on screen, it is countable, and it displaces nothing. */

    return [...kept, ...live.filter((item) => !held.includes(this.#key(item)))];
  }

  /** What the list element listens for, so a caller cannot wire up three of the four. */
  get holdAttrs() {
    return {
      onpointerenter: () => (this.pointerInside = true),
      onpointerleave: () => (this.pointerInside = false),
      onfocusin: () => (this.focusInside = true),
      /* Focus has to be checked rather than assumed: `focusout` fires as focus moves
         BETWEEN two rows, and letting go there would re-sort the list under the very key
         press moving through it. */
      onfocusout: (event: FocusEvent) => {
        const leaving = event.currentTarget;
        const next = event.relatedTarget;
        if (leaving instanceof Node && next instanceof Node && leaving.contains(next)) return;
        this.focusInside = false;
      },
    };
  }
}
