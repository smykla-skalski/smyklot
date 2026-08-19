<script lang="ts" generics="Key extends string">
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
  import Button from '#lib/components/Button.svelte';
  import Icon from '#lib/components/Icon.svelte';
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
    onPick,
    onCancel,
    disabled = false,
    picking = false,
    children,
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
    /**
     * The rest: named in the line under the rows, and offered as chips once the
     * picker is open. Both, from one list, because they are the same set said
     * twice and a group that named one set and offered another would be a bug
     * nothing reports.
     */
    unmanaged?: readonly { key: Key; label: string }[];
    /** Opens the picker in place. Omitted once every setting is managed. */
    onManage?: () => void;
    /**
     * A save is in flight. The chips alone take it: `onManage` is already
     * withheld while it is true, so this is for a picker that was open when the
     * save started - Cancel stays live, because dismissing is not a write.
     */
    disabled?: boolean;
    /** One of `unmanaged` was picked. Closing the picker is the caller's. */
    onPick?: (key: Key) => void;
    /** The picker was dismissed without picking. */
    onCancel?: () => void;
    /**
     * Whether the picker is open. The caller's, not this component's: settings
     * open one picker at a time across every group, which is a fact no single
     * group can hold.
     */
    picking?: boolean;
    /** The managed rows - `PolicyRow`s. Absent while nothing here is managed. */
    children?: import('svelte').Snippet;
  } = $props();

  const rest = $derived(total === undefined ? 0 : total - (managed ?? 0));
</script>

<Plate label={name}>
  {#snippet status()}
    {#if total !== undefined}
      <span class="group-tally band-trim">{managed ?? 0} of {total} {tallyWord}</span>
    {/if}
  {/snippet}

  {#if note !== undefined}
    <p class="group-note">{note}</p>
  {/if}

  {#if children !== undefined}
    <!-- `.ruled-rows` rather than a border of its own: it draws the seam as a
         pseudo-element inset from the edges and clears it under the pointer,
         which a `border-top` cannot do - and a policy row has a hover ground,
         so the line was drawn across it. -->
    <div class="policy-rows ruled-rows">
      {@render children()}
    </div>
  {/if}

  {#if rest > 0}
    <div class="group-rest">
      <!-- The names are the scent that answers "is X managed?" without turning
           the page back into a form. Once the picker is open they are on the
           chips themselves, so the sentence says what the chips are for
           instead of naming them twice. -->
      <span class="rest-say band-trim">
        <span class="rest-count"
          >{restSay ?? `${rest} follow${rest === 1 ? 's' : ''} each repository`}</span
        >{#if picking}&nbsp;— pick one to manage:{:else if unmanaged.length > 0}&nbsp;—
          {unmanaged.map((one) => one.label).join(', ')}{/if}
      </span>
      {#if picking}
        <span class="rest-picks">
          {#each unmanaged as one (one.key)}
            <button type="button" class="add-chip" {disabled} onclick={() => onPick?.(one.key)}>
              <Icon name="plus" size={11} strokeWidth={2} />
              <span class="cap-trim">{one.label}</span>
            </button>
          {/each}
          <Button tone="quiet" onclick={onCancel}>Cancel</Button>
        </span>
      {:else if onManage !== undefined}
        <Button tone="quiet" class="manage" onclick={onManage}>Manage</Button>
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

  .group-rest {
    align-items: center;
    border-top: 1px solid var(--border-subtle);
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-top: var(--space-2);
    padding-top: var(--space-3);
  }

  /* `.band-trim` in the markup, because what stands beside it is a control and
     a control is a box: `align-items: center` puts the two boxes on one centre,
     and an untrimmed line's box is not its band - the sentence sat 0.47px off
     the button once the button became a real one. */
  .rest-say {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .rest-count {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .rest-picks {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  /* `Button` draws the control - the ground, the press, the hover overlay, and
     the `.button-label` that puts the word on its own cap height, which eleven
     hand-rolled declarations here did not. What is left is the one thing no
     tone offers: quiet, in the action colour rather than in the dim one. */
  /* `:global`, and anchored through the row, because a class handed to a child
     component is not in this component's scope - Svelte scopes the markup it
     renders itself, and the `<button>` is `Button`'s. */
  .group-rest :global(.manage),
  .group-rest :global(.manage:hover:not(:disabled)) {
    color: var(--brand-action-text);
  }

  @media (max-width: 40rem) {
    .group-rest {
      align-items: start;
      flex-direction: column;
    }
  }
</style>
