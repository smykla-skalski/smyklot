<script module lang="ts">
  /**
   * The identity hue: hashed once from the login and rendered as `data-h`.
   * The stylesheet does everything else - tint, line, ink and the selected
   * aurora all derive from this one number in OKLCH.
   */
  export function workspaceHue(login: string): number {
    let hash = 5381;
    for (let i = 0; i < login.length; i += 1) {
      hash = (hash * 33) ^ login.charCodeAt(i);
    }
    return (hash >>> 0) % 360;
  }

  /** "Smykla Skalski" -> "SS", "bartsmykla" -> "B", "Oak & Pine" -> "OP". */
  export function workspaceInitials(name: string): string {
    const words = name.split(/[^\p{L}\p{N}]+/u).filter((word) => word.length > 0);
    if (words.length === 0) return '?';
    return words
      .slice(0, 2)
      .map((word) => word[0]!.toUpperCase())
      .join('');
  }
</script>

<script lang="ts">
  import haloUrl from '../../assets/smyklot-halo.svg';

  import type { PanelTarget, PanelViewer } from '../types';
  import type { ThemeDisplay } from '../preferences';
  import Avatar from './Avatar.svelte';
  import ClippedLabel from './ClippedLabel.svelte';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';
  import ThemeSwitch from './ThemeSwitch.svelte';

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
    theme,
    onSelectTheme,
    onSignOut,
    pagesOpen = false,
    onTogglePages,
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
    theme: ThemeDisplay;
    onSelectTheme: (theme: ThemeDisplay) => void;
    onSignOut: () => void | Promise<void>;
    /** Narrow shells only: whether the pages drawer is open. */
    pagesOpen?: boolean;
    onTogglePages?: () => void;
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

  let moreOpen = $state(false);
  let query = $state('');
  const foldMatches = $derived.by(() => {
    const needle = query.trim().toLowerCase();
    if (needle === '') return folded;
    return folded.filter((target) => {
      const name = target.account.display_name || target.account.login;
      return (
        name.toLowerCase().includes(needle) ||
        target.account.login.toLowerCase().includes(needle) ||
        workspaceInitials(name).toLowerCase().includes(needle)
      );
    });
  });

  function nameOf(target: PanelTarget): string {
    return target.account.display_name || target.account.login;
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
  const viewerName = $derived(viewer?.account.display_name || viewer?.account.login || '');
  const viewerTip = $derived(
    viewer === null ? 'Account' : `${viewerName} - @${viewer.account.login}`,
  );
</script>

<nav class="rail" bind:this={railEl} aria-label="Consoles">
  <img class="rail-halo" src={haloUrl} alt="Smyklot" width="34" height="34" decoding="async" />

  {#if onTogglePages !== undefined}
    <button
      class="rail-tile rail-pages"
      type="button"
      data-tip="Pages"
      aria-expanded={pagesOpen}
      aria-label="Pages"
      onclick={onTogglePages}
    >
      <Icon name={pagesOpen ? 'sidebar-collapse' : 'sidebar-expand'} size={18} />
    </button>
  {/if}

  {#each shown as target (target.id)}
    <a
      class="rail-tile rail-ws"
      class:is-active={!rootMode && !inboxActive && target.id === selectedId}
      href={targetHref(target)}
      data-h={workspaceHue(target.account.login)}
      data-tip={nameOf(target)}
      aria-label={nameOf(target)}
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
    </a>
  {/each}

  {#if folded.length > 0}
    <span class="rail-more-wrap">
      <Popover
        bind:open={moreOpen}
        side="right"
        align="start"
        offset={8}
        role="menu"
        label="More workspaces"
        skin="sidebar"
        itemSelector=".menu-item"
        focusSelector=".menu-search input"
        onclose={() => (query = '')}
      >
        {#snippet trigger(attributes)}
          <button
            {...attributes}
            class="rail-tile rail-more"
            class:menu-open={moreOpen}
            type="button"
            data-tip={`${folded.length} more workspace${folded.length === 1 ? '' : 's'}`}
          >
            <span class="t">+{folded.length}</span>
          </button>
        {/snippet}
        <div class="console-menu" role="none">
          <div class="menu-search">
            <Icon name="search" size={12} />
            <input
              type="search"
              placeholder="Find a workspace"
              aria-label="Find a workspace"
              bind:value={query}
            />
          </div>
          <div class="menu-scroll" role="none">
            {#each foldMatches as target (target.id)}
              <a
                class="menu-item"
                role="menuitem"
                href={targetHref(target)}
                onclick={(event) => {
                  moreOpen = false;
                  selectFromClick(event, target);
                }}
              >
                {#if target.account.avatar_url !== null}
                  <Avatar account={target.account} size={20} shape="workspace" />
                {:else}
                  <span class="ws-mini" data-h={workspaceHue(target.account.login)}>
                    <span class="t">{workspaceInitials(nameOf(target))}</span>
                  </span>
                {/if}
                <ClippedLabel class="mi-label" text={nameOf(target)} />
              </a>
            {/each}
          </div>
          {#if foldMatches.length === 0}
            <div class="menu-hint">No workspace matches</div>
          {/if}
        </div>
      </Popover>
    </span>
  {/if}

  <span class="rail-rule" aria-hidden="true"></span>

  {#if rootEnabled || rootMode}
    <a
      class="rail-tile rail-root"
      class:is-active={rootMode}
      href={rootEntryHref}
      data-tip="Root console"
      aria-label="Root console"
      aria-current={rootMode ? 'true' : undefined}
      onclick={enterRootFromClick}
    >
      <Icon name="shield" size={18} />
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
    <Icon name="notifications" size={18} />
    {#if unreadCount > 0}
      <span class="rail-badge" aria-hidden="true"><span class="t">{unreadLabel}</span></span>
    {/if}
  </a>

  {#if viewer !== null}
    <Popover
      side="right"
      align="end"
      offset={8}
      role="menu"
      label="Account"
      skin="sidebar"
      itemSelector=".menu-item"
    >
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
      <div class="console-menu account-menu" role="none">
        <div class="menu-eyebrow">{viewerName} - @{viewer.account.login}</div>
        <div class="menu-theme-row">
          <ThemeSwitch name="rail-theme" {theme} surface="sidebar" onSelect={onSelectTheme} />
        </div>
        <div class="menu-sep" role="none"></div>
        <button class="menu-item is-danger" role="menuitem" onclick={() => void onSignOut()}>
          <span class="mi-label">Sign out</span>
        </button>
      </div>
    </Popover>
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
    gap: 10px;
    height: 100dvh;
    inline-size: 60px;
    overflow: visible;
    padding-block: 14px;
    position: sticky;
    top: 0;
    z-index: var(--layer-rail);
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

  .rail-tile:active {
    background: var(--sidebar-item-pressed);
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }

  .rail-tile.is-active {
    background: var(--sidebar-thumb);
    border-color: var(--sidebar-border);
    box-shadow: var(--sidebar-thumb-shadow);
    color: var(--sidebar-item-active-text);
  }

  .rail-tile.is-active:hover {
    translate: 0 -1px;
  }

  .rail-tile.is-active:active {
    box-shadow: var(--sidebar-thumb-shadow-pressed);
    translate: 0 1px;
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
    translate: none;
  }

  .rail-ws.is-active:active {
    --ws-ground: oklch(43% 0.14 var(--ws-h));
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
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
    animation: ws-turn 9s linear infinite;
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

  /* Open, the trigger holds the same ground an active tile does. */
  .rail-more.menu-open {
    background: var(--sidebar-thumb);
    border-color: var(--sidebar-border);
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
    line-height: 1;
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
    z-index: 60;
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

  .rail-pages {
    display: none;
  }

  @media (max-width: 64rem) {
    .rail {
      position: relative;
      z-index: var(--layer-rail);
    }

    .rail-pages {
      display: inline-flex;
    }
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

  /* ---------- The rail's menus: console material ---------- */
  .console-menu {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    min-inline-size: 13rem;
    max-inline-size: min(24rem, calc(100vw - 24px));
    padding: var(--space-1);
    /* Rows breathe 2px apart, so a hovered row and its chosen neighbour never
       read as one fused pill. */
    row-gap: 2px;
  }

  .console-menu :global(.menu-item),
  .console-menu .menu-item {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 6px;
    block-size: 32px;
    color: var(--sidebar-menu-text);
    cursor: pointer;
    display: flex;
    font-size: var(--font-size-control);
    gap: var(--space-2);
    inline-size: 100%;
    padding-inline: var(--space-3);
    text-align: start;
    text-decoration: none;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  .console-menu .menu-item:hover {
    background: var(--sidebar-menu-hover);
  }

  .console-menu .menu-item:focus-visible {
    background: var(--sidebar-menu-hover);
    outline: none;
  }

  .console-menu .menu-item:active {
    background: var(--sidebar-menu-pressed);
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }

  .console-menu .menu-item.is-danger {
    color: var(--sidebar-stop);
  }

  .console-menu .menu-item.is-danger:hover,
  .console-menu .menu-item.is-danger:focus-visible {
    background: var(--sidebar-stop-tint);
  }

  /* No cap trim here: a menu label is a name with descenders, and the trim
     cut them off. The flex row centres it on its own. Anchored through the
     item because the span may be ClippedLabel's markup, outside this
     component's scope. */
  .menu-item :global(.mi-label),
  .mi-label {
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .menu-eyebrow {
    color: var(--sidebar-menu-muted);
    font-size: var(--font-size-micro);
    font-weight: 600;
    letter-spacing: 0.07em;
    line-height: 16px;
    padding: var(--space-2) var(--space-3) var(--space-1);
    text-transform: uppercase;
  }

  .menu-sep {
    background: var(--sidebar-popover-border);
    block-size: 1px;
    margin: var(--space-1) calc(var(--space-1) * -1);
  }

  .menu-theme-row {
    display: flex;
    padding: var(--space-1) var(--space-2) var(--space-2);
  }

  .menu-search {
    align-items: center;
    /* Declared 36, and the seam is a drawn line in the gap below, not part of
       the row's box - a border made the row 37 and the text ride 0.5 high. */
    block-size: 36px;
    box-shadow: 0 1px 0 var(--sidebar-popover-border);
    color: var(--sidebar-menu-muted);
    display: flex;
    gap: var(--space-2);
    margin: calc(var(--space-1) * -1) calc(var(--space-1) * -1) var(--space-1);
    padding: 0 var(--space-3);
  }

  .menu-search input {
    background: none;
    block-size: 100%;
    border: 0;
    color: var(--sidebar-menu-text);
    flex: 1;
    font-size: var(--font-size-control);
    outline: none;
    padding: 0;
  }

  .menu-search input::placeholder {
    color: var(--sidebar-menu-muted);
  }

  .menu-scroll {
    display: grid;
    max-block-size: 288px;
    overflow: auto;
    overscroll-behavior: contain;
  }

  .menu-hint {
    color: var(--sidebar-menu-muted);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    line-height: 16px;
    padding: var(--space-1) var(--space-3) var(--space-2);
  }

  /* The workspace initial at menu size: the identity at rest voice, 20px. */
  .ws-mini {
    align-items: center;
    background: var(--ws-tint);
    border-radius: 6px;
    block-size: 20px;
    color: var(--ws-ink);
    display: inline-flex;
    flex: none;
    font-size: 0.5625rem;
    font-weight: 700;
    inline-size: 20px;
    justify-content: center;
  }

  .ws-mini .t {
    text-box: trim-both cap alphabetic;
  }

  @media (prefers-reduced-motion: reduce) {
    .rail-ws.is-active:hover::before {
      animation: none;
    }
  }
</style>
