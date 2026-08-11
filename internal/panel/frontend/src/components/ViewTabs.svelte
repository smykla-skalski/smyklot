<script lang="ts">
  import type { PanelView, RootSection } from '../lib/routes';
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
    if (view === 'users' || view === 'invitations') return 'Access';
    return view.slice(0, 1).toUpperCase() + view.slice(1);
  }

  function icon(view: PanelView): IconName {
    if (view === 'settings') return 'settings';
    if (view === 'repositories') return 'repositories';
    if (view === 'users' || view === 'invitations') return 'users';
    return 'history';
  }

  function rootLabel(section: RootSection): string {
    return section.slice(0, 1).toUpperCase() + section.slice(1);
  }

  function rootIcon(section: RootSection): IconName {
    if (section === 'overview') return 'system';
    if (section === 'installations') return 'organization';
    if (section === 'access') return 'users';
    if (section === 'history') return 'history';
    return 'settings';
  }
</script>

<nav class={['panel-navigation', collapsed && 'collapsed']} aria-label="Panel navigation">
  <div class="view-links">
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
    transform: translateY(1px);
  }

  /* Selected item is a raised thumb, the same language as the app's segmented
     controls: the accent lives in the text and icon, not a bar or a tint. */
  a.active {
    background: var(--sidebar-thumb);
    box-shadow: var(--sidebar-thumb-shadow);
    color: var(--sidebar-item-active-text);
    font-weight: 700;
  }

  a.active:hover {
    background: color-mix(in srgb, var(--sidebar-item-active-text) 6%, var(--sidebar-thumb));
    color: var(--sidebar-item-active-text);
  }

  a.active:active {
    background: color-mix(in srgb, var(--sidebar-item-active-text) 12%, var(--sidebar-thumb));
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
