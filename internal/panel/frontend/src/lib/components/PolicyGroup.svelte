<script lang="ts">
  /**
   * A card of settings this installation decides, and one line about the ones
   * it does not.
   *
   * The page is the policy. A settings page that lists every switch GitHub has
   * makes a reader work out which of forty rows this installation actually
   * decides, and the answer - usually nine - is the only thing on the page
   * worth knowing. So the managed ones are rows, and the rest is a sentence
   * that names them: enough scent to answer "is X managed?" without turning
   * the page back into a form.
   *
   * The tally in the head is the same fact as a number, for a reader who came
   * to compare groups rather than read one.
   *
   * The card itself is a `Plate`, like every other card in the panel - a
   * settings group is not a new kind of surface, only a new thing to put on
   * one.
   */
  import Plate from '#lib/components/Plate.svelte';

  const {
    name,
    note,
    tallyWord = 'managed',
    restSay,
    managed,
    total,
    unmanaged = [],
    onManage,
    picking = false,
    children,
    picker,
  }: {
    name: string;
    /**
     * How many of this group's things are managed, and how many there are.
     * Both absent where the group is not a choice of some out of many - a
     * tally reading "1 of 1 managed" is a number that answers nothing.
     */
    managed?: number;
    total?: number;
    /** What the tally counts, where it is not settings - "rules on". */
    tallyWord?: string;
    /**
     * What the group as a whole does, where its rows cannot say it themselves -
     * a rule about when a value is withheld belongs to all of them at once.
     * Omitted where each row already carries its own sentence.
     */
    note?: string;
    /**
     * What the unmanaged rest is, in the caller's own words - "13 rules are
     * off". Defaults to the settings sentence, which is the one place the
     * absence of a value means "follows the repository".
     */
    restSay?: string;
    /** The names of the rest, said in the line under the rows. */
    unmanaged?: readonly string[];
    /** Opens the picker in place. Omitted once every setting is managed. */
    onManage?: () => void;
    /**
     * Whether the picker is open. A prop rather than "was a picker snippet
     * given", because a snippet is passed once and rendered many times: the
     * caller passes one for every group and only one of them is open.
     */
    picking?: boolean;
    /** The managed rows - `PolicyRow`s. Absent while nothing here is managed. */
    children?: import('svelte').Snippet;
    /** The add-picker, once it is open, rendered where the Manage press was. */
    picker?: import('svelte').Snippet;
  } = $props();

  const rest = $derived(total === undefined ? 0 : total - (managed ?? 0));
</script>

<Plate label={name}>
  {#snippet status()}
    {#if total !== undefined}
      <span class="group-tally">{managed ?? 0} of {total} {tallyWord}</span>
    {/if}
  {/snippet}

  {#if note !== undefined}
    <p class="group-note">{note}</p>
  {/if}

  {#if children !== undefined}
    <div class="policy-rows">
      {@render children()}
    </div>
  {/if}

  {#if rest > 0}
    <div class="group-rest">
      <!-- The names are the scent that answers "is X managed?" without turning
           the page back into a form. Once the picker is open they are on the
           chips themselves, so the sentence says what the chips are for
           instead of naming them twice. -->
      <span class="rest-say">
        <span class="rest-count"
          >{restSay ?? `${rest} follow${rest === 1 ? 's' : ''} each repository`}</span
        >{#if picking}&nbsp;— pick one to manage:{:else if unmanaged.length > 0}&nbsp;—
          {unmanaged.join(', ')}{/if}
      </span>
      {#if picking && picker !== undefined}
        {@render picker()}
      {:else if onManage !== undefined}
        <button type="button" class="manage" onclick={onManage}>Manage</button>
      {/if}
    </div>
  {/if}
</Plate>

<style>
  .group-tally {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    text-box: trim-both cap alphabetic;
  }

  .group-note {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: 0 0 var(--space-2);
    max-width: 68ch;
  }

  .policy-rows {
    display: grid;
  }

  /* Every row but the first is separated from the one above it. Written on the
     container so a row knows nothing about its neighbours. */
  .policy-rows > :global(.policy-row + .policy-row) {
    border-top: 1px solid var(--border-subtle);
  }

  .group-rest {
    align-items: center;
    border-top: 1px solid var(--border-subtle);
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-top: var(--space-2);
    padding-top: var(--space-3);
  }

  .rest-say {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .rest-count {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .manage {
    appearance: none;
    background: none;
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--brand-action-text);
    cursor: pointer;
    font: inherit;
    font-size: var(--font-size-compact);
    font-weight: 500;
    padding: 0.35rem 0.5rem;
    white-space: nowrap;
  }

  .manage:hover {
    background-image: linear-gradient(
      var(--interactive-hover-layer),
      var(--interactive-hover-layer)
    );
  }

  @media (max-width: 40rem) {
    .group-rest {
      align-items: start;
      flex-direction: column;
    }
  }
</style>
