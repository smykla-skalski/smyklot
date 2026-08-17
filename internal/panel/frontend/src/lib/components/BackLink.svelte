<script lang="ts">
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
   * The console's is the one kept - small, set in capitals, and tracked out - so
   * it reads as a label on the page rather than as a sentence someone started.
   *
   * A link, never a button: it carries a real address, so it opens in a new tab
   * on a modified click and reads as somewhere to go. `onNavigate` is only for
   * the surfaces that want to handle it in the panel; a modified click is left
   * to the browser.
   */

  const {
    href,
    label,
    onNavigate,
  }: {
    href: string;
    label: string;
    /** Handles a plain click in the panel. Omit to let the router do it. */
    onNavigate?: () => void;
  } = $props();

  function follow(event: MouseEvent): void {
    if (onNavigate === undefined) return;
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }
    event.preventDefault();
    onNavigate();
  }
</script>

<a class="back-link" {href} onclick={follow}>
  <Icon name="chevron-left" size={14} />
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

  .back-link:hover {
    color: var(--text-primary);
  }

  /* Leans the way it points, which is the queue's own press and worth keeping:
     the origin is the left edge so the chevron stays put and the word moves. */
  .back-link:active {
    transform: scale(var(--press-scale-compact));
    transform-origin: left center;
  }
</style>
