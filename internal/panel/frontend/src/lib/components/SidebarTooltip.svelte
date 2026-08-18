<script lang="ts">
  /**
   * The label that appears beside a sidebar control once the rail is collapsed.
   *
   * Three triggers carried this by hand - the collapse button, the workspace switcher
   * and the account row - each with its own copy of the same span. The word is the
   * only thing that differed.
   *
   * **Not `AppTooltip`, and this is the reason.** That one is a bits-ui layer: it
   * portals, it positions itself against the viewport, and it opens on a delay. This
   * is a span inside its trigger, positioned against the rail and shown by a CSS hover
   * on an ancestor. On a 56px rail the two behave differently in the way that matters -
   * a portalled layer has to be told where the rail ends, and it gets that wrong the
   * moment the sidebar animates.
   *
   * It is hidden with `visibility` AND `opacity`, not `display`: the transition needs
   * something to interpolate, and `pointer-events: none` alone would leave it in the
   * hit-testing tree while invisible.
   *
   * **Which triggers show it lives with the parent**, because it is the parent that
   * knows when the rail is collapsed and which of its rows is hovered. Those rules are
   * `:global` and anchored, in `IdentityBar`.
   */
  const { text }: { text: string } = $props();
</script>

<span class="sidebar-tooltip">{text}</span>

<style>
  .sidebar-tooltip {
    background: var(--sidebar-popover-bg);
    border: 1px solid var(--sidebar-popover-border);
    border-radius: var(--radius-control);
    box-shadow: var(--shadow-popover);
    color: var(--sidebar-menu-text);
    font-size: var(--font-size-meta);
    font-weight: 500;
    /* Clear of the sidebar rather than of the row it belongs to: the row stops one
       padding inside the sidebar, so the same air on the outside is that padding, the
       border, and one more. The collapsed rail pads by --space-2, which is the padding
       this has to match. */
    left: calc(100% + var(--space-2) * 2 + 1px);
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
</style>
