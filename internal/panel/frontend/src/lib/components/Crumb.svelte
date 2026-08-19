<script lang="ts">
  import { plainClick } from '#lib/follow.js';
  import Icon from './Icon.svelte';

  /**
   * The way back UP from a drill-down: one link to the parent route.
   *
   * Hierarchy, never history - the browser already owns Back, the crumb owns
   * Up, so it always targets the parent address rather than wherever the
   * reader happens to have come from. Only pages deeper than their section's
   * tab strip carry one, and the current page is never in it: at one level of
   * depth a trail would just restate the tab strip above it.
   */
  const {
    href,
    label,
    onNavigate,
  }: {
    href: string;
    /** The parent's own name - "Rulesets", "Files". */
    label: string;
    /**
     * SPA navigation; the href stays real for middle-click and copy.
     *
     * Which only holds because `plainClick` lets every other click through.
     * This carried the sentence and not the guard, so a Cmd-click was answered
     * with `preventDefault()` and the new tab never opened.
     */
    onNavigate?: () => void;
  } = $props();
</script>

<a
  class="crumb"
  {href}
  onclick={(event) => {
    if (onNavigate === undefined || !plainClick(event)) return;
    event.preventDefault();
    onNavigate();
  }}
>
  <Icon name="chevron-left" size={12} />
  <span class="cap-trim">{label}</span>
</a>

<style>
  .crumb {
    align-items: center;
    color: var(--text-secondary);
    display: inline-flex;
    gap: 0.4rem;
    font-size: var(--font-size-meta);
    text-decoration: none;
  }

  .crumb:hover {
    color: var(--text-primary);
  }

  .crumb:focus-visible {
    border-radius: 4px;
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
</style>
