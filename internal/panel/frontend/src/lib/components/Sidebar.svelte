<script module lang="ts">
  import type { ThemeDisplay } from '../preferences';
  import type { PanelTarget, PanelViewer } from '../types';
  import type { IconName } from './Icon.svelte';

  /** A destination in the tree: one row, one address. */
  export interface SidebarRow {
    kind?: 'row';
    id: string;
    label: string;
    icon: IconName;
    href: string;
    active: boolean;
    /** A count that demands attention (a waiting plan) speaks, quietly. */
    count?: number | string;
    signal?: boolean;
    /** This destination contains configuration that has not been saved. */
    dirty?: boolean;
    /** The one row that stands apart from every group: workspace settings. */
    foot?: boolean;
  }

  /**
   * A heading over the rows that follow it. A label, never a link - the pages
   * that used to be half-real parent rows (Sync, Access, History) are these,
   * and their sections are rows of their own.
   */
  export interface SidebarGroup {
    kind: 'group';
    id: string;
    label: string;
  }

  export type SidebarEntry = SidebarGroup | SidebarRow;

  export function isGroup(entry: SidebarEntry): entry is SidebarGroup {
    return entry.kind === 'group';
  }

  /**
   * Everything the rail carries, for the shell that has no rail.
   *
   * Collapsed, the pages column is the only chrome the panel draws: two narrow
   * columns side by side was the arrangement the design rejected, and folding one
   * of them away means the workspace switch, the inbox and the account have to be
   * somewhere. They are here, in the head and the foot.
   */
  export interface SidebarChrome {
    targets: readonly PanelTarget[];
    selected: PanelTarget | null;
    targetHref: (target: PanelTarget) => string;
    onSelectTarget: (targetId: string) => void;
    dirtyTargetIds?: ReadonlySet<string>;
    rootMode: boolean;
    rootEnabled: boolean;
    rootEntryHref: string;
    onEnterRoot: () => void;
    inboxHref: string;
    inboxActive: boolean;
    onSelectInbox: () => void;
    unreadCount: number;
    viewer: PanelViewer | null;
    theme: ThemeDisplay;
    onSelectTheme: (theme: ThemeDisplay) => void;
    onSignOut: () => void | Promise<void>;
  }
</script>

<script lang="ts">
  import { tick } from 'svelte';

  import { workspaceInitials } from '../workspace-mark.js';
  import AccountMenu from './AccountMenu.svelte';
  import Avatar from './Avatar.svelte';
  import Icon from './Icon.svelte';
  import WorkspaceMenu from './WorkspaceMenu.svelte';

  const {
    kicker,
    title,
    entries,
    collapsed,
    onToggleCollapsed,
    onSelectRow,
    label = 'Pages',
    chrome = null,
    onOpenSearch,
    searchLabel = 'Search',
  }: {
    /** The console's voice above the tree: "Workspace" / "Console". */
    kicker: string;
    /** The console's name: the workspace, or the Root console's purpose. */
    title: string;
    entries: readonly SidebarEntry[];
    collapsed: boolean;
    onToggleCollapsed: () => void;
    onSelectRow: (row: SidebarRow) => void;
    label?: string;
    /** What the rail carries, for the shells that draw no rail. */
    chrome?: SidebarChrome | null;
    onOpenSearch?: () => void;
    /** What the field searches: "Search this workspace". */
    searchLabel?: string;
  } = $props();

  const consoleEntry = $derived(
    chrome === null || !(chrome.rootEnabled || chrome.rootMode)
      ? null
      : { href: chrome.rootEntryHref, onEnter: chrome.onEnterRoot },
  );
  const switchLabel = $derived(
    chrome === null
      ? ''
      : chrome.rootMode
        ? 'Switch workspace - the Operations console is open'
        : `Switch workspace - ${title} is open`,
  );
  const unreadLabel = $derived(
    chrome === null ? '' : chrome.unreadCount > 99 ? '99+' : String(chrome.unreadCount),
  );
  const inboxTip = $derived(
    chrome === null || chrome.unreadCount === 0 ? 'Inbox' : `Inbox - ${chrome.unreadCount} unread`,
  );
  const viewerName = $derived(
    chrome?.viewer === null || chrome?.viewer === undefined
      ? ''
      : chrome.viewer.account.display_name || chrome.viewer.account.login,
  );

  function inboxFromClick(event: MouseEvent): void {
    if (!plainClick(event) || chrome === null) return;
    event.preventDefault();
    chrome.onSelectInbox();
  }

  const activeRow = $derived(
    entries.find((entry): entry is SidebarRow => !isGroup(entry) && entry.active),
  );
  const selectionKey = $derived(`${activeRow?.id ?? ''}:${collapsed}`);

  /* ---------- The collapsed strip's one tooltip ----------

     Every `[data-tip]` control in the strip shares it - the rows, the fold, the
     workspace switch, Search and the foot - because a name-on-hover per control is a
     name-on-hover positioned inside the tree, and the tree is the scroll region. */

  let tip = $state<{ text: string; x: number; y: number } | null>(null);

  function showTip(target: EventTarget | null, keyboard = false): void {
    if (!collapsed || !(target instanceof Element)) {
      tip = null;
      return;
    }
    const anchor = target.closest<HTMLElement>('[data-tip]');
    // A keyboard tip belongs to a visible focus ring, not to any focus at all.
    if (anchor === null || (keyboard && !anchor.matches(':focus-visible'))) {
      tip = null;
      return;
    }
    const text = anchor.dataset.tip ?? '';
    if (text === '') {
      tip = null;
      return;
    }
    const box = anchor.getBoundingClientRect();
    tip = {
      text,
      x: Math.round(box.right + 10),
      y: Math.round(box.top + box.height / 2),
    };
  }

  function hideTip(): void {
    tip = null;
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

  function rowFromClick(event: MouseEvent, row: SidebarRow): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onSelectRow(row);
  }

  /**
   * The app's travelling selection, verbatim: one element is the selected
   * row's ground, and switching pages MOVES it - two rows that swap grounds
   * only say something changed; one that slides says where it went.
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
    const target = (): HTMLElement | null => node.querySelector<HTMLElement>('.tree-row.is-active');

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
      /* By the row's own class rather than by `a`, which is what the tree happens to
         render one as. Every row is an anchor today and the two agree; the class is what
         this is actually asking about, and a row that opened something instead of
         navigating would still be a row. */
      const row = (event.target as Element | null)?.closest?.('.tree-row');
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
      /* Rects rather than offsets, which round to whole pixels. The tree is
         the scroll region, so the row's position has to be read in the
         scroller's own content coordinates - a rect difference alone parks the
         thumb scrollTop pixels above its row once the tree has scrolled. */
      const origin = node.getBoundingClientRect();
      const bounds = active.getBoundingClientRect();
      const top = bounds.top - origin.top + node.scrollTop;
      const left = bounds.left - origin.left;
      const radius = getComputedStyle(active).borderRadius;
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

<!--
@component
The pages inside one console, as a flat list under section headings. It is the second
level of navigation: the rail chooses the console, and this chooses the page within it.

Every page is one row with its own icon and its own address. The tree used to carry
half-real parent rows - Sync, Access, History were links that only opened their first
section - and the sections hung under them as a second row grammar. A heading is a
label, so nothing in the tree is pressable that is not a destination.

`kicker` is the console's voice and `title` its name, and together they are what tells a
reader which console they are in without reading the page. The pair is why this
component is shared between the workspace and the Root console rather than written
twice.

Collapsed is a state the shell owns rather than the sidebar, because the content beside
it has to answer to the same fact.
-->

<aside
  class="side"
  aria-label={kicker}
  onpointerover={(event) => showTip(event.target)}
  onpointerout={hideTip}
  onfocusin={(event) => showTip(event.target, true)}
  onfocusout={hideTip}
>
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
        <Icon name="chevron-left" size="sm" />
      </span>
    </button>
    {#if chrome !== null}
      <WorkspaceMenu
        targets={chrome.targets}
        targetHref={chrome.targetHref}
        onSelectTarget={chrome.onSelectTarget}
        dirtyTargetIds={chrome.dirtyTargetIds}
        console={consoleEntry}
        label="Switch workspace"
      >
        {#snippet trigger(attributes)}
          <button {...attributes} class="side-ws-mini" type="button" aria-label={switchLabel}>
            {#if chrome.rootMode}
              <span class="ws-mini is-console"><Icon name="shield" size="xs" /></span>
            {:else if chrome.selected?.account.avatar_url != null}
              <Avatar account={chrome.selected.account} size={24} shape="workspace" />
            {:else}
              <span class="ws-mini"><span class="t">{workspaceInitials(title)}</span></span>
            {/if}
          </button>
        {/snippet}
      </WorkspaceMenu>
    {/if}
  </div>
  {#if onOpenSearch !== undefined}
    <button
      class="side-search"
      type="button"
      data-tip="Search"
      aria-label={searchLabel}
      aria-keyshortcuts="Meta+K Control+K"
      onclick={onOpenSearch}
    >
      <span class="gi"><Icon name="search" size="base" /></span>
      <span class="t">Search</span>
      <kbd>⌘K</kbd>
    </button>
  {/if}
  <!-- A scrolling tree moves the anchors out from under a fixed layer, so the tip goes
       rather than hanging beside a row that has left. -->
  <nav class="tree" aria-label={label} use:followSelection={selectionKey} onscroll={hideTip}>
    <span class="nav-thumb" aria-hidden="true"></span>
    {#each entries as entry (entry.id)}
      {#if isGroup(entry)}
        <span class="tree-group">{entry.label}</span>
      {:else}
        <a
          class="tree-row"
          class:is-active={entry.active}
          class:is-foot={entry.foot === true}
          class:has-dirty={entry.dirty === true}
          href={entry.href}
          data-tip={entry.label}
          aria-current={entry.active ? 'page' : undefined}
          onclick={(event) => rowFromClick(event, entry)}
        >
          <span class="gi"><Icon name={entry.icon} size="base" /></span>
          <span class="t">{entry.label}</span>
          {#if entry.count !== undefined}
            <span class="tab-count" class:is-signal={entry.signal === true}>
              <span class="t">{entry.count}</span>
            </span>
          {/if}
          {#if entry.dirty === true}
            <span class="dirty-mark" aria-hidden="true">*</span>
            <span class="visually-hidden">Unsaved changes</span>
          {/if}
        </a>
      {/if}
    {/each}
  </nav>
  {#if chrome !== null}
    <!-- The foot the rail used to be: where a reader reaches their inbox and their
         own account when the pages column is the only column. -->
    <div class="side-foot">
      <a
        class="tree-row"
        class:is-active={chrome.inboxActive}
        href={chrome.inboxHref}
        data-tip={inboxTip}
        aria-label={inboxTip}
        aria-current={chrome.inboxActive ? 'page' : undefined}
        onclick={inboxFromClick}
      >
        <span class="gi"><Icon name="notifications" size="base" /></span>
        <span class="t">Inbox</span>
        {#if chrome.unreadCount > 0}
          <span class="rail-badge" aria-hidden="true"><span class="t">{unreadLabel}</span></span>
        {/if}
      </a>
      {#if chrome.viewer !== null}
        <AccountMenu
          viewer={chrome.viewer}
          theme={chrome.theme}
          onSelectTheme={chrome.onSelectTheme}
          onSignOut={chrome.onSignOut}
          name="side-theme"
          align="end"
        >
          {#snippet trigger(attributes)}
            <button
              {...attributes}
              class="tree-row"
              type="button"
              data-tip="{viewerName} - @{chrome.viewer?.account.login}"
              aria-label="Account menu for {viewerName}"
            >
              {#if chrome.viewer?.account.avatar_url != null}
                <Avatar account={chrome.viewer.account} size={24} />
              {:else}
                <span class="ws-mini"><span class="t">{workspaceInitials(viewerName)}</span></span>
              {/if}
              <span class="t">{viewerName}</span>
            </button>
          {/snippet}
        </AccountMenu>
      {/if}
    </div>
  {/if}
  <!-- ONE TOOLTIP FOR THE WHOLE STRIP, and it is fixed. A name-on-hover written as a
       pseudo-element on each row is positioned inside the tree, which is the scroll
       region - so the row nearest either end had its name clipped away by the very
       overflow that makes the tree scrollable. -->
  {#if tip !== null}
    <!-- `style:` and not a style attribute: the panel's CSP allows the directive's
         property set and refuses an inline attribute. -->
    <span
      class="side-tip"
      role="presentation"
      aria-hidden="true"
      style:--tip-x="{tip.x}px"
      style:--tip-y="{tip.y}px">{tip.text}</span
    >
  {/if}
</aside>

<style>
  .side {
    /* THE TREE'S GUTTER, and there is only one of it.
       ------------------------------------------------------------------------
       A row's glyph and a section heading's first letter stand in the same column, and
       they do so because the row's inline padding and the heading's inline margin are
       THE SAME NUMBER rather than two numbers that happen to agree. Written separately
       they drifted - 8px on the row and 12px on the heading - so every heading in the
       sidebar sat four pixels to the right of the icons it named. The search field takes
       it too: it is the tree's first row in everything but name. */
    --tree-gutter: var(--space-3);

    align-self: start;
    background: var(--sidebar-bg);
    border-inline-end: 1px solid var(--sidebar-border);
    box-sizing: border-box;
    color: var(--sidebar-text);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    block-size: 100dvh;
    inline-size: var(--sidebar-width);
    /* The column itself never scrolls: the TREE is the scroll region, so the
       head and the fold stay pinned at any viewport height. Tooltips and
       flyouts position fixed, so this clip cannot cut them. */
    overflow: hidden;
    padding: var(--space-4) 12px;
    padding-block-end: max(var(--space-4), env(safe-area-inset-bottom));
    position: sticky;
    /* Lifts the tree's scroll timeline to the column, so the search field - the
       tree's sibling - can cast its shadow from it. */
    timeline-scope: --side-tree;
    /* Structure moves at --duration-normal, like the app shell's own columns. */
    transition: inline-size var(--duration-normal) var(--ease-standard);
    top: 0;
  }

  /* Only the navigation body gives way at short heights - it shrinks to the
     room left after the head and scrolls itself. align-content pins the rows:
     a stretched grid otherwise distributes the grown flex height into its row
     tracks and the 2px gaps become 12.
     The scrollport claims the column's full width and both flex gaps - the
     negative margins reach the sidebar's edges and the padding puts the same
     insets back INSIDE the scroller. So the scrollbar lives in the right inset
     instead of over the rows, and the scroll shadow ends on the edge below.

     AND THE TOP PADDING PAYS THE COLUMN'S GAP BACK. Reaching the sidebar's top edge
     means eating the 16px the column puts between the search field and the tree, and
     nothing else was replacing it: the first row began 16px under the search, which is
     the gap between two rows plus a hair. Doubled here it is the column's gap and the
     scroller's own inset, which is the distance the search was always meant to have -
     and the scrollport still starts where it did, so the shadow and the scrollbar are
     untouched. */
  .side > .tree {
    align-content: start;
    flex: 1 1 auto;
    margin-block: calc(-1 * var(--space-4));
    margin-inline: -12px;
    min-block-size: 0;
    overflow: hidden auto;
    /* One space under the search, the same one the content pane puts under its
       own. Bart's call: the drawing gives the tree a doubled one and the two
       search bars then disagreed about how far their list sits below them. */
    padding-block: var(--space-4);
    padding-inline: 12px;
    scroll-timeline: --side-tree block;
    scrollbar-width: thin;
  }

  /* THE CLIPPED-EDGE CUE, bottom edge. Overlay scrollbars are not a reliable
     sign that rows continue past the fold, so the tree wears a classic scroll
     shadow: the shadow layer sticks to the scrollport's bottom edge (scroll)
     while a same-coloured cover travels with the content (local) and blots it
     out exactly when the content's end is on screen. Pure paint - no layout,
     no listeners. The shade spans the rail because the bottom edge is the
     sidebar's own, not any row's. */
  .side > .tree {
    background-attachment: local, scroll;
    background-image:
      linear-gradient(to top, var(--sidebar-bg), transparent),
      linear-gradient(to top, var(--sidebar-scroll-shadow), transparent);
    background-position:
      0 100%,
      0 100%;
    background-repeat: no-repeat;
    background-size:
      100% 20px,
      100% 10px;
  }

  /* No end padding: the fold's right edge lands exactly on the rows' right
     edge below it. */
  .side-head {
    align-items: center;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: 1fr auto;
    padding-inline: var(--space-2) 0;
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

  /* The fold control uses the same 34px target as the sidebar's navigation
     rows. The chevron stays optically small; the reliable hit area does not. */
  .side-fold {
    align-items: center;
    background: none;
    block-size: var(--control-height-compact);
    border: 0;
    border-radius: 6px;
    color: var(--sidebar-text-muted);
    cursor: pointer;
    display: inline-flex;
    grid-column: 2;
    grid-row: 1 / 3;
    inline-size: var(--control-height-compact);
    justify-content: center;
    padding: 0;
  }

  .side-fold:hover {
    background: var(--sidebar-item-hover);
    color: var(--sidebar-text);
  }

  .side-fold:active {
    background: var(--sidebar-item-pressed);
  }

  .fold-glyph {
    display: inline-flex;
  }

  /* The field that summons the palette, wearing the surface the sidebar gives its
     floating panels: one elevation step off the rail, in its own hue, in every
     palette. An ink wash read as a hover state at rest, and the page's own
     surface was a white slab on the console's dark violet. */
  .side-search {
    align-items: center;
    background: var(--sidebar-popover-bg);
    block-size: var(--control-height-compact);
    border: 1px solid var(--sidebar-popover-border);
    border-radius: var(--radius-control);
    box-sizing: border-box;
    color: var(--sidebar-text-secondary);
    cursor: pointer;
    display: flex;
    flex: none;
    font: inherit;
    font-size: var(--font-size-meta);
    font-weight: 500;
    /* The tree's own columns, through the tree's own gutter - less this field's own
       rule, which the rows do not have. Border-box counts a border inside the box and
       not inside the padding, so the same gutter put this glyph one pixel right of every
       glyph below it. */
    gap: var(--space-2);
    padding: 0 calc(var(--tree-gutter) - 1px);
    position: relative;
  }

  /* The tree's top continuation cue: when rows sit scrolled under the field, the
     field casts a shadow onto them - a real box-shadow, so it hugs the card's
     rounded corners, which no straight gradient band can. It lives on a pseudo so
     the button's own pressed shadow stays its own, and it fades in over the tree's
     first 20px of scroll. Guarded: a browser without scroll timelines would run the
     animation on TIME and pin the shadow on permanently, so without support there
     is simply no top cue. */
  .side-search::after {
    border-radius: inherit;
    box-shadow: 0 6px 10px -4px var(--sidebar-scroll-shadow);
    content: '';
    inset: 0;
    opacity: 0;
    pointer-events: none;
    position: absolute;
  }

  @supports (animation-timeline: scroll()) {
    .side-search::after {
      animation: side-cast var(--ease-linear) both;
      animation-range: 0 20px;
      animation-timeline: --side-tree;
    }
  }

  @keyframes side-cast {
    to {
      opacity: 1;
    }
  }

  .side-search:hover {
    background: var(--sidebar-menu-hover);
    color: var(--sidebar-text);
  }

  /* Only the ground. The crease, the sink and the scale come from the shell press law,
     and this scoped copy carried the PAGE's crease - which outranked the chrome's. */
  .side-search:active {
    background: var(--sidebar-menu-pressed);
  }

  /* Both runs trimmed to their own cap band, because they are two different sizes
     on one line: an untrimmed box is ascender-to-descender, and two of those centre
     their INK in two different places even when the boxes agree. */
  .side-search .t {
    flex: 1;
    text-align: start;
    text-box: trim-both cap alphabetic;
  }

  .side-search kbd {
    color: var(--sidebar-text-muted);
    font-family: inherit;
    font-size: var(--font-size-micro);
    text-box: trim-both cap alphabetic;
  }

  /* The workspace mark, and the switch behind it. Drawn only where the rail is
     not: with both on screen the same choice would be offered twice. */
  .side-ws-mini {
    align-items: center;
    background: none;
    block-size: var(--touch-target);
    border: 0;
    border-radius: 6px;
    cursor: pointer;
    display: none;
    grid-column: 1;
    grid-row: 2;
    inline-size: 100%;
    justify-content: center;
    padding: 0;
  }

  .side-ws-mini:hover {
    background: var(--sidebar-item-hover);
  }

  .side-ws-mini:active {
    background: var(--sidebar-item-pressed);
  }

  .side-ws-mini :global(.ws-mini) {
    block-size: 28px;
    font-size: 0.625rem;
    inline-size: 28px;
  }

  .side-ws-mini :global(.ws-mini.is-console) {
    background: var(--sidebar-active-bg);
    color: var(--sidebar-item-active-text);
  }

  /* Inbox and the account, pinned under the tree. */
  .side-foot {
    border-block-start: 1px solid var(--sidebar-border);
    display: none;
    gap: 2px;
    margin-block-start: auto;
    padding-block-start: var(--space-3);
  }

  .side-foot .tree-row {
    background: none;
    border: 0;
    cursor: pointer;
    font-family: inherit;
    inline-size: 100%;
    text-align: start;
  }

  .side-foot :global(.ws-mini) {
    border-radius: 50%;
  }

  .rail-badge {
    align-items: center;
    background: var(--sidebar-count-signal-bg);
    border-radius: 999px;
    block-size: var(--tier-mark);
    color: var(--sidebar-count-signal-ink);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    justify-content: center;
    line-height: var(--leading-flat);
    /* The unread badge is about the row's STATE rather than its name, so it takes the
       end of the row - unlike `.tab-count`, which rides its label. */
    margin-inline-start: auto;
    min-inline-size: var(--tier-mark);
    padding-inline: var(--space-1);
  }

  .rail-badge .t {
    text-box: trim-both cap alphabetic;
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
    /* The solid selection pair, not a near-white thumb. A pale fill carrying an
       inverse ink is white on white; the selected row is the console's own accent and
       the ink above it is that accent's. */
    background: var(--sidebar-active-bg);
    block-size: var(--nav-thumb-height, 0px);
    border-radius: var(--nav-thumb-radius, var(--radius-control));
    box-shadow: var(--sidebar-thumb-shadow);
    box-sizing: border-box;
    inline-size: var(--nav-thumb-width, 100%);
    inset-block-start: var(--nav-thumb-top, 0px);
    inset-inline-start: var(--nav-thumb-left, 0px);
    pointer-events: none;
    position: absolute;
    /* The four that carry it from one row to another take the TRAVEL pair; the two that
       answer a pointer take the press one. They were all on the state ramp, which is a
       frame quicker and an expo-out - a curve that arrives almost at once and then
       creeps, so a thumb the width of the tree crawled the last of the way. */
    transition:
      block-size var(--duration-travel) var(--ease-travel),
      inline-size var(--duration-travel) var(--ease-travel),
      inset-inline-start var(--duration-travel) var(--ease-travel),
      border-radius var(--duration-travel) var(--ease-travel),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  .tree:not(:global(.thumb-ready)) .nav-thumb {
    display: none;
  }

  /* NOTHING IN HERE LIFTS. The thumb had a hover lift mirroring the row's, from back
     when the rows lifted too; they do not, and neither does it - the mock declares that
     lift and then cancels it with a later rule of its own. What the pointer gets in this
     tree is a ground change, and what a press gets is the sink below.

     The press, on the selected row. Every pressable surface owes the shared
     pressed fill AND the inset crease, and this thumb had neither: its throw
     simply collapsed, which reads as a shadow leaving rather than as a surface
     being held. The fill advances from the thumb's OWN material, a veil laid
     over the identity colour rather than a neutral grey film: a grey one made
     the selection read as a plain hovered row, and the selection vanished.

     THE THUMB IS THE SELECTED ROW'S SURFACE, so it takes the row's whole press and not
     a part of it: the same sink, the same scale, about the same centre, because it is
     drawn at exactly the row's bounds. Sinking without scaling left the ink shrinking
     inside a fill that did not, which is the seam the exemption above was written to
     avoid rather than to fix. Through `scale` rather than `transform`: travel between
     rows is a WAAPI animation on this element's transform, and a press must not be
     something that animation has to carry. */
  .tree:has(.tree-row.is-active:active) .nav-thumb {
    background-image: linear-gradient(var(--interactive-pressed), var(--interactive-pressed));
    box-shadow: var(--sidebar-pressed-inset), var(--sidebar-thumb-shadow-pressed);
    translate: 0 1px;
  }

  .tree-row {
    align-items: center;
    /* Declared, whole: a row's height is a decision, not an outcome. */
    block-size: var(--control-height-compact);
    border-radius: var(--radius-control);
    box-sizing: border-box;
    color: var(--sidebar-text-secondary);
    display: flex;
    font-size: var(--font-size-meta);
    font-weight: 500;
    gap: var(--space-2);
    padding: 0 var(--tree-gutter);
    /* Positioned so the ink paints above the thumb sliding underneath. */
    position: relative;
    text-decoration: none;
    /* NO `transition: none` here, and none on the three controls above. It was on all
       four, uncommented, and it cancelled the shared ground fade every button and link
       in the product gets - so the tree's rows snapped between grounds while the thumb
       gliding under them eased for the whole travel duration and its pressed fill for
       the press one. What a reader saw was one object moving and its neighbours
       flicking. */
  }

  .tree-row .t {
    text-box: trim-both cap alphabetic;
  }

  /* The glyph sits in a gutter rather than hugging its word: a column of icons
     down the tree stays a column, which is what the app's ink-bearing pull
     would break if the symbol were the row's own first child.

     ONE TIER FOR THE WHOLE TREE, and it is the one the search field already wore. The
     rows were a tier and a half below it - 12px glyphs against the search's 16 - so the
     column a reader scans down changed weight at its own first row, and a 12px mark in a
     34px row read as a row with a gap in it rather than a row with a symbol. */
  .gi {
    align-items: center;
    display: inline-flex;
    flex: none;
  }

  .tree-row:hover {
    background: var(--sidebar-item-hover);
    color: var(--sidebar-text);
  }

  .tree-row:active {
    background: var(--sidebar-item-pressed);
  }

  /* THE BOX GOES IN - stated for the whole rail-and-sidebar family in `app.css`, with
     the reason. Only the ground is this row's own. */

  .tree-row:focus-visible {
    /* THE SIDEBAR'S OWN FOCUS, not the page's. `--focus` answers the grounds the PAGE
       has, and the Root rail is dark in both page themes - so in light Root the page's
       violet ring measured 2.76:1 against this chrome and was a ring nobody could see. */
    outline: var(--focus-ring-width) solid var(--sidebar-focus);
    outline-offset: var(--focus-ring-offset);
  }

  .tree-row.is-active {
    /* Ink only - the ground is the nav-thumb parked underneath. */
    background: transparent;
    color: var(--sidebar-item-active-text);
    font-weight: 600;
  }

  /* The selected row sheds every ground and every edge of its own: the thumb underneath
     is its surface, and a second crease drawn over it is a well inside a well. What the
     row keeps is the sink and the scale the shell law gives it, which is what carries
     its ink down with the thumb. */
  .tree-row.is-active:hover,
  .tree-row.is-active:active {
    background: transparent;
    box-shadow: none;
  }

  /* Section headers in the sidebar - ksai's tree-group, verbatim scale.

     The inline margin is the tree's gutter, so the word starts on the left edge of the
     glyphs below it. The block pair is unequal on purpose and in one direction: a
     heading belongs to what FOLLOWS it, so the air above it is the boundary between two
     groups and the air below it is one step of the tree's own rhythm. */
  .tree-group {
    color: var(--sidebar-text-muted);
    font-size: 0.625rem;
    font-weight: 650;
    letter-spacing: 0.09em;
    line-height: var(--leading-tight);
    margin: var(--space-5) var(--tree-gutter) var(--space-2);
    text-transform: uppercase;
  }

  .tree > .tree-group:first-of-type {
    margin-block-start: 0;
  }

  /* The one row that stands apart without a heading of its own. */
  .tree-row.is-foot {
    margin-block-start: var(--space-5);
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
    line-height: var(--leading-flat);
    margin-inline-start: auto;
    text-box: trim-both cap alphabetic;
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
    block-size: var(--tier-mark);
    color: var(--sidebar-text-secondary);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    line-height: var(--leading-flat);
    /* A COUNT RIDES ITS LABEL. It is part of what the row is called - "Plan 14" is one
       reading - and pushed to the far end it became a column of numbers down the tree's
       right edge, belonging to the sidebar rather than to any row in it. The end of a
       row belongs to the marks that are about its STATE: the unread badge and the
       unsaved mark, which keep their auto margin. */
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

  /* A count on the selected row rides that row's own inverse pair. */
  /* AN EDGE, NOT A FILL. Veiling the thumb with 16% of the ink about to be
     written on it moved the ground toward the text by definition: white on the
     petrol thumb reads 5.47:1, and white on white-at-16%-over-petrol reads
     4.00, which is under the floor. A border delineates the count without
     putting anything between the digits and the thumb they sit on. */
  .tree-row.is-active .tab-count {
    background: none;
    border: 1px solid color-mix(in srgb, var(--sidebar-item-active-text) 40%, transparent);
    color: var(--sidebar-item-active-text);
  }

  /* A count that demands attention keeps the thumb's own inverse pair, as the
     drawing has it - the signal tint is mixed for the sidebar's ground and has
     no business being read against the thumb. */
  .tree-row.is-active .tab-count.is-signal {
    background: var(--sidebar-item-active-text);
    border-color: var(--sidebar-item-active-text);
    color: var(--sidebar-active-bg);
    font-weight: 700;
  }

  /* ---------- Collapsed pages column ----------
     The pages column folds to an icon strip and keeps every destination
     reachable: rows become centred glyphs with their names as tooltips, a
     group's label goes but its boundary stays as a hairline, and a signal
     survives as a dot on its row's glyph. Only where the sidebar is in flow -
     below 64rem the drawer owns the narrow behaviour. */
  @media (min-width: 64.0625rem) {
    :global(.app-shell.sidebar-collapsed) .side {
      inline-size: var(--sidebar-width-collapsed);
      /* 12+11+1px border = rows an even 48, so a 16px glyph centres on a
         whole pixel. */
      padding-inline: 12px 11px;
      /* Sticky makes the column its own stacking context and the pane paints
         after it - without this a tooltip renders UNDER the content. */
      z-index: var(--layer-chrome);
    }

    /* The strip's asymmetric 12/11 insets carry into the scroller so rows stay
       a whole 48 wide, and the shade pulls in to the tiles - a full-width band
       would dwarf the narrow column it annotates. */
    :global(.app-shell.sidebar-collapsed) .side > .tree {
      background-position:
        0 100%,
        12px 100%;
      background-size:
        100% 20px,
        calc(100% - 23px) 10px;
      margin-inline: -12px -11px;
      padding-inline: 12px 11px;
    }

    :global(.app-shell.sidebar-collapsed) .side-head {
      grid-template-columns: 1fr;
      inline-size: 100%;
      justify-items: center;
      padding-inline: 0;
      row-gap: var(--space-2);
    }

    :global(.app-shell.sidebar-collapsed) .side-kicker,
    :global(.app-shell.sidebar-collapsed) .side-title {
      display: none;
    }

    :global(.app-shell.sidebar-collapsed) .side-fold {
      block-size: var(--touch-target);
      grid-column: 1;
      grid-row: 1;
      inline-size: 100%;
    }

    :global(.app-shell.sidebar-collapsed) .side-ws-mini {
      display: inline-flex;
    }

    :global(.app-shell.sidebar-collapsed) .side-search {
      block-size: var(--touch-target);
      gap: 0;
      justify-content: center;
      padding: 0;
    }

    :global(.app-shell.sidebar-collapsed) .side-search .t,
    :global(.app-shell.sidebar-collapsed) .side-search kbd {
      display: none;
    }

    /* The scroll cue ends on the footer's separator - the foot pulls itself up
       over the column's flex gap. */
    :global(.app-shell.sidebar-collapsed) .side > .tree {
      margin-block-end: 0;
    }

    :global(.app-shell.sidebar-collapsed) .side-foot {
      display: grid;
      margin-block-start: calc(var(--space-4) * -1);
    }

    :global(.app-shell.sidebar-collapsed) .side-foot .tree-row {
      gap: 0;
      justify-content: center;
      padding: 0;
      position: relative;
    }

    /* The unread mark leaves the flex flow for the row's upper-right corner, in
       the same dot grammar the page rows use, so the bell holds its column
       whether or not anything is unread. */
    :global(.app-shell.sidebar-collapsed) .side-foot .rail-badge {
      background: var(--sidebar-count-signal-ink);
      block-size: 6px;
      inline-size: 6px;
      inset-block-start: 6px;
      inset-inline-end: 8px;
      min-inline-size: 0;
      padding: 0;
      position: absolute;
    }

    :global(.app-shell.sidebar-collapsed) .side-foot .rail-badge .t {
      display: none;
    }

    :global(.app-shell.sidebar-collapsed) .tree-row {
      block-size: var(--touch-target);
      gap: 0;
      justify-content: center;
      padding: 0;
    }

    /* Keep each destination's word in the accessibility tree while the strip is
       icon-only: clipped, not removed. */
    :global(.app-shell.sidebar-collapsed) .tree-row > .t {
      block-size: 1px;
      clip-path: inset(50%);
      inline-size: 1px;
      overflow: hidden;
      padding: 0;
      position: absolute;
      white-space: nowrap;
    }

    /* Collapsed, a group's LABEL goes but its boundary stays: the heading
       renders as a hairline, so the icon stack keeps the tree's rhythm without
       bringing text back. Screen readers keep the group names. */
    :global(.app-shell.sidebar-collapsed) .tree-group {
      background: var(--sidebar-border);
      block-size: 1px;
      color: transparent;
      margin: var(--space-1) var(--space-2);
      overflow: hidden;
      padding: 0;
    }

    /* A signal is a waiting plan. Dirty state has its own keyed mark below. */
    :global(.app-shell.sidebar-collapsed) .tree-row:has(.tab-count.is-signal)::before {
      background: var(--sidebar-count-signal-ink);
      border-radius: 50%;
      block-size: 6px;
      content: '';
      inline-size: 6px;
      inset-block-start: 6px;
      inset-inline-end: 8px;
      position: absolute;
    }

    :global(.app-shell.sidebar-collapsed) .tree-row .tab-count {
      display: none;
    }

    :global(.app-shell.sidebar-collapsed) .tree-row > .dirty-mark {
      inset-block-start: -3px;
      inset-inline-end: -3px;
      margin: 0;
      position: absolute;
    }
  }

  /* The collapsed strip's one tooltip. It lives inside the sidebar so it wears the
     ACTIVE console's material - the Root strip is dark in both page themes and its tip
     stays dark with it - and it is FIXED, which is what carries it out of the tree's
     own overflow. */
  .side-tip {
    background: var(--sidebar-popover-bg);
    border: 1px solid var(--sidebar-popover-border);
    border-radius: 6px;
    box-shadow: var(--shadow-popover);
    color: var(--sidebar-menu-text);
    font-size: var(--font-size-micro);
    inset-block-start: var(--tip-y, 0);
    inset-inline-start: var(--tip-x, 0);
    padding: var(--space-1) var(--space-2);
    pointer-events: none;
    position: fixed;
    translate: 0 -50%;
    white-space: nowrap;
    z-index: var(--layer-flyout);
  }

  /* ---------- Responsive: the sidebar becomes a drawer, never nothing ----------
     Below 48rem the pages column cannot share the row with content, and neither can
     the rail: both leave together and the top bar takes their jobs. The drawer slides
     over the content from the viewport's own edge - there is no rail left to start
     from - above a scrim. */
  @media (max-width: 47.9375rem) {
    .side {
      box-shadow: 8px 0 24px var(--shadow-color);
      inline-size: min(20rem, 86vw);
      inset-block: 0;
      inset-inline-start: 0;
      position: fixed;
      transition:
        translate var(--duration-fast) var(--ease-standard),
        visibility 0s var(--duration-fast);
      translate: -110% 0;
      visibility: hidden;
      z-index: var(--layer-side);
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

    /* The drawer holds the whole shell, so it holds the foot too. */
    .side-foot {
      display: grid;
    }
  }
</style>
