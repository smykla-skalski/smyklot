<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';

  import type { PanelAccount } from '#lib/types.js';
  import Avatar from './Avatar.svelte';
  import Icon from './Icon.svelte';
  import SidebarTooltip from './SidebarTooltip.svelte';

  /**
   * The row at the foot of the sidebar that opens the account menu.
   *
   * It is a trigger, so `attributes` is whatever the popover needs on it -
   * `aria-expanded`, `aria-controls`, the handlers that open it. Spread rather than
   * enumerated, because that set belongs to the popover primitive and would go stale
   * here the moment it changed.
   *
   * **What it looks like when the rail is collapsed is not decided here.** `.collapsed`
   * is on the sidebar, which is the parent's element, so the rules that shrink this to
   * an avatar and swap in the tooltip stay there, anchored. This component owns the row
   * itself; the rail owns what the rail does to it.
   *
   * Not `IdentityRow`, which draws the same three things: that one is written against
   * the page's palette and this against the rail's, with its own gap and a smaller step
   * of type. The note on `IdentityRow` says so at more length.
   */
  const {
    account,
    handle,
    attributes,
  }: {
    account: PanelAccount;
    /** The `@login` as it should read, already resolved by the caller. */
    handle: string;
    /**
     * Whatever the popover puts on its trigger.
     *
     * Typed as the button's own attributes rather than a bag of unknowns: `Popover`
     * hands over a real `HTMLButtonAttributes`, and widening it here would mean this
     * component accepted anything while the thing supplying it was precise.
     */
    attributes?: HTMLButtonAttributes;
  } = $props();
</script>

<div class="account-card">
  <!-- No unread dot here any more. It marked a count that could only be read by opening
       this menu; the count is on the Inbox row now, and a second mark on a card holding
       nothing about notifications would point at nothing. -->
  <button
    class="who"
    type="button"
    aria-label={`Account menu for ${account.display_name}`}
    {...attributes ?? {}}
  >
    <span class="who-avatar">
      <Avatar {account} size={32} />
    </span>
    <span class="who-text band-trim-stack">
      <span class="who-name">{account.display_name}</span>
      <span class="who-handle mono">{handle}</span>
    </span>
    <span class="menu-chevron" aria-hidden="true">
      <Icon name="chevron-up" size={14} strokeWidth={2} />
    </span>
    <SidebarTooltip text="Account" />
  </button>
</div>

<style>
  .account-card {
    border-top: 1px solid var(--sidebar-border);
    margin-top: var(--space-2);
    padding-top: var(--space-2);
  }

  .who {
    align-items: center;
    background: transparent;
    border: 0;
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
    width: 100%;
  }

  .who:hover,
  .who[aria-expanded='true'] {
    background: var(--sidebar-item-hover);
  }

  .who:active {
    background: var(--sidebar-item-pressed);
  }

  .who-avatar {
    display: inline-flex;
    flex: none;
  }

  .who-text {
    display: flex;
    flex-direction: column;
    /* The whole of the space between the two lines. It used to be 0.3rem with
       the handle pulled 0.2rem back up into it, which is a nudge standing in
       for a measurement. */
    gap: 0.1rem;
    min-width: 0;
    text-align: left;
  }

  .who-name {
    color: var(--sidebar-text);
    font-size: var(--font-size-meta);
    font-weight: 600;
    line-height: 1.2;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .who-handle {
    color: var(--sidebar-text-muted);
    font-size: var(--font-size-micro);
    font-weight: 500;
    line-height: 1.35;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .who-name,
  .who-handle {
    overflow-x: clip;
    overflow-y: visible;
  }

  /*
   * On a phone the row is a target, not a row.
   * -----------------------------------------
   * These live here rather than in `IdentityBar`'s phone query, and that is not a
   * preference. A scoped rule here carries a class AND this component's scope class;
   * a `:global(.who-text)` written from the parent carries one class. The parent
   * loses, silently - the name and handle stayed visible and the account button
   * measured 146px against a 44px thumb target. `mobile-layout` caught it.
   */
  @media (max-width: 48rem) {
    /* Out of the rail and into the top bar: the card is positioned against the bar
       rather than stacked at the foot of a sidebar that is not there. The three
       custom properties are the bar's own and inherit down. */
    .account-card {
      border: 0;
      margin: 0;
      padding: 0;
      position: absolute;
      right: var(--bar-slot-account);
      top: calc((var(--bar-height) - var(--bar-control)) / 2);
    }

    /* A target rather than a row: no surface of its own, and sized by what is left
       inside it once the name and the chevron have gone. */
    .who {
      background: transparent;
      border: 0;
      box-shadow: none;
      display: flex;
      min-height: var(--bar-control);
      padding: 0;
      width: auto;
    }

    .who-text,
    .menu-chevron {
      display: none;
    }
  }

  /* `.menu-chevron`'s own look stays with the sidebar: the workspace switcher wears it too, and
     four declarations shared by two triggers is a class doing its job rather than a
     component waiting to be written. It reaches in from there. */
</style>
