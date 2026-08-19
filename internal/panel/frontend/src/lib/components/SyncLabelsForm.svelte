<script lang="ts">
  /**
   * The labels an installation expects its repositories to carry.
   *
   * Labels were the first kind to sync and the last to get a form. The view
   * above listed them and offered a switch, so the only way to put a label into
   * an installation was the API the panel itself calls - which is how an
   * organization came to be told its labels were synced from a page that could
   * not name one.
   *
   * They travel as typed fields rather than as one document, unlike every kind
   * after them. That is not a shape worth copying; it is the shape the stored
   * row already has.
   */
  import { untrack } from 'svelte';

  import { patchedAt, rowKeys, withoutAt } from '#lib/form-lists.js';
  import { canonicalStringify } from '#lib/preferences-sync.js';
  import type { SyncLabel } from '#lib/types.js';

  import Button from './Button.svelte';
  import Plate from './Plate.svelte';
  import RemovalPolicy from './RemovalPolicy.svelte';
  import SyncKindHead from './SyncKindHead.svelte';

  const {
    labels,
    allowRemoval,
    excludes: excluded,
    enabled,
    unreadable,
    unavailable = '',
    problem = null,
    readOnly,
    saving,
    onSave,
  }: {
    labels: readonly SyncLabel[];
    allowRemoval: boolean;
    excludes: readonly string[];
    enabled: boolean;
    unreadable: boolean;
    unavailable?: string;
    problem?: string | null;
    readOnly: boolean;
    saving: boolean;
    onSave: (
      enabled: boolean,
      labels: SyncLabel[],
      allowRemoval: boolean,
      excludes: string[],
    ) => void;
  } = $props();

  /* What the form was given, with the description rule already applied, so a row
     that arrives carrying an empty one is held exactly like a row this form
     emptied. Both the draft and the saved side below are read from this: healing
     one on the way in is not somebody having edited it, and comparing a healed
     draft against an unhealed saved side would put Save live the moment the page
     opened. */
  const arriving = $derived<SyncLabel[]>(
    labels.map((label) => described(label, label.description)),
  );

  /* Written over as somebody edits, and reseeded when what is SAVED changes -
     never on every render. The switch beside the heading writes at once, like
     every other kind's, and a draft derived straight from the props would throw
     away whatever somebody had typed at the moment they flipped it. */
  let drafts = $state<SyncLabel[]>([]);
  let removal = $state(false);
  let excludes = $state<string[]>([]);

  const disabled = $derived(saving || readOnly || unreadable);

  /* Two label sets that would be saved the same way compare the same way. The
     saved side is rendered from the props rather than from the draft's own
     starting value, so a save landing from another tab settles the button. */
  const untouched = $derived(
    canonicalStringify({ labels: arriving, allow_removal: allowRemoval, excludes: excluded }),
  );
  const changed = $derived(
    canonicalStringify({ labels: drafts, allow_removal: removal, excludes }) !== untouched,
  );

  function patch(index: number, change: Partial<SyncLabel>): void {
    drafts = patchedAt(drafts, index, change);
  }

  /**
   * A description is either absent or has something in it. There is no third
   * state here, and this is the one place that decides so.
   *
   * The stored shape does have three: no key, a key with text, and a key with
   * an empty string, which asks every repository to have its description
   * cleared. A text box has two, so one of the three has to give, and it is the
   * clearing - nobody has asked to empty a description across an organization,
   * and the state a box cannot show should be the one nothing needs.
   *
   * Letting an emptied box mean clearing was the alternative, and it is worse in
   * both directions. A description typed and then thought better of would go out
   * as an instruction to wipe that label's description everywhere. And a row
   * that arrived already carrying an empty string would render as an empty box,
   * identical to one that leaves each repository alone, with no way to tell and
   * no way to fix it: an empty box that is left empty fires no change event, so
   * nothing this form does could ever take the key back off, and every save
   * would carry that standing instruction along with it.
   *
   * So this runs where a row arrives as well as where one is edited. Applied by
   * copying the row and deleting from the copy rather than by naming the keys to
   * keep, which would drop whatever a later version of SyncLabel adds beside
   * them; and not through patchedAt, which merges and so cannot take a key off.
   */
  function described(label: SyncLabel, value: string | undefined): SyncLabel {
    const next = { ...label };

    if (value === undefined || value === '') {
      delete next.description;
    } else {
      next.description = value;
    }

    return next;
  }

  function describe(index: number, value: string): void {
    drafts = drafts.map((label, at) => (at === index ? described(label, value) : label));
  }

  /**
   * Which row is being edited, as its position, kept in step with the list.
   *
   * One at a time, because a list of twelve labels opened as twelve forms is a
   * page nobody reads - the approved design shows the list and lets one row
   * become a form in place.
   *
   * This said "by key rather than by index" and held `rowKey(index)`, which is
   * the string `label-${index}` - the index with a prefix. So the hazard the
   * sentence described was the behaviour: removing a row ABOVE an open one left
   * the editor pointing at whatever moved up into its place, and Cancel then
   * wrote the opened row's values over that one, destroying a label nobody had
   * touched. A position is the honest thing to hold here, as long as removing a
   * row before it moves it, which `remove` now does.
   */
  let editing = $state<number | null>(null);

  /** What the row held when it was opened, so Cancel has something to go back to. */
  let opened = $state<SyncLabel | null>(null);

  function open(index: number): void {
    editing = index;
    opened = drafts[index] ?? null;
  }

  function cancel(index: number): void {
    const before = opened;
    // A row added and then cancelled was never a label: it goes away rather
    // than staying behind as a nameless row somebody has to notice and remove.
    if (before === null || before.name === '') drafts = withoutAt(drafts, index);
    else drafts = drafts.map((label, at) => (at === index ? before : label));
    editing = null;
    opened = null;
  }

  function add(): void {
    drafts = [...drafts, { name: '', color: '' }];
    open(drafts.length - 1);
  }

  function remove(index: number): void {
    if (editing === index) {
      editing = null;
      opened = null;
    } else if (editing !== null && index < editing) {
      // The open row just moved up one. Left alone, the editor would be
      // attached to its neighbour and Cancel would overwrite that neighbour.
      editing -= 1;
    }
    drafts = withoutAt(drafts, index);
  }

  /**
   * What the swatch shows, or nothing while the colour is not six hexadecimal
   * digits.
   *
   * Rendered here rather than trusted to CSS, because a half-typed colour is a
   * value the browser silently discards - so the swatch would go on showing the
   * last one that parsed, which reads as the field having been accepted.
   */
  function swatch(color: string): string {
    return /^[0-9a-fA-F]{6}$/.test(color) ? `#${color}` : 'transparent';
  }

  const rowKey = rowKeys('label');

  /* One effect, because a save landing is one thing happening.
     ---------------------------------------------------------
     The drafts are reseeded from what is now SAVED - never on every render, or
     a draft would be thrown away the moment somebody flipped the switch beside
     the heading, which writes at once like every other kind's.

     And the open editor closes with them: the whole list is reseeded, so a row
     left open would be showing a copy nothing writes to.

     It ran as two effects on the same trigger, which is two chances for the
     halves of one act to come apart. */
  $effect(() => {
    void untouched;
    untrack(() => {
      drafts = arriving;
      removal = allowRemoval;
      excludes = [...excluded];
      editing = null;
      opened = null;
    });
  });
</script>

<SyncKindHead
  title="Labels"
  lead="The labels every repository in this installation should carry. Smyklot works out what would change and asks before changing anything"
  noun="labels"
  {enabled}
  {unreadable}
  {unavailable}
  {problem}
  {readOnly}
  {saving}
  onToggle={(next) => onSave(next, arriving, allowRemoval, [...excluded])}
/>

<Plate label="{drafts.length} {drafts.length === 1 ? 'label' : 'labels'}">
  {#snippet status()}
    {#if !readOnly}
      <Button tone="quiet" {disabled} onclick={add}>Add a label</Button>
    {/if}
  {/snippet}

  {#if drafts.length === 0}
    <p class="form-note empty-note labels-empty">No labels yet.</p>
  {:else}
    <ul class="label-rows">
      {#each drafts as label, index (rowKey(index))}
        <li class="label-row" class:is-editing={editing === index}>
          <!-- The colour is the label's own, so it goes through a custom
               property: the panel serves style-src 'self', under which a style
               attribute is parsed and then discarded. -->
          <span class="label-swatch" style:--swatch={swatch(label.color)} aria-hidden="true"></span>

          {#if editing === index}
            <span class="label-edit">
              <input
                class="text-inline"
                type="text"
                value={label.name}
                {disabled}
                placeholder="kind/bug"
                aria-label="Label name"
                oninput={(event) => patch(index, { name: event.currentTarget.value })}
              />
              <input
                class="text-inline is-wide"
                type="text"
                value={label.description ?? ''}
                {disabled}
                placeholder="Something isn't working"
                aria-label="Label description"
                aria-describedby="labels-description-note"
                oninput={(event) => describe(index, event.currentTarget.value)}
              />
              <input
                class="text-inline is-color"
                type="text"
                value={label.color}
                {disabled}
                spellcheck="false"
                placeholder="d73a4a"
                aria-label="Label colour"
                oninput={(event) => patch(index, { color: event.currentTarget.value })}
              />
              <Button tone="brand" {disabled} onclick={() => (editing = null)}>Done</Button>
              <Button tone="quiet" {disabled} onclick={() => cancel(index)}>Cancel</Button>
            </span>
          {:else}
            <span class="label-name">{label.name || 'Unnamed'}</span>
            <span class="label-desc">{label.description ?? ''}</span>
            {#if !readOnly}
              <span class="label-acts">
                <Button tone="quiet" {disabled} onclick={() => open(index)}>Edit</Button>
                <Button tone="stop-quiet" {disabled} onclick={() => remove(index)}>Remove</Button>
              </span>
            {/if}
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  <p class="form-note label-note" id="labels-description-note">
    Six hexadecimal digits for the colour, with no <code>#</code>, which is how GitHub stores it. An
    empty description leaves whatever each repository wrote, so a label is given one here only where
    every repository should read the same.
  </p>

  {#if !readOnly}
    <!-- The one kind whose controls are free text, so this one waits for a
         Save: a name written a character at a time would otherwise be sent, and
         planned against, on every keystroke. -->
    <div class="form-actions">
      <Button
        tone="signal"
        disabled={disabled || !changed}
        onclick={() => onSave(enabled, drafts, removal, excludes)}
      >
        {saving ? 'Saving' : 'Save labels'}
      </Button>
      {#if changed}
        <span class="form-note">Unsaved changes</span>
      {/if}
    </div>
  {/if}
</Plate>

<RemovalPolicy
  noun="labels"
  {removal}
  {excludes}
  {disabled}
  onRemovalChange={(next) => (removal = next)}
  onExcludesChange={(next) => (excludes = next)}
/>

<style>
  /* The global rule has no margin. These notes sit directly under the control
     they describe, and the sliver of side inset lines them up with the field's
     own text. */
  .label-note {
    margin: 0.25rem 0.125rem 0;
  }

  .labels-empty {
    margin: var(--space-4) 0 0;
  }

  /*
   * The list, as the approved design draws it: a row per label, hairlines
   * between, and one row that becomes a form in place.
   *
   * `baseline` rather than `center`, because the three things on a row are all
   * text and a reader lines text up by its feet. The swatch is sized in `cap`
   * and sits on that same baseline, so it spans exactly the band the letters do
   * with nothing to compute - and a row whose description wraps keeps the mark
   * beside the first line rather than floating it beside the gap.
   */
  .label-rows {
    display: grid;
    list-style: none;
    margin: var(--space-3) 0 0;
    padding: 0;
  }

  .label-row {
    align-items: baseline;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto 11rem 1fr auto;
    min-block-size: 2.6rem;
    padding: 0.4rem var(--space-2);
  }

  .label-row + .label-row {
    border-top: 1px solid var(--border-subtle);
  }

  /* The editor takes the whole row, so the columns collapse to the mark and it. */
  .label-row.is-editing {
    align-items: center;
    grid-template-columns: auto 1fr;
  }

  .label-edit {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .label-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
  }

  .label-desc {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
  }

  /* Kept in the layout at all times and revealed on hover, so a row never
     changes size under the hand that is reaching for it. */
  .label-acts {
    display: flex;
    gap: var(--space-1);
    opacity: 0;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .label-row:hover .label-acts,
  .label-row:focus-within .label-acts {
    opacity: 1;
  }

  /* Sized in the label's own cap and sitting on the shared baseline, so the mark
     spans exactly the band the letters do. */
  .label-swatch {
    background: var(--swatch);
    block-size: 1cap;
    border-radius: 50%;
    box-shadow: inset 0 0 0 1px var(--rule);
    inline-size: 1cap;
  }

  /* The inline fields the editing row holds. Narrower than a full control and
     laid beside each other, because the row is a row and not a form. */
  .text-inline {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-size: var(--font-size-control);
    min-block-size: var(--control-height-compact);
    padding-inline: 0.55rem;
    width: 11rem;
  }

  .text-inline.is-wide {
    flex: 1;
    min-width: 14rem;
    width: auto;
  }

  .text-inline.is-color {
    font-family: var(--mono);
    width: 6.5rem;
  }

  @media (max-width: 44rem) {
    /* The name and its description stack, so neither is squeezed to a column
       too narrow to read. */
    .label-row {
      grid-template-columns: auto 1fr;
    }

    .label-row .label-desc,
    .label-row .label-acts {
      grid-column: 2;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .label-acts {
      transition: none;
    }
  }
</style>
