<script lang="ts">
  import { plainClick } from '#lib/follow.js';
  import { revealInline } from '#lib/reveal-inline.js';
  import { tick } from 'svelte';

  /**
   * Navigation tabs for the sub-views of one feature area: every tab is an
   * address, rendered as a real link carrying `aria-current="page"`.
   *
   * The open tab says it twice - weight and the ink bar - because one
   * indicator on two tabs is unreadable, and the bar is deliberately NOT the
   * brand colour: "you are here" is not "you can act". Hover previews the
   * same affordance in the border's voice, and the hover bar never moves the
   * active one. Each label reserves its bold width, so selection moving
   * never shifts the strip.
   *
   * At rest the bar is the width of the label, which is what makes it read
   * as naming the word rather than filling the tab. Under the pointer it
   * spreads to the whole padded box - the width the hover ground covers -
   * and it does that by growing, so the reach of the target is shown rather
   * than stated. Leaving takes it back to the word the same way.
   *
   * The bar is measured from the DOM rather than computed from CSS, and
   * hidden until the first measurement lands so it never animates in from
   * nowhere. It travels between siblings with the sidebar thumb's own curve,
   * turned on its side: a wind-up away from where it is going, a landing
   * past it, and a settle back. `scaleX` rides along with the shift so a bar
   * arriving at a wider tab grows into it rather than snapping to the new
   * width and then sliding. A spread is the same movement with the plainer
   * easing - it is not going anywhere, only opening.
   *
   * This is one of the three strip-shaped controls and the only one that
   * navigates: `SegmentedControl` filters what is already on screen, and a
   * `Switch` is a setting. Never mix jobs inside one strip.
   */
  interface SectionTab {
    id: string;
    label: string;
    href: string;
    /** A quiet neutral figure beside the label - open issues, plan size. */
    count?: string;
    /** Only for a count that waits on the reader; it speaks the info tone. */
    signal?: boolean;
  }

  const {
    items,
    active,
    label,
    onNavigate,
  }: {
    items: readonly SectionTab[];
    active: string;
    /** Names the strip for assistive tech - "Sync sections". */
    label: string;
    /** SPA navigation; the href stays real for middle-click and copy. */
    onNavigate?: (id: string) => void;
  } = $props();

  let nav = $state<HTMLElement | null>(null);
  let rail = $state<HTMLElement | null>(null);
  let bar = $state({ left: 0, width: 0 });
  /* Where the bar is, kept a second time outside the rune. `place` runs inside
     the effect and needs the rect it is about to replace, and reading the state
     it writes makes the effect its own dependency - which is the panel's oldest
     footgun, and here it was an update-depth crash on the first paint. */
  let resting = { left: 0, width: 0 };
  /**
   * The tab under the pointer, by id.
   *
   * An id rather than a flag, because the tab that is hovered when a click
   * lands becomes the open one: the bar has to arrive already spread, and no
   * second `pointerenter` is coming to tell it so.
   */
  let hovered = $state<string | null>(null);
  /** The tab the bar is parked under, so a re-measure is told from a move. */
  let parked: string | null = null;
  /** Whether it is parked spread, so a spread is told from a stay. */
  let opened = false;
  let travelling: Animation | null = null;

  type Motion = 'none' | 'travel' | 'spread';

  const still = (): boolean => window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  /**
   * Move the bar from where it was to where it now is.
   *
   * The rect is already written when this runs, so the animation starts by
   * putting the bar back: `translateX` restores the old left edge and `scaleX`
   * the old width, both against a left origin, and the two unwind together.
   *
   * The multipliers are the sidebar thumb's, on the other axis. Each one is a
   * fraction of the distance still to cover, so `1` is where it started and `0`
   * is the destination - which makes the first step (1.055) a wind-up away from
   * the target and the third (-0.085) a landing past it.
   */
  function travel(
    from: { left: number; width: number },
    to: { left: number; width: number },
    motion: Motion,
  ): void {
    const shift = from.left - to.left;
    const squeeze = to.width === 0 ? 1 : from.width / to.width;
    if (rail === null || still() || motion === 'none') return;
    if (Math.abs(shift) < 1 && Math.abs(squeeze - 1) < 0.01) return;
    const at = (left: number): string =>
      `translateX(${shift * left}px) scaleX(${squeeze + (1 - squeeze) * (1 - left)})`;
    travelling?.cancel();
    travelling =
      motion === 'travel'
        ? rail.animate(
            [
              { transform: at(1), easing: 'cubic-bezier(0.5, 0, 0.8, 0.2)' },
              { offset: 0.1, transform: at(1.055), easing: 'cubic-bezier(0.25, 0, 0.15, 1)' },
              { offset: 0.62, transform: at(-0.085), easing: 'ease-out' },
              { offset: 0.82, transform: at(0.02), easing: 'ease-out' },
              { transform: at(0) },
            ],
            { duration: 280, fill: 'none' },
          )
        : rail.animate([{ transform: at(1) }, { transform: at(0) }], {
            duration: 220,
            easing: 'cubic-bezier(0.16, 1, 0.3, 1)',
            fill: 'none',
          });
    const mine = travelling;
    const settle = (): void => {
      if (travelling === mine) travelling = null;
    };
    mine.addEventListener('finish', settle);
    mine.addEventListener('cancel', settle);
  }

  /**
   * The rect the bar is painting right now, mid-flight included.
   *
   * A move that interrupts another has to start from what is on screen rather
   * than from where the last one was aiming: hovering off a tab within the
   * 280ms of a switch would otherwise snap the bar to the tab it had not
   * finished arriving at, and then animate from there. Against a left origin
   * the painted rect is the parked one with `translateX` added and `scaleX`
   * multiplied, which is the whole conversion.
   */
  function shown(): { left: number; width: number } {
    if (rail === null || travelling === null) return resting;
    const matrix = new DOMMatrixReadOnly(getComputedStyle(rail).transform);

    return { left: resting.left + matrix.e, width: resting.width * matrix.a };
  }

  /**
   * A tab's ink: the label, and the count beside it where it carries one.
   *
   * The count is part of what the tab says - "Plan 16" is one phrase - so the
   * bar covers both. Under the word alone it stopped short of the pill and read
   * as underlining half a name.
   */
  function inkOf(link: HTMLElement): DOMRect | null {
    const word = link.querySelector<HTMLElement>('.tab-word');
    if (word === null) return null;
    const box = word.getBoundingClientRect();
    const count = link.querySelector<HTMLElement>('.tab-count');
    if (count === null) return box;
    const tail = count.getBoundingClientRect();

    return new DOMRect(box.left, box.top, tail.right - box.left, box.height);
  }

  /**
   * Tell every tab how much of itself its label is, and where.
   *
   * The preview bar under an unopened tab does what the open one's does - it
   * starts at the word and opens to the box - and it is a pseudo-element on the
   * link, so it is already the box's width. A share scales it down to the word,
   * and an origin puts that share over the word rather than over the middle:
   * a tab carrying a count has its label left of centre, and scaling from the
   * middle would sit the resting bar half under the pill.
   */
  function share(): void {
    for (const link of nav?.querySelectorAll<HTMLElement>('a') ?? []) {
      const ink = inkOf(link);
      if (ink === null) continue;
      const box = link.getBoundingClientRect();
      if (box.width === 0) continue;
      link.style.setProperty('--word-share', String(ink.width / box.width));
      const middle = ((ink.left + ink.width / 2 - box.left) / box.width) * 100;
      link.style.setProperty('--word-middle', `${middle}%`);
    }
  }

  /**
   * Measure the open tab and put the bar under it - the label, or the whole
   * padded box while the pointer is on it.
   *
   * A resize, or fonts landing, is the same tab in a new place: it re-measures
   * with no motion at all and the bar is simply there, exactly as the sidebar's
   * thumb does.
   */
  async function place(motion: Motion, spread: boolean): Promise<void> {
    const link = nav?.querySelector<HTMLElement>("[aria-current='page']");
    if (link === null || link === undefined || nav === null) return;
    /* The word reserves its bold width, so the ink is the same rect whether or
       not the label is currently the bold one - which is what keeps a spread
       from measuring one width on the way out and another on the way back. */
    const ink = inkOf(link);
    const box = spread || ink === null ? link.getBoundingClientRect() : ink;
    if (box.width === 0) return;
    if (motion !== 'spread') share();
    const before = shown();
    resting = {
      left: box.left - nav.getBoundingClientRect().left + nav.scrollLeft,
      width: box.width,
    };
    bar = resting;
    if (motion === 'none' || before.width === 0) return;
    // The rect is written by the template, so the bar has to hold its new size
    // before an animation can start by undoing it.
    await tick();
    travel(before, resting, motion);
  }

  /** Keep selection visible when selection or available tabs change, never on hover. */
  function revealCurrent(): void {
    const link = nav?.querySelector<HTMLElement>("[aria-current='page']");
    if (link === null || link === undefined || nav === null) return;
    revealInline(nav, link);
  }

  $effect(() => {
    const current = active;
    const currentNav = nav;
    void items;
    if (currentNav === null) return;
    void current;
    revealCurrent();
    const scrollAtRegistration = currentNav.scrollLeft;
    let currentEffect = true;
    // Fonts can move the selected tab after its first measurement. Preserve any
    // scroll the reader made while they were loading; only the bar needs a new rect.
    void document.fonts?.ready.then(() => {
      if (!currentEffect) return;
      if (currentNav.scrollLeft === scrollAtRegistration) revealCurrent();
      void place('none', hovered === active);
    });
    return () => {
      currentEffect = false;
    };
  });

  $effect(() => {
    const current = active;
    /* The open tab is the only one whose hover the bar answers: the others draw
       their own preview, and a bar that left the open tab to follow a pointer
       would stop saying where the reader is. */
    const spread = hovered === current;
    void items;
    const motion: Motion =
      parked === null || parked === current ? (spread === opened ? 'none' : 'spread') : 'travel';
    parked = current;
    opened = spread;
    void place(motion, spread);
    /* Nothing is cancelled here on the way out. The cleanup runs before every
       re-run as well as on unmount, and cancelling there would put the bar back
       to its parked rect a frame before `shown` reads where it actually is -
       which is the one thing that reading it was for. `travel` cancels its own
       predecessor, and an animation on a removed element stops mattering. */
  });

  function resize(): void {
    revealCurrent();
    void place('none', hovered === active);
  }
</script>

<svelte:window onresize={resize} />

<nav class="section-tabs" aria-label={label} bind:this={nav}>
  <ul>
    {#each items as item (item.id)}
      <li>
        <a
          href={item.href}
          aria-current={item.id === active ? 'page' : undefined}
          onmouseenter={() => (hovered = item.id)}
          onmouseleave={() => {
            if (hovered === item.id) hovered = null;
          }}
          onclick={(event) => {
            if (onNavigate === undefined || !plainClick(event)) return;
            event.preventDefault();
            onNavigate(item.id);
          }}
        >
          <span class="tab-word" data-word={item.label}
            ><span class="cap-trim">{item.label}</span></span
          >
          {#if item.count !== undefined}
            <span class="tab-count" class:is-signal={item.signal === true}>
              <span class="cap-trim">{item.count}</span>
            </span>
          {/if}
        </a>
      </li>
    {/each}
  </ul>
  {#if bar.width > 0}
    <span
      class="section-tabs-bar"
      style:left="{bar.left}px"
      style:width="{bar.width}px"
      bind:this={rail}
      aria-hidden="true"
    ></span>
  {/if}
</nav>

<style>
  /* The rule is drawn as a shadow rather than a border, because it is the seam
     under the strip and not the underside of it. A border joins the nav's own
     box, which then stands a half-pixel lower than its words - and beside a
     search field the grid centres those boxes, so the tab labels read 1.30px
     high against the field they sit next to. */
  .section-tabs {
    box-shadow: 0 1px 0 var(--border-subtle);
    max-width: 100%;
    min-width: 0;
    overflow-x: auto;
    position: relative;
  }

  /* The gap shrinks by what the links now take as padding, so the distance
     between two labels is the 1.25rem it has always been - the padding moved
     inside the target rather than adding to the strip. */
  ul {
    display: flex;
    gap: var(--space-1);
    list-style: none;
    margin: 0;
    padding: 0;
  }

  /*
   * The whole padded box is the target.
   *
   * It used to be the text and nothing else: the pointer changed to a hand only
   * over the letters, and the gap between two tabs answered neither of them.
   * The bar underneath still hugs the label rather than this box - it is
   * measured from `.tab-word` - which is the one part of this that should not
   * grow with the target.
   */
  a {
    align-items: center;
    border-radius: var(--r-ctl) var(--r-ctl) 0 0;
    color: var(--tab-muted);
    display: flex;
    gap: 0.45rem;
    font-size: var(--font-size-meta);
    /* Symmetric down the block axis, so the label sits on the middle of the
       strip's own box and therefore on the middle of anything the strip shares
       a row with. */
    padding: 0.7rem var(--space-2);
    position: relative;
    text-decoration: none;
    transition: background-image var(--duration-fast) var(--ease-standard);
  }

  /* The app's own two states, over whatever ground the strip sits on. Square at
     the bottom, because that edge is the seam the bar rides. */
  a:hover {
    background-image: linear-gradient(
      var(--interactive-hover-layer),
      var(--interactive-hover-layer)
    );
  }

  a:active {
    background-image: linear-gradient(var(--press), var(--press));
  }

  /* The reserved bold: a hidden copy at weight 600 sets the width, so the
     visible word can change weight without moving its neighbours. */
  .tab-word {
    display: inline-grid;
  }

  .tab-word::before {
    content: attr(data-word);
    font-weight: 600;
    grid-area: 1 / 1;
    visibility: hidden;
  }

  .tab-word > span {
    align-self: center;
    grid-area: 1 / 1;
  }

  a:hover,
  a:active {
    color: var(--tab-ink);
  }

  /* The preview under an unopened tab, doing what the open tab's bar does: it
     rests at the label and opens to the whole padded box, which is the width
     the hover ground covers. Painted on the link rather than on the word,
     because that is the box it has to be able to reach; the share and the
     origin, measured in script, are what hold it over the word until then. */
  a::after {
    background: var(--tab-indicator-hover);
    border-radius: 2px 2px 0 0;
    bottom: -1px;
    content: '';
    height: 2px;
    inset-inline: 0;
    opacity: 0;
    position: absolute;
    transform: scaleX(var(--word-share, 0.6));
    transform-origin: var(--word-middle, 50%) 50%;
    transition:
      opacity var(--duration-fast) var(--ease-standard),
      transform var(--duration-normal) var(--ease-standard);
  }

  a:hover::after {
    opacity: 1;
    transform: scaleX(1);
  }

  a[aria-current='page'] {
    color: var(--tab-ink);
    font-weight: 600;
  }

  /* The open tab already has a bar, in the ink that means "here". */
  a[aria-current='page']::after {
    content: none;
  }

  a:focus-visible {
    border-radius: 4px;
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }

  /* Placed by measurement and moved by animation, so neither `left` nor `width`
     is transitioned: a transition would race the keyframes, which start by
     undoing exactly the rect that was just written. The left origin is what
     makes that undoing exact - `translateX` then puts the left edge back and
     `scaleX` the width, with no third term for the centre drifting. */
  .section-tabs-bar {
    background: var(--tab-indicator);
    border-radius: 2px 2px 0 0;
    bottom: -1px;
    height: 2px;
    position: absolute;
    transform-origin: 0 50%;
  }

  /* A flex container so the trimmed figure inside is a flex item and not an
     inline box on a strut: a strut's half-leading is not the same above the cap
     as below the baseline, which put the figure 0.52px above the middle of its
     own pill. As a flex item the trimmed box IS the band, and the equal padding
     centres it. */
  .tab-count {
    align-items: center;
    background: var(--tab-count-bg);
    border-radius: 6px;
    color: var(--tab-count-ink);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    line-height: 1;
    padding: 0.22rem 0.4rem;
  }

  .tab-count.is-signal {
    background: var(--info-tint);
    color: var(--info);
    font-weight: 500;
  }

  /* The travel is refused in script; this is the hover bar's own growth, which
     becomes an appearance rather than a movement. */
  @media (prefers-reduced-motion: reduce) {
    a,
    a::after {
      transition: none;
    }

    a::after {
      transform: none;
    }
  }
</style>
