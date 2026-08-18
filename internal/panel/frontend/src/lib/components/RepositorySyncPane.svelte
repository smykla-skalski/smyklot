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
  import { OFF, ON, SWITCH } from '#lib/form-switch.js';
  import type {
    SyncArrayRule,
    SyncFileMerge,
    SyncOverride,
    SyncPatch,
    SyncSection,
  } from '#lib/types.js';

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

  /*
   * Offered only for a Markdown path, and the three above only for a structured
   * one. The engine refuses either crossed over, and the engine this replaces
   * did not: it let a Markdown strategy be configured for a JSON file,
   * discovered it at apply time, and wrote the raw template over the
   * repository's copy. A choice that cannot be made is a refusal nobody has to
   * read.
   */
  const MARKDOWN_STRATEGIES = [
    { value: '', label: 'By extension' },
    { value: 'markdown', label: 'Markdown' },
  ] as const;

  const ARRAY_STRATEGIES = [
    { value: 'append', label: 'Append' },
    { value: 'prepend', label: 'Prepend' },
    { value: 'replace', label: 'Replace' },
  ] as const;

  /** What one section does. Which fields it needs follows from it. */
  const SECTION_ACTIONS = [
    { value: 'after', label: 'After' },
    { value: 'before', label: 'Before' },
    { value: 'replace', label: 'Replace' },
    { value: 'delete', label: 'Delete' },
    { value: 'patch', label: 'Patch' },
    { value: 'append', label: 'Append to document' },
    { value: 'prepend', label: 'Prepend to document' },
  ] as const;

  /** The extensions the engine edits by heading, spelled the same way. */
  const MARKDOWN_PATH = /\.(?:md|markdown)$/i;

  /* Two lines rather than one, because what goes in the box is a fragment of a
     document and the heading it opens with is the part people get wrong. */
  const SECTION_CONTENT_PLACEHOLDER = '### Prerequisites\n\nRun `mise install`';

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
  /* A Markdown row's box is not read, so text left in one from before the row
     pointed at a `.md` file is not a reason to refuse the save. */
  const malformed = $derived(
    values.findIndex((value, at) => value === undefined && !editsMarkdown(drafts[at].merge)),
  );

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
      document.merges = drafts.map((draft, at) => composed(draft, values[at]));
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

  /**
   * How this row is edited, decided the way the engine decides it: what the
   * strategy says, and where it says nothing, what the extension says.
   *
   * Read from the draft rather than stored, so pointing a row at a `.md` file
   * turns it into a Markdown row as the path is typed rather than after a save.
   */
  function editsMarkdown(merge: SyncFileMerge): boolean {
    if (merge.strategy === 'markdown') return true;
    if (merge.strategy === 'deep-merge' || merge.strategy === 'shallow-merge') return false;

    return MARKDOWN_PATH.test(merge.path);
  }

  /**
   * One adjustment as it will be stored.
   *
   * The keys that belong to the other mode are dropped rather than carried:
   * the engine refuses a spec holding both, so a row switched from JSON to
   * Markdown would otherwise save something it will not accept, and the
   * refusal would arrive from the planner rather than from this form.
   *
   * Unknown keys survive, which is the point of spreading the stored merge: a
   * key a newer version of the service wrote is sent back rather than dropped
   * by a browser running an older build.
   */
  function composed(draft: Draft, value: Record<string, unknown> | undefined): SyncFileMerge {
    const merge = { ...draft.merge };

    if (editsMarkdown(merge)) {
      delete merge.overrides;
      delete merge.arrays;
      delete merge.deduplicate;

      if ((merge.sections ?? []).length === 0) delete merge.sections;

      return merge;
    }

    delete merge.sections;

    if (value !== undefined && Object.keys(value).length > 0) {
      merge.overrides = value;
    } else {
      // An empty box sets nothing, which is the absence of the key rather than
      // an empty object: the two mean the same thing to the merge and only one
      // of them reads that way in the stored document.
      delete merge.overrides;
    }

    // Nothing is deduplicated without a list rule, because a list with no rule
    // is replaced whole - so the flag is never written on its own, which is a
    // pair the engine refuses rather than ignores.
    if ((merge.arrays ?? []).length === 0) {
      delete merge.arrays;
      delete merge.deduplicate;
    } else if (merge.deduplicate !== true) {
      delete merge.deduplicate;
    }

    return merge;
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

  /* The rows inside a row. Each list is edited through the merge it belongs to,
     so every one of these ends at `patch`, and a new list rather than an edit
     in place is what makes the draft compare unequal to what is stored. */
  function rulesOf(index: number): SyncArrayRule[] {
    return drafts[index].merge.arrays ?? [];
  }

  function sectionsOf(index: number): SyncSection[] {
    return drafts[index].merge.sections ?? [];
  }

  function patchRule(index: number, at: number, change: Partial<SyncArrayRule>): void {
    patch(index, { arrays: patchedAt(rulesOf(index), at, change) });
  }

  function addRule(index: number): void {
    // Append, because appending is what every list rule in the organization
    // this was written for does, and a rule added with no strategy is one the
    // engine refuses.
    patch(index, { arrays: [...rulesOf(index), { path: '', strategy: 'append' }] });
  }

  function removeRule(index: number, at: number): void {
    patch(index, { arrays: withoutAt(rulesOf(index), at) });
  }

  function replaceSection(index: number, at: number, section: SyncSection): void {
    patch(index, {
      sections: sectionsOf(index).map((existing, which) => (which === at ? section : existing)),
    });
  }

  function patchSection(index: number, at: number, change: Partial<SyncSection>): void {
    patch(index, { sections: patchedAt(sectionsOf(index), at, change) });
  }

  function addSection(index: number): void {
    patch(index, { sections: [...sectionsOf(index), { action: 'after', heading: '' }] });
  }

  function removeSection(index: number, at: number): void {
    patch(index, { sections: withoutAt(sectionsOf(index), at) });
  }

  /**
   * What a section does, and the fields that stop applying when it changes.
   *
   * Appending and prepending address the document rather than a heading, and
   * the engine refuses one carrying a heading rather than ignoring it - so the
   * heading is dropped here instead of being left to be refused at apply time.
   */
  function setAction(index: number, at: number, action: string): void {
    const section: SyncSection = { ...sectionsOf(index)[at], action };

    if (action === 'append' || action === 'prepend') {
      delete section.heading;
      delete section.occurrence;
    }

    replaceSection(index, at, section);
  }

  /**
   * Which heading of that name, where a document repeats one.
   *
   * Absent rather than zero where the box is empty: left out, a heading that
   * appears twice is refused rather than quietly resolved to the first, and
   * writing a zero would say something the engine does not read.
   */
  function setOccurrence(index: number, at: number, text: string): void {
    const section = { ...sectionsOf(index)[at] };
    const which = Number.parseInt(text, 10);

    if (Number.isInteger(which) && which > 0) {
      section.occurrence = which;
    } else {
      delete section.occurrence;
    }

    replaceSection(index, at, section);
  }

  function patchesOf(index: number, at: number): SyncPatch[] {
    return sectionsOf(index)[at].patches ?? [];
  }

  function patchSubstitution(
    index: number,
    at: number,
    which: number,
    change: Partial<SyncPatch>,
  ): void {
    patchSection(index, at, { patches: patchedAt(patchesOf(index, at), which, change) });
  }

  function addSubstitution(index: number, at: number): void {
    patchSection(index, at, { patches: [...patchesOf(index, at), { find: '', replace: '' }] });
  }

  function removeSubstitution(index: number, at: number, which: number): void {
    patchSection(index, at, { patches: withoutAt(patchesOf(index, at), which) });
  }

  /** Whether this section addresses a heading, which decides what it shows. */
  function addressesHeading(action: string): boolean {
    return action !== 'append' && action !== 'prepend';
  }

  /** Whether it carries a body. Delete takes none, and patch takes pairs. */
  function carriesContent(action: string): boolean {
    return action !== 'delete' && action !== 'patch';
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
          options={editsMarkdown(draft.merge) ? MARKDOWN_STRATEGIES : STRATEGIES}
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

      {#if editsMarkdown(draft.merge)}
        <!-- Markdown is edited by its headings, so the keys-and-lists controls
             are not shown rather than shown and refused. Which one a row gets
             follows the engine's own reading of the strategy and the extension. -->
        {#each draft.merge.sections ?? [] as section, at (`${rowKey(index)}-section-${at}`)}
          <div class="sync-merge-section">
            <div class="sync-pane-row">
              <SegmentedControl
                name="repository-sync-section-{index}-{at}"
                label="What section {at + 1} of {draft.merge.path || 'this file'} does"
                compact
                options={SECTION_ACTIONS}
                value={section.action}
                {disabled}
                onSelect={(selection) => setAction(index, at, selection)}
              />

              {#if !readOnly}
                <button
                  class="btn btn-quiet"
                  type="button"
                  {disabled}
                  onclick={() => removeSection(index, at)}
                >
                  <span class="button-label">Remove</span>
                </button>
              {/if}
            </div>

            {#if addressesHeading(section.action)}
              <div class="sync-pane-row">
                <label class="sync-merge-path">
                  <span class="entry-field-label">Heading</span>
                  <input
                    type="text"
                    value={section.heading ?? ''}
                    {disabled}
                    placeholder="### Prerequisites"
                    onchange={(event) =>
                      patchSection(index, at, { heading: event.currentTarget.value })}
                  />
                </label>

                <label class="sync-merge-occurrence">
                  <span class="entry-field-label">Which one</span>
                  <input
                    type="number"
                    min="1"
                    value={section.occurrence ?? ''}
                    {disabled}
                    onchange={(event) => setOccurrence(index, at, event.currentTarget.value)}
                  />
                </label>
              </div>
            {/if}

            {#if carriesContent(section.action)}
              <label class="entry-field">
                <span class="entry-field-label">What this repository writes</span>
                <textarea
                  class="entry-code"
                  rows="5"
                  {disabled}
                  value={section.content ?? ''}
                  placeholder={SECTION_CONTENT_PLACEHOLDER}
                  onchange={(event) =>
                    patchSection(index, at, { content: event.currentTarget.value })}></textarea>
              </label>
            {/if}

            {#if section.action === 'patch'}
              {#each section.patches ?? [] as substitution, which (`${rowKey(index)}-patch-${at}-${which}`)}
                <div class="sync-pane-row">
                  <label class="sync-merge-path">
                    <span class="entry-field-label">Find</span>
                    <input
                      type="text"
                      value={substitution.find}
                      {disabled}
                      placeholder="make check"
                      onchange={(event) =>
                        patchSubstitution(index, at, which, { find: event.currentTarget.value })}
                    />
                  </label>

                  <label class="sync-merge-path">
                    <span class="entry-field-label">Replace with</span>
                    <input
                      type="text"
                      value={substitution.replace}
                      {disabled}
                      placeholder="mise run check"
                      onchange={(event) =>
                        patchSubstitution(index, at, which, { replace: event.currentTarget.value })}
                    />
                  </label>

                  {#if !readOnly}
                    <button
                      class="btn btn-quiet"
                      type="button"
                      {disabled}
                      onclick={() => removeSubstitution(index, at, which)}
                    >
                      <span class="button-label">Remove</span>
                    </button>
                  {/if}
                </div>
              {/each}

              {#if !readOnly}
                <button
                  class="btn btn-quiet"
                  type="button"
                  {disabled}
                  onclick={() => addSubstitution(index, at)}
                >
                  <span class="button-label">Add a substitution</span>
                </button>
              {/if}
            {/if}
          </div>
        {/each}

        {#if !readOnly}
          <button class="btn btn-quiet" type="button" {disabled} onclick={() => addSection(index)}>
            <span class="button-label">Edit a section</span>
          </button>
        {/if}
      {:else}
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

        {#each draft.merge.arrays ?? [] as rule, at (`${rowKey(index)}-rule-${at}`)}
          <div class="sync-pane-row">
            <label class="sync-merge-path">
              <span class="entry-field-label">List</span>
              <input
                type="text"
                value={rule.path}
                {disabled}
                placeholder="$.packageRules"
                onchange={(event) => patchRule(index, at, { path: event.currentTarget.value })}
              />
            </label>

            <SegmentedControl
              name="repository-sync-array-{index}-{at}"
              label="What happens to {rule.path || 'this list'}"
              compact
              options={ARRAY_STRATEGIES}
              value={rule.strategy}
              {disabled}
              onSelect={(selection) => patchRule(index, at, { strategy: selection })}
            />

            {#if !readOnly}
              <button
                class="btn btn-quiet"
                type="button"
                {disabled}
                onclick={() => removeRule(index, at)}
              >
                <span class="button-label">Remove</span>
              </button>
            {/if}
          </div>
        {/each}

        <!-- Offered only beside a list rule, because a list with no rule is
             replaced whole and there is nothing left to deduplicate: the engine
             refuses that pair rather than ignoring the flag. -->
        {#if (draft.merge.arrays ?? []).length > 0}
          <div class="sync-pane-row">
            <span class="sync-pane-label">Drop repeated entries</span>
            <span class="sync-pane-spacer"></span>
            <SegmentedControl
              name="repository-sync-deduplicate-{index}"
              label="Drop repeated entries from {draft.merge.path || 'this file'}"
              compact
              options={SWITCH}
              value={draft.merge.deduplicate === true ? ON : OFF}
              {disabled}
              onSelect={(selection) => patch(index, { deduplicate: selection === ON })}
            />
          </div>
        {/if}

        {#if !readOnly}
          <button class="btn btn-quiet" type="button" {disabled} onclick={() => addRule(index)}>
            <span class="button-label">Add a list rule</span>
          </button>
        {/if}
      {/if}
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

  /* Wide enough for a count and no wider: it holds a small ordinal, and a box
     sized like the heading beside it would read as somewhere to type words. */
  .sync-merge-occurrence {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    width: 6rem;
  }

  /* A hairline between sections rather than a card around each: they are steps
     in one document's edit, and boxing every one of them turned a file with six
     into six files. Drawn between rather than around, so the first sits flush
     against the strategy row above it. */
  .sync-merge-section + .sync-merge-section {
    border-top: 1px solid var(--rule);
    margin-top: var(--space-3);
    padding-top: var(--space-3);
  }
</style>
