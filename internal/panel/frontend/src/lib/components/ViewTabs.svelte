<script lang="ts">
  import { tick } from 'svelte';

  import { panelViewSection, routeSegmentLabel, type PanelView, type RootSection } from '../routes';
  import Icon, { type IconName } from './Icon.svelte';

  const {
    value,
    hrefFor,
    onSelect,
    showUsers,
    showViews,
    collapsed,
    rootMode,
    rootEnabled,
    rootValue,
    rootHrefFor,
    onSelectRoot,
    rootEntryHref,
    onEnterRoot,
    returnHref,
    onReturnToPanel,
    inboxHref,
    inboxActive,
    onSelectInbox,
    unreadCount,
  }: {
    value: PanelView;
    hrefFor: (view: PanelView) => string;
    onSelect: (view: PanelView) => void;
    showUsers: boolean;
    /**
     * Whether a workspace is settled for the view links to lead to.
     *
     * A personal page names no workspace, so it can be reached before one has been
     * chosen - by a Root who owns nothing, arriving straight at `/inbox`. Rows
     * that would all point at `#` are worse than no rows.
     */
    showViews: boolean;
    collapsed: boolean;
    rootMode: boolean;
    rootEnabled: boolean;
    rootValue: RootSection;
    rootHrefFor: (section: RootSection) => string;
    onSelectRoot: (section: RootSection) => void;
    rootEntryHref: string;
    onEnterRoot: () => void;
    returnHref: string;
    onReturnToPanel: () => void;
    inboxHref: string;
    inboxActive: boolean;
    onSelectInbox: () => void;
    unreadCount: number;
  } = $props();

  const NAVIGATION_VIEWS = [
    'settings',
    'repositories',
    'sync',
    'users',
    'history',
  ] as const satisfies readonly PanelView[];

  const visibleViews = $derived(NAVIGATION_VIEWS.filter((view) => view !== 'users' || showUsers));
  /* Queue sits under Overview: it is the one section that is about what the
     service is going to do rather than about what it is configured with, so it
     reads as the console's second answer to "what is happening". */
  const ROOT_SECTIONS = [
    'overview',
    'queue',
    'installations',
    'access',
    'history',
    'settings',
  ] as const satisfies readonly RootSection[];

  /* A personal page stands over the view the reader was on, so while one is open
     nothing in the list is current - the selection is on the Inbox row instead. */
  function isActive(item: PanelView): boolean {
    if (inboxActive) return false;
    return value === item || (item === 'users' && value === 'invitations');
  }

  const unreadLabel = $derived(unreadCount > 99 ? '99+' : String(unreadCount));
  const inboxLabel = $derived(unreadCount === 0 ? 'Inbox' : `Inbox, ${unreadCount} unread`);

  function destination(item: PanelView): PanelView {
    return item === 'users' && value === 'invitations' ? 'invitations' : item;
  }

  function selectFromClick(event: MouseEvent, next: PanelView): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onSelect(next);
  }

  function selectRootFromClick(event: MouseEvent, next: RootSection): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onSelectRoot(next);
  }

  function enterRootFromClick(event: MouseEvent): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onEnterRoot();
  }

  function selectInboxFromClick(event: MouseEvent): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onSelectInbox();
  }

  function returnFromClick(event: MouseEvent): void {
    if (!plainClick(event) || returnHref === '#') return;
    event.preventDefault();
    onReturnToPanel();
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

  function label(view: PanelView): string {
    return routeSegmentLabel(panelViewSection(view));
  }

  function icon(view: PanelView): IconName {
    if (view === 'settings') return 'settings';
    if (view === 'repositories') return 'repositories';
    if (view === 'sync') return 'refresh';
    if (view === 'users' || view === 'invitations') return 'users';
    return 'history';
  }

  function rootLabel(section: RootSection): string {
    return routeSegmentLabel(section);
  }

  function rootIcon(section: RootSection): IconName {
    if (section === 'overview') return 'system';
    /* The clock face, not the arrow-and-clock `history` uses: a queue is what has
       not happened yet, and the two sections sit four rows apart in the same
       list. */
    if (section === 'queue') return 'pending';
    if (section === 'installations') return 'organization';
    if (section === 'access') return 'users';
    if (section === 'history') return 'history';
    return 'settings';
  }
  /**
   * The selected row is a thumb that travels, the same object the segmented control moves.
   *
   * It was a background painted on whichever link happened to be current, so selection appeared in
   * one place and vanished from another with nothing connecting the two. One element that slides
   * says where the selection went; two that swap grounds only say that something changed.
   */
  /**
   * The selected row's ground, kept under whichever row is current.
   *
   * Everything here is driven by explicit state rather than by the style engine. The fret used to
   * be `:has(a:not(.active):active)`, which asks the browser to re-evaluate an ancestor selector on
   * every pointer state change in the subtree, and the move used to be scheduled through a pair of
   * animation frames that a ResizeObserver tick could cancel out from under it - so a click that
   * landed while the sidebar was still settling produced no movement at all. Now a pointer handler
   * owns the fret, and each placement is a numbered job where only the newest may finish.
   */
  function followSelection(node: HTMLElement, current: string) {
    let selection = current;
    /** Where the thumb is parked, in the list's own coordinates. */
    let restingTop: number | null = null;
    let travelling: Animation | null = null;
    let fretting: Animation | null = null;
    let fretTimer: ReturnType<typeof setTimeout> | undefined;
    let job = 0;

    const still = (): boolean => window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const thumbOf = (): HTMLElement | null => node.querySelector<HTMLElement>('.nav-thumb');

    /** How far the thumb currently sits from its parked position, mid-flight included. */
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

    /**
     * Held down on a row that is not the selected one, the selection frets.
     *
     * After 120ms, so an ordinary click never sets it off: holding is the only time the question
     * of whether this is about to be taken away stays open long enough to be worth answering.
     */
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
      if (row === null || row === undefined || row.classList.contains('active')) return;
      if (!node.contains(row)) return;
      fretTimer = setTimeout(beginFret, 120);
    }

    /**
     * The thumb gathers itself before it goes, and lands a little past where it is going.
     *
     * The wind-up is 28ms of the 280. Any longer and the control reads as slow to answer rather
     * than as gathering itself: the first thing that happens after a click has to be movement, and
     * a wind-up the eye can time is read as lag.
     *
     * `distance` is measured from where the thumb *is*, not from where it was parked, so a click
     * that interrupts a move continues from the position on screen instead of snapping back.
     */
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

    /**
     * Measure and park the thumb. `moved` asks for the travel; a resize never does.
     *
     * Numbered so a placement queued behind a newer one cannot land after it: the old job returns
     * without touching anything rather than writing a stale position.
     */
    async function place(moved: boolean): Promise<void> {
      const mine = ++job;
      await tick();
      if (mine !== job) return;
      const active = node.querySelector<HTMLElement>('a.active');
      const thumb = thumbOf();
      if (active === null || thumb === null) {
        node.style.setProperty('--nav-thumb-height', '0px');
        node.classList.remove('thumb-ready');
        restingTop = null;
        return;
      }
      /* Rects rather than `offsetTop` and `offsetHeight`, which are rounded to
         whole pixels. The rows above the console's list sit on a fraction - the
         section header is trimmed to its cap band, and a cap height is not an
         integer - so the rounded figure put the thumb 0.19px above every row it
         was supposed to be the ground of. `.view-links` is the thumb's
         containing block and carries no border, so subtracting its top is the
         same coordinate the thumb is placed in. */
      const origin = node.getBoundingClientRect().top;
      const bounds = active.getBoundingClientRect();
      const top = bounds.top - origin;
      const parked = node.classList.contains('thumb-ready');
      const from = parked && restingTop !== null ? restingTop + offsetNow(thumb) : top;
      node.style.setProperty('--nav-thumb-top', `${top}px`);
      node.style.setProperty('--nav-thumb-height', `${bounds.height}px`);
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
    // A width change is not a move: collapsing the sidebar re-measures without anything travelling.
    const resize = new ResizeObserver(() => void place(false));
    resize.observe(node);

    return {
      update(next: string) {
        if (next === selection) return;
        /* Every field but the last says where the selection is; the last is the
           rail's width, and changing that is a re-measure rather than a move. */
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

<nav class={['panel-navigation', collapsed && 'collapsed']} aria-label="Panel navigation">
  <div
    class="view-links"
    use:followSelection={`${rootMode ? 'root' : 'panel'}:${rootValue ?? ''}:${value ?? ''}:${inboxActive}:${collapsed}`}
  >
    <span class="nav-thumb" aria-hidden="true"></span>
    {#if rootMode}
      <p class="nav-label">Administration</p>
      {#each ROOT_SECTIONS as section (section)}
        <a
          href={rootHrefFor(section)}
          class:active={rootValue === section}
          aria-label={collapsed ? rootLabel(section) : undefined}
          aria-current={rootValue === section ? 'page' : undefined}
          onclick={(event) => selectRootFromClick(event, section)}
        >
          <span class="navigation-icon"><Icon name={rootIcon(section)} size={20} /></span>
          <span class="navigation-label">{rootLabel(section)}</span>
          <span class="navigation-tooltip">{rootLabel(section)}</span>
        </a>
      {/each}
    {:else if showViews}
      {#each visibleViews as item (item)}
        <a
          href={hrefFor(destination(item))}
          id={`${item}-navigation`}
          class={isActive(item) ? 'active' : undefined}
          aria-label={collapsed ? label(item) : undefined}
          aria-current={isActive(item) ? 'page' : undefined}
          onclick={(event) => selectFromClick(event, destination(item))}
        >
          <span class="navigation-icon"><Icon name={icon(item)} size={20} /></span>
          <span class="navigation-label">{label(item)}</span>
          <span class="navigation-tooltip">{label(item)}</span>
        </a>
      {/each}
    {/if}

    <div class="navigation-spacer"></div>

    <!-- The zone the list does not reach: what is here belongs to the reader or to
         the application, not to the workspace above it, and it is the same zone in
         both modes so the inbox does not move when somebody enters the console. -->
    <div class="foot-zone">
      {#if rootMode}
        <a
          class="foot-entry"
          class:disabled={returnHref === '#'}
          href={returnHref}
          aria-label={collapsed ? 'Exit Root' : undefined}
          aria-disabled={returnHref === '#' ? 'true' : undefined}
          onclick={returnFromClick}
        >
          <span class="navigation-icon"><Icon name="chevron-left" size={20} /></span>
          <span class="navigation-label">Exit Root</span>
          <span class="navigation-tooltip">Exit Root</span>
        </a>
      {:else if rootEnabled}
        <a
          class="foot-entry root-entry"
          href={rootEntryHref}
          aria-label={collapsed ? 'Root console' : undefined}
          onclick={enterRootFromClick}
        >
          <span class="navigation-icon"><Icon name="shield" size={20} /></span>
          <span class="navigation-label">Root console</span>
          <span class="navigation-tooltip">Root console</span>
        </a>
      {/if}

      <a
        class="foot-entry inbox-entry"
        class:active={inboxActive}
        href={inboxHref}
        aria-label={collapsed || unreadCount > 0 ? inboxLabel : undefined}
        aria-current={inboxActive ? 'page' : undefined}
        onclick={selectInboxFromClick}
      >
        <span class="navigation-icon">
          <Icon name="notifications" size={20} />
          {#if unreadCount > 0}<span class="unread-dot" aria-hidden="true"></span>{/if}
        </span>
        <span class="navigation-label">Inbox</span>
        {#if unreadCount > 0}<span class="unread-count" aria-hidden="true"
            ><span class="cap-trim">{unreadLabel}</span></span
          >{/if}
        <span class="navigation-tooltip">{inboxLabel}</span>
      </a>
    </div>
  </div>
</nav>

<style>
  .panel-navigation,
  .view-links {
    min-height: 0;
  }

  .panel-navigation {
    height: 100%;
  }

  .view-links {
    display: flex;
    flex-direction: column;
    gap: 3px;
    height: 100%;
    /* The travelling thumb is positioned against this. */
    position: relative;
  }

  /* Trimmed to its cap band, so the margins around it are the gaps that are seen.
     Untrimmed, the line box carried ~1.5px of ascender air above the caps and as
     much below the baseline, which is space nothing here asked for and space that
     would change with the font.

     The top margin is the mark's clearance. Root mode has no workspace card
     between the two, so this label was the next thing under the wordmark - and it
     is uppercase, letterspaced and bold, exactly like the two lines above it, so
     at the rail's default 8px it read as a third line of the mark rather than as
     the heading of the list. It stands further from the mark than from the rows
     it heads, which is what makes it belong to them. */
  .nav-label {
    color: var(--sidebar-text-muted);
    font: 700 var(--font-size-micro) / 1 var(--sans);
    letter-spacing: 0.09em;
    margin: var(--space-4) 0 var(--space-2);
    padding-inline: var(--space-3);
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .navigation-spacer {
    flex: 1;
  }

  a {
    align-items: center;
    border-radius: var(--radius-control);
    /* Above the thumb, which is painted behind every row. */
    isolation: isolate;
    z-index: 1;
    color: var(--sidebar-text-secondary);
    display: flex;
    font-size: var(--font-size-control);
    font-weight: 600;
    gap: var(--space-3);
    line-height: 1;
    min-height: 2.375rem;
    padding: 0 var(--space-3);
    position: relative;
    text-decoration: none;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
  }

  a:hover {
    background: var(--sidebar-item-hover);
    color: var(--sidebar-text);
  }

  a:active {
    background: var(--sidebar-item-pressed);
  }

  /* Selected item is a raised thumb, the same language as the app's segmented
     controls: the accent lives in the text and icon, not a bar or a tint. The ground is drawn by
     .nav-thumb, which slides between rows, so the link itself only carries its ink. */
  a.active {
    color: var(--sidebar-item-active-text);
    font-weight: 700;
  }

  .nav-thumb {
    background: var(--sidebar-thumb);
    border-radius: var(--radius-control);
    box-shadow: var(--sidebar-thumb-shadow);
    height: var(--nav-thumb-height, 0);
    inset-inline: 0;
    pointer-events: none;
    position: absolute;
    top: var(--nav-thumb-top, 0);
    transition:
      height 240ms cubic-bezier(0.22, 1, 0.36, 1),
      background-color var(--duration-fast) var(--ease-standard);
    z-index: 0;
  }

  /* Until the first measurement lands there is nothing to travel from. */
  .view-links:not(.thumb-ready) .nav-thumb {
    transition: none;
  }

  .view-links:has(a.active:hover) .nav-thumb {
    background: color-mix(in srgb, var(--sidebar-item-active-text) 2.5%, var(--sidebar-thumb));
  }

  .view-links:has(a.active:active) .nav-thumb {
    background: color-mix(in srgb, var(--sidebar-item-active-text) 5%, var(--sidebar-thumb));
  }

  /* The selected row's own states stay under the selection they sit on. At 6% and 12% the press
     measured 6.14 dE00 against a 3.74 fill in the light panel - the acknowledgement was louder than
     the state. Same pair as the segmented control's thumb, for the same reasons. */
  a.active:hover,
  a.active:active {
    background: transparent;
    color: var(--sidebar-item-active-text);
  }

  /* Named for where it is rather than for what it holds: it carries the reader's
     own inbox as well as the way into and out of the console. No rule above it -
     the space the spacer opens already separates it from the list, and the card
     below draws the only line the foot of the rail needs. */
  .foot-zone {
    display: flex;
    flex-direction: column;
    gap: 3px;
    margin-top: var(--space-2);
  }

  .root-entry .navigation-icon {
    color: var(--sidebar-root-accent);
  }

  .root-entry:hover {
    background: color-mix(in srgb, var(--sidebar-root-accent) 10%, transparent);
  }

  .foot-entry.disabled {
    cursor: default;
    opacity: 0.45;
  }

  /* The count reads as a count, not as a control: it takes no press or hover
     state of its own, and the row it rides carries both. */
  .unread-count {
    align-items: center;
    background: var(--unread-badge-bg);
    border-radius: 999px;
    color: var(--unread-badge-text);
    display: inline-flex;
    flex: none;
    font: 700 var(--font-size-micro) / 1 var(--sans);
    height: 1.25rem;
    justify-content: center;
    margin-left: auto;
    min-width: 1.25rem;
    padding-inline: 0.3rem;
  }

  /* Collapsed, the number has nowhere to go, so the fact that there is a number
     rides the icon instead. Ringed in the rail's own ground so it reads as
     sitting on the glyph rather than as part of it. */
  .unread-dot {
    display: none;
  }

  .collapsed .unread-count {
    display: none;
  }

  .collapsed .unread-dot {
    background: var(--unread-badge-bg);
    border: 2px solid var(--sidebar-bg);
    border-radius: 50%;
    display: block;
    height: 0.5rem;
    position: absolute;
    right: -0.1875rem;
    top: -0.1875rem;
    width: 0.5rem;
  }

  .inbox-entry .navigation-icon {
    position: relative;
  }

  /* The trimmed label puts the cap band on the row's centre, so centring the icon
     on the row centres it on the cap band, in every row, with nothing that
     depends on which letters the label happens to contain. This is where the
     descender nudge was found out and where it was dropped first - see app.css. */
  .navigation-icon {
    align-items: center;
    display: inline-flex;
    flex: none;
    justify-content: center;
  }

  /* Trimmed to its glyph bounds, cap height to baseline, so the box the row
     centres IS the letters. The panel's rule everywhere else, and the reason for
     it is here in miniature: this used to be a 1.25rem box with the text centred
     inside it, which centres a line box - ascender to descender - and leaves the
     letters sitting low by whatever the descender is worth in this font. The row
     then carried a compensating 1px of top padding, which is the shape of a fix
     that works at one size, in one font, for one row. Trimmed, `align-items:
     center` puts the letters and the icon on the same middle at any size. */
  .navigation-label {
    text-box: trim-both cap alphabetic;
  }

  .navigation-tooltip {
    display: none;
  }

  .collapsed a {
    justify-content: center;
    padding: 0;
  }

  .collapsed .nav-label {
    display: none;
  }

  .collapsed .navigation-label {
    display: none;
  }

  .collapsed .navigation-tooltip {
    background: var(--sidebar-popover-bg);
    border: 1px solid var(--sidebar-popover-border);
    border-radius: var(--radius-control);
    box-shadow: var(--shadow-popover);
    color: var(--sidebar-menu-text);
    display: block;
    font-size: var(--font-size-meta);
    font-weight: 500;
    /* Clear of the sidebar rather than of the row. The row stops one padding inside the sidebar,
       so reaching the same air on the outside is that padding, the border, and one more. The
       collapsed rail pads by --space-2, which is the padding this has to match. */
    left: calc(100% + var(--space-2) * 2 + 1px);
    opacity: 0;
    padding: var(--space-2) var(--space-3);
    pointer-events: none;
    position: absolute;
    top: 50%;
    transform: translate(-4px, -50%);
    transition:
      opacity var(--duration-fast) var(--ease-standard),
      transform var(--duration-fast) var(--ease-standard);
    visibility: hidden;
    white-space: nowrap;
    z-index: var(--layer-popover);
  }

  .collapsed a:hover .navigation-tooltip,
  .collapsed a:focus-visible .navigation-tooltip {
    opacity: 1;
    transform: translate(0, -50%);
    visibility: visible;
  }

  @media (max-width: 48rem) {
    .view-links {
      gap: var(--space-1);
    }

    /* No mark above it here - the drawer hangs below the top bar and this is the
       first thing in it, so the clearance the rail needs would be a hole. */
    .nav-label {
      margin-top: 0;
    }

    a {
      min-height: 2.75rem;
    }

    .navigation-spacer {
      display: none;
    }

    .collapsed a {
      justify-content: flex-start;
      padding: 0 var(--space-3);
    }

    .collapsed .nav-label {
      display: block;
    }

    .collapsed .navigation-label {
      display: inline-flex;
    }

    .collapsed .navigation-tooltip {
      display: none;
    }

    /* The drawer is a full-width list whatever the rail is doing behind it, so
       the count goes back to being a number beside its label. */
    .collapsed .unread-count {
      display: inline-flex;
    }

    .collapsed .unread-dot {
      display: none;
    }
  }
</style>
