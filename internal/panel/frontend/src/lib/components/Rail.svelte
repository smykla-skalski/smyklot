<script lang="ts">
  /* The BRAND cut of the halo, which is the one the rail wears: a solid teal ring and
     the interior painted. The other cut leaves the interior transparent and draws the
     rainbow ring, and it is built for one page - the invitation's night sky reads
     through the emblem, so the mark there is a window rather than a badge. At 34px on a
     sidebar ground the window showed the sidebar, and the rainbow ring read as noise. */
  import haloUrl from '../../assets/smyklot-halo-brand.svg';

  import type { PanelTarget, PanelViewer } from '../types';
  import type { ThemeDisplay } from '../preferences';
  import { workspaceHue, workspaceInitials } from '../workspace-mark.js';
  import AccountMenu from './AccountMenu.svelte';
  import Avatar from './Avatar.svelte';
  import Icon from './Icon.svelte';
  import WorkspaceMenu from './WorkspaceMenu.svelte';

  const {
    viewer,
    targets,
    selectedId,
    targetHref,
    onSelectTarget,
    rootMode,
    rootEnabled,
    rootEntryHref,
    onEnterRoot,
    inboxHref,
    inboxActive,
    onSelectInbox,
    unreadCount,
    dirtyTargetIds,
    rootDirty = false,
    theme,
    onSelectTheme,
    onSignOut,
  }: {
    viewer: PanelViewer | null;
    targets: readonly PanelTarget[];
    selectedId: string | null;
    targetHref: (target: PanelTarget) => string;
    onSelectTarget: (targetId: string) => void;
    rootMode: boolean;
    rootEnabled: boolean;
    rootEntryHref: string;
    onEnterRoot: () => void;
    inboxHref: string;
    inboxActive: boolean;
    onSelectInbox: () => void;
    unreadCount: number;
    /** Workspace IDs whose configuration has not been saved. */
    dirtyTargetIds?: ReadonlySet<string>;
    /** Whether Root configuration has not been saved. */
    rootDirty?: boolean;
    theme: ThemeDisplay;
    onSelectTheme: (theme: ThemeDisplay) => void;
    onSignOut: () => void | Promise<void>;
  } = $props();

  /* Workspaces are the one rail section that grows without bound. The rail
     never scrolls and never wastes a slot: it shows as many full tiles as the
     height takes, and only when the next whole tile no longer fits does the
     run end in one counted button. The active workspace never folds - it
     trades places with the last tile that stays. */
  let railEl = $state<HTMLElement | null>(null);
  let shownCount = $state(Infinity);
  /* One tile plus the rail's gap. Fixed by the stylesheet below; the fixture
     measurement is what keeps the arithmetic honest when the rail's other
     occupants change. */
  const SLOT = 48;

  function measure(): void {
    if (railEl === null) return;
    const height = railEl.clientHeight;
    /* Everything that is not a workspace tile keeps its slot: sum the fixture
       children plus the column's gaps and paddings, then hand the rest to the
       run in whole 48px slots. */
    const styles = getComputedStyle(railEl);
    const gap = parseFloat(styles.rowGap) || 0;
    const padding = parseFloat(styles.paddingBlockStart) + parseFloat(styles.paddingBlockEnd);
    let fixture = 0;
    let fixtureCount = 0;
    for (const child of railEl.children) {
      const el = child as HTMLElement;
      if (el.classList.contains('rail-ws') || el.classList.contains('rail-more-wrap')) continue;
      if (el.classList.contains('rail-gap')) continue;
      if (el.hidden || el.offsetParent === null) continue;
      fixture += el.offsetHeight;
      fixtureCount += 1;
    }
    const room = height - padding - fixture - fixtureCount * gap;
    const slots = Math.max(1, Math.floor(room / SLOT));
    if (slots >= targets.length) {
      shownCount = Infinity;
      return;
    }
    /* The fold button takes the last slot. */
    shownCount = Math.max(1, slots - 1);
  }

  $effect(() => {
    if (railEl === null) return;
    void targets.length;
    void rootMode;
    measure();
    const observer = new ResizeObserver(() => measure());
    observer.observe(railEl);
    return () => observer.disconnect();
  });

  const shown = $derived.by(() => {
    if (shownCount >= targets.length) return [...targets];
    const visible = targets.slice(0, shownCount);
    const active = targets.find((target) => target.id === selectedId);
    if (active !== undefined && !visible.includes(active)) {
      visible[visible.length - 1] = active;
    }
    return visible;
  });
  const folded = $derived(targets.filter((target) => !shown.includes(target)));
  const foldedDirtyCount = $derived(
    folded.filter((target) => dirtyTargetIds?.has(target.id) === true).length,
  );

  let moreOpen = $state(false);

  function nameOf(target: PanelTarget): string {
    return target.account.display_name || target.account.login;
  }

  function targetIsDirty(target: PanelTarget): boolean {
    return dirtyTargetIds?.has(target.id) === true;
  }

  function dirtyTip(label: string, dirty: boolean): string {
    return dirty ? `${label} - unsaved changes` : label;
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

  function selectFromClick(event: MouseEvent, target: PanelTarget): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onSelectTarget(target.id);
  }

  function enterRootFromClick(event: MouseEvent): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    if (!rootMode) onEnterRoot();
  }

  function selectInboxFromClick(event: MouseEvent): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onSelectInbox();
  }

  const unreadLabel = $derived(unreadCount > 99 ? '99+' : String(unreadCount));
  const inboxTip = $derived(unreadCount === 0 ? 'Inbox' : `Inbox - ${unreadCount} unread`);
  const foldedTip = $derived.by(() => {
    const label = `${folded.length} more workspace${folded.length === 1 ? '' : 's'}`;
    if (foldedDirtyCount === 0) return label;
    return `${label} - ${foldedDirtyCount} with unsaved changes`;
  });
  const rootTip = $derived(dirtyTip('Root console', rootDirty));
  const viewerName = $derived(viewer?.account.display_name || viewer?.account.login || '');
  const viewerTip = $derived(
    viewer === null ? 'Account' : `${viewerName} - @${viewer.account.login}`,
  );
</script>

<!--
@component
The workspace switcher, and the way into the Root console. It is the outermost
navigation the panel has: everything else moves within a workspace, and this is what
changes which one you are in.

A dot marks a workspace with an unsaved draft, which is how a reader who has left a
settings page open somewhere else finds their way back to it - the same fact the draft
notice carries, said where the workspace is chosen.

The Root entry is drawn only for a viewer who has one, and drawing it disabled instead
would be telling everybody else about a console they cannot open.
-->

<nav class="rail" bind:this={railEl} aria-label="Consoles">
  <img class="rail-halo" src={haloUrl} alt="Smyklot" width="34" height="34" decoding="async" />

  {#each shown as target (target.id)}
    <a
      class="rail-tile rail-ws"
      class:is-active={!rootMode && !inboxActive && target.id === selectedId}
      class:has-dirty={targetIsDirty(target)}
      href={targetHref(target)}
      data-h={workspaceHue(target.account.login)}
      data-tip={dirtyTip(nameOf(target), targetIsDirty(target))}
      aria-label={dirtyTip(nameOf(target), targetIsDirty(target))}
      aria-current={!rootMode && !inboxActive && target.id === selectedId ? 'true' : undefined}
      onclick={(event) => selectFromClick(event, target)}
    >
      <!-- The real profile picture when the account carries one; the generated
           mark is the fallback, not the identity. Avatar owns the broken-image
           retry, so a failing URL lands on its monogram rather than a glyph. -->
      {#if target.account.avatar_url !== null}
        <span class="t is-avatar"
          ><Avatar account={target.account} size={36} shape="workspace" /></span
        >
      {:else}
        <span class="t">{workspaceInitials(nameOf(target))}</span>
      {/if}
      {#if targetIsDirty(target)}
        <span class="rail-dirty" aria-hidden="true">*</span>
      {/if}
    </a>
  {/each}

  {#if folded.length > 0}
    <span class="rail-more-wrap">
      <WorkspaceMenu
        bind:open={moreOpen}
        targets={folded}
        {targetHref}
        {onSelectTarget}
        {dirtyTargetIds}
        label="More workspaces"
      >
        {#snippet trigger(attributes)}
          <button
            {...attributes}
            class="rail-tile rail-more"
            class:menu-open={moreOpen}
            class:has-dirty={foldedDirtyCount > 0}
            type="button"
            data-tip={foldedTip}
            aria-label={foldedTip}
          >
            <span class="t">+{folded.length}</span>
            {#if foldedDirtyCount > 0}
              <span class="rail-dirty" aria-hidden="true">*</span>
            {/if}
          </button>
        {/snippet}
      </WorkspaceMenu>
    </span>
  {/if}

  <span class="rail-rule" aria-hidden="true"></span>

  {#if rootEnabled || rootMode}
    <a
      class="rail-tile rail-root"
      class:is-active={rootMode}
      class:has-dirty={rootDirty}
      href={rootEntryHref}
      data-tip={rootTip}
      aria-label={rootTip}
      aria-current={rootMode ? 'true' : undefined}
      onclick={enterRootFromClick}
    >
      <Icon name="shield" size="md" />
      {#if rootDirty}
        <span class="rail-dirty" aria-hidden="true">*</span>
      {/if}
    </a>
  {/if}

  <span class="rail-gap" aria-hidden="true"></span>

  <a
    class="rail-tile rail-inbox"
    class:is-active={inboxActive}
    href={inboxHref}
    data-tip={inboxTip}
    aria-label={inboxTip}
    aria-current={inboxActive ? 'true' : undefined}
    onclick={selectInboxFromClick}
  >
    <Icon name="notifications" size="md" />
    {#if unreadCount > 0}
      <span class="rail-badge" aria-hidden="true"><span class="t">{unreadLabel}</span></span>
    {/if}
  </a>

  {#if viewer !== null}
    <AccountMenu {viewer} {theme} {onSelectTheme} {onSignOut} name="rail-theme">
      {#snippet trigger(attributes)}
        <button
          {...attributes}
          class="rail-tile rail-user"
          type="button"
          data-tip={viewerTip}
          aria-label="Account menu for {viewerName}"
        >
          {#if viewer.account.avatar_url !== null}
            <span class="t is-avatar"><Avatar account={viewer.account} size={36} /></span>
          {:else}
            <span class="t">{workspaceInitials(viewerName)}</span>
          {/if}
        </button>
      {/snippet}
    </AccountMenu>
  {/if}
</nav>

<style>
  .rail {
    align-items: center;
    align-self: start;
    background: var(--sidebar-bg);
    border-inline-end: 1px solid var(--sidebar-border);
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    /* The mock's two numbers, and they are a pair: 8px between tiles and 12px of pad
       above the first one. At 10 and 14 the mark sat 2px lower than the mock's and every
       tile below it drifted a further 2px, so by the account menu at the foot the rail
       was a different rail. */
    gap: var(--space-2);
    height: 100dvh;
    inline-size: 60px;
    overflow: visible;
    padding-block: var(--space-3);
    position: sticky;
    top: 0;
    z-index: var(--layer-rail);
  }

  /* THE RAIL LEAVES ON A PHONE. Said here rather than only in `app.css`, because a
     scoped `.rail` carries the component's hash and outranks the shared one at the
     same class count - so the sheet asked for this and the component went on drawing
     itself beside a 320px page. The top bar does all three of its jobs there. */
  @media (max-width: 47.9375rem) {
    .rail {
      display: none;
    }
  }

  .rail > :global(:not(.rail-gap)) {
    flex: none;
  }

  .rail-halo {
    block-size: 34px;
    inline-size: 34px;
    margin-bottom: 4px;
  }

  .rail-rule {
    background: var(--sidebar-border);
    block-size: 1px;
    inline-size: 24px;
  }

  .rail-gap {
    flex: 1;
  }

  .rail-tile {
    align-items: center;
    block-size: 38px;
    border: 1px solid transparent;
    border-radius: 10px;
    color: var(--sidebar-text-secondary);
    display: inline-flex;
    inline-size: 38px;
    justify-content: center;
    position: relative;
    text-decoration: none;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  .rail-tile:hover {
    background: var(--sidebar-item-hover);
    color: var(--sidebar-text);
  }

  /* The ground is this tile's own; the sink, the scale and the crease come from the
     shell press law in `app.css`, which this tile now takes whole. */
  .rail-tile:active {
    background: var(--sidebar-item-pressed);
  }

  /* The solid selection pair, like the nav thumb: the old near-white fill under this
     inverse ink left the console shield white on white in the light workspace. */
  .rail-tile.is-active {
    background: var(--sidebar-active-bg);
    border-color: transparent;
    box-shadow: var(--sidebar-thumb-shadow);
    color: var(--sidebar-item-active-text);
  }

  /* Both, as the nav thumb takes both: the selection's own throw settles to its pressed
     value and the crease of a held surface is laid inside it. Stating only the throw
     replaced the crease the shell law gives every other tile, so the one tile a reader
     presses most was the one that did not read as going in. */
  .rail-tile.is-active:active {
    box-shadow: var(--sidebar-pressed-inset), var(--sidebar-thumb-shadow-pressed);
  }

  /* ---------- Workspace identity paint ----------
     A workspace without a profile picture wears a generated mark - and the rail stays
     calm: at rest every tile wears plain rail material, because eleven
     simultaneous colours are noise, not a plan. The identity wakes exactly
     twice: on HOVER the tile lights in its own colour, and on the SELECTED
     workspace the colour becomes a painted mark - an aurora of two conic
     beams whose rotation derives from the hue itself. One number does all of
     it: data-h, read by typed attr(); every colour derives in OKLCH,
     light-dark() picks the chrome recipe, and the Root console forces the
     dark one onto its rail. Nothing animates at idle. */
  .rail-ws,
  .ws-mini {
    --ws-h: attr(data-h type(<number>), 152);
    --ws-tint: light-dark(oklch(92% 0.06 var(--ws-h)), oklch(36% 0.07 var(--ws-h)));
    --ws-line: light-dark(oklch(74% 0.11 var(--ws-h)), oklch(56% 0.11 var(--ws-h)));
    --ws-ink: light-dark(oklch(36% 0.1 var(--ws-h)), oklch(85% 0.08 var(--ws-h)));
  }

  .rail-ws {
    font-size: var(--font-size-micro);
    font-weight: 700;
  }

  .rail-ws .t {
    position: relative;
    text-box: trim-both cap alphabetic;
    z-index: 1;
  }

  /* A real avatar IS the tile: it fills the face inside the 1px edge, above
     the aurora, and takes the tile's own rounding - a second, tighter radius
     inside the tile read as a picture floating on a box. */
  .rail-tile .t.is-avatar {
    display: inline-flex;
  }

  .rail-ws .t.is-avatar :global(.avatar) {
    border-radius: 9px;
  }

  .rail-ws:hover {
    background: var(--ws-tint);
    border-color: var(--ws-line);
    color: var(--ws-ink);
  }

  .rail-ws:active {
    background: light-dark(oklch(88% 0.08 var(--ws-h)), oklch(31% 0.07 var(--ws-h)));
    color: var(--ws-ink);
  }

  /* Selected: the painted mark. The ground holds L 50 whatever the hue, so
     the white letters always clear it; the pool under them keeps the beams
     out from behind ink. Beam angles are the hue read as degrees. */
  .rail-ws.is-active {
    --ws-ground: oklch(50% 0.14 var(--ws-h));
    --ws-a: calc(var(--ws-h) * 1deg);
    background: var(--ws-ground);
    border-color: light-dark(oklch(42% 0.14 var(--ws-h)), oklch(64% 0.12 var(--ws-h)));
    box-shadow: var(--sidebar-thumb-shadow);
    color: #fff;
  }

  .rail-ws.is-active:hover {
    --ws-ground: oklch(46% 0.14 var(--ws-h));
    background: var(--ws-ground);
    border-color: light-dark(oklch(42% 0.14 var(--ws-h)), oklch(64% 0.12 var(--ws-h)));
    color: #fff;
  }

  .rail-ws.is-active:active {
    --ws-ground: oklch(43% 0.14 var(--ws-h));
    box-shadow: var(--sidebar-pressed-inset);
  }

  .rail-ws.is-active::before {
    background-image:
      linear-gradient(148deg, rgb(255 255 255 / 14%), transparent 55%),
      radial-gradient(circle at 50% 54%, var(--ws-ground) 0 32%, transparent 62%),
      conic-gradient(
        from calc(var(--ws-a) + var(--ws-spin, 0deg)) at 26% 20%,
        transparent 0 6%,
        oklch(74% 0.16 calc(var(--ws-h) + 50) / 0.62) 15% 26%,
        transparent 42% 100%
      ),
      conic-gradient(
        from calc(160deg - var(--ws-a) - var(--ws-spin, 0deg)) at 76% 84%,
        transparent 0 8%,
        oklch(36% 0.16 calc(var(--ws-h) - 40) / 0.72) 18% 32%,
        transparent 48% 100%
      ),
      linear-gradient(var(--ws-ground), var(--ws-ground));
    border-radius: inherit;
    content: '';
    inset: 0;
    pointer-events: none;
    position: absolute;
  }

  /* Alive only under the pointer - zero cost at idle. */
  .rail-ws.is-active:hover::before {
    animation: ws-turn var(--rhythm-turn) var(--ease-linear) infinite;
  }

  @keyframes ws-turn {
    to {
      --ws-spin: 360deg;
    }
  }

  @property --ws-spin {
    syntax: '<angle>';
    inherits: false;
    initial-value: 0deg;
  }

  /* The Root console's chrome is dark in both themes - its identities too. */
  :global(.app-shell.root-mode) .rail {
    color-scheme: dark;
  }

  .rail-more {
    color: var(--sidebar-text-secondary);
    font-size: var(--font-size-micro);
    font-weight: 700;
  }

  .rail-more .t {
    text-box: trim-both cap alphabetic;
  }

  /* Open, the trigger holds the same ground an active tile does - the active PAIR,
     not the fill alone: the ink is inverse, and over a near-white fill "+N" was
     white on white. */
  .rail-more.menu-open {
    background: var(--sidebar-active-bg);
    border-color: transparent;
    box-shadow: var(--sidebar-thumb-shadow);
    color: var(--sidebar-item-active-text);
  }

  .rail-more.menu-open::after {
    content: none;
  }

  .rail-user {
    background: var(--surface-inset);
    border-color: var(--sidebar-border);
    border-radius: 50%;
    color: var(--sidebar-text-secondary);
    font-size: var(--font-size-micro);
    font-weight: 700;
  }

  .rail-user .t {
    text-box: trim-both cap alphabetic;
  }

  .rail-badge {
    align-items: center;
    background: var(--unread-badge-bg);
    border-radius: 999px;
    /* Declared 14px round - a padding-built pill was 13.2px with soft edges. */
    block-size: 14px;
    box-sizing: border-box;
    color: var(--unread-badge-text);
    display: inline-flex;
    font-size: 0.625rem;
    font-variant-numeric: tabular-nums;
    justify-content: center;
    line-height: var(--leading-flat);
    min-inline-size: 14px;
    padding: 0 4px;
    position: absolute;
    right: -3px;
    top: -3px;
  }

  .rail-badge .t {
    display: block;
    text-box: trim-both cap alphabetic;
  }

  /* Rail tooltips: the name on hover, popover material. */
  .rail-tile::after {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: 6px;
    box-shadow: var(--shadow-popover);
    color: var(--text-primary);
    content: attr(data-tip);
    font-size: var(--font-size-micro);
    left: calc(100% + 10px);
    opacity: 0;
    padding: 0.3rem 0.5rem;
    pointer-events: none;
    position: absolute;
    top: 50%;
    translate: 0 -50%;
    white-space: nowrap;
    z-index: var(--layer-flyout);
  }

  .rail-tile:hover::after,
  .rail-tile:focus-visible::after {
    opacity: 1;
  }

  /* A tile whose menu is open keeps its name to itself - the menu sits where
     the tip would. */
  .rail-tile[aria-expanded='true']::after {
    content: none;
  }

  /* On a touch screen the name-on-hover never fires, so the tile's ::after
     serves the thumb instead: an invisible overlay that grows the 38px tile
     to the 44px target the platforms ask for. */
  @media (hover: none), (max-width: 64rem) {
    .rail-tile::after {
      background: none;
      border: 0;
      border-radius: 0;
      box-shadow: none;
      content: '';
      /* Exactly the 44px square, centred on the tile whatever its size. */
      inset: calc((100% - 44px) / 2);
      opacity: 1;
      padding: 0;
      pointer-events: auto;
      translate: none;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .rail-ws.is-active:hover::before {
      animation: none;
    }
  }
</style>
