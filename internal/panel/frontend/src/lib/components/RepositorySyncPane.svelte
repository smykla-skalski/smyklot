<script lang="ts">
  /**
   * What one repository says about the files the organization keeps in step.
   *
   * Two answers, and they are one row: whether the sync runs here at all, and
   * what this repository adjusts about it. A repository knows things the
   * template cannot - one of them ignores a directory the others do not - and
   * this is where that is written down.
   *
   * Against the repository rather than keyed by name in the installation's own
   * document, so a rename cannot orphan an adjustment. A file sync that quietly
   * stopped applying one would write the plain template over exactly the
   * customization it described.
   */
  import { canonicalStringify } from '#lib/preferences-sync.js';
  import { asList, lines, patchedAt, rowKeys, storedList, withoutAt } from '#lib/form-lists.js';
  import { formatRelative } from '#lib/format.js';
  import type { SyncFileMerge, SyncOverride } from '#lib/types.js';

  import InheritControl from './InheritControl.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const {
    stored,
    readOnly,
    saving,
    now,
    saveProblem = null,
    onSave,
  }: {
    stored: SyncOverride;
    readOnly: boolean;
    saving: boolean;
    /**
     * The list's clock, so a refusal can say how long ago it was found. Passed
     * rather than read here, because a second timer in a dialog the list
     * already ticks for would say a different thing on the same screen.
     */
    now: number;
    /**
     * Why the last save did not land, which is this dialog's own and belongs to
     * the moment. Not to be confused with `stored.problem`, which is why the
     * planner is not syncing this repository at all.
     */
    saveProblem?: string | null;
    onSave: (enabled: boolean | null, document: Record<string, unknown>) => void;
  } = $props();

  /** What a merge does to a structured template. Markdown has its own. */
  const STRATEGIES = [
    { value: '', label: 'By extension' },
    { value: 'deep-merge', label: 'Deep' },
    { value: 'shallow-merge', label: 'Shallow' },
  ] as const;

  const ENABLEMENT = [
    { value: 'enabled', label: 'Enabled' },
    { value: 'disabled', label: 'Disabled' },
  ] as const;

  /**
   * One adjustment as it is being edited.
   *
   * The overrides ride along as text rather than as a parsed value, because a
   * half-typed object is not an object and a form that reparsed on every
   * keystroke would blank the box the moment a brace was opened. Text and merge
   * travel together in one draft: two lists indexed in step are two lists that
   * every add, remove and edit has to keep in step, and the first one to forget
   * puts one repository's overrides on another repository's file.
   */
  type Draft = { merge: SyncFileMerge; text: string };

  /* Derived from what is saved and written over as somebody edits, so a save
     landing from anywhere reseeds it. */
  let drafts = $derived<Draft[]>(storedDrafts(stored.document));
  let excludes = $derived<string[]>(storedList<string>(stored.document, 'excludes'));
  let wanted = $derived<boolean | null>(stored.enabled);

  const disabled = $derived(saving || readOnly || stored.unreadable);

  /**
   * Why the planner is not syncing this repository at all, and how long ago it
   * found that. Null where nothing is wrong.
   *
   * Not every reason can be edited away here - a repository with no commits
   * has nowhere to propose against, whatever this form says - so it reads as a
   * standing notice rather than as a validation message on a field.
   */
  const notSyncing = $derived.by(() => {
    if (stored.problem === undefined || stored.problem === '') return null;

    return {
      reason: stored.problem,
      when: stored.problem_at === undefined ? null : formatRelative(stored.problem_at, now),
    };
  });

  /* Read once per draft rather than once per question. Both the refusal below
     and the payload need to know what a box says, and parsing it twice for
     that is parsing every adjustment twice on every keystroke. */
  const values = $derived(drafts.map((draft) => parsed(draft.text)));

  /** The first adjustment whose overrides are not JSON, or nothing. */
  const malformed = $derived(values.findIndex((value) => value === undefined));

  const payload = $derived(asDocument());

  /* Two documents that would be saved the same way have to compare the same
     way, whatever order their keys happen to be in. Comparing the raw text put
     Save live the moment the page loaded, for a document nobody had touched. */
  const untouched = $derived(canonicalStringify(stored.document ?? {}));
  const changed = $derived(
    wanted !== stored.enabled || canonicalStringify(payload) !== untouched || textsDiffer(),
  );

  /**
   * The whole document rather than the parts with controls, so a key a newer
   * version of the service wrote is sent back rather than dropped by a browser
   * running an older build of this page.
   *
   * The server refuses it by name - it decodes what it is sent strictly - which
   * is the point: after a rollback, somebody editing this pane is told that the
   * document holds something this version does not understand, rather than
   * saving over it and finding out when they roll forward again.
   *
   * An empty list is left out rather than written as an empty one, so a
   * repository that adjusts nothing says so in the one shape everything else
   * reads as nothing.
   */
  function asDocument(): Record<string, unknown> {
    const document: Record<string, unknown> = { ...stored.document };

    if (drafts.length > 0) {
      document.merges = drafts.map((draft, at) => withOverrides(draft, values[at]));
    } else {
      delete document.merges;
    }

    if (excludes.length > 0) {
      document.excludes = excludes;
    } else {
      delete document.excludes;
    }

    return document;
  }

  /**
   * Whether the boxes say something the saved document does not. Compared as
   * text rather than through the payload, because an unparsed box contributes
   * nothing to the payload and would otherwise read as no change at all.
   */
  function textsDiffer(): boolean {
    const saved = storedDrafts(stored.document);

    return (
      drafts.length !== saved.length || drafts.some((draft, at) => draft.text !== saved[at].text)
    );
  }

  function withOverrides(draft: Draft, value: Record<string, unknown> | undefined): SyncFileMerge {
    if (value !== undefined && Object.keys(value).length > 0) {
      return { ...draft.merge, overrides: value };
    }

    // An empty box sets nothing, which is the absence of the key rather than an
    // empty object: the two mean the same thing to the merge and only one of
    // them reads that way in the stored document.
    const rest = { ...draft.merge };
    delete rest.overrides;

    return rest;
  }

  function parsed(text: string): Record<string, unknown> | undefined {
    if (text.trim() === '') return {};

    try {
      const value: unknown = JSON.parse(text);

      return value !== null && typeof value === 'object' && !Array.isArray(value)
        ? (value as Record<string, unknown>)
        : undefined;
    } catch {
      return undefined;
    }
  }

  function storedDrafts(from: Record<string, unknown>): Draft[] {
    return storedList<SyncFileMerge>(from, 'merges').map((merge) => ({
      merge,
      text: merge.overrides === undefined ? '' : JSON.stringify(merge.overrides, null, 2),
    }));
  }

  function patch(index: number, change: Partial<SyncFileMerge>): void {
    drafts = patchedAt(drafts, index, {
      merge: { ...drafts[index].merge, ...change },
    });
  }

  function setText(index: number, text: string): void {
    drafts = patchedAt(drafts, index, { text });
  }

  function add(): void {
    drafts = [...drafts, { merge: { path: '' }, text: '' }];
  }

  function remove(index: number): void {
    drafts = withoutAt(drafts, index);
  }

  const rowKey = rowKeys('merge');

  function enablementValue(): string | null {
    if (wanted === null) return null;

    return wanted ? 'enabled' : 'disabled';
  }
</script>

<section class="sync-pane">
  <p class="form-lead">
    Whether the organization's files are kept in step here, and what this repository changes about
    them. Nothing reaches GitHub until a plan is approved
  </p>

  {#if saveProblem !== null}
    <p class="form-error" role="alert">{saveProblem}</p>
  {/if}

  <!-- What the planner made of this repository, which is the question somebody
       opening this pane came to ask. A refusal is fail-closed and correct, and
       before this the only account of it was a line in the service log. -->
  {#if notSyncing !== null}
    <p class="form-notice sync-pane-standdown" role="status">
      <strong>The organization's files are not being synced here</strong>
      <span>{notSyncing.reason}</span>
      {#if notSyncing.when !== null}
        <span class="sync-pane-standdown-when">Last looked at {notSyncing.when}</span>
      {/if}
    </p>
  {/if}

  {#if stored.unreadable}
    <p class="form-notice" role="alert">
      What this repository adjusts is stored in a form this version of Smyklot cannot read, so it is
      not shown and nothing here can be changed. Nothing has been lost
    </p>
  {/if}

  <div class="sync-pane-row">
    <span class="sync-pane-label">File sync</span>
    <span class="sync-pane-spacer"></span>
    <InheritControl
      label="File sync"
      source="the installation"
      sourcePronoun="it"
      inheritedLabel="whatever the installation says"
      value={enablementValue()}
      options={ENABLEMENT}
      {disabled}
      onSelect={(selection) => (wanted = selection === 'enabled')}
      onRestore={() => (wanted = null)}
    />
  </div>

  <label class="entry-field">
    <span class="entry-field-label">Files to leave alone here</span>
    <textarea
      rows="2"
      {disabled}
      aria-describedby="repository-sync-excludes-note"
      value={lines(excludes)}
      placeholder="renovate.json"
      onchange={(event) => (excludes = asList(event.currentTarget.value))}></textarea>
  </label>
  <p class="form-note" id="repository-sync-excludes-note">
    One path or pattern per line. These narrow what the installation synchronizes; they never widen
    it.
  </p>

  {#if drafts.length === 0}
    <p class="form-note">This repository takes every file as the organization writes it.</p>
  {/if}

  {#each drafts as draft, index (rowKey(index))}
    <article class="entry-card sync-merge">
      <div class="sync-pane-row">
        <label class="sync-merge-path">
          <span class="entry-field-label">File</span>
          <input
            type="text"
            value={draft.merge.path}
            {disabled}
            placeholder="renovate.json"
            onchange={(event) => patch(index, { path: event.currentTarget.value })}
          />
        </label>

        <SegmentedControl
          name="repository-sync-strategy-{index}"
          label="How {draft.merge.path || 'this file'} is composed"
          compact
          options={STRATEGIES}
          value={draft.merge.strategy ?? ''}
          {disabled}
          onSelect={(selection) => patch(index, { strategy: selection })}
        />

        {#if !readOnly}
          <!-- Every bare word inside a button is wrapped, here and below: a button
               is a flex container, so its text sits in an anonymous box no selector
               can reach, and `text-box` on the button itself never touches it. See
               `.button-label` in `app.css`. Unwrapped, each sat 0.47px high. -->
          <button class="btn btn-quiet" type="button" {disabled} onclick={() => remove(index)}>
            <span class="button-label">Remove</span>
          </button>
        {/if}
      </div>

      <label class="entry-field">
        <span class="entry-field-label">What this repository sets</span>
        <textarea
          class="entry-code sync-merge-overrides"
          rows="6"
          {disabled}
          aria-describedby="repository-sync-overrides-note"
          value={draft.text}
          placeholder={'{\n  "timezone": "Europe/Warsaw"\n}'}
          onchange={(event) => setText(index, event.currentTarget.value)}></textarea>
      </label>
    </article>
  {/each}

  <p class="form-note" id="repository-sync-overrides-note">
    A JSON object, merged onto the organization's template. <code>null</code> removes a key.
  </p>

  {#if malformed >= 0}
    <p class="form-error" role="alert">
      What this repository sets for {drafts[malformed]?.merge.path || 'a file'} is not a JSON object.
    </p>
  {/if}

  {#if !readOnly}
    <div class="form-actions">
      <button class="btn btn-quiet" type="button" {disabled} onclick={add}>
        <span class="button-label">Adjust a file</span>
      </button>
      <button
        class="btn btn-signal"
        type="button"
        disabled={disabled || !changed || malformed >= 0}
        onclick={() => onSave(wanted, payload)}
      >
        <span class="button-label">{saving ? 'Saving' : 'Save'}</span>
      </button>
    </div>
  {/if}
</section>

<style>
  .sync-pane {
    display: flex;
    flex-direction: column;
  }

  /* The global rule has no margin. These notes sit directly under the control
     they describe rather than in a gapped column, and the sliver of side inset
     lines them up with the field's own text. */
  .form-note {
    margin: 0.25rem 0.125rem 0;
  }

  /* Three lines rather than one run-on paragraph: what is happening, the
     planner's own words for why, and how long ago it found that. The reason is
     an error string and can run long, so it gets a line of its own. */
  .sync-pane-standdown {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .sync-pane-standdown-when {
    color: var(--dim);
  }

  .sync-pane-row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    padding-block: 0.7rem;
  }

  .sync-pane-label {
    font-size: 0.875rem;
    font-weight: 600;
  }

  /* The control sits at the end of its row rather than at the end of the pane:
     the spacer collapses when the row wraps, which puts the control under its
     own name at a narrow width. */
  .sync-pane-spacer {
    flex: 1;
  }

  /* Narrower than the shared-files form's, because an adjustment names a path
     the installation already lists rather than one somebody is typing out. */
  .sync-merge-path {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 12rem;
  }
</style>
