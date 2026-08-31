<script lang="ts">
  import type { PanelTarget } from '../types';
  import Icon from './Icon.svelte';
  import WorkspaceMenu from './WorkspaceMenu.svelte';
  import { workspaceInitials } from '../workspace-mark';

  const {
    open,
    onToggle,
    title,
    targets,
    selected,
    targetHref,
    onSelectTarget,
    dirtyTargetIds,
    rootMode,
    console: consoleEntry = null,
  }: {
    /** Whether the pages drawer is showing, which is what the menu button reports. */
    open: boolean;
    onToggle: () => void;
    /** The page being read, which is the one thing the bar has room to say. */
    title: string;
    targets: readonly PanelTarget[];
    selected: PanelTarget | null;
    targetHref: (target: PanelTarget) => string;
    onSelectTarget: (targetId: string) => void;
    dirtyTargetIds?: ReadonlySet<string>;
    rootMode: boolean;
    /** The Operations console, when this viewer has one to cross to. */
    console?: { href: string; onEnter: () => void } | null;
  } = $props();

  const scope = $derived(
    rootMode
      ? 'Operations'
      : (selected?.account.display_name ?? selected?.account.login ?? 'Workspace'),
  );
  const mark = $derived(rootMode ? 'OP' : workspaceInitials(scope));
</script>

<!--
@component
The shell on a phone, where there is no room for a rail beside a sidebar beside the
content: one 56px bar carrying the way into the pages, the name of the page you are on,
and the way across to another workspace.

It is drawn only below 48rem - above that the rail and the sidebar are both in flow -
and it is a sibling of `.app-shell` rather than a cell inside it, because a sticky
element inside a grid is confined to its own row and would have nowhere to travel.
-->

<header class="top-bar">
  <button
    class="top-menu"
    type="button"
    aria-expanded={open}
    aria-label={open ? 'Hide the pages' : 'Show the pages'}
    onclick={onToggle}
  >
    <Icon name={open ? 'sidebar-collapse' : 'sidebar-expand'} size="md" />
  </button>

  <span class="top-title">{title}</span>

  <WorkspaceMenu
    {targets}
    {targetHref}
    {onSelectTarget}
    {dirtyTargetIds}
    console={consoleEntry}
    side="below"
    align="end"
  >
    {#snippet trigger(props)}
      <button class="top-ws" type="button" {...props}>
        <span class="top-ws-mark" aria-hidden="true">{mark}</span>
        <span class="t">{scope}</span>
        <Icon name="chevron-down" size="xs" />
      </button>
    {/snippet}
  </WorkspaceMenu>
</header>

<style>
  /* Hidden until the rail leaves. Both decisions are the same decision, so they are
     made at the same width - see `.rail` in `app.css`. */
  .top-bar {
    align-items: center;
    background: var(--sidebar-bg);
    border-block-end: 1px solid var(--sidebar-border);
    color: var(--sidebar-text);
    display: none;
    gap: var(--space-2);
    inset-block-start: 0;
    min-block-size: 56px;
    padding-inline: var(--space-2) var(--space-3);
    position: sticky;
    z-index: var(--layer-top-bar);
  }

  @media (max-width: 47.9375rem) {
    .top-bar {
      display: flex;
    }
  }

  .top-menu {
    align-items: center;
    background: none;
    block-size: var(--touch-target);
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--sidebar-text-secondary);
    display: inline-flex;
    inline-size: var(--touch-target);
    justify-content: center;
    padding: 0;
  }

  .top-menu:hover {
    background: var(--sidebar-item-hover);
    color: var(--sidebar-text);
  }

  .top-menu:active {
    background: var(--sidebar-item-pressed);
  }

  /* The page, ellipsised rather than wrapped: the bar is one line and the page's own
     h1 below it is never abbreviated. */
  .top-title {
    flex: 1;
    font-size: var(--font-size-title);
    font-weight: 650;
    min-inline-size: 0;
    overflow: hidden;
    text-box: trim-both cap alphabetic;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .top-ws {
    align-items: center;
    background: none;
    block-size: var(--touch-target);
    border: 1px solid transparent;
    border-radius: var(--r-ctl);
    color: var(--sidebar-text-secondary);
    display: inline-flex;
    font-size: var(--font-size-compact);
    font-weight: 600;
    gap: var(--space-2);
    max-inline-size: 46vw;
    padding-inline: var(--space-2);
  }

  .top-ws:hover {
    background: var(--sidebar-item-hover);
    color: var(--sidebar-text);
  }

  .top-ws:active {
    background: var(--sidebar-item-pressed);
  }

  .top-ws .t {
    min-inline-size: 0;
    overflow: hidden;
    text-box: trim-both cap alphabetic;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .top-ws-mark {
    align-items: center;
    background: var(--sidebar-item-hover);
    border-radius: var(--r-chip);
    block-size: 24px;
    color: var(--sidebar-text);
    display: inline-flex;
    flex: none;
    font-size: var(--font-size-micro);
    inline-size: 24px;
    justify-content: center;
  }
</style>
