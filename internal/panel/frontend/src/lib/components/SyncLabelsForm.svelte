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
  import PatternList from './PatternList.svelte';
  import Plate from './Plate.svelte';
  import PolicyRow from './PolicyRow.svelte';
  import Switch from './Switch.svelte';
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

  $effect(() => {
    void untouched;
    untrack(() => {
      drafts = arriving;
      removal = allowRemoval;
      excludes = [...excluded];
    });
  });

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

  <div class="label-settings">
    <PolicyRow
      name="Remove labels this list does not name"
      why="Off, a repository may keep labels of its own. On, everything unnamed is deleted"
      value={removal ? 'On' : 'Off'}
    >
      {#snippet control()}
        <!-- The one control here that destroys something. A label this list
             does not name goes on existing unless this is on. -->
        <Switch
          checked={removal}
          ariaLabel="Remove labels this list does not name"
          {disabled}
          onChange={(next) => (removal = next)}
        />
      {/snippet}
    </PolicyRow>

    <PolicyRow
      name="Labels to leave alone"
      why="Name or pattern, where * stands for any run of characters. Neither written nor removed, whatever the list below says"
    >
      {#snippet control()}
        <!-- The safety valve beside the switch above, and the reason it is here
             rather than only in the API: somebody who can turn removal on from
             this page has to be able to protect something from this page too. -->
        <PatternList
          values={excludes}
          label="Labels to leave alone"
          addLabel="Add a pattern"
          placeholder="hand-made-*"
          {disabled}
          onChange={(next) => (excludes = next)}
        />
      {/snippet}
    </PolicyRow>
  </div>

  {#if drafts.length === 0}
    <p class="form-note labels-empty">No labels yet.</p>
  {/if}

  {#each drafts as label, index (rowKey(index))}
    <article class="entry-card">
      <div class="label-row">
        <label class="label-name">
          <span class="entry-field-label">Name</span>
          <input
            class="text-input"
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
              class="text-input"
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
          class="text-input"
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
        <p class="form-note">Nothing is changed on GitHub until a plan is approved</p>
      {/if}
    </div>
  {/if}
</Plate>

<style>
  .label-settings {
    display: grid;
    margin-bottom: var(--space-3);
  }

  .label-settings > :global(.policy-row + .policy-row) {
    border-top: 1px solid var(--border-subtle);
  }

  /* The global rule has no margin. These notes sit directly under the control
     they describe, and the sliver of side inset lines them up with the field's
     own text. */
  .label-note {
    margin: 0.25rem 0.125rem 0;
  }

  .labels-empty {
    margin: var(--space-4) 0 0;
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
