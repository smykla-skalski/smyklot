<script lang="ts">
  import {
    panelViewSection,
    routeSegmentLabel,
    type PanelView,
    type RootSection,
  } from '../lib/routes';
  import Icon, { type IconName } from './Icon.svelte';

  const {
    value,
    hrefFor,
    onSelect,
    showUsers,
    collapsed,
    rootMode,
    rootEnabled,
    rootValue,
    rootHrefFor,
    onSelectRoot,
    rootDashboardHref,
    onEnterRoot,
    returnHref,
    onReturnToPanel,
  }: {
    value: PanelView;
    hrefFor: (view: PanelView) => string;
    onSelect: (view: PanelView) => void;
    showUsers: boolean;
    collapsed: boolean;
    rootMode: boolean;
    rootEnabled: boolean;
    rootValue: RootSection;
    rootHrefFor: (section: RootSection) => string;
    onSelectRoot: (section: RootSection) => void;
    rootDashboardHref: string;
    onEnterRoot: () => void;
    returnHref: string;
    onReturnToPanel: () => void;
  } = $props();

  const NAVIGATION_VIEWS = [
    'settings',
    'repositories',
    'users',
    'history',
  ] as const satisfies readonly PanelView[];

  const visibleViews = $derived(NAVIGATION_VIEWS.filter((view) => view !== 'users' || showUsers));
  const ROOT_SECTIONS = [
    'overview',
    'installations',
    'access',
    'history',
    'settings',
  ] as const satisfies readonly RootSection[];

  function isActive(item: PanelView): boolean {
    return value === item || (item === 'users' && value === 'invitations');
  }

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
    if (view === 'users' || view === 'invitations') return 'users';
    return 'history';
  }

  function rootLabel(section: RootSection): string {
    return routeSegmentLabel(section);
  }

  function rootIcon(section: RootSection): IconName {
    if (section === 'overview') return 'system';
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
  function followSelection(node: HTMLElement, current: string) {
    let frame: number | undefined;
    let selection = current;

    function place(): void {
      if (frame !== undefined) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        frame = requestAnimationFrame(() => {
          frame = undefined;
          const active = node.querySelector<HTMLElement>('a.active');
          if (active === null) {
            node.style.setProperty('--nav-thumb-height', '0px');
            node.classList.remove('thumb-ready');
            return;
          }
          node.style.setProperty('--nav-thumb-top', `${active.offsetTop}px`);
          node.style.setProperty('--nav-thumb-height', `${active.offsetHeight}px`);
          node.classList.add('thumb-ready');
        });
      });
    }

    place();
    const resize = new ResizeObserver(place);
    resize.observe(node);

    return {
      update(next: string) {
        if (next === selection) return;
        selection = next;
        place();
      },
      destroy() {
        resize.disconnect();
        if (frame !== undefined) cancelAnimationFrame(frame);
      },
    };
  }
</script>

<nav class={['panel-navigation', collapsed && 'collapsed']} aria-label="Panel navigation">
  <div
    class="view-links"
    use:followSelection={`${rootMode ? 'root' : 'panel'}:${rootValue ?? ''}:${value ?? ''}:${collapsed}`}
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
      <div class="navigation-spacer"></div>
      <div class="admin-zone">
        <a
          class="admin-entry"
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
      </div>
    {:else}
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
      {#if rootEnabled}
        <div class="navigation-spacer"></div>
        <div class="admin-zone">
          <a
            class="admin-entry root-entry"
            href={rootDashboardHref}
            aria-label={collapsed ? 'Root console' : undefined}
            onclick={enterRootFromClick}
          >
            <span class="navigation-icon"><Icon name="shield" size={20} /></span>
            <span class="navigation-label">Root console</span>
            <span class="navigation-tooltip">Root console</span>
          </a>
        </div>
      {/if}
    {/if}
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

  .nav-label {
    color: var(--sidebar-text-muted);
    font: 700 var(--font-size-micro) / 1 var(--sans);
    letter-spacing: 0.09em;
    margin: 0 0 var(--space-2);
    padding-inline: var(--space-3);
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
    padding: 1px var(--space-3) 0;
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
      top 240ms cubic-bezier(0.22, 1, 0.36, 1),
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

  .admin-zone {
    border-top: 1px solid var(--sidebar-border);
    margin-top: var(--space-2);
    padding-top: var(--space-2);
  }

  .root-entry .navigation-icon {
    color: var(--sidebar-root-accent);
  }

  .root-entry:hover {
    background: color-mix(in srgb, var(--sidebar-root-accent) 10%, transparent);
  }

  .admin-entry.disabled {
    cursor: default;
    opacity: 0.45;
  }

  .navigation-icon {
    align-items: center;
    display: inline-flex;
    flex: none;
    justify-content: center;
  }

  .navigation-label {
    align-items: center;
    display: inline-flex;
    height: 1.25rem;
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
    left: calc(100% + var(--space-2));
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
  }
</style>
