<script lang="ts">
  import { plainClick } from '#lib/follow.js';
  import Icon from './Icon.svelte';

  /**
   * The way back from a page that stands inside a list.
   *
   * One of these, because there were three and they agreed on nothing: the
   * console's was uppercase micro type in the action ink, the queue's was
   * sentence-case meta type in `--text-soft` with a press, and the repository's
   * was sentence-case meta type in `--text-secondary` with neither. Three ways
   * of drawing the same idea on three pages a reader moves between.
   *
   * Then a fourth arrived as `Crumb`, for the sync detail pages, and it is the
   * `quiet` tone below. It is a real difference rather than another accident:
   * those two pages sit under a tab strip that is already loud, so the way back
   * is said in the page's own secondary ink instead of competing with it. What
   * it must NOT be is a second component - it carried the sentence about
   * keeping the href real for a modified click and none of the guard that makes
   * that true, so a Cmd-click on a crumb opened nothing.
   *
   * A link, never a button: it carries a real address, so it opens in a new tab
   * on a modified click and reads as somewhere to go. `onNavigate` is only for
   * the surfaces that want to handle it in the panel; `plainClick` leaves every
   * other click to the browser.
   */

  const {
    href,
    label,
    tone = 'label',
    onNavigate,
  }: {
    href: string;
    label: string;
    /**
     * `label` is the console's: capitals, tracked out, in the action ink, so it
     * reads as a label on the page rather than a sentence someone started.
     * `quiet` is the sync detail pages': sentence case in secondary ink, under
     * a tab strip that is already carrying the emphasis.
     */
    tone?: 'label' | 'quiet';
    /** Handles a plain click in the panel. Omit to let the router do it. */
    onNavigate?: () => void;
  } = $props();

  function follow(event: MouseEvent): void {
    if (onNavigate === undefined || !plainClick(event)) return;
    event.preventDefault();
    onNavigate();
  }
</script>

<a class="back-link" class:is-quiet={tone === 'quiet'} {href} onclick={follow}>
  <Icon name="chevron-left" size={tone === 'quiet' ? 12 : 14} />
  <!-- Trimmed to the cap band, which is what makes the row centre natively: the
       chevron's path is centred in its own box, so a box-centring flex row puts
       its ink on the middle of the capitals rather than on the middle of a line
       box that carries leading above them and descender room below. -->
  <span class="cap-trim">{label}</span>
</a>

<style>
  .back-link {
    align-items: center;
    align-self: start;
    color: var(--brand-action-text);
    display: inline-flex;
    font: 700 var(--font-size-micro) / 1 var(--sans);
    gap: var(--space-1);
    letter-spacing: 0.08em;
    margin-bottom: var(--space-3);
    text-decoration: none;
    text-transform: uppercase;
    transition: color var(--duration-fast) var(--ease-standard);
    width: fit-content;
  }

  /* Everything the loud one declares, said again as itself rather than unset:
     a tone that only removes is a tone that inherits whatever the other one
     grows next. */
  .back-link.is-quiet {
    color: var(--text-secondary);
    font: 400 var(--font-size-meta) / 1 var(--sans);
    gap: 0.4rem;
    letter-spacing: normal;
    margin-bottom: 0;
    text-transform: none;
  }

  .back-link:hover {
    color: var(--text-primary);
  }

  .back-link.is-quiet:focus-visible {
    border-radius: 4px;
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }

  /* Leans the way it points, which is the queue's own press and worth keeping:
     the origin is the left edge so the chevron stays put and the word moves. */
  .back-link:active {
    transform: scale(var(--press-scale-compact));
    transform-origin: left center;
  }
</style>
