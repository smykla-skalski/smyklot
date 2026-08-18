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
  import { asList, lines, patchedAt, rowKeys, withoutAt } from '#lib/form-lists.js';
  import { OFF, ON, SWITCH } from '#lib/form-switch.js';
  import { canonicalStringify } from '#lib/preferences-sync.js';
  import type { SyncLabel } from '#lib/types.js';

  import SegmentedControl from './SegmentedControl.svelte';
  import SyncDocumentForm from './SyncDocumentForm.svelte';

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

  /* Written over as somebody edits, and reseeded by a save landing from
     anywhere rather than left describing a document that is gone. */
  let drafts = $derived<SyncLabel[]>(arriving);
  let removal = $derived(allowRemoval);
  let excludes = $derived<string[]>([...excluded]);
  let wanted = $derived(enabled);

  const disabled = $derived(saving || readOnly || unreadable);

  /* Two label sets that would be saved the same way compare the same way. The
     saved side is rendered from the props rather than from the draft's own
     starting value, so a save landing from another tab settles the button. */
  const untouched = $derived(
    canonicalStringify({ labels: arriving, allow_removal: allowRemoval, excludes: excluded }),
  );
  const changed = $derived(
    wanted !== enabled ||
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

  function add(): void {
    drafts = [...drafts, { name: '', color: '' }];
  }

  function remove(index: number): void {
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
</script>

<SyncDocumentForm
  heading="Labels"
  noun="labels"
  lead="The labels every repository in this installation should carry. Smyklot works out what would
        change and asks before changing anything"
  enabled={wanted}
  {unreadable}
  {unavailable}
  {problem}
  {readOnly}
  {saving}
  {changed}
  {disabled}
  onToggle={(value) => (wanted = value)}
  onSave={() => onSave(wanted, drafts, removal, excludes)}
>
  {#snippet actions()}
    <!-- Wrapped, like every bare word in a button here: a button is a flex
         container, so its text sits in an anonymous box no selector reaches and
         the `text-box` trim never touches it. See `.button-label` in app.css. -->
    <button class="btn btn-quiet" type="button" {disabled} onclick={add}>
      <span class="button-label">Add a label</span>
    </button>
  {/snippet}

  <!-- The one control here that destroys something. A label this list does not
       name goes on existing unless this is on, and turning it on proposes
       deleting every label a repository has that is not named below. -->
  <div class="label-switch">
    <span class="sync-form-label">Remove labels this list does not name</span>
    <SegmentedControl
      name="labels-removal"
      label="Remove labels this list does not name"
      options={SWITCH}
      value={removal ? ON : OFF}
      {disabled}
      onSelect={(chosen) => (removal = chosen === ON)}
    />
  </div>

  <!-- The safety valve beside the switch above, and the reason it is here
       rather than only in the API: somebody who can turn removal on from this
       page has to be able to protect something from this page too. -->
  <label class="entry-field">
    <span class="entry-field-label">Labels to leave alone</span>
    <textarea
      rows="2"
      {disabled}
      aria-describedby="labels-excludes-note"
      value={lines(excludes)}
      placeholder="hand-made-*"
      onchange={(event) => (excludes = asList(event.currentTarget.value))}></textarea>
  </label>
  <p class="form-note label-note" id="labels-excludes-note">
    One name or pattern per line, where <code>*</code> stands for any run of characters. These are neither
    written nor removed, whatever the list below says.
  </p>

  {#if drafts.length === 0}
    <p class="form-note labels-empty">No labels yet.</p>
  {/if}

  {#each drafts as label, index (rowKey(index))}
    <article class="entry-card">
      <div class="label-row">
        <label class="label-name">
          <span class="entry-field-label">Name</span>
          <input
            type="text"
            value={label.name}
            {disabled}
            placeholder="kind/bug"
            onchange={(event) => patch(index, { name: event.currentTarget.value })}
          />
        </label>

        <label class="label-color">
          <span class="entry-field-label">Colour</span>
          <span class="label-color-field">
            <!-- The colour is the label's own, so it goes through a custom
                 property: the panel serves style-src 'self', under which a
                 style attribute is parsed and then discarded. -->
            <span class="label-swatch" style:--swatch={swatch(label.color)} aria-hidden="true"
            ></span>
            <input
              type="text"
              value={label.color}
              {disabled}
              spellcheck="false"
              placeholder="d73a4a"
              onchange={(event) => patch(index, { color: event.currentTarget.value })}
            />
          </span>
        </label>

        {#if !readOnly}
          <button class="btn btn-quiet" type="button" {disabled} onclick={() => remove(index)}>
            <span class="button-label">Remove</span>
          </button>
        {/if}
      </div>

      <label class="entry-field">
        <span class="entry-field-label">Description</span>
        <input
          type="text"
          value={label.description ?? ''}
          {disabled}
          aria-describedby="labels-description-note"
          placeholder="Something isn't working"
          onchange={(event) => describe(index, event.currentTarget.value)}
        />
      </label>
    </article>
  {/each}

  <p class="form-note label-note" id="labels-description-note">
    Six hexadecimal digits for the colour, with no <code>#</code>, which is how GitHub stores it. An
    empty description leaves whatever each repository wrote, so a label is given one here only where
    every repository should read the same.
  </p>
</SyncDocumentForm>

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

  .label-switch {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    justify-content: space-between;
  }

  .label-row {
    align-items: flex-end;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .label-name {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 12rem;
  }

  .label-color {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .label-color-field {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  .label-color input {
    width: 8rem;
  }

  /* A ring rather than a border, so a white label and a colour that will not
     parse read as two things rather than as one pale square. */
  .label-swatch {
    background: var(--swatch);
    block-size: 1rem;
    border-radius: 50%;
    box-shadow: inset 0 0 0 1px var(--rule);
    flex: none;
    inline-size: 1rem;
  }
</style>
