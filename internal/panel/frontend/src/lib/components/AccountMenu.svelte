<script lang="ts">
  import type { Snippet } from 'svelte';

  import type { ThemeDisplay } from '../preferences';
  import type { PanelViewer } from '../types';
  import Popover, { type PopoverTriggerAttributes } from './Popover.svelte';
  import ThemeSwitch from './ThemeSwitch.svelte';

  const {
    viewer,
    theme,
    onSelectTheme,
    onSignOut,
    name,
    side = 'right',
    align = 'end',
    trigger,
  }: {
    viewer: PanelViewer;
    theme: ThemeDisplay;
    onSelectTheme: (theme: ThemeDisplay) => void;
    onSignOut: () => void | Promise<void>;
    /** Distinguishes the theme control from any other on the page. */
    name: string;
    side?: 'above' | 'below' | 'left' | 'right';
    align?: 'start' | 'center' | 'end';
    trigger: Snippet<[PopoverTriggerAttributes]>;
  } = $props();

  const viewerName = $derived(viewer.account.display_name || viewer.account.login);
</script>

<!--
@component
Who is signed in, how the panel is themed, and the way out.

Shared between the rail and the collapsed sidebar's foot, where the rail is gone: the
account menu a reader learns in one shell has to be the same menu in the other.
-->

<Popover
  {side}
  {align}
  {trigger}
  offset={8}
  role="menu"
  label="Account"
  skin="sidebar"
  itemSelector=".menu-item"
>
  <div class="console-menu account-menu" role="none">
    <div class="menu-eyebrow">{viewerName} - @{viewer.account.login}</div>
    <div class="menu-theme-row">
      <ThemeSwitch {name} {theme} surface="sidebar" onSelect={onSelectTheme} />
    </div>
    <div class="menu-sep" role="none"></div>
    <button class="menu-item is-danger" role="menuitem" onclick={() => void onSignOut()}>
      <span class="mi-label">Sign out</span>
    </button>
  </div>
</Popover>
