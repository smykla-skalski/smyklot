<script lang="ts">
  import { fuzzyCandidates } from '../fuzzy';
  import { handleLabel, readHandle } from '../identity';
  import type { ThemeDisplay } from '../preferences';
  import type { PanelView, RootSection } from '../routes';
  import type { PanelTarget, PanelViewer } from '../types';
  import Avatar from './Avatar.svelte';
  import Icon from './Icon.svelte';
  import AccountTrigger from './AccountTrigger.svelte';
  import BrandRow from './BrandRow.svelte';
  import WorkspaceTrigger from './WorkspaceTrigger.svelte';
  import Popover from './Popover.svelte';
  import ThemeSwitch from './ThemeSwitch.svelte';
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
    showViews,
    showNavigation,
    collapsed,
    onToggleCollapsed,
    theme,
    onSelectTheme,
    rootMode,
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
    /** Whether there is a workspace for the view links to lead to. */
    showViews: boolean;
    showNavigation: boolean;
    collapsed: boolean;
    onToggleCollapsed: () => void;
    theme: ThemeDisplay;
    onSelectTheme: (theme: ThemeDisplay) => void;
    rootMode: boolean;
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
    /** Unread notifications, for the badge on the Inbox row and the account card. */
    unreadCount: number;
  } = $props();

  let accountOpen = $state(false);
  let targetOpen = $state(false);
  let targetQuery = $state('');
  let mobileNavigationOpen = $state(false);
  /**
   * The stylesheet's own breakpoint, asked of the browser rather than reproduced.
   *
   * Placement is measured now, so where these menus open is decided in script
   * while the rail around them is still laid out by a media query - and the two
   * have to change over at the same width. Comparing `innerWidth` to 768 only
   * agrees with `48rem` while the root font size is exactly 16px; anyone who has
   * changed it in their browser would get the mobile layout with the desktop
   * placement, or the reverse. `matchMedia` resolves the same units the
   * stylesheet does, so there is one breakpoint rather than two that usually
   * agree.
   */
  let narrow = $state(false);

  /* The collapsed rail: a column of icons with its menus flying out beside it.
     Only on a wide window - narrow, the rail is a top bar and they drop from it
     like any other menu. The wider gap goes with the fly-out, because there it
     crosses the rail's edge rather than hanging off a control. */
  const flyout = $derived(!narrow && collapsed);
  const railOffset = $derived(flyout ? 10 : 6);

  $effect(() => {
    const breakpoint = window.matchMedia('(max-width: 48rem)');
    const sync = (): void => {
      narrow = breakpoint.matches;
    };

    sync();
    breakpoint.addEventListener('change', sync);
    return () => breakpoint.removeEventListener('change', sync);
  });

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

  /* The two menus dismiss themselves: they are layers in the top layer, and the
     platform closes one when the other opens, on a press outside, and on Escape,
     returning focus to whichever trigger was named. What is left here is the
     mobile navigation drawer, which is none of those things. */
  function closeMenus(): void {
    accountOpen = false;
    targetOpen = false;
  }

  function closeFromOutside(event: PointerEvent): void {
    if (!(event.target instanceof Node) || !mobileNavigationOpen) return;
    const sidebar = document.querySelector('.panel-sidebar');
    if (sidebar !== null && !sidebar.contains(event.target)) mobileNavigationOpen = false;
  }

  function closeFromKeyboard(event: KeyboardEvent): void {
    if (event.key !== 'Escape' || event.defaultPrevented) return;
    /* An open layer dismisses itself on Escape and returns focus where it came
       from. Taking the key here would cancel both, so the drawer waits its turn
       - which is what the innermost thing closing first means, and what the
       menus themselves used to do before the platform took the job on. */
    if (accountOpen || targetOpen || !mobileNavigationOpen) return;
    event.preventDefault();
    mobileNavigationOpen = false;
    document.querySelector<HTMLElement>('.mobile-navigation-trigger')?.focus();
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

  function selectInbox(): void {
    mobileNavigationOpen = false;
    onSelectInbox();
  }

  function enterRoot(): void {
    mobileNavigationOpen = false;
    onEnterRoot();
  }

  function returnToPanel(): void {
    mobileNavigationOpen = false;
    onReturnToPanel();
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
  <BrandRow
    part={rootMode ? 'ROOT MODE' : 'PANEL'}
    {showNavigation}
    {collapsed}
    {onToggleCollapsed}
    bind:navigationOpen={mobileNavigationOpen}
  />

  {#if !rootMode && selectedTarget !== null}
    <Popover
      bind:open={targetOpen}
      skin="sidebar"
      role="dialog"
      label="Switch workspace"
      side={flyout ? 'right' : 'below'}
      align={narrow ? 'end' : 'start'}
      width={narrow || collapsed ? 'auto' : 'trigger'}
      offset={railOffset}
      focusSelector=".target-search input"
      onclose={() => (targetQuery = '')}
    >
      {#snippet trigger(attributes)}
        <WorkspaceTrigger account={selectedTarget.account} {attributes} />
      {/snippet}

      <!-- `rail`, not `collapsed`: scoped styles are scoped to the component and
           not to the element they were written for, so the rail's own
           `.collapsed` padding reached in here and pushed the search strip off
           the popover's edges. -->
      <div class="target-body" class:rail={collapsed}>
        <label class="target-search">
          <span class="visually-hidden">Search workspaces</span>
          <span class="target-search-icon" aria-hidden="true"><Icon name="search" size={18} /></span
          >
          <input type="search" placeholder="Search workspaces" bind:value={targetQuery} />
        </label>
        <div class="target-options">
          {#snippet targetOption(target: PanelTarget)}
            <a
              href={targetHref(target)}
              class={['target-option', target.id === selectedId && 'current']}
              aria-current={target.id === selectedId ? 'page' : undefined}
              onclick={(event) => selectTarget(event, target.id)}
            >
              <Avatar account={target.account} size={28} shape="workspace" />
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
    </Popover>
  {/if}

  {#if showNavigation}
    <div id="panel-navigation-drawer" class="navigation-shell">
      <ViewTabs
        value={view}
        hrefFor={viewHref}
        onSelect={selectView}
        {showUsers}
        {showViews}
        {collapsed}
        {rootMode}
        rootEnabled={systemRoleLabel !== ''}
        {rootValue}
        {rootHrefFor}
        onSelectRoot={selectRoot}
        {rootEntryHref}
        onEnterRoot={enterRoot}
        {returnHref}
        onReturnToPanel={returnToPanel}
        {inboxHref}
        {inboxActive}
        onSelectInbox={selectInbox}
        {unreadCount}
      />
    </div>
  {/if}

  {#if viewer !== null && handle !== null}
    <Popover
      bind:open={accountOpen}
      skin="sidebar"
      role="dialog"
      label="Account"
      side={flyout ? 'right' : narrow ? 'below' : 'above'}
      align={narrow || collapsed ? 'end' : 'start'}
      offset={railOffset}
      focusOnOpen={false}
    >
      {#snippet trigger(attributes)}
        <AccountTrigger account={viewer.account} handle={handle.handle} {attributes} />
      {/snippet}

      <div class="account-body">
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
        <div class="theme-row">
          <span class="theme-icon" aria-hidden="true"><Icon name="sun-moon" size={15} /></span>
          <span class="theme-label">Theme</span>
          <ThemeSwitch name="panel-theme" {theme} surface="sidebar" onSelect={onSelectTheme} />
        </div>
        <hr class="menu-divider" />
        <button class="account-action" type="button" onclick={signOut}>
          <span class="action-icon"><Icon name="sign-out" size={16} /></span>
          <span class="action-text">Sign out</span>
        </button>
      </div>
    </Popover>
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

  /* The mark itself is `BrandMark`, shared with the invitation page so the two
     cannot drift. What stays here is only what the rail does to it. */
  .panel-sidebar.root-mode :global(.mark-part) {
    color: var(--sidebar-root-accent);
  }

  .panel-sidebar:hover :global(.sidebar-collapse-trigger),
  :global(.sidebar-collapse-trigger:focus-visible) {
    opacity: 1;
  }

  /* No stacking context of their own any more: both menus are in the top layer,
     which nothing in the page can be painted over. */

  /* Trimmed to the baseline by `.band-trim-stack`, so the descenders in a workspace
     name paint below the box and `overflow: hidden` took them off. The account card
     below solves the same thing by opening the block axis; here the room is bounded
     instead, because the switcher sits in a rail whose neighbours are close. */

  /* `:global`, because two triggers wear this and one of them - the account row - is
     `AccountTrigger`'s markup now. Four declarations shared by two triggers is a class
     doing its job rather than a component waiting to be written; what it costs is
     saying so here. */
  :global(.menu-chevron) {
    color: var(--sidebar-text-secondary);
    display: grid;
    place-items: center;
  }

  /* Inside the layer, which carries the sidebar's own surface and puts itself
     under, over or beside the rail depending on whether it is collapsed. */
  .target-body,
  .account-body {
    color: var(--sidebar-menu-text);
    padding: 6px;
  }

  /*
   * A column that can shrink, so the search box keeps its size and the list
   * below it takes whatever room the layer measured for itself. It clips
   * because `.target-options` inside it is what scrolls.
   *
   * The account menu deliberately does not: it has no inner scroller, so
   * clipping there hid whatever did not fit instead of letting the layer scroll
   * to it. On a short enough window that was Sign out - drawn, measured, and
   * beyond the layer's bottom edge with no way to reach it.
   */
  .target-body {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
  }

  .target-body.rail {
    width: 17rem;
  }

  .account-body {
    width: min(16.5rem, calc(100vw - 2rem));
  }

  .target-options {
    align-content: start;
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
  /* The rule and the gap above the card, not on it: a border on `.who` itself
     would follow its corner radius and sit inside its hover fill. */

  /* Truncate sideways, and only sideways.
     -------------------------------------
     `.band-trim-stack` trims this pair to the cap above and the baseline below,
     which is what centres the ink in the card rather than the line boxes around
     it. The ascenders and descenders still paint - trimming moves the box, not
     the glyphs - so `overflow: hidden` cut the tail off the handle's "@" and
     "y" along the bottom edge, and would take the accent off a capital in a
     name. Chrome is the only engine that implements the trim, so it was the
     only one showing it.

     Clipping one axis leaves the other alone, which `hidden` cannot do: a box
     that is hidden on one axis and visible on the other resolves both to
     something clipping. The ellipsis needs the horizontal clip and nothing
     needs the vertical one. */

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
    background: var(--sidebar-stop-tint);
    color: var(--sidebar-stop);
  }

  .account-action:active {
    background: var(--sidebar-menu-pressed);
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
    color: var(--sidebar-stop);
  }

  .action-text {
    text-box: trim-both cap alphabetic;
  }

  /* ---- collapsed rail ---- */
  /* Which triggers show it stays here, because it is this component that knows the
     rail is collapsed and which row the pointer is on. `:global` on the tooltip half
     only: the span is `SidebarTooltip`'s element, the triggers are this file's. */
  .collapsed :global(.sidebar-collapse-trigger:hover .sidebar-tooltip),
  .collapsed :global(.sidebar-collapse-trigger:focus-visible .sidebar-tooltip),
  .collapsed :global(.target-trigger:hover .sidebar-tooltip),
  .collapsed :global(.target-trigger:focus-visible .sidebar-tooltip),
  .collapsed :global(.who:hover .sidebar-tooltip),
  .collapsed :global(.who:focus-visible .sidebar-tooltip) {
    opacity: 1;
    transform: translate(0, -50%);
    visibility: visible;
  }

  /* A tooltip never fights the popover it would describe. */
  :global(.target-trigger[aria-expanded='true'] .sidebar-tooltip),
  :global(.who[aria-expanded='true'] .sidebar-tooltip) {
    visibility: hidden !important;
  }

  .collapsed {
    align-items: stretch;
    padding-left: var(--space-2);
    padding-right: var(--space-2);
  }

  /* Collapsed, the toggle joins the rail flow under the mark: always visible,
     nothing floating over the sidebar edge. */
  /* Same height as the expanded row, so the mark's centre does not move. Collapsing dropped
     min-height, the row shrank to the 34px mark inside it, and the mark's centre stepped from 35
     to 33 - a two-pixel hop in the middle of a width animation. */
  .collapsed :global(.brand-row) {
    flex-direction: column;
    gap: var(--space-2);
    justify-content: center;
    min-height: 2.375rem;
    padding: 0;
  }

  /* Collapsed, the trigger sits ON the mark rather than under it. It waits for a
     hover like the expanded one does, so the mark is what the sidebar shows at
     rest. */
  /* The target is the whole row - the same reach the workspace tile below it has - while the disc
     stays the size of the halo it sits on. A 32px circle is a small thing to hit for the control
     that opens the sidebar. */
  .collapsed :global(.sidebar-collapse-trigger) {
    border: 0;
    border-radius: var(--radius-control);
    box-shadow: none;
    height: auto;
    inset: 0;
    position: absolute;
    translate: none;
    width: auto;
  }

  /* The glyph goes inside the halo, not over it.
     -------------------------------------------
     `smyklot-halo.svg` draws the ring in a 1340 box as a circle of r=556 stroked
     at 84, so at the mark's 36px the ring is 32.13px across its outer edge with a
     2.26px stroke, and the interior inside it is 27.62px. This disc used to be
     the outer figure with a grey ring of its own, which meant hovering swapped
     the rainbow halo for a plain circle - the mark disappeared exactly when
     someone reached for it. Sized to the interior instead, the halo stays and the
     glyph sits where the robot was.

     A third of a pixel over the interior on each side, because the ring's inner
     edge is antialiased and lands on a fraction: an exactly-sized disc leaves a
     hairline of interior showing between the two. It is far less than the ring's
     own 2.26px, so the halo is not visibly eaten into. */
  .collapsed :global(.sidebar-collapse-trigger::before) {
    border-radius: 50%;
    box-sizing: border-box;
    content: '';
    height: 28.3px;
    left: 50%;
    position: absolute;
    top: 50%;
    translate: -50% -50%;
    width: 28.3px;
  }

  .collapsed :global(.sidebar-collapse-trigger > svg) {
    position: relative;
    z-index: 1;
  }

  /* The states belong to the disc, not to the row-sized target it is drawn on: a background on the
     button itself would cover the mark it is meant to sit over. */
  /* The row-sized target keeps no surface of its own. It is drawn over the mark,
     so a background on the button is a background over the halo - which is how
     hovering used to wipe the ring off the rail even before the disc was drawn.
     Every state belongs to the disc instead. */
  .collapsed :global(.sidebar-collapse-trigger),
  .collapsed :global(.sidebar-collapse-trigger:hover),
  .collapsed :global(.sidebar-collapse-trigger:focus-visible),
  .collapsed :global(.sidebar-collapse-trigger:active) {
    background: transparent;
  }

  /* Opaque: the robot behind must not read through the glyph. */
  .collapsed :global(.sidebar-collapse-trigger::before) {
    background: var(--sidebar-bg);
  }

  .collapsed :global(.sidebar-collapse-trigger:hover)::before,
  .collapsed :global(.sidebar-collapse-trigger:focus-visible)::before {
    background: var(--sidebar-item-hover);
  }

  .collapsed :global(.sidebar-collapse-trigger:hover),
  .collapsed :global(.sidebar-collapse-trigger:focus-visible) {
    color: var(--sidebar-text);
  }

  .collapsed :global(.sidebar-collapse-trigger:active)::before {
    background: var(--sidebar-item-pressed);
  }

  /* The mark shrinks with the disc that covers it. They are concentric, so scaling only the disc
     let the halo underneath show past its own edge - a lit crescent at the bottom left, where the
     halo's stroke is thickest. Pressed, the logo and the ring over it are one object. */
  .collapsed :global(.brand-row:has(.sidebar-collapse-trigger:active) .mark-icon),
  .collapsed :global(.sidebar-collapse-trigger:active) {
    transform: scale(var(--press-scale-disc));
  }

  .collapsed :global(.mark-icon) {
    transition: transform var(--duration-press) var(--ease-standard);
  }

  .collapsed :global(.mark) {
    justify-content: center;
  }

  .collapsed :global(.mark-copy),
  .collapsed :global(.target-trigger-copy),
  .collapsed :global(.menu-chevron),
  .collapsed :global(.who-text) {
    display: none;
  }

  .collapsed :global(.target-trigger) {
    display: flex;
    justify-content: center;
    padding: var(--space-2) 0;
  }

  .collapsed :global(.who) {
    display: flex;
    justify-content: center;
    padding: var(--space-2) 0;
  }

  /* ---- mobile: the sidebar becomes a top bar ---- */
  @media (max-width: 48rem) {
    .panel-sidebar,
    .panel-sidebar.collapsed {
      /* The rail becomes a bar, and two of the things standing on it - the
         workspace switcher and the account card - are not inside it: they are
         siblings, placed into it from here. So the bar's height and the height
         of a control on it are named once, up on the ancestor both can read,
         and every offset below is derived from the pair rather than written
         out. The switcher and the account sat 2px under the bar's centre line
         for exactly as long as that offset was a number somebody typed. */
      --bar-height: 3.75rem;
      --bar-control: 2.125rem;

      /* The right-hand controls, measured from the screen's edge inwards. Each
         one starts where the one outside it ended, plus the gap, so the row
         packs itself and there is no offset written twice. The two widths are
         the avatar sizes their markup asks for. */
      --bar-gap: var(--space-5);
      --bar-slot-account: var(--space-4);
      --bar-slot-switcher: calc(var(--bar-slot-account) + 2rem + var(--bar-gap));
      --bar-slot-menu: calc(var(--bar-slot-switcher) + 1.75rem + var(--bar-gap));

      border-bottom: 1px solid var(--sidebar-border);
      border-right: 0;
      display: block;
      height: auto;
      min-height: 0;
      padding: 0;
    }

    /* No bottom margin. It separates the brand row from the navigation under it
       in the rail, and there is no navigation under it here - the drawer is a
       layer. Left in, it was 8px of nothing between the row and the bar's own
       rule, so the bar measured 69px while its contents centred on the row's 60:
       everything in it sat 4px above the line the reader sees it against. */
    /* The row's own phone shape is in `BrandRow`. Written here it would be one
       class against that component's class-plus-scope, and would lose. */

    .collapsed :global(.mark-copy) {
      display: grid;
    }

    /* The Root console has no workspace to switch, so the switcher is not
       rendered and its slot would otherwise stay empty - the menu button hung
       68px off the account avatar with nothing between them. It moves out to
       take the vacant slot, keeping the row packed against the edge. */
    .panel-sidebar:not(:has(:global(.target-trigger))) :global(.mobile-navigation-trigger) {
      right: var(--bar-slot-switcher);
    }

    .navigation-shell {
      background: var(--sidebar-bg);
      border-bottom: 1px solid var(--sidebar-border);
      box-shadow: var(--shadow-popover);
      display: none;
      left: 0;
      max-height: calc(100dvh - var(--bar-height));
      overflow: auto;
      padding: var(--space-3);
      position: absolute;
      right: 0;
      top: 100%;
    }

    .mobile-navigation-open .navigation-shell {
      display: block;
    }

    /* The two triggers' own phone layouts are in `WorkspaceTrigger` and
       `AccountTrigger`. Written here they would be a single class against each
       component's class-plus-scope, and would lose. */

    /* `.who-text` and the account row's chevron are NOT here. They are
       `AccountTrigger`'s elements, and its own scoped rule - a class plus its scope
       class - outranks a bare `:global(.who-text)` written from out here, so this
       hid nothing and the account button stayed 146px wide on a phone. The row
       carries its own phone layout instead. */
    :global(.menu-chevron) {
      display: none;
    }

    /* Separately, and `:global`, because the span is `SidebarTooltip`'s element.
       Folded into the list above it silently stopped matching, and a tooltip that
       is only hidden by `visibility` still occupies its box: 396px of "Account"
       hanging off a 320px screen, which Chrome answers by zooming the whole page
       out to 79%. `tests/browser/mobile-layout.test.ts` caught it. */
    :global(.sidebar-tooltip) {
      display: none;
    }

    /* Where they open is measured now, from `narrow` above - both drop below
       their trigger and line up with its right edge. Only the width is left. */
    .target-body,
    .target-body.rail {
      width: min(19rem, calc(100vw - 2rem));
    }
  }

  @media (max-width: 30rem) {
    .panel-sidebar :global(.mark-part) {
      display: none;
    }
  }

  /* A finger needs more room than an eye does. These three are 28-32px squares
     because that is the weight the bar wants them to carry, and the menu is the
     one control on a phone that every other page is reached through - at 28px
     it was the smallest thing in the bar and the most often pressed.

     So the target grows and the control does not: a coarse pointer gets a 44px
     square laid over each, invisible, taking the presses. Nothing moves and
     nothing is redrawn. There is room for it - the bar is 60px tall and the
     three sit 20px apart, so the expanded squares still clear each other by 4px
     and the last one stops 10px short of the screen edge.

     The percentages resolve against the control's own padding box, so the same
     expression centres 44px on whichever size each one is. Where a control is
     already larger - `.who` is a full row in the sidebar, not a disc - the
     result goes positive and the overlay sits inside it, changing nothing.

     No `position` of its own, deliberately. All three are positioned already:
     the switcher and the account menu relatively, the menu button absolutely
     once the rail becomes a drawer. Adding a `relative` here broke the header -
     same specificity as the drawer rule's `absolute` and later in the file, so
     it won, and the menu button left the corner it is placed in. */
  @media (pointer: coarse) {
    :global(.mobile-navigation-trigger::after),
    :global(.target-trigger::after),
    :global(.who::after) {
      content: '';
      inset: calc((2.75rem - 100%) / -2) calc((2.75rem - 100%) / -2);
      position: absolute;
    }
  }
</style>
