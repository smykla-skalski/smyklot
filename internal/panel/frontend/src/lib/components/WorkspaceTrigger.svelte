<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';

  import type { PanelAccount } from '#lib/types.js';
  import Avatar from './Avatar.svelte';
  import Icon from './Icon.svelte';
  import SidebarTooltip from './SidebarTooltip.svelte';

  /**
   * The row near the top of the sidebar that opens the workspace list.
   *
   * Sibling to `AccountTrigger`, and deliberately not the same component: the two look
   * alike in the rail and answer different questions. This one is a kicker over a name,
   * because "Workspace" says what the name below it IS - the account row needs no such
   * label, since a face and a handle say what they are.
   *
   * **Its phone layout is here, not in `IdentityBar`.** A scoped rule here carries a
   * class plus this component's scope class; one written from the parent as
   * `:global(.target-trigger)` carries a single class and loses. That is not theory -
   * it is how the account row next door came to measure 146px against a 44px target.
   * Only what the parent's state decides - what the collapsed rail does to this row -
   * stays out there, anchored.
   */
  const {
    account,
    attributes,
  }: {
    account: PanelAccount;
    /** Whatever the popover puts on its trigger. */
    attributes?: HTMLButtonAttributes;
  } = $props();
</script>

<button
  class="target-trigger"
  type="button"
  aria-label={`Switch workspace, currently ${account.display_name}`}
  {...attributes ?? {}}
>
  <Avatar {account} size={28} shape="workspace" />
  <!-- `band-trim-kids`, not `band-trim-stack`: the stack trims a block's OUTER
       edges so it sits on the avatar beside it and leaves the inner ones alone,
       which left the kicker its 1.3px under the baseline and the name its 6.3px
       above the cap - a 4.8px gap drawing as 12.4. Both rows want both edges.
       The two cannot be worn together: `band-trim-stack > :first-child` is the
       more specific rule and would win on the first row alone. -->
  <span class="target-trigger-copy band-trim-kids">
    <span class="target-kicker">Workspace</span>
    <strong>{account.display_name}</strong>
  </span>
  <span class="menu-chevron" aria-hidden="true">
    <Icon name="chevrons-up-down" size={14} strokeWidth={2} />
  </span>
  <SidebarTooltip text="Switch workspace" />
</button>

<style>
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
    /* Nothing on top: the row above owns the space under the mark. */
    margin: 0 0 var(--space-3);
    min-height: 3.25rem;
    padding: var(--space-2) 0.625rem;
    position: relative;
    width: 100%;
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

  .target-trigger[aria-expanded='true'] {
    border-color: color-mix(in srgb, var(--focus) 55%, var(--switcher-card-border));
  }

  .target-trigger:active {
    background: var(--sidebar-item-pressed);
    box-shadow: none;
  }

  .target-trigger-copy {
    display: grid;
    /* Cap band to cap band, because both rows are trimmed through below. The
       declared value used to be 0.3rem and the gap somebody saw was 12.4px. */
    gap: 0.25rem;
    min-width: 0;
    text-align: left;
  }

  .target-kicker {
    color: var(--sidebar-text-muted);
    font: 700 0.625rem / 1 var(--sans);
    letter-spacing: 0.11em;
    text-transform: uppercase;
  }

  /* Out of the rail and into the top bar, for the reason in the note above: left in
     `IdentityBar`, these lose to this component's own scoped rules and never apply. */
  @media (max-width: 48rem) {
    .target-trigger {
      background: transparent;
      border: 0;
      box-shadow: none;
      display: flex;
      margin: 0;
      min-height: var(--bar-control);
      padding: 0;
      position: absolute;
      right: var(--bar-slot-switcher);
      top: calc((var(--bar-height) - var(--bar-control)) / 2);
      /* Sized by its contents rather than by the rail it no longer sits in. */
      width: auto;
    }

    .target-trigger-copy {
      display: none;
    }
  }
</style>
