<script module lang="ts">
  import type { IconName } from './Icon.svelte';

  /** A section nested under its page: a sidebar row that is an address. */
  export interface SidebarKid {
    id: string;
    label: string;
    href: string;
    active: boolean;
    /** A count that demands attention (a waiting plan) speaks, quietly. */
    count?: number | string;
    signal?: boolean;
    /** This destination contains configuration that has not been saved. */
    dirty?: boolean;
  }

  /** A page of the active console: one row of the tree. */
  export interface SidebarPage {
    id: string;
    label: string;
    icon: IconName;
    href: string;
    active: boolean;
    kids?: readonly SidebarKid[];
    /** This page itself contains configuration that has not been saved. */
    dirty?: boolean;
  }
</script>

<script lang="ts">
  import { tick } from 'svelte';

  import Icon from './Icon.svelte';

  const {
    kicker,
    title,
    pages,
    collapsed,
    onToggleCollapsed,
    onSelectPage,
    onSelectKid,
    label = 'Pages',
  }: {
    /** The console's voice above the tree: "Workspace" / "Root console". */
    kicker: string;
    /** The console's name: the workspace, or the Root console's purpose. */
    title: string;
    pages: readonly SidebarPage[];
    collapsed: boolean;
    onToggleCollapsed: () => void;
    onSelectPage: (page: SidebarPage) => void;
    onSelectKid: (page: SidebarPage, kid: SidebarKid) => void;
    label?: string;
  } = $props();

  const activePage = $derived(pages.find((page) => page.active));
  const activeKid = $derived(activePage?.kids?.find((kid) => kid.active));
  const selectionKey = $derived(`${activePage?.id ?? ''}:${activeKid?.id ?? ''}:${collapsed}`);

  function hasDirtyKid(page: SidebarPage): boolean {
    return page.kids?.some((kid) => kid.dirty === true) ?? false;
  }

  /**
   * A hidden child hands its state to the nearest visible row. Expanded active
   * groups keep the state on the precise child instead; a page's own state is
   * always its own to show.
   */
  function bubblesDirty(page: SidebarPage): boolean {
    return page.dirty === true || ((collapsed || !page.active) && hasDirtyKid(page));
  }

  function plainClick(event: MouseEvent): boolean {
    return !(
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    );
  }

  function pageFromClick(event: MouseEvent, page: SidebarPage): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onSelectPage(page);
  }

  function kidFromClick(event: MouseEvent, page: SidebarPage, kid: SidebarKid): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onSelectKid(page, kid);
  }

  /* Pointer intent for the collapsed flyout: opening is immediate, closing
     waits 300ms - a shallow diagonal that clips the next row on its way to a
     menu item no longer slams the menu shut. Keyboard opens via :focus-within
     in the stylesheet. */
  let flownPage = $state<string | null>(null);
  let flyTimer: ReturnType<typeof setTimeout> | undefined;

  function flyOver(pageId: string): void {
    if (!collapsed) return;
    clearTimeout(flyTimer);
    flownPage = pageId;
  }

  function flyOut(event: PointerEvent): void {
    const wrap = event.currentTarget as HTMLElement;
    if (event.relatedTarget instanceof Node && wrap.contains(event.relatedTarget)) return;
    clearTimeout(flyTimer);
    flyTimer = setTimeout(() => (flownPage = null), 300);
  }

  /**
   * The app's travelling selection, verbatim: one element is the selected
   * row's ground, and switching pages MOVES it - two rows that swap grounds
   * only say something changed; one that slides says where it went.
   *
   * Extended from `ViewTabs.followSelection` for the tree: the target may be
   * a page row (34px, control radius) or a kid (28px, 6px radius), so the
   * thumb morphs width, edge and radius on the segmented control's curve
   * while travel itself stays WAAPI transform.
   */
  function followSelection(node: HTMLElement, current: string) {
    let selection = current;
    let restingTop: number | null = null;
    let travelling: Animation | null = null;
    let fretting: Animation | null = null;
    let fretTimer: ReturnType<typeof setTimeout> | undefined;
    let job = 0;

    const still = (): boolean => window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const thumbOf = (): HTMLElement | null => node.querySelector<HTMLElement>('.nav-thumb');

    /** The row the thumb should ground: the active kid, or - when the kid is
     * away in a closed flyout - its page row. */
    function target(): HTMLElement | null {
      const kid = node.querySelector<HTMLElement>('.tree-kid.is-active');
      if (kid !== null && kid.offsetParent !== null) {
        const kids = kid.closest('.tree-kids');
        if (!(kids instanceof HTMLElement) || getComputedStyle(kids).position !== 'absolute') {
          return kid;
        }
      }
      const active = node.querySelector<HTMLElement>('.tree-page.is-active > .tree-row');
      if (active !== null) return active;
      return node.querySelector<HTMLElement>('.tree-row.is-active');
    }

    function offsetNow(thumb: HTMLElement): number {
      const matrix = new DOMMatrixReadOnly(getComputedStyle(thumb).transform);
      return matrix.f;
    }

    function stopFret(): void {
      clearTimeout(fretTimer);
      fretTimer = undefined;
      fretting?.cancel();
      fretting = null;
    }

    function beginFret(): void {
      const thumb = thumbOf();
      if (thumb === null || still() || travelling !== null) return;
      fretting = thumb.animate(
        [
          { transform: 'translateX(0) rotate(0deg)' },
          { offset: 0.25, transform: 'translateX(-1.6px) rotate(-0.35deg)' },
          { offset: 0.75, transform: 'translateX(1.6px) rotate(0.35deg)' },
          { transform: 'translateX(0) rotate(0deg)' },
        ],
        { duration: 200, iterations: Infinity, easing: 'ease-in-out' },
      );
    }

    function pointerDown(event: PointerEvent): void {
      stopFret();
      const row = (event.target as Element | null)?.closest?.('a');
      if (row === null || row === undefined || row.classList.contains('is-active')) return;
      if (!node.contains(row)) return;
      fretTimer = setTimeout(beginFret, 120);
    }

    function travel(thumb: HTMLElement, distance: number): void {
      if (Math.abs(distance) < 1 || still()) return;
      travelling?.cancel();
      travelling = thumb.animate(
        [
          {
            transform: `translateY(${distance}px) scale(1)`,
            easing: 'cubic-bezier(0.5, 0, 0.8, 0.2)',
          },
          {
            offset: 0.1,
            transform: `translateY(${distance * 1.055}px) scale(0.972)`,
            easing: 'cubic-bezier(0.25, 0, 0.15, 1)',
          },
          {
            offset: 0.62,
            transform: `translateY(${distance * -0.085}px) scale(0.985)`,
            easing: 'ease-out',
          },
          {
            offset: 0.82,
            transform: `translateY(${distance * 0.02}px) scale(1.012)`,
            easing: 'ease-out',
          },
          { transform: 'translateY(0) scale(1)' },
        ],
        { duration: 280, fill: 'none' },
      );
      const mine = travelling;
      const settle = (): void => {
        if (travelling === mine) travelling = null;
      };
      mine.addEventListener('finish', settle);
      mine.addEventListener('cancel', settle);
    }

    async function place(moved: boolean): Promise<void> {
      const mine = ++job;
      await tick();
      if (mine !== job) return;
      const active = target();
      const thumb = thumbOf();
      if (active === null || thumb === null) {
        node.style.setProperty('--nav-thumb-height', '0px');
        node.classList.remove('thumb-ready');
        restingTop = null;
        return;
      }
      /* Rects rather than offsets, which round to whole pixels. The anchor is
         stationary; only its inner visual row moves under the pointer. */
      const origin = node.getBoundingClientRect();
      const bounds = active.getBoundingClientRect();
      const top = bounds.top - origin.top;
      const left = bounds.left - origin.left;
      const surface = active.querySelector<HTMLElement>('.row-visual');
      const radius = getComputedStyle(surface ?? active).borderRadius;
      const parked = node.classList.contains('thumb-ready');
      const from = parked && restingTop !== null ? restingTop + offsetNow(thumb) : top;
      node.style.setProperty('--nav-thumb-top', `${top}px`);
      node.style.setProperty('--nav-thumb-height', `${bounds.height}px`);
      node.style.setProperty('--nav-thumb-left', `${left}px`);
      node.style.setProperty('--nav-thumb-width', `${bounds.width}px`);
      node.style.setProperty('--nav-thumb-radius', radius);
      node.classList.add('thumb-ready');
      restingTop = top;
      if (moved && parked) {
        stopFret();
        travel(thumb, from - top);
      }
    }

    void place(false);
    node.addEventListener('pointerdown', pointerDown);
    node.addEventListener('pointerup', stopFret);
    node.addEventListener('pointercancel', stopFret);
    node.addEventListener('pointerleave', stopFret);
    window.addEventListener('blur', stopFret);
    const resize = new ResizeObserver(() => void place(false));
    resize.observe(node);

    return {
      update(next: string) {
        if (next === selection) return;
        /* The last field is the fold - changing that is a re-measure, not a move. */
        const moved =
          next.split(':').slice(0, -1).join(':') !== selection.split(':').slice(0, -1).join(':');
        selection = next;
        void place(moved);
      },
      destroy() {
        resize.disconnect();
        stopFret();
        travelling?.cancel();
        node.removeEventListener('pointerdown', pointerDown);
        node.removeEventListener('pointerup', stopFret);
        node.removeEventListener('pointercancel', stopFret);
        node.removeEventListener('pointerleave', stopFret);
        window.removeEventListener('blur', stopFret);
      },
    };
  }
</script>

<aside class="side" aria-label={kicker}>
  <div class="side-head">
    <span class="side-kicker">{kicker}</span>
    <span class="side-title">{title}</span>
    <button
      class="side-fold"
      type="button"
      aria-label={collapsed ? 'Expand pages' : 'Collapse pages'}
      aria-expanded={!collapsed}
      onclick={onToggleCollapsed}
    >
      <span class="fold-glyph" class:folded={collapsed}>
        <Icon name="chevron-left" size={14} />
      </span>
    </button>
  </div>
  <nav class="tree" aria-label={label} use:followSelection={selectionKey}>
    <span class="nav-thumb" aria-hidden="true"></span>
    {#each pages as page (page.id)}
      {#if page.kids !== undefined && page.kids.length > 0}
        <!-- The pointer handlers carry hover INTENT for the collapsed flyout,
             not interaction - the links inside are the interactive things. -->
        <div
          class="tree-page"
          class:is-active={page.active}
          class:is-flown={flownPage === page.id}
          class:has-signal={page.kids.some((kid) => kid.signal === true)}
          class:has-dirty={bubblesDirty(page)}
          role="none"
          onpointerover={() => flyOver(page.id)}
          onpointerout={flyOut}
        >
          <a
            class="tree-row"
            class:is-active={page.active && activeKid === undefined}
            href={page.href}
            data-tip={page.label}
            aria-current={page.active && activeKid === undefined ? 'page' : undefined}
            onclick={(event) => pageFromClick(event, page)}
          >
            <span class="row-visual">
              <Icon name={page.icon} size={16} />
              <span class="t">{page.label}</span>
              {#if bubblesDirty(page)}
                <span class="dirty-mark" aria-hidden="true">*</span>
                <span class="visually-hidden">Unsaved changes</span>
              {/if}
            </span>
          </a>
          <!-- Always in the DOM, shown by CSS: an anchor destroyed between a
               pointerdown and its click swallows the press, and these unmount
               exactly when a navigation lands - the moment a second quick
               press is most likely to be in flight. -->
          <div class="tree-kids" data-label={page.label}>
            {#each page.kids as kid (kid.id)}
              <a
                class="tree-kid"
                class:is-active={kid.active}
                class:has-dirty={kid.dirty === true}
                href={kid.href}
                aria-current={kid.active ? 'page' : undefined}
                onclick={(event) => kidFromClick(event, page, kid)}
              >
                <span class="row-visual">
                  <span class="t">{kid.label}</span>
                  {#if kid.count !== undefined}
                    <span class="tab-count" class:is-signal={kid.signal === true}>
                      <span class="t">{kid.count}</span>
                    </span>
                  {/if}
                  {#if kid.dirty === true}
                    <span class="dirty-mark" aria-hidden="true">*</span>
                    <span class="visually-hidden">Unsaved changes</span>
                  {/if}
                </span>
              </a>
            {/each}
          </div>
        </div>
      {:else}
        <a
          class="tree-row"
          class:is-active={page.active}
          class:has-dirty={page.dirty === true}
          href={page.href}
          data-tip={page.label}
          aria-current={page.active ? 'page' : undefined}
          onclick={(event) => pageFromClick(event, page)}
        >
          <span class="row-visual">
            <Icon name={page.icon} size={16} />
            <span class="t">{page.label}</span>
            {#if page.dirty === true}
              <span class="dirty-mark" aria-hidden="true">*</span>
              <span class="visually-hidden">Unsaved changes</span>
            {/if}
          </span>
        </a>
      {/if}
    {/each}
  </nav>
</aside>

<style>
  .side {
    align-self: start;
    background: var(--sidebar-bg);
    border-inline-end: 1px solid var(--sidebar-border);
    box-sizing: border-box;
    color: var(--sidebar-text);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    height: 100dvh;
    inline-size: var(--sidebar-width);
    overflow: hidden auto;
    /* Structure moves at --duration-normal, like the app shell's own columns. */
    transition: inline-size var(--duration-normal) var(--ease-standard);
    padding: var(--space-4) 12px;
    position: sticky;
    top: 0;
  }

  .side-head {
    align-items: center;
    display: grid;
    gap: 0.5rem;
    grid-template-columns: 1fr auto;
    padding-inline: 10px;
  }

  .side-kicker {
    color: var(--sidebar-text-muted);
    font-size: 0.625rem;
    font-weight: 600;
    grid-column: 1;
    letter-spacing: 0.09em;
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .side-title {
    font-size: var(--font-size-meta);
    font-weight: 650;
    grid-column: 1;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  /* The fold control: 24px square, sidebar material, spanning both text rows. */
  .side-fold {
    align-items: center;
    background: none;
    block-size: 24px;
    border: 0;
    border-radius: 6px;
    color: var(--sidebar-text-muted);
    cursor: pointer;
    display: inline-flex;
    grid-column: 2;
    grid-row: 1 / 3;
    inline-size: 24px;
    justify-content: center;
    padding: 0;
    transition: none;
  }

  .side-fold:hover {
    background: var(--sidebar-item-hover);
    box-shadow: 0 1px 0 var(--sidebar-border);
    color: var(--sidebar-text);
  }

  .side-fold:active {
    background: var(--sidebar-item-hover);
    box-shadow: none;
    translate: 0 1px;
  }

  .fold-glyph {
    display: inline-flex;
  }

  .fold-glyph.folded {
    scale: -1 1;
  }

  .tree {
    display: grid;
    gap: 2px;
    position: relative;
  }

  /* The selected row's ground, parked by `followSelection` and flown with the
     app's own keyframes. Height, width, edge and radius morph on the
     segmented control's curve; travel itself is WAAPI transform - `top`
     deliberately never eases. */
  .nav-thumb {
    background: var(--sidebar-thumb);
    block-size: var(--nav-thumb-height, 0px);
    border-radius: var(--nav-thumb-radius, var(--radius-control));
    box-shadow: var(--sidebar-thumb-shadow);
    inline-size: var(--nav-thumb-width, 100%);
    inset-block-start: var(--nav-thumb-top, 0px);
    inset-inline-start: var(--nav-thumb-left, 0px);
    pointer-events: none;
    position: absolute;
    transition:
      block-size 240ms cubic-bezier(0.22, 1, 0.36, 1),
      inline-size 240ms cubic-bezier(0.22, 1, 0.36, 1),
      inset-inline-start 240ms cubic-bezier(0.22, 1, 0.36, 1),
      border-radius 240ms cubic-bezier(0.22, 1, 0.36, 1);
  }

  .tree:not(:global(.thumb-ready)) .nav-thumb {
    display: none;
  }

  /* The thumb is one physical pixel tall: pressing consumes its hard edge and
     moves its face by exactly that amount. The anchor hit target never moves. */
  .tree:has(.tree-row.is-active:active, .tree-kid.is-active:active) .nav-thumb {
    box-shadow: var(--sidebar-thumb-shadow-pressed);
    translate: 0 1px;
  }

  .tree-row {
    /* Declared, whole: a row's height is a decision, not an outcome. */
    block-size: 34px;
    box-sizing: border-box;
    color: var(--sidebar-text-secondary);
    display: block;
    font-size: var(--font-size-meta);
    font-weight: 500;
    position: relative;
    text-decoration: none;
    transition: none;
  }

  .tree-row .t {
    text-box: trim-both cap alphabetic;
  }

  /* The anchor stays rectangular and stationary while this inner row carries
     the original rounded ground, ink, and movement. A press can no longer move
     the link out from under a pointer held on its edge. */
  .row-visual {
    align-items: center;
    background: transparent;
    display: flex;
    inset: 0;
    pointer-events: none;
    position: absolute;
    transition: none;
  }

  .tree-row > .row-visual {
    border-radius: var(--radius-control);
    gap: 10px;
    padding: 0 10px;
  }

  .tree-kid > .row-visual {
    border-radius: 6px;
    gap: 0.5rem;
    padding: 0 9px;
  }

  .tree-row:focus-visible,
  .tree-kid:focus-visible {
    outline: none;
  }

  /* The app-wide link press paints and scales the anchor itself. Sidebar
     anchors are stationary hit targets, so their rounded inner row owns the
     entire visual response instead. */
  a.tree-row:active,
  a.tree-kid:active {
    background-image: none;
    transform: none;
  }

  .tree-row:focus-visible > .row-visual,
  .tree-kid:focus-visible > .row-visual {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }

  .tree-row:hover {
    color: var(--sidebar-text);
  }

  .tree-row:hover > .row-visual {
    background: var(--sidebar-item-hover);
    box-shadow: 0 1px 0 var(--sidebar-border);
  }

  .tree-row:active > .row-visual {
    background: var(--sidebar-item-hover);
    box-shadow: none;
    translate: 0 1px;
  }

  .tree-page.is-active > .tree-row {
    color: var(--sidebar-text);
    font-weight: 600;
  }

  .tree-row.is-active {
    /* Ink only - the ground is the nav-thumb parked underneath. */
    color: var(--sidebar-item-active-text);
    font-weight: 600;
  }

  .tree-row.is-active:hover > .row-visual {
    background: var(--interactive-hover-layer);
    box-shadow: none;
  }

  .tree-row.is-active:active > .row-visual {
    background: var(--interactive-hover-layer);
    translate: 0 1px;
  }

  /* Sections: nested under their page, indented to the label's start, a
     hairline guide holding the group together. The active section wears the
     thumb; its page row only carries weight. */
  .tree-kids {
    border-inline-start: 1px solid var(--sidebar-border);
    /* Hidden, never unmounted - see the template note. */
    display: none;
    /* 2px, not 1: the thumb's half-pixel ring eats a 1px seam. */
    gap: 2px;
    /* 16, not 18: the guide starts on the page rows' icon centre, so the
       thread visibly hangs from the glyph column. */
    margin: 2px 0 4px 16px;
    padding-inline-start: 12px;
  }

  .tree-page.is-active > .tree-kids {
    display: grid;
  }

  .tree-kid {
    block-size: 28px;
    box-sizing: border-box;
    color: var(--sidebar-text-secondary);
    display: block;
    font-size: var(--font-size-meta);
    position: relative;
    text-decoration: none;
    transition: none;
  }

  .tree-kid .t {
    text-box: trim-both cap alphabetic;
  }

  .tree-kid:hover {
    color: var(--sidebar-text);
  }

  .tree-kid:hover > .row-visual {
    background: var(--sidebar-item-hover);
    box-shadow: 0 1px 0 var(--sidebar-border);
  }

  .tree-kid:active > .row-visual {
    background: var(--sidebar-item-hover);
    box-shadow: none;
    translate: 0 1px;
  }

  .tree-kid.is-active {
    /* Ink only - the thumb slides under the kid the same as under a page. */
    color: var(--sidebar-item-active-text);
    font-weight: 600;
  }

  .tree-kid.is-active:hover > .row-visual {
    background: var(--interactive-hover-layer);
    box-shadow: none;
  }

  .tree-kid.is-active:active > .row-visual {
    background: var(--interactive-hover-layer);
    translate: 0 1px;
  }

  .tree-kid .tab-count {
    margin-inline-start: auto;
    /* The chip sits 4px off the row's top and bottom (28 - 20, halved); the
       8px pad + 1px border left it 9px off the right edge, which read as
       the chip drifting. Pull it out until every gap is the same 4px. */
    margin-inline-end: -5px;
  }

  /* A state mark, not a count: the asterisk keeps it meaningful without its
     warning color and the compact keyed square stays subordinate to selection. */
  .dirty-mark {
    align-items: center;
    background: var(--sidebar-bg);
    block-size: 14px;
    border: 1px solid currentColor;
    border-radius: 4px;
    box-sizing: border-box;
    color: var(--warning);
    display: inline-flex;
    flex: none;
    font-family: var(--mono);
    font-size: 0.6875rem;
    font-weight: 800;
    inline-size: 14px;
    justify-content: center;
    line-height: 1;
    margin-inline-start: auto;
    text-box: trim-both cap alphabetic;
  }

  .tree-kids .dirty-mark {
    background: var(--sidebar-popover-bg);
  }

  /* In the sidebar the chip wears sidebar material - content tokens follow
     the theme, but the Root sidebar is dark in both. */
  .tab-count {
    align-items: center;
    background: var(--sidebar-item-hover);
    /* 6px, not the pill: a count in the tree is a tag on a row, and the pill
       radius belongs to the status chips in content. */
    border-radius: 6px;
    /* A chip's height is a decision: 20px, the app's chip-small. */
    block-size: 20px;
    color: var(--sidebar-text-secondary);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    line-height: 1;
    padding: 0 var(--space-2);
  }

  .tab-count .t {
    display: block;
    text-box: trim-both cap alphabetic;
  }

  .tab-count.is-signal {
    background: var(--sidebar-count-signal-bg);
    color: var(--sidebar-count-signal-ink);
    font-weight: 500;
  }

  /* ---------- Collapsed pages column ----------
     The pages column folds to a 4.5rem icon strip and keeps every destination
     reachable: page rows become centred glyphs with their names as tooltips,
     a page's sections open as a flyout on hover or keyboard focus, and a
     section signal survives as a dot on its page's glyph. Only where the
     sidebar is in-flow - below 64rem the drawer owns the narrow behaviour. */
  @media (min-width: 64.0625rem) {
    :global(.app-shell.sidebar-collapsed) .side {
      inline-size: var(--sidebar-width-collapsed);
      /* The flyout and tooltips hang outside the strip. */
      overflow: visible;
      /* 12+11+1px border = rows an even 48, so a 16px glyph centres on a
         whole pixel. */
      padding-inline: 12px 11px;
      /* Sticky makes the column its own stacking context and the pane paints
         after it - without this the flyout renders UNDER the content. */
      z-index: 10;
    }

    :global(.app-shell.sidebar-collapsed) .side-head {
      grid-template-columns: 1fr;
      justify-items: center;
    }

    :global(.app-shell.sidebar-collapsed) .side-kicker,
    :global(.app-shell.sidebar-collapsed) .side-title {
      display: none;
    }

    :global(.app-shell.sidebar-collapsed) .side-fold {
      grid-column: 1;
      grid-row: 1;
    }

    :global(.app-shell.sidebar-collapsed) .tree-row > .row-visual {
      gap: 0;
      justify-content: center;
      padding: 0;
    }

    :global(.app-shell.sidebar-collapsed) .tree-row .t {
      display: none;
    }

    :global(.app-shell.sidebar-collapsed) .tree-page.is-active > .tree-row {
      /* Ink only: the thumb parks on the page row while its kids are away. */
      color: var(--sidebar-item-active-text);
    }

    /* Standing for the selection, the page row uses the same neutral state
       layers as the expanded selected row. */
    :global(.app-shell.sidebar-collapsed) .tree-page.is-active > .tree-row:active > .row-visual {
      background: var(--interactive-hover-layer);
      box-shadow: none;
      translate: 0 1px;
    }

    :global(.app-shell.sidebar-collapsed) .tree-page.is-active > .tree-row:hover > .row-visual {
      background: var(--interactive-hover-layer);
      box-shadow: none;
    }

    :global(.app-shell.sidebar-collapsed)
      .tree:has(.tree-page.is-active > .tree-row:active)
      .nav-thumb {
      box-shadow: var(--sidebar-thumb-shadow-pressed);
      translate: 0 1px;
    }

    /* A signal is a waiting plan. Dirty state has its own keyed mark below. */
    :global(.app-shell.sidebar-collapsed) .tree-page.has-signal > .tree-row::before {
      background: var(--sidebar-count-signal-ink);
      border-radius: 50%;
      block-size: 6px;
      content: '';
      inline-size: 6px;
      inset-block-start: 6px;
      inset-inline-end: 8px;
      position: absolute;
    }

    :global(.app-shell.sidebar-collapsed) .tree-row > .row-visual > .dirty-mark {
      inset-block-start: -3px;
      inset-inline-end: -3px;
      margin: 0;
      position: absolute;
    }

    :global(.app-shell.sidebar-collapsed) .tree-page {
      position: relative;
    }

    /* Through .tree-page, to outrank the expanded active-page rule above -
       collapsed, kids appear only as the flyout below. */
    :global(.app-shell.sidebar-collapsed) .tree-page .tree-kids {
      display: none;
    }

    /* Sections fly out beside their page in the menu component's own grammar -
       console-material popover, its eyebrow, its 32px rows - restated onto
       the tree-kids DOM rather than invented here. */
    :global(.app-shell.sidebar-collapsed) .tree-page:is(.is-flown, :focus-within) > .tree-kids {
      background: var(--sidebar-popover-bg);
      border: 1px solid var(--sidebar-popover-border);
      border-radius: 10px;
      box-shadow:
        0 12px 32px var(--shadow-color),
        0 2px 8px var(--shadow-color);
      display: grid;
      gap: 0;
      grid-template-columns: minmax(0, 1fr);
      inset-block-start: 0;
      inset-inline-start: 100%;
      margin: 0;
      min-inline-size: 12rem;
      opacity: 1;
      padding: var(--space-1);
      position: absolute;
      /* Enters like the app's popovers: a fast fade with a 4px settle,
         compositor-only. Closing stays immediate - the grace timer already
         held the panel open past the pointer's exit. */
      transition:
        display var(--duration-fast) allow-discrete,
        opacity var(--duration-fast) var(--ease-standard),
        translate var(--duration-fast) var(--ease-standard);
      translate: 0 0;
      z-index: 60;
    }

    @starting-style {
      :global(.app-shell.sidebar-collapsed) .tree-page:is(.is-flown, :focus-within) > .tree-kids {
        opacity: 0;
        translate: -4px 0;
      }
    }

    /* The flyout names its page, in the menu's eyebrow voice. */
    :global(.app-shell.sidebar-collapsed)
      .tree-page:is(.is-flown, :focus-within)
      > .tree-kids::before {
      color: var(--sidebar-menu-muted);
      content: attr(data-label);
      font-size: var(--font-size-micro);
      font-weight: 600;
      letter-spacing: 0.07em;
      line-height: 16px;
      padding: var(--space-2) var(--space-3) var(--space-1);
      text-transform: uppercase;
    }

    /* Rows wear the menu item's geometry and the console menu's states - the
       strip tints were mixed for the sidebar's ground and read muddy on the
       popover. */
    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid {
      block-size: 32px;
      color: var(--sidebar-menu-text);
      font-size: var(--font-size-control);
    }

    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid > .row-visual {
      gap: var(--space-2);
      padding-inline: var(--space-3);
    }

    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid:hover > .row-visual {
      background: var(--sidebar-menu-hover);
    }

    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid:hover {
      color: var(--sidebar-menu-text);
    }

    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid:focus-visible > .row-visual {
      background: var(--sidebar-menu-hover);
    }

    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid:focus-visible {
      outline: none;
    }

    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid:active > .row-visual {
      background: var(--sidebar-menu-pressed);
    }

    /* The OPEN page reads as navigation, not as a picked option: an accent-
       tinted row in the console's own active ink, mixed over the popover so
       every sidebar palette lands legible without a new literal. Hover and
       press ride the veil layer, so the tint stays underneath. */
    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid.is-active > .row-visual {
      background: color-mix(
        in srgb,
        var(--sidebar-item-active-text) 12%,
        var(--sidebar-popover-bg)
      );
      box-shadow: none;
    }

    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid.is-active {
      color: var(--sidebar-item-active-text);
      font-weight: 600;
    }

    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid::before {
      background: var(--table-row-pressed);
      border-radius: 6px;
      content: '';
      inset: 0;
      opacity: 0;
      pointer-events: none;
      position: absolute;
      transition: opacity var(--duration-fast) var(--ease-standard);
    }

    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid.is-active:hover::before {
      opacity: 0.5;
    }

    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid.is-active:active::before {
      opacity: 1;
    }

    /* The signal chip is the same ink veil here - it rides the row's hover
       fill instead of matching only the resting popover. */
    :global(.app-shell.sidebar-collapsed) .tree-kids .tab-count.is-signal {
      background: color-mix(in srgb, var(--sidebar-count-signal-ink) 14%, transparent);
      color: var(--sidebar-count-signal-ink);
    }

    /* On the active row the content tint sank into the accent fill - the
       badge inverts to the row's own ink and stays a badge on every palette. */
    :global(.app-shell.sidebar-collapsed) .tree-kids .tree-kid.is-active .tab-count.is-signal {
      background: var(--sidebar-item-active-text);
      color: var(--sidebar-popover-bg);
    }

    /* Name-on-hover for the icon rows, same recipe as the rail; a page with
       sections answers with its flyout instead. */
    :global(.app-shell.sidebar-collapsed) .tree-row::after {
      background: var(--popover-bg);
      border: 1px solid var(--popover-border);
      border-radius: 6px;
      box-shadow: var(--shadow-popover);
      color: var(--text-primary);
      content: attr(data-tip);
      font-size: var(--font-size-micro);
      inset-inline-start: calc(100% + 10px);
      opacity: 0;
      padding: 0.3rem 0.5rem;
      pointer-events: none;
      position: absolute;
      top: 50%;
      translate: 0 -50%;
      white-space: nowrap;
      z-index: 60;
    }

    :global(.app-shell.sidebar-collapsed) .tree-row:is(:hover, :focus-visible)::after {
      opacity: 1;
    }

    :global(.app-shell.sidebar-collapsed) .tree-page .tree-row::after {
      content: none;
    }
  }

  /* ---------- Responsive: the sidebar becomes a drawer, never nothing ----------
     Below 64rem the pages column cannot share the row with content. The rail
     stays at every width, grows a pages toggle, and the active console's
     sidebar slides over the content from the rail's edge, above a scrim. */
  @media (max-width: 64rem) {
    .side {
      box-shadow: 8px 0 24px var(--shadow-color);
      inset-block: 0;
      inset-inline-start: 60px;
      position: fixed;
      transition:
        translate var(--duration-fast) var(--ease-standard),
        visibility 0s var(--duration-fast);
      translate: -110% 0;
      visibility: hidden;
      z-index: 40;
    }

    :global(.app-shell.side-open) .side {
      translate: 0 0;
      transition: translate var(--duration-fast) var(--ease-standard);
      visibility: visible;
    }

    /* The drawer owns narrow widths; the fold control is a wide-shell
       affordance. */
    .side-fold {
      display: none;
    }
  }
</style>
