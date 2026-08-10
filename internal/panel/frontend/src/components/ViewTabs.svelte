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
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
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
    if (view === 'history') return 'history';
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
      <a
        class="return-entry"
        class:disabled={returnHref === '#'}
        href={returnHref}
        aria-label={collapsed ? 'Return to panel' : undefined}
        aria-disabled={returnHref === '#' ? 'true' : undefined}
        onclick={returnFromClick}
      >
        <span class="navigation-icon"><Icon name="chevron-left" size={20} /></span>
        <span class="navigation-label">Return to panel</span>
        <span class="navigation-tooltip">Return to panel</span>
      </a>
    {:else}
      {#if rootEnabled}
        <a
          class="root-entry"
          href={rootDashboardHref}
          aria-label={collapsed ? 'Root dashboard' : undefined}
          onclick={enterRootFromClick}
        >
          <span class="navigation-icon"><Icon name="admin" size={20} strokeWidth={2} /></span>
          <span class="navigation-label">Root dashboard</span>
          <span class="navigation-tooltip">Root dashboard</span>
        </a>
      {/if}
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
    gap: var(--space-1);
    height: 100%;
  }

  a {
    align-items: center;
    border-radius: var(--radius-control);
    color: var(--sidebar-text-muted);
    display: flex;
    font-size: var(--font-size-control);
    font-weight: 600;
    gap: var(--space-3);
    line-height: 1;
    min-height: 2.75rem;
    padding: 1px var(--space-3) 0;
    position: relative;
    text-decoration: none;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard);
  }

  a:hover {
    background: var(--sidebar-item-hover);
    color: var(--sidebar-text);
  }

  a:active {
    background: var(--sidebar-item-pressed);
  }

  a.active {
    background: var(--sidebar-item-active);
    color: var(--sidebar-item-active-text);
    font-weight: 700;
  }

  a.active:hover {
    background-color: var(--interactive-selected-hover);
    color: var(--sidebar-item-active-text);
  }

  a.active:active {
    background-color: var(--interactive-selected-pressed);
  }

  .root-entry {
    background:
      linear-gradient(var(--sidebar-bg), var(--sidebar-bg)) padding-box,
      var(--footer-spectrum) border-box;
    border: 2px solid transparent;
    color: var(--sidebar-text);
    font-weight: 700;
    margin-bottom: var(--space-2);
  }

  .root-entry:hover {
    background:
      linear-gradient(var(--sidebar-item-hover), var(--sidebar-item-hover)) padding-box,
      var(--footer-spectrum) border-box;
  }

  .root-entry:active {
    background:
      linear-gradient(var(--sidebar-item-pressed), var(--sidebar-item-pressed)) padding-box,
      var(--footer-spectrum) border-box;
  }

  .return-entry {
    border-top: 1px solid var(--sidebar-border);
    border-radius: 0;
    margin-top: auto;
    padding-top: var(--space-3);
  }

  .return-entry.disabled {
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

  a.active .navigation-icon {
    color: var(--sidebar-item-active-text);
  }

  .navigation-tooltip {
    display: none;
  }

  .collapsed a {
    justify-content: center;
    padding: 0;
  }

  .collapsed .navigation-label {
    display: none;
  }

  .collapsed .navigation-tooltip {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-control);
    box-shadow: var(--shadow-popover);
    color: var(--text-primary);
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

  @media (max-width: 64rem) {
    .view-links {
      gap: var(--space-1);
    }

    a {
      min-height: 2.75rem;
    }

    .collapsed a {
      justify-content: flex-start;
      padding: 0 var(--space-3);
    }

    .collapsed .navigation-label {
      display: inline;
    }

    .collapsed .navigation-tooltip {
      display: none;
    }
  }
</style>
