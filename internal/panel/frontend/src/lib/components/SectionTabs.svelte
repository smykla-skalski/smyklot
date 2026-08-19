<script lang="ts">
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
   * The bar hugs the active label's own width and slides between siblings.
   * It is measured from the DOM rather than computed from CSS, and hidden
   * until the first measurement lands so it never animates in from nowhere.
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
  let bar = $state({ left: 0, width: 0 });

  const measure = () => {
    const word = nav?.querySelector<HTMLElement>("[aria-current='page'] .tab-word");
    if (word === null || word === undefined || nav === null) return;
    const navBox = nav.getBoundingClientRect();
    const box = word.getBoundingClientRect();
    if (box.width === 0) return;
    bar = { left: box.left - navBox.left, width: box.width };
  };

  $effect(() => {
    void active;
    void items;
    measure();
    // Fonts settle after first paint and the cap width moves with them.
    void document.fonts?.ready.then(measure);
  });
</script>

<svelte:window onresize={measure} />

<nav class="section-tabs" aria-label={label} bind:this={nav}>
  <ul>
    {#each items as item (item.id)}
      <li>
        <a
          href={item.href}
          aria-current={item.id === active ? 'page' : undefined}
          onclick={(event) => {
            if (onNavigate === undefined) return;
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
    position: relative;
  }

  ul {
    display: flex;
    gap: var(--space-5);
    list-style: none;
    margin: 0;
    padding: 0;
  }

  a {
    align-items: center;
    color: var(--tab-muted);
    display: flex;
    gap: 0.45rem;
    font-size: var(--font-size-meta);
    /* Symmetric, so the label sits on the middle of the strip's own box and
       therefore on the middle of anything the strip shares a row with. */
    padding: 0.7rem 0;
    position: relative;
    text-decoration: none;
  }

  /* The reserved bold: a hidden copy at weight 600 sets the width, so the
     visible word can change weight without moving its neighbours. */
  .tab-word {
    display: inline-grid;
    position: relative;
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

  a:hover {
    color: var(--tab-ink);
  }

  /* The hover preview hugs the label's own width, exactly like the active
     bar - one affordance, one geometry. On the word, not the link: the link
     also holds the count pill and a bar under a pill promises nothing. */
  a:hover .tab-word::after {
    background: var(--tab-indicator-hover);
    border-radius: 2px 2px 0 0;
    content: '';
    height: 2px;
    inset-inline: 0;
    position: absolute;
    top: calc(100% + 0.7rem - 1px);
  }

  a[aria-current='page'] {
    color: var(--tab-ink);
    font-weight: 600;
  }

  a[aria-current='page']:hover .tab-word::after {
    content: none;
  }

  a:focus-visible {
    border-radius: 4px;
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }

  .section-tabs-bar {
    background: var(--tab-indicator);
    border-radius: 2px 2px 0 0;
    bottom: -1px;
    height: 2px;
    position: absolute;
    transition:
      left var(--duration-normal) var(--ease-standard),
      width var(--duration-normal) var(--ease-standard);
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

  @media (prefers-reduced-motion: reduce) {
    .section-tabs-bar {
      transition: none;
    }
  }
</style>
