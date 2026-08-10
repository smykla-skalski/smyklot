<script lang="ts">
  import { fuzzyCandidates } from '../lib/fuzzy';
  import { handleLabel, readHandle } from '../lib/identity';
  import type { ThemeDisplay } from '../lib/preferences';
  import type { PanelView } from '../lib/routes';
  import type { PanelTarget, PanelViewer } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Icon from './Icon.svelte';
  import ViewTabs from './ViewTabs.svelte';

  const {
    viewer,
    iconUrl,
    targets,
    selectedId,
    targetHref,
    onSelectTarget,
    onSignOut,
    view,
    viewHref,
    onSelectView,
    showUsers,
    showNavigation,
    collapsed,
    onToggleCollapsed,
    theme,
    onSelectTheme,
  }: {
    viewer: PanelViewer | null;
    iconUrl: string;
    targets: PanelTarget[];
    selectedId: string | null;
    targetHref: (target: PanelTarget) => string;
    onSelectTarget: (targetId: string) => void;
    onSignOut: () => void | Promise<void>;
    view: PanelView;
    viewHref: (view: PanelView) => string;
    onSelectView: (view: PanelView) => void;
    showUsers: boolean;
    showNavigation: boolean;
    collapsed: boolean;
    onToggleCollapsed: () => void;
    theme: ThemeDisplay;
    onSelectTheme: (theme: ThemeDisplay) => void;
  } = $props();

  let accountMenu = $state<HTMLDetailsElement | null>(null);
  let accountTrigger = $state<HTMLElement | null>(null);
  let targetMenu = $state<HTMLDetailsElement | null>(null);
  let targetTrigger = $state<HTMLElement | null>(null);
  let targetSearchInput = $state<HTMLInputElement | null>(null);
  let targetQuery = $state('');
  let mobileNavigationOpen = $state(false);

  const handle = $derived(
    viewer === null ? null : readHandle(viewer.account.provider, viewer.account.login),
  );
  const selectedTarget = $derived(
    selectedId === null ? null : (targets.find((target) => target.id === selectedId) ?? null),
  );
  const targetCandidates = $derived(
    fuzzyCandidates(
      targets.map((target) => ({
        ...target,
        label: target.account.display_name,
        keywords: [target.account.login, target.type],
      })),
      targetQuery,
    ),
  );
  const organizationTargets = $derived(
    targetCandidates.filter((target) => target.type === 'Organization'),
  );
  const personalTargets = $derived(targetCandidates.filter((target) => target.type === 'User'));

  function closeMenus(except?: HTMLDetailsElement): void {
    if (accountMenu !== null && accountMenu !== except) accountMenu.open = false;
    if (targetMenu !== null && targetMenu !== except) {
      targetMenu.open = false;
      targetQuery = '';
    }
  }

  function closeFromOutside(event: PointerEvent): void {
    if (!(event.target instanceof Node)) return;
    if (accountMenu?.open === true && !accountMenu.contains(event.target)) accountMenu.open = false;
    if (targetMenu?.open === true && !targetMenu.contains(event.target)) {
      targetMenu.open = false;
      targetQuery = '';
    }
    const sidebar = document.querySelector('.panel-sidebar');
    if (mobileNavigationOpen && sidebar !== null && !sidebar.contains(event.target)) {
      mobileNavigationOpen = false;
    }
  }

  function closeFromKeyboard(event: KeyboardEvent): void {
    if (event.key !== 'Escape') return;
    if (accountMenu?.open === true) {
      event.preventDefault();
      accountMenu.open = false;
      accountTrigger?.focus();
      return;
    }
    if (targetMenu?.open === true) {
      event.preventDefault();
      targetMenu.open = false;
      targetQuery = '';
      targetTrigger?.focus();
      return;
    }
    if (mobileNavigationOpen) {
      event.preventDefault();
      mobileNavigationOpen = false;
      document.querySelector<HTMLElement>('.mobile-navigation-trigger')?.focus();
    }
  }

  async function signOut(): Promise<void> {
    closeMenus();
    mobileNavigationOpen = false;
    await onSignOut();
  }

  function selectTarget(event: MouseEvent, targetId: string): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
    event.preventDefault();
    closeMenus();
    targetQuery = '';
    mobileNavigationOpen = false;
    onSelectTarget(targetId);
  }

  function selectView(next: PanelView): void {
    mobileNavigationOpen = false;
    onSelectView(next);
  }

  function toggleDetails(event: Event, menu: HTMLDetailsElement | null): void {
    if (!(event.currentTarget instanceof HTMLDetailsElement) || menu === null) return;
    if (event.currentTarget.open) closeMenus(menu);
  }

  function toggleTargetDetails(event: Event): void {
    toggleDetails(event, targetMenu);
    if (!(event.currentTarget instanceof HTMLDetailsElement)) return;
    targetQuery = '';
    if (event.currentTarget.open) queueMicrotask(() => targetSearchInput?.focus());
  }
</script>

<svelte:window onkeydown={closeFromKeyboard} />
<svelte:document onpointerdown={closeFromOutside} />

<aside
  class={[
    'panel-sidebar',
    mobileNavigationOpen && 'mobile-navigation-open',
    collapsed && 'collapsed',
  ]}
>
  <div class="brand-row">
    <h1 class="mark">
      <img class="mark-icon" src={iconUrl} alt="" width="32" height="32" decoding="async" />
      <span class="mark-copy">
        <span class="mark-name">Smyklot</span>
        <span class="mark-part">PANEL</span>
      </span>
    </h1>

    {#if showNavigation}
      <button
        class="sidebar-collapse-trigger"
        type="button"
        aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        aria-expanded={!collapsed}
        onclick={onToggleCollapsed}
      >
        <Icon name={collapsed ? 'chevron-right' : 'chevron-left'} size={14} strokeWidth={2} />
        <span class="sidebar-tooltip">{collapsed ? 'Expand sidebar' : 'Collapse sidebar'}</span>
      </button>

      <button
        class="mobile-navigation-trigger"
        type="button"
        aria-label="Toggle panel navigation"
        aria-expanded={mobileNavigationOpen}
        aria-controls="panel-navigation-drawer"
        onclick={() => (mobileNavigationOpen = !mobileNavigationOpen)}
      >
        <span aria-hidden="true"></span>
        <span aria-hidden="true"></span>
        <span aria-hidden="true"></span>
        <span>Menu</span>
      </button>
    {/if}
  </div>

  {#if showNavigation && selectedTarget !== null}
    <details class="target-menu" bind:this={targetMenu} ontoggle={toggleTargetDetails}>
      <summary
        class="target-trigger"
        bind:this={targetTrigger}
        aria-label={`Switch installation, currently ${selectedTarget.account.display_name}`}
      >
        <Avatar account={selectedTarget.account} size={28} />
        <span class="target-trigger-copy">
          <strong>{selectedTarget.account.display_name}</strong>
          <span class="mono">@{selectedTarget.account.login}</span>
        </span>
        <span class="menu-chevron" aria-hidden="true"><Icon name="chevron-down" size={16} /></span>
        <span class="sidebar-tooltip">Switch installation</span>
      </summary>

      <div class="target-popover">
        <label class="target-search">
          <span class="visually-hidden">Search installations</span>
          <span class="target-search-icon" aria-hidden="true"><Icon name="search" size={18} /></span
          >
          <input
            type="search"
            placeholder="Search installations"
            bind:this={targetSearchInput}
            bind:value={targetQuery}
          />
        </label>
        <div class="target-options">
          {#snippet targetOption(target: PanelTarget)}
            <a
              href={targetHref(target)}
              class={['target-option', target.id === selectedId && 'current']}
              aria-current={target.id === selectedId ? 'page' : undefined}
              onclick={(event) => selectTarget(event, target.id)}
            >
              <Avatar account={target.account} size={28} />
              <span class="option-copy">
                <strong>{target.account.display_name}</strong>
                <span class="mono">@{target.account.login}</span>
              </span>
              <span class="option-check" aria-hidden="true">
                {#if target.id === selectedId}<Icon name="success" size={16} />{/if}
              </span>
            </a>
          {/snippet}

          {#if organizationTargets.length > 0}
            <p class="target-group-label">Organizations</p>
            {#each organizationTargets as target (target.id)}
              {@render targetOption(target)}
            {/each}
          {/if}

          {#if personalTargets.length > 0}
            <p class="target-group-label">Personal</p>
            {#each personalTargets as target (target.id)}
              {@render targetOption(target)}
            {/each}
          {/if}

          {#if targetCandidates.length === 0}
            <p class="target-empty">No installations match “{targetQuery.trim()}”</p>
          {/if}
        </div>
      </div>
    </details>
  {/if}

  {#if showNavigation}
    <div id="panel-navigation-drawer" class="navigation-shell">
      <ViewTabs value={view} hrefFor={viewHref} onSelect={selectView} {showUsers} {collapsed} />
    </div>
  {/if}

  {#if viewer !== null && handle !== null}
    <details
      class="account-menu"
      bind:this={accountMenu}
      ontoggle={(event) => toggleDetails(event, accountMenu)}
    >
      <summary
        class="who"
        bind:this={accountTrigger}
        aria-label={`Account menu for ${viewer.account.display_name}, ${selectedTarget?.account.display_name ?? handleLabel(handle)}`}
      >
        <Avatar account={viewer.account} size={34} />
        <span class="who-text">
          <span class="who-name">{viewer.account.display_name}</span>
          <span class="who-meta">
            <span class="who-context">
              {selectedTarget?.account.display_name ?? handleLabel(handle)}
            </span>
          </span>
        </span>
        <span class="menu-chevron" aria-hidden="true"><Icon name="chevron-down" size={16} /></span>
        <span class="sidebar-tooltip">Account menu</span>
      </summary>
      <div class="account-popover">
        <div class="theme-setting">
          <span class="theme-label">Theme</span>
          <div class="theme-options" role="group" aria-label="Theme">
            <button
              type="button"
              class:selected={theme === 'system'}
              aria-pressed={theme === 'system'}
              onclick={() => onSelectTheme('system')}
            >
              <Icon name="system" size={15} />
              <span>System</span>
            </button>
            <button
              type="button"
              class:selected={theme === 'light'}
              aria-pressed={theme === 'light'}
              onclick={() => onSelectTheme('light')}
            >
              <Icon name="sun" size={15} />
              <span>Light</span>
            </button>
            <button
              type="button"
              class:selected={theme === 'dark'}
              aria-pressed={theme === 'dark'}
              onclick={() => onSelectTheme('dark')}
            >
              <Icon name="moon" size={15} />
              <span>Dark</span>
            </button>
          </div>
        </div>
        <button class="account-action" type="button" onclick={signOut}>
          <Icon name="sign-out" size={16} />
          <span>Sign out</span>
        </button>
      </div>
    </details>
  {/if}
</aside>

<style>
  .panel-sidebar {
    background: var(--sidebar-bg);
    border-right: 1px solid var(--sidebar-border);
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    height: 100dvh;
    min-height: 36rem;
    padding: var(--space-5) var(--space-4) 0;
    position: sticky;
    top: 0;
    transition: padding var(--duration-normal) var(--ease-standard);
    z-index: var(--layer-sticky);
  }

  .brand-row {
    align-items: center;
    display: flex;
    justify-content: space-between;
    min-height: 2.5rem;
  }

  .mark {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    margin: 0;
    min-width: 0;
  }

  .mark-icon {
    flex: none;
    object-fit: contain;
  }

  .mark-copy {
    display: grid;
    gap: 0.2rem;
    min-width: 0;
  }

  .mark-name {
    color: var(--sidebar-text);
    font: 700 var(--font-size-body) / 1 var(--sans);
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .mark-part {
    color: var(--sidebar-text-muted);
    font: 500 var(--font-size-micro) / 1 var(--sans);
    letter-spacing: 0.08em;
  }

  .sidebar-collapse-trigger,
  .mobile-navigation-trigger {
    align-items: center;
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--radius-control);
    color: var(--sidebar-text-muted);
    display: flex;
    flex: none;
    height: 1.75rem;
    justify-content: center;
    padding: 0;
    width: 1.75rem;
  }

  .sidebar-collapse-trigger {
    background: transparent;
    color: var(--sidebar-text-muted);
    cursor: pointer;
    opacity: 0;
    pointer-events: none;
    position: static;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      opacity var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
    z-index: 2;
  }

  .panel-sidebar:hover .sidebar-collapse-trigger,
  .panel-sidebar:focus-within .sidebar-collapse-trigger {
    opacity: 1;
    pointer-events: auto;
  }

  .sidebar-collapse-trigger:hover,
  .sidebar-collapse-trigger:focus-visible,
  .mobile-navigation-trigger:hover,
  .mobile-navigation-trigger:focus-visible {
    background: var(--sidebar-item-hover);
    color: var(--sidebar-text);
  }

  .sidebar-collapse-trigger:active,
  .mobile-navigation-trigger:active {
    background: var(--sidebar-item-pressed);
    transform: translateY(1px);
  }

  .mobile-navigation-trigger {
    display: none;
  }

  .mobile-navigation-trigger > span[aria-hidden='true'] {
    background: currentColor;
    display: block;
    height: 1px;
    position: absolute;
    width: 0.875rem;
  }

  .mobile-navigation-trigger > span[aria-hidden='true']:first-child {
    transform: translateY(-4px);
  }

  .mobile-navigation-trigger > span[aria-hidden='true']:nth-child(3) {
    transform: translateY(4px);
  }

  .mobile-navigation-trigger > span:last-child {
    margin-left: 1.25rem;
  }

  .target-menu,
  .account-menu {
    isolation: isolate;
    position: relative;
    z-index: var(--layer-popover);
  }

  .target-trigger,
  .who {
    align-items: center;
    background: var(--identity-bg);
    border: 1px solid var(--identity-border);
    border-radius: var(--radius-control);
    cursor: pointer;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 3.75rem;
    padding: var(--space-2) var(--space-3);
    position: relative;
    user-select: none;
  }

  .target-trigger::-webkit-details-marker,
  .who::-webkit-details-marker {
    display: none;
  }

  .target-trigger::marker,
  .who::marker {
    content: '';
  }

  .target-trigger:hover,
  .target-menu[open] .target-trigger,
  .who:hover,
  .account-menu[open] .who {
    background: var(--identity-hover-bg);
    border-color: color-mix(in srgb, var(--focus) 40%, var(--identity-border));
  }

  .target-trigger:active,
  .who:active {
    background: var(--sidebar-item-pressed);
  }

  .target-trigger-copy,
  .who-text {
    display: flex;
    flex-direction: column;
    gap: 0.18rem;
    min-width: 0;
    text-align: left;
  }

  .target-trigger-copy strong,
  .who-name {
    color: var(--sidebar-text);
    font-size: var(--font-size-body);
    font-weight: 600;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .target-trigger-copy span,
  .who-context {
    color: var(--sidebar-text-muted);
    font-size: var(--font-size-compact);
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .menu-chevron {
    color: var(--sidebar-text-muted);
    display: grid;
    place-items: center;
    transition: transform var(--duration-fast) var(--ease-standard);
  }

  details[open] > summary .menu-chevron {
    transform: rotate(180deg);
  }

  .target-popover,
  .account-popover {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    left: var(--space-2);
    max-height: min(32rem, calc(100dvh - 2rem));
    overflow: hidden;
    padding: var(--space-2);
    position: absolute;
    z-index: var(--layer-popover);
  }

  .target-popover {
    top: calc(100% + var(--space-2));
    width: min(19rem, calc(100vw - 2rem));
  }

  .account-popover {
    bottom: calc(100% + var(--space-2));
    left: 0;
    right: auto;
    width: min(17rem, calc(100vw - 2rem));
  }

  .target-options {
    display: grid;
    gap: 2px;
    max-height: min(24rem, calc(100dvh - 10rem));
    overflow: auto;
  }

  .target-search {
    background: var(--popover-bg);
    border-bottom: 1px solid var(--border-subtle);
    display: block;
    padding: 0 0 var(--space-2);
    position: relative;
    z-index: 2;
  }

  .target-search input {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text-primary);
    font: 500 var(--font-size-meta) / 1 var(--sans);
    height: var(--control-height);
    padding: 0 var(--space-3) 0 2.25rem;
    width: 100%;
  }

  .target-search input:focus-visible {
    border-color: var(--focus);
    box-shadow: inset 0 0 0 1px var(--focus);
    outline: 0;
  }

  .target-search input::placeholder {
    color: var(--text-muted);
  }

  .target-search-icon {
    color: var(--text-muted);
    display: grid;
    left: 0.625rem;
    place-items: center;
    position: absolute;
    top: 0.6875rem;
  }

  .target-group-label {
    background: var(--popover-bg);
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    font-weight: 650;
    letter-spacing: 0.04em;
    margin: 0;
    padding: var(--space-1) var(--space-2);
    position: sticky;
    top: 0;
    text-transform: uppercase;
    z-index: 1;
  }

  .target-empty {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: 0;
    padding: var(--space-5) var(--space-3);
    text-align: center;
  }

  .target-option {
    align-items: center;
    background: transparent;
    border-radius: calc(var(--radius-control) - 2px);
    color: var(--text-primary);
    display: grid;
    gap: var(--space-2);
    grid-template-columns: auto minmax(0, 1fr) 1rem;
    min-height: 2.75rem;
    padding: var(--space-2);
    text-align: left;
    text-decoration: none;
  }

  .target-option:hover,
  .target-option:focus-visible {
    background: var(--interactive-hover);
  }

  .target-option.current {
    background: var(--brand-action-tint);
    color: var(--brand-action-text);
  }

  .target-option:active {
    background: var(--sidebar-item-pressed);
  }

  .option-copy {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .option-copy strong,
  .option-copy span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .option-copy strong {
    font-size: var(--font-size-meta);
  }

  .option-copy span,
  .option-check {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
  }

  .option-check {
    color: var(--success);
    font-weight: 700;
    text-align: center;
  }

  .navigation-shell {
    flex: 1;
    min-height: 0;
  }

  .account-menu {
    border-top: 1px solid var(--sidebar-border);
    margin: auto calc(var(--space-4) * -1) 0;
  }

  .who {
    background: transparent;
    border: 0;
    border-radius: 0;
    min-height: 4.5rem;
    padding: var(--space-3) var(--space-4);
    overflow: hidden;
  }

  .who-meta {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-width: 0;
  }

  .who-context {
    min-width: 0;
  }

  .account-action {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--radius-control);
    color: var(--text-primary);
    display: flex;
    font-size: var(--font-size-meta);
    gap: var(--space-2);
    height: var(--control-height);
    padding: 0 var(--space-3);
    text-align: left;
    width: 100%;
  }

  .theme-setting {
    border-bottom: 1px solid var(--border-subtle);
    display: grid;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
    padding: var(--space-2) var(--space-1) var(--space-3);
  }

  .theme-label {
    color: var(--text-muted);
    font: 600 var(--font-size-compact) / 1 var(--sans);
    padding-inline: var(--space-1);
  }

  .theme-options {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: grid;
    gap: var(--control-inset);
    grid-template-columns: repeat(3, minmax(0, 1fr));
    padding: var(--control-inset);
  }

  .theme-options button {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: calc(var(--radius-control) - var(--control-inset));
    color: var(--text-muted);
    display: inline-flex;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    gap: var(--space-1);
    height: var(--control-height-compact);
    justify-content: center;
    padding: 0 var(--space-1);
  }

  .theme-options button:hover {
    background: var(--interactive-hover);
    color: var(--text-primary);
  }

  .theme-options button.selected {
    background: var(--surface-base);
    box-shadow: 0 1px 2px var(--shadow-color);
    color: var(--brand-action-text);
  }

  .theme-options button:active {
    background: var(--interactive-pressed);
  }

  .account-action:hover,
  .account-action:focus-visible {
    background: var(--interactive-hover);
  }

  .account-action:active {
    background: var(--interactive-pressed);
  }

  .sidebar-tooltip {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-control);
    box-shadow: var(--shadow-popover);
    color: var(--text-primary);
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

  .collapsed .sidebar-collapse-trigger:hover .sidebar-tooltip,
  .collapsed .sidebar-collapse-trigger:focus-visible .sidebar-tooltip,
  .collapsed .target-trigger:hover .sidebar-tooltip,
  .collapsed .target-trigger:focus-visible .sidebar-tooltip,
  .collapsed .who:hover .sidebar-tooltip,
  .collapsed .who:focus-visible .sidebar-tooltip {
    opacity: 1;
    transform: translate(0, -50%);
    visibility: visible;
  }

  .collapsed {
    align-items: stretch;
    padding-left: var(--space-2);
    padding-right: var(--space-2);
  }

  .collapsed .brand-row,
  .collapsed .mark {
    justify-content: center;
  }

  .collapsed .brand-row {
    min-height: 2.5rem;
  }

  .collapsed .sidebar-collapse-trigger {
    background: var(--sidebar-bg);
    border-color: var(--sidebar-border);
    border-radius: var(--radius-control);
    box-shadow: 0 3px 10px rgb(0 0 0 / 18%);
    position: absolute;
    right: -0.875rem;
    top: calc(var(--space-5) + 0.375rem);
  }

  .collapsed .mark-copy,
  .collapsed .target-trigger-copy,
  .collapsed .menu-chevron,
  .collapsed .who-text {
    display: none;
  }

  .collapsed .target-trigger,
  .collapsed .who {
    display: flex;
    justify-content: center;
    padding: var(--space-2);
  }

  .collapsed .account-menu {
    margin-left: calc(var(--space-2) * -1);
    margin-right: calc(var(--space-2) * -1);
  }

  .collapsed .target-popover,
  .collapsed .account-popover {
    left: calc(100% + var(--space-2));
  }

  .collapsed .target-popover {
    top: 0;
  }

  .collapsed .account-popover {
    bottom: 0;
    left: calc(100% + var(--space-2));
    right: auto;
  }

  @media (max-width: 64rem) {
    .panel-sidebar,
    .panel-sidebar.collapsed {
      border-bottom: 1px solid var(--sidebar-border);
      border-right: 0;
      display: block;
      height: auto;
      min-height: 0;
      padding: 0;
    }

    .sidebar-collapse-trigger,
    .target-menu {
      display: none;
    }

    .brand-row,
    .collapsed .brand-row {
      flex-direction: row;
      height: 3.75rem;
      justify-content: space-between;
      padding: 0 var(--space-4);
    }

    .collapsed .mark-copy {
      display: grid;
    }

    .mobile-navigation-trigger {
      display: flex;
      margin: 0;
      position: absolute;
      right: 5rem;
      top: 0.8125rem;
    }

    .navigation-shell {
      background: var(--sidebar-bg);
      border-bottom: 1px solid var(--sidebar-border);
      box-shadow: var(--shadow-popover);
      display: none;
      left: 0;
      max-height: calc(100dvh - 3.75rem);
      overflow: auto;
      padding: var(--space-3);
      position: absolute;
      right: 0;
      top: 100%;
    }

    .mobile-navigation-open .navigation-shell {
      display: block;
    }

    .account-menu,
    .collapsed .account-menu {
      border: 0;
      margin: 0;
      position: absolute;
      right: var(--space-4);
      top: 0.8125rem;
    }

    .who,
    .collapsed .who {
      background: transparent;
      border: 0;
      display: flex;
      min-height: 2.125rem;
      padding: 0;
    }

    .who-text,
    .menu-chevron,
    .sidebar-tooltip {
      display: none;
    }

    .account-popover,
    .collapsed .account-popover {
      bottom: auto;
      left: auto;
      right: 0;
      top: calc(100% + var(--space-2));
    }
  }

  @media (max-width: 30rem) {
    .mark-part,
    .mobile-navigation-trigger > span:last-child {
      display: none;
    }
  }
</style>
