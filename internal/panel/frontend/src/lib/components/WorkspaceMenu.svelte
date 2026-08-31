<script lang="ts">
  import type { Snippet } from 'svelte';

  import type { PanelTarget } from '../types';
  import { workspaceHue, workspaceInitials } from '../workspace-mark.js';
  import Avatar from './Avatar.svelte';
  import ClippedLabel from './ClippedLabel.svelte';
  import Icon from './Icon.svelte';
  import Popover, { type PopoverTriggerAttributes } from './Popover.svelte';

  let {
    open = $bindable(false),
    targets,
    targetHref,
    onSelectTarget,
    dirtyTargetIds,
    label = 'Workspaces',
    side = 'right',
    align = 'start',
    console: consoleEntry = null,
    trigger,
  }: {
    open?: boolean;
    targets: readonly PanelTarget[];
    targetHref: (target: PanelTarget) => string;
    onSelectTarget: (targetId: string) => void;
    /** Workspace IDs whose configuration has not been saved. */
    dirtyTargetIds?: ReadonlySet<string>;
    label?: string;
    side?: 'above' | 'below' | 'left' | 'right';
    align?: 'start' | 'center' | 'end';
    /** The Operations console, for a menu that is the whole console switch. */
    console?: { href: string; onEnter: () => void } | null;
    trigger: Snippet<[PopoverTriggerAttributes]>;
  } = $props();

  let query = $state('');

  const matches = $derived.by(() => {
    const needle = query.trim().toLowerCase();
    if (needle === '') return targets;
    return targets.filter((target) => {
      const name = nameOf(target);
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

  function isDirty(target: PanelTarget): boolean {
    return dirtyTargetIds?.has(target.id) === true;
  }

  function tipFor(target: PanelTarget): string {
    return isDirty(target) ? `${nameOf(target)} - unsaved changes` : nameOf(target);
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
</script>

<!--
@component
The workspaces a reader can open, as a menu.

It exists in two places at once - behind the rail's counted fold button, and behind the
collapsed sidebar's own mark, where the rail is gone and this is the only way to change
workspace. One component, because a switcher that lists different things in the two
places is a switcher a reader cannot trust.
-->

<Popover
  bind:open
  {side}
  {align}
  {trigger}
  offset={8}
  role="menu"
  {label}
  skin="sidebar"
  itemSelector=".menu-item"
  focusSelector=".menu-search input"
  onclose={() => (query = '')}
>
  <div class="console-menu" role="none">
    <div class="menu-search">
      <Icon name="search" size="xs" />
      <input
        type="search"
        placeholder="Find a workspace"
        aria-label="Find a workspace"
        bind:value={query}
      />
    </div>
    <div class="menu-scroll" role="none">
      {#each matches as target (target.id)}
        <a
          class="menu-item"
          class:has-dirty={isDirty(target)}
          role="menuitem"
          href={targetHref(target)}
          aria-label={tipFor(target)}
          onclick={(event) => {
            if (!plainClick(event)) return;
            event.preventDefault();
            open = false;
            onSelectTarget(target.id);
          }}
        >
          {#if target.account.avatar_url !== null}
            <Avatar account={target.account} size={18} shape="workspace" />
          {:else}
            <span class="ws-mini" data-h={workspaceHue(target.account.login)}>
              <span class="t">{workspaceInitials(nameOf(target))}</span>
            </span>
          {/if}
          <ClippedLabel class="mi-label" text={nameOf(target)} />
          {#if isDirty(target)}
            <span class="rail-dirty menu-dirty" aria-hidden="true">*</span>
          {/if}
        </a>
      {/each}
    </div>
    {#if matches.length === 0}
      <div class="menu-hint">No workspace matches</div>
    {/if}
    {#if consoleEntry !== null}
      <div class="menu-sep" role="none"></div>
      <a
        class="menu-item"
        role="menuitem"
        href={consoleEntry.href}
        onclick={(event) => {
          if (!plainClick(event)) return;
          event.preventDefault();
          open = false;
          consoleEntry.onEnter();
        }}
      >
        <Icon name="shield" size="xs" />
        <span class="mi-label">Operations console</span>
      </a>
    {/if}
  </div>
</Popover>
