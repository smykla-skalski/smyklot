<script lang="ts">
  import haloUrl from '../assets/smyklot-halo.svg';
  import type { PanelApi } from '../lib/api';
  import { fuzzyCandidates } from '../lib/fuzzy';
  import { handleLabel, readHandle } from '../lib/identity';
  import type { ThemeDisplay } from '../lib/preferences';
  import type { PanelView, RootSection } from '../lib/routes';
  import type { PanelTarget, PanelViewer } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Icon from './Icon.svelte';
  import NotificationInbox from './NotificationInbox.svelte';
  import ViewTabs from './ViewTabs.svelte';

  const {
    viewer,
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
    rootMode,
    rootValue,
    rootHrefFor,
    onSelectRoot,
    rootDashboardHref,
    onEnterRoot,
    returnHref,
    onReturnToPanel,
    fetchNotifications,
    markNotificationRead,
    notificationVersion,
  }: {
    viewer: PanelViewer | null;
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
    rootMode: boolean;
    rootValue: RootSection;
    rootHrefFor: (section: RootSection) => string;
    onSelectRoot: (section: RootSection) => void;
    rootDashboardHref: string;
    onEnterRoot: () => void;
    returnHref: string;
    onReturnToPanel: () => void;
    fetchNotifications: PanelApi['fetchNotifications'];
    markNotificationRead: PanelApi['markNotificationRead'];
    notificationVersion: number;
  } = $props();

  let accountMenu = $state<HTMLDetailsElement | null>(null);
  let accountTrigger = $state<HTMLElement | null>(null);
  let targetMenu = $state<HTMLDetailsElement | null>(null);
  let targetTrigger = $state<HTMLElement | null>(null);
  let targetSearchInput = $state<HTMLInputElement | null>(null);
  let targetQuery = $state('');
  let mobileNavigationOpen = $state(false);
  let unreadCount = $state(0);

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
  const systemRoleLabel = $derived(
    viewer?.system_role === 'super_root'
      ? 'Super Root'
      : viewer?.system_role === 'root'
        ? 'Root'
        : '',
  );

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
    if (event.key !== 'Escape' || event.defaultPrevented) return;
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

  function selectRoot(next: RootSection): void {
    mobileNavigationOpen = false;
    onSelectRoot(next);
  }

  function enterRoot(): void {
    mobileNavigationOpen = false;
    onEnterRoot();
  }

  function returnToPanel(): void {
    mobileNavigationOpen = false;
    onReturnToPanel();
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
    rootMode && 'root-mode',
  ]}
>
  <div class="brand-row">
    <h1 class="mark">
      <img class="mark-icon" src={haloUrl} alt="" width="34" height="34" decoding="async" />
      <span class="mark-copy">
        <span class="mark-name">Smyklot</span>
        <span class="mark-part">{rootMode ? 'ROOT MODE' : 'PANEL'}</span>
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

  {#if !rootMode && selectedTarget !== null}
    <details class="target-menu" bind:this={targetMenu} ontoggle={toggleTargetDetails}>
      <summary
        class="target-trigger"
        bind:this={targetTrigger}
        aria-label={`Switch workspace, currently ${selectedTarget.account.display_name}`}
      >
        <Avatar account={selectedTarget.account} size={28} />
        <span class="target-trigger-copy">
          <span class="target-kicker">Workspace</span>
          <strong>{selectedTarget.account.display_name}</strong>
        </span>
        <span class="menu-chevron" aria-hidden="true">
          <Icon name="chevrons-up-down" size={14} strokeWidth={2} />
        </span>
        <span class="sidebar-tooltip">Switch workspace</span>
      </summary>

      <div class="target-popover">
        <label class="target-search">
          <span class="visually-hidden">Search workspaces</span>
          <span class="target-search-icon" aria-hidden="true"><Icon name="search" size={18} /></span
          >
          <input
            type="search"
            placeholder="Search workspaces"
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
            <p class="target-empty">No workspaces match “{targetQuery.trim()}”</p>
          {/if}
        </div>
      </div>
    </details>
  {/if}

  {#if showNavigation}
    <div id="panel-navigation-drawer" class="navigation-shell">
      <ViewTabs
        value={view}
        hrefFor={viewHref}
        onSelect={selectView}
        {showUsers}
        {collapsed}
        {rootMode}
        rootEnabled={systemRoleLabel !== ''}
        {rootValue}
        {rootHrefFor}
        onSelectRoot={selectRoot}
        {rootDashboardHref}
        onEnterRoot={enterRoot}
        {returnHref}
        onReturnToPanel={returnToPanel}
      />
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
        aria-label={unreadCount === 0
          ? `Account menu for ${viewer.account.display_name}`
          : `Account menu for ${viewer.account.display_name}, ${unreadCount} unread notifications`}
      >
        <span class="who-avatar">
          <Avatar account={viewer.account} size={32} />
          {#if unreadCount > 0}<span class="unread-dot" aria-hidden="true"></span>{/if}
        </span>
        <span class="who-text">
          <span class="who-name">{viewer.account.display_name}</span>
          <span class="who-handle mono">{handle.handle}</span>
        </span>
        <span class="menu-chevron" aria-hidden="true">
          <Icon name="chevron-up" size={14} strokeWidth={2} />
        </span>
        <span class="sidebar-tooltip">Account</span>
      </summary>
      <div class="account-popover">
        <div class="account-header">
          <Avatar account={viewer.account} size={36} />
          <span class="account-header-copy">
            <span class="account-name-row">
              <strong>{viewer.account.display_name}</strong>
              {#if systemRoleLabel !== ''}
                <span class="role-chip">{systemRoleLabel}</span>
              {/if}
            </span>
            <small class="mono">{handleLabel(handle)}</small>
          </span>
        </div>
        <hr class="menu-divider" />
        <NotificationInbox
          fetchPage={fetchNotifications}
          markRead={markNotificationRead}
          refreshVersion={notificationVersion}
          onUnread={(next) => (unreadCount = next)}
        />
        <hr class="menu-divider" />
        <div class="theme-row">
          <span class="theme-icon" aria-hidden="true"><Icon name="sun-moon" size={15} /></span>
          <span class="theme-label">Theme</span>
          <div class="theme-options" role="group" aria-label="Theme">
            <button
              type="button"
              class:selected={theme === 'system'}
              aria-pressed={theme === 'system'}
              aria-label="System theme"
              title="System theme"
              onclick={() => onSelectTheme('system')}
            >
              <Icon name="system" size={14} />
            </button>
            <button
              type="button"
              class:selected={theme === 'light'}
              aria-pressed={theme === 'light'}
              aria-label="Light theme"
              title="Light theme"
              onclick={() => onSelectTheme('light')}
            >
              <Icon name="sun" size={14} />
            </button>
            <button
              type="button"
              class:selected={theme === 'dark'}
              aria-pressed={theme === 'dark'}
              aria-label="Dark theme"
              title="Dark theme"
              onclick={() => onSelectTheme('dark')}
            >
              <Icon name="moon" size={14} />
            </button>
          </div>
        </div>
        <hr class="menu-divider" />
        <button class="account-action" type="button" onclick={signOut}>
          <span class="action-icon"><Icon name="sign-out" size={16} /></span>
          <span class="action-text">Sign out</span>
        </button>
      </div>
    </details>
  {/if}
</aside>

<style>
  /* Spacing is per-element rather than a flex gap: the bottom zone must keep
     one rhythm (divider, 8px, admin entry, 8px, divider, 8px, user card), and
     a container gap would stack onto those margins. */
  .panel-sidebar {
    background: var(--sidebar-bg);
    border-right: 1px solid var(--sidebar-border);
    display: flex;
    flex-direction: column;
    height: 100dvh;
    min-height: 36rem;
    padding: var(--space-4) var(--space-3) var(--space-3);
    position: sticky;
    top: 0;
    transition: padding var(--duration-normal) var(--ease-standard);
    z-index: var(--layer-sticky);
  }

  .brand-row {
    align-items: center;
    display: flex;
    justify-content: space-between;
    margin-bottom: var(--space-2);
    min-height: 2.375rem;
    padding: 0 var(--space-2);
    position: relative;
  }

  .mark {
    align-items: center;
    display: flex;
    gap: 0.625rem;
    margin: 0;
    min-width: 0;
  }

  .mark-icon {
    flex: none;
    object-fit: contain;
  }

  .mark-copy {
    display: grid;
    gap: 0.3rem;
    min-width: 0;
  }

  .mark-name {
    color: var(--sidebar-text);
    font: 700 0.8125rem / 1 var(--sans);
    letter-spacing: 0.11em;
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .mark-part {
    color: var(--sidebar-text-muted);
    font: 700 0.65625rem / 1 var(--sans);
    letter-spacing: 0.12em;
    text-box: trim-both cap alphabetic;
  }

  .panel-sidebar.root-mode .mark-part {
    color: var(--sidebar-root-accent);
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
    cursor: pointer;
    opacity: 0;
    position: relative;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      opacity var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
    z-index: 2;
  }

  .panel-sidebar:hover .sidebar-collapse-trigger,
  .sidebar-collapse-trigger:focus-visible {
    opacity: 1;
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

  .target-menu {
    margin: var(--space-2) 0 var(--space-3);
  }

  /* ---- workspace switcher: context selection lives at the top ---- */
  .target-trigger {
    align-items: center;
    background: var(--switcher-card-bg);
    border: 1px solid var(--switcher-card-border);
    border-radius: var(--radius-control);
    box-shadow: var(--sidebar-thumb-shadow);
    cursor: pointer;
    display: grid;
    gap: 0.625rem;
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 3.25rem;
    padding: var(--space-2) 0.625rem;
    position: relative;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
    user-select: none;
  }

  .target-trigger:hover {
    background: var(--switcher-card-hover);
    border-color: color-mix(in srgb, var(--focus) 40%, var(--switcher-card-border));
  }

  .target-menu[open] .target-trigger {
    border-color: color-mix(in srgb, var(--focus) 55%, var(--switcher-card-border));
  }

  .target-trigger:active {
    background: var(--sidebar-item-pressed);
    box-shadow: none;
    transform: translateY(1px);
  }

  .target-trigger::-webkit-details-marker,
  .who::-webkit-details-marker {
    display: none;
  }

  .target-trigger::marker,
  .who::marker {
    content: '';
  }

  .target-trigger-copy {
    display: grid;
    gap: 0.3rem;
    min-width: 0;
    text-align: left;
  }

  .target-kicker {
    color: var(--sidebar-text-muted);
    font: 700 0.625rem / 1 var(--sans);
    letter-spacing: 0.11em;
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .target-trigger-copy strong {
    color: var(--sidebar-text);
    font-size: var(--font-size-meta);
    font-weight: 600;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .menu-chevron {
    color: var(--sidebar-text-secondary);
    display: grid;
    place-items: center;
  }

  .target-popover,
  .account-popover {
    background: var(--sidebar-popover-bg);
    border: 1px solid var(--sidebar-popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    color: var(--sidebar-menu-text);
    left: 0;
    overflow: hidden;
    padding: 6px;
    position: absolute;
    z-index: var(--layer-popover);
  }

  .target-popover {
    display: grid;
    grid-template-rows: auto minmax(8rem, 1fr);
    max-height: min(30rem, calc(100dvh - 8rem));
    right: 0;
    top: calc(100% + 6px);
  }

  .account-popover {
    bottom: calc(100% + 6px);
    width: min(16.5rem, calc(100vw - 2rem));
  }

  .target-options {
    display: grid;
    gap: 2px;
    min-height: 0;
    overflow: auto;
  }

  .target-search {
    background: var(--sidebar-popover-bg);
    border-bottom: 1px solid var(--sidebar-popover-border);
    display: block;
    margin: 0 -6px;
    padding: 2px 6px 8px;
    position: relative;
    z-index: 2;
  }

  .target-search input {
    background: var(--sidebar-seg-track);
    border: 1px solid var(--sidebar-seg-border);
    border-radius: var(--radius-control);
    color: var(--sidebar-menu-text);
    font: 500 var(--font-size-meta) / 1 var(--sans);
    height: 2.125rem;
    padding: 0 var(--space-3) 0 2.25rem;
    width: 100%;
  }

  .target-search input:focus-visible {
    border-color: var(--focus);
    box-shadow: inset 0 0 0 1px var(--focus);
    outline: 0;
  }

  .target-search input::placeholder {
    color: var(--sidebar-menu-muted);
  }

  .target-search-icon {
    color: var(--sidebar-menu-muted);
    display: grid;
    left: 0.875rem;
    place-items: center;
    position: absolute;
    top: 0.625rem;
  }

  .target-group-label {
    background: var(--sidebar-popover-bg);
    color: var(--sidebar-menu-muted);
    font-size: 0.65625rem;
    font-weight: 700;
    letter-spacing: 0.09em;
    margin: 0;
    padding: var(--space-2) var(--space-2) var(--space-1);
    position: sticky;
    top: 0;
    text-transform: uppercase;
    z-index: 1;
  }

  .target-empty {
    color: var(--sidebar-menu-muted);
    font-size: var(--font-size-meta);
    margin: 0;
    padding: var(--space-5) var(--space-3);
    text-align: center;
  }

  .target-option {
    align-items: center;
    background: transparent;
    border-radius: calc(var(--radius-popover) - 6px);
    color: var(--sidebar-menu-text);
    display: grid;
    gap: var(--space-2);
    grid-template-columns: auto minmax(0, 1fr) 1rem;
    min-height: 2.5rem;
    padding: var(--space-1) var(--space-2);
    text-align: left;
    text-decoration: none;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
  }

  .target-option:hover,
  .target-option:focus-visible {
    background: var(--sidebar-menu-hover);
  }

  .target-option:active {
    background: var(--sidebar-menu-pressed);
    transform: translateY(1px);
  }

  .option-copy {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .option-copy strong,
  .option-copy span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .option-copy strong {
    font-size: var(--font-size-compact);
    font-weight: 600;
    line-height: 1.2;
  }

  .option-copy span {
    color: var(--sidebar-menu-muted);
    font-size: 0.625rem;
    line-height: 1.2;
  }

  .option-check {
    color: var(--brand-action);
    display: grid;
    place-items: center;
  }

  .panel-sidebar.root-mode .option-check {
    color: var(--sidebar-root-accent);
  }

  .navigation-shell {
    flex: 1;
    min-height: 0;
  }

  /* ---- account: the card at the bottom is only about the signed-in user.
     The bottom zone keeps one rhythm: divider, 8px, admin entry, 8px,
     divider, 8px, this card. ---- */
  .account-menu {
    border-top: 1px solid var(--sidebar-border);
    margin-top: var(--space-2);
    padding-top: var(--space-2);
  }

  .who {
    align-items: center;
    background: transparent;
    border-radius: var(--radius-control);
    cursor: pointer;
    display: grid;
    gap: 0.625rem;
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 3rem;
    padding: var(--space-2) 0.625rem;
    position: relative;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
    user-select: none;
  }

  .who:hover,
  .account-menu[open] .who {
    background: var(--sidebar-item-hover);
  }

  .who:active {
    background: var(--sidebar-item-pressed);
    transform: translateY(1px);
  }

  .who-avatar {
    display: inline-flex;
    flex: none;
    position: relative;
  }

  .unread-dot {
    background: var(--unread-badge-bg);
    border: 2px solid var(--sidebar-bg);
    border-radius: 50%;
    height: 0.625rem;
    position: absolute;
    right: -1px;
    top: -1px;
    width: 0.625rem;
  }

  .who-text {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
    text-align: left;
  }

  .who-name {
    color: var(--sidebar-text);
    font-size: var(--font-size-meta);
    font-weight: 600;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* No text-box trim and no tight line-height here: with overflow hidden they
     clip the descenders of the handle ("@", "y", "g") at the bottom edge. */
  .who-handle {
    color: var(--sidebar-text-muted);
    font-size: var(--font-size-micro);
    font-weight: 500;
    line-height: 1.35;
    margin-top: -0.2rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .account-header {
    align-items: center;
    display: grid;
    gap: 0.625rem;
    grid-template-columns: auto minmax(0, 1fr);
    padding: 8px 10px 12px;
  }

  .account-header-copy {
    display: grid;
    gap: 0.35rem;
    min-width: 0;
  }

  .account-name-row {
    align-items: center;
    display: flex;
    gap: 0.4375rem;
    min-width: 0;
  }

  .account-name-row strong {
    font-size: var(--font-size-meta);
    font-weight: 600;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .role-chip {
    align-items: center;
    background: var(--role-chip-bg);
    border-radius: var(--radius-chip);
    color: var(--role-chip-text);
    display: inline-flex;
    flex: none;
    font: 700 0.5938rem / 1 var(--sans);
    height: 1.125rem;
    letter-spacing: 0.08em;
    padding-inline: 0.4375rem;
    text-transform: uppercase;
  }

  .account-header-copy small {
    color: var(--sidebar-menu-muted);
    font-size: var(--font-size-micro);
    font-weight: 500;
    line-height: 1.35;
    margin-top: -0.2rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .menu-divider {
    background: var(--sidebar-popover-border);
    border: 0;
    height: 1px;
    margin: 4px -6px;
  }

  .theme-row {
    align-items: center;
    display: flex;
    gap: 0.625rem;
    min-height: 2.5rem;
    padding: 0 10px;
  }

  .theme-icon {
    color: var(--sidebar-menu-muted);
    display: grid;
    flex: none;
    place-items: center;
    width: 1.125rem;
  }

  .theme-label {
    color: var(--sidebar-menu-text);
    flex: 1;
    font: 500 var(--font-size-meta) / 1 var(--sans);
    text-box: trim-both cap alphabetic;
  }

  .theme-options {
    background: var(--sidebar-seg-track);
    border: 1px solid var(--sidebar-seg-border);
    border-radius: 7px;
    display: inline-flex;
    gap: 2px;
    padding: 2px;
  }

  .theme-options button {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: 5px;
    color: var(--sidebar-menu-muted);
    cursor: pointer;
    display: inline-flex;
    height: 1.625rem;
    justify-content: center;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
    width: 1.875rem;
  }

  .theme-options button:hover:not(.selected) {
    background: color-mix(in srgb, var(--sidebar-menu-text) 8%, transparent);
    color: var(--sidebar-menu-text);
  }

  .theme-options button:active {
    transform: scale(0.92);
  }

  .theme-options button.selected {
    background: var(--sidebar-seg-thumb);
    box-shadow: 0 1px 3px rgb(0 0 0 / 18%);
    color: var(--sidebar-menu-text);
  }

  .account-action {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: calc(var(--radius-popover) - 6px);
    color: var(--sidebar-menu-text);
    cursor: pointer;
    display: flex;
    font: 500 var(--font-size-meta) / 1 var(--sans);
    gap: 0.625rem;
    min-height: 2.5rem;
    padding: 0 10px;
    text-align: left;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
    width: 100%;
  }

  .account-action:hover,
  .account-action:focus-visible {
    background: var(--stop-tint);
    color: var(--stop);
  }

  .account-action:active {
    background: var(--sidebar-menu-pressed);
    transform: translateY(1px);
  }

  .action-icon {
    color: var(--sidebar-menu-muted);
    display: grid;
    flex: none;
    place-items: center;
    width: 1.125rem;
  }

  .account-action:hover .action-icon,
  .account-action:focus-visible .action-icon {
    color: var(--stop);
  }

  .action-text {
    text-box: trim-both cap alphabetic;
  }

  /* ---- collapsed rail ---- */
  .sidebar-tooltip {
    background: var(--sidebar-popover-bg);
    border: 1px solid var(--sidebar-popover-border);
    border-radius: var(--radius-control);
    box-shadow: var(--shadow-popover);
    color: var(--sidebar-menu-text);
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

  /* A tooltip never fights the popover it would describe. */
  .target-menu[open] .sidebar-tooltip,
  .account-menu[open] .sidebar-tooltip {
    visibility: hidden !important;
  }

  .collapsed {
    align-items: stretch;
    padding-left: var(--space-2);
    padding-right: var(--space-2);
  }

  /* Collapsed, the toggle joins the rail flow under the mark: always visible,
     nothing floating over the sidebar edge. */
  .collapsed .brand-row {
    flex-direction: column;
    gap: var(--space-2);
    justify-content: center;
    min-height: 0;
    padding: 0;
  }

  .collapsed .sidebar-collapse-trigger {
    opacity: 1;
  }

  .collapsed .mark {
    justify-content: center;
  }

  .collapsed .mark-copy,
  .collapsed .target-trigger-copy,
  .collapsed .menu-chevron,
  .collapsed .who-text {
    display: none;
  }

  .collapsed .target-trigger {
    display: flex;
    justify-content: center;
    padding: var(--space-2) 0;
  }

  .collapsed .who {
    display: flex;
    justify-content: center;
    padding: var(--space-2) 0;
  }

  .collapsed .target-popover {
    left: calc(100% + 10px);
    right: auto;
    top: 0;
    width: 17rem;
  }

  .collapsed .account-popover {
    bottom: 0;
    left: calc(100% + 10px);
  }

  /* ---- mobile: the sidebar becomes a top bar ---- */
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

    .sidebar-collapse-trigger {
      display: none;
    }

    .brand-row,
    .collapsed .brand-row {
      flex-direction: row;
      height: 3.75rem;
      justify-content: space-between;
      min-height: 0;
      padding: 0 var(--space-4);
    }

    .collapsed .mark-copy {
      display: grid;
    }

    .mobile-navigation-trigger {
      display: flex;
      margin: 0;
      position: absolute;
      right: 7.25rem;
      top: 1rem;
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

    .target-menu,
    .account-menu,
    .collapsed .target-menu,
    .collapsed .account-menu {
      border: 0;
      margin: 0;
      padding: 0;
      position: absolute;
      top: 0.9375rem;
    }

    .target-menu {
      right: 4.25rem;
    }

    .account-menu {
      right: var(--space-4);
    }

    .target-trigger,
    .who,
    .collapsed .target-trigger,
    .collapsed .who {
      background: transparent;
      border: 0;
      box-shadow: none;
      display: flex;
      min-height: 2.125rem;
      padding: 0;
    }

    .who-text,
    .target-trigger-copy,
    .menu-chevron,
    .sidebar-tooltip {
      display: none;
    }

    .account-popover,
    .target-popover,
    .collapsed .account-popover,
    .collapsed .target-popover {
      bottom: auto;
      left: auto;
      right: 0;
      top: calc(100% + var(--space-2));
    }

    .target-popover {
      width: min(19rem, calc(100vw - 2rem));
    }
  }

  @media (max-width: 30rem) {
    .mark-part,
    .mobile-navigation-trigger > span:last-child {
      display: none;
    }
  }
</style>
