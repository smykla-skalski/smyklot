<script lang="ts">
  import BrandMark from './BrandMark.svelte';
  import Icon from './Icon.svelte';
  import SidebarTooltip from './SidebarTooltip.svelte';

  /**
   * The top of the sidebar: the mark, and the two controls that change the shape of the
   * panel around it.
   *
   * Two triggers that look like a pair and are not. The collapse button belongs to the
   * rail and is only there on a desktop; the menu button belongs to the top bar and is
   * only there on a phone. They share this row and nothing else, which is why each
   * carries its own rules rather than being drawn from one.
   *
   * **`navigationOpen` is `$bindable`, and it has to be.** The drawer it controls is
   * elsewhere in `IdentityBar`, and so are the three things that close it - a click
   * outside the sidebar, Escape, and a route change. A callback would let this row
   * open the drawer but never be told it had closed, so the button's `aria-expanded`
   * would drift from what is on screen. Binding keeps one value.
   *
   * What the collapsed rail does to the collapse button stays in `IdentityBar`:
   * `.collapsed` is on the sidebar, which is the parent's element. What this row does
   * to itself - including its own phone layout - is here, because a rule written from
   * the parent is one class against this component's class-plus-scope, and loses.
   */
  let {
    part,
    showNavigation = true,
    collapsed = false,
    onToggleCollapsed,
    navigationOpen = $bindable(false),
  }: {
    /** The word under the mark: which console this is. */
    part: string;
    /** Off where there is nothing to navigate - the invitation page, signed out. */
    showNavigation?: boolean;
    collapsed?: boolean;
    onToggleCollapsed?: () => void;
    /** Whether the phone's navigation drawer is open. Two-way; see the note above. */
    navigationOpen?: boolean;
  } = $props();
</script>

<div class="brand-row">
  <BrandMark {part} heading />

  {#if showNavigation}
    <button
      class="sidebar-collapse-trigger"
      type="button"
      aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
      aria-expanded={!collapsed}
      onclick={() => onToggleCollapsed?.()}
    >
      <Icon name={collapsed ? 'sidebar-expand' : 'sidebar-collapse'} size={16} strokeWidth={1.75} />
      <SidebarTooltip text={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} />
    </button>

    <!-- Three bars and a word. The bars are `aria-hidden` because they are the icon;
         the word is what the button is called, and it is hidden by CSS rather than
         dropped, so a screen reader still reads a named control. -->
    <button
      class="mobile-navigation-trigger"
      type="button"
      aria-label="Toggle panel navigation"
      aria-expanded={navigationOpen}
      aria-controls="panel-navigation-drawer"
      onclick={() => (navigationOpen = !navigationOpen)}
    >
      <span aria-hidden="true"></span>
      <span aria-hidden="true"></span>
      <span aria-hidden="true"></span>
      <span>Menu</span>
    </button>
  {/if}
</div>

<style>
  .brand-row {
    align-items: center;
    display: flex;
    justify-content: space-between;
    /* The whole gap under the mark, rather than half of it here and half on the
       switcher below. Split, it only measured 16px on a surface that has a switcher:
       the console has none, so its navigation stood 8px under the mark - close enough
       to the 3px between two navigation items to read as one more of them. */
    margin-bottom: var(--space-4);
    min-height: 2.375rem;
    /* No padding on the closing edge: it held the collapse trigger 8px inside the right edge every
       navigation row below it lines up on. The mark keeps its own inset on the opening edge.
       Collapsed, the row zeroes this out and centres instead. */
    padding: 0 0 0 var(--space-2);
    position: relative;
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
    /* A 28px square either way, so it takes the figure meant for a disc: the ordinary 0.98 would
       move its edge a third of a pixel and read as nothing happening. */
    --press-scale: var(--press-scale-disc);
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
    transform: scale(var(--press-scale));
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

  /*
   * On a phone the row is the top bar.
   * ---------------------------------
   * These are here and not in `IdentityBar`'s phone query, and that is not a
   * preference: a rule written from the parent is one class against this component's
   * class-plus-scope and loses silently. The account row next door proved it, at 146px
   * against a 44px target.
   *
   * The row is `--bar-height` tall and its contents centre on that. It used to be
   * sized by its padding and its tallest child while the bar was told 60, so
   * everything in it sat 4px above the line a reader sees it against.
   */
  @media (max-width: 48rem) {
    .brand-row {
      flex-direction: row;
      height: var(--bar-height);
      justify-content: space-between;
      margin-bottom: 0;
      min-height: 0;
      padding: 0 var(--space-4);
    }

    /* The rail is gone, so the control that collapses it has nothing to act on. It
       was hidden from `IdentityBar`'s phone query, where a single class lost to this
       component's own `display: inline-flex` - so it stayed on the bar and its 44px
       touch target reached over the account menu beside it. */
    .sidebar-collapse-trigger {
      display: none;
    }

    .mobile-navigation-trigger {
      display: flex;
      margin: 0;
      position: absolute;
      right: var(--bar-slot-menu);
      /* Centred on the bar by subtraction, like the two beside it. */
      top: calc((var(--bar-height) - 1.75rem) / 2);
    }
  }

  /* The word goes and the three bars stay: at this width the bar has three controls
     in it and the one that is only an icon everywhere else can be one here too. */
  @media (max-width: 30rem) {
    .mobile-navigation-trigger > span:last-child {
      display: none;
    }
  }
</style>
