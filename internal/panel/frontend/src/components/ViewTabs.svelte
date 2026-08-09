<script lang="ts">
  import { PANEL_VIEWS, type PanelView } from '../lib/routes';
  import Icon, { type IconName } from './Icon.svelte';

  const {
    value,
    hrefFor,
    onSelect,
    showUsers,
    collapsed,
  }: {
    value: PanelView;
    hrefFor: (view: PanelView) => string;
    onSelect: (view: PanelView) => void;
    showUsers: boolean;
    collapsed: boolean;
  } = $props();

  const visibleViews = $derived(PANEL_VIEWS.filter((view) => view !== 'users' || showUsers));

  function selectFromClick(event: MouseEvent, next: PanelView): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
    event.preventDefault();
    onSelect(next);
  }

  function label(view: PanelView): string {
    return view.slice(0, 1).toUpperCase() + view.slice(1);
  }

  function icon(view: PanelView): IconName {
    if (view === 'settings') return 'settings';
    if (view === 'repositories') return 'repositories';
    if (view === 'users') return 'users';
    if (view === 'history') return 'history';
    return 'help';
  }
</script>

<nav class={['panel-navigation', collapsed && 'collapsed']} aria-label="Panel navigation">
  <div class="view-links">
    {#each visibleViews as item (item)}
      <a
        href={hrefFor(item)}
        id={`${item}-navigation`}
        class={[value === item && 'active', item === 'help' && 'help-link']}
        aria-label={collapsed ? label(item) : undefined}
        aria-current={value === item ? 'page' : undefined}
        onclick={(event) => selectFromClick(event, item)}
      >
        <span class="navigation-icon"><Icon name={icon(item)} size={20} /></span>
        <span class="navigation-label">{label(item)}</span>
        <span class="navigation-tooltip">{label(item)}</span>
      </a>
    {/each}
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

  .help-link {
    margin-top: auto;
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

    .help-link {
      border-top: 1px solid var(--border-subtle);
      border-radius: 0;
      margin-top: var(--space-2);
      padding-top: var(--space-2);
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
