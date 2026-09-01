import { MediaQuery } from 'svelte/reactivity';
import type { TransitionConfig } from 'svelte/transition';

/**
 * A card whose contents change while somebody is reading it.
 *
 * Two overview cards draw the queue as it moves: the workspace's Active work and the
 * console's Queue. What they show has to stay current without the page becoming a moving
 * target, and the standards are clear about which way that trade goes.
 *
 * **A row's contents are live. The set of rows is not.** Text, marks and countdowns
 * follow the service as it acts. Rows never enter, leave or reorder on their own -
 * arrivals and departures collect into a count, and the reader presses to take them.
 *
 * Three things fall out of that, and each is a rule this would otherwise have broken:
 *
 *  - **WCAG 2.2.2 Pause, Stop, Hide (Level A)** asks for a way to pause, stop, hide or
 *    slow information that updates itself beside other content. Nothing here updates
 *    itself structurally, so there is nothing to pause: the reader is already the one
 *    who decides when the list moves. Holding the list still while it is hovered - which
 *    is what this used to do - is not that mechanism. It is undiscoverable, it is not a
 *    control, and it only guards the card the pointer happens to be over.
 *  - **Cumulative Layout Shift** counts a shift as unexpected unless it follows within
 *    half a second of something the reader did. Every shift these cards make now follows
 *    a press, which is also why the motion below may animate a box rather than having to
 *    move it with a transform.
 *  - **A live region that changes every second floods a screen reader.** A count that
 *    changes when the queue does, and a list that changes when a person asks, is
 *    something assistive technology can actually narrate.
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
 * Leaving is quicker than arriving: a row that has gone should be out of the way before
 * the rows below close the gap, or the two movements read as one confused one. Both sit
 * inside the 100-500ms every motion guideline converges on - longer reads as waiting,
 * shorter is not seen at all.
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
 * A fade alone is what a table can afford, because the table holds the row's space until
 * it is gone. A list of blocks has nothing holding it, so every box property that
 * contributes to the row's height comes down with the opacity.
 *
 * This only ever runs behind a press, which is what makes animating the box acceptable
 * rather than sloppy: a shift that follows a person's own action is not one the layout
 * owes them an explanation for.
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
 * The rows a card is showing, and how far behind the service they have fallen.
 *
 * Constructed with a getter rather than a value, so the caller keeps ownership of where
 * the rows come from and this stays a rule about *when* the set of them may change.
 */
export class LiveList<T> {
  #live: () => readonly T[];
  #key: (item: T) => string;
  /**
   * The set on screen, which is a plain field rather than `$state` on purpose.
   *
   * Nothing subscribes to it: `rows` and `changed` both re-run when the live list does,
   * which is the only moment either answer can differ. `#taken` is what makes a press
   * reach them, because a press changes neither the live list nor anything else they
   * read.
   */
  #showing: T[] | null = null;
  #taken = $state(0);

  constructor(live: () => readonly T[], key: (item: T) => string) {
    this.#live = live;
    this.#key = key;
  }

  /**
   * What to draw: the set as it was taken, with every row's contents read fresh.
   *
   * A row still on the live list is rendered from the live copy, so its words, its mark
   * and its countdown are the service's own. A row that has left is drawn from the last
   * copy seen of it - it is on screen because the reader has not asked for it to go, and
   * a card cannot show a row it has thrown away.
   */
  get rows(): readonly T[] {
    void this.#taken;
    const live = this.#live();
    /* A card with nothing on it has nothing to protect, so it takes what arrives. This
       is also the first load: the query answers with an empty list before it answers
       with rows, and a card that froze on that would open by announcing that everything
       on it had changed. */
    if (this.#showing === null || this.#showing.length === 0) this.#showing = [...live];

    return this.#showing.map((shown) => this.#find(live, this.#key(shown)) ?? shown);
  }

  /** How many rows have arrived or gone since the reader last took the list. */
  get changed(): number {
    void this.#taken;
    const live = this.#live();
    const showing = this.#showing;
    if (showing === null) return 0;

    const arrived = live.filter((item) => this.#find(showing, this.#key(item)) === undefined);
    const gone = showing.filter((item) => this.#find(live, this.#key(item)) === undefined);

    return arrived.length + gone.length;
  }

  /** Take what the service has now. The one thing that moves the set of rows. */
  refresh(): void {
    this.#showing = [...this.#live()];
    this.#taken += 1;
  }

  /* Scanned rather than indexed: an overview card holds a handful of rows, and a Map
     built per read would be a fresh collection on every tick of the page's clock. */
  #find(within: readonly T[], key: string): T | undefined {
    return within.find((item) => this.#key(item) === key);
  }
}
