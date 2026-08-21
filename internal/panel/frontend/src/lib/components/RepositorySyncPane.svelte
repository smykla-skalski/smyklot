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
  import { patchedAt, rowKeys, storedList, withoutAt } from '#lib/form-lists.js';
  import { formatRelative } from '#lib/format.js';
  import type { SyncFileMerge, SyncOverride } from '#lib/types.js';

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Switch from './Switch.svelte';

  const {
    stored,
    readOnly,
    saving,
    now,
    saveProblem = null,
    filesHref = null,
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
    /** The workspace's Files page, where the templates themselves live. */
    filesHref?: string | null;
    onSave: (enabled: boolean | null, document: Record<string, unknown>) => void;
  } = $props();

  /** What a merge does to a structured template. Markdown has its own. */
  const STRATEGIES = [
    { value: '', label: 'By extension' },
    { value: 'deep-merge', label: 'Deep' },
    { value: 'shallow-merge', label: 'Shallow' },
  ] as const;

  const LIST_STRATEGIES = [
    { value: 'replace', label: 'Replace' },
    { value: 'append', label: 'Append' },
    { value: 'prepend', label: 'Prepend' },
  ] as const;

  const SECTION_ACTIONS = [
    { value: 'before', label: 'Before' },
    { value: 'after', label: 'After' },
    { value: 'replace', label: 'Replace' },
    { value: 'delete', label: 'Delete' },
  ] as const;

  /** One list rule as stored: which list, and how two of them combine. */
  type ArrayRule = { path: string; strategy: string };
  /** One markdown operation as stored. */
  type Section = {
    action: string;
    heading?: string;
    occurrence?: number;
    content?: string;
    [key: string]: unknown;
  };

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
  type Draft = {
    merge: SyncFileMerge & { arrays?: ArrayRule[]; sections?: Section[] };
    text: string;
  };

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
    return storedList<Draft['merge']>(from, 'merges').map((merge) => ({
      merge,
      text: merge.overrides === undefined ? '' : JSON.stringify(merge.overrides, null, 2),
    }));
  }

  function patch(index: number, change: Partial<Draft['merge']>): void {
    drafts = patchedAt(drafts, index, {
      merge: { ...drafts[index].merge, ...change },
    });
    queueSave();
  }

  function setText(index: number, text: string): void {
    drafts = patchedAt(drafts, index, { text });
    queueSave();
  }

  function add(): void {
    drafts = [...drafts, { merge: { path: '' }, text: '' }];
  }

  function remove(index: number): void {
    drafts = withoutAt(drafts, index);
    queueSave();
  }

  /* ---------- List rules: which list, and how two of them combine ---------- */

  function rulesOf(draft: Draft): ArrayRule[] {
    return draft.merge.arrays ?? [];
  }

  function patchRule(index: number, at: number, change: Partial<ArrayRule>): void {
    const rules = rulesOf(drafts[index]).map((rule, held) =>
      held === at ? { ...rule, ...change } : rule,
    );
    patch(index, { arrays: rules });
  }

  function addRule(index: number): void {
    patch(index, { arrays: [...rulesOf(drafts[index]), { path: '', strategy: 'append' }] });
  }

  function removeRule(index: number, at: number): void {
    const rules = rulesOf(drafts[index]).filter((_, held) => held !== at);
    patch(index, rules.length > 0 ? { arrays: rules } : { arrays: undefined });
  }

  /* ---------- Markdown sections ---------- */

  function sectionsOf(draft: Draft): Section[] {
    return draft.merge.sections ?? [];
  }

  function patchSection(index: number, at: number, change: Partial<Section>): void {
    const sections = sectionsOf(drafts[index]).map((section, held) =>
      held === at ? { ...section, ...change } : section,
    );
    patch(index, { sections });
  }

  function addSection(index: number): void {
    patch(index, {
      sections: [...sectionsOf(drafts[index]), { action: 'replace', heading: '', content: '' }],
    });
  }

  function removeSection(index: number, at: number): void {
    const sections = sectionsOf(drafts[index]).filter((_, held) => held !== at);
    patch(index, sections.length > 0 ? { sections } : { sections: undefined });
  }

  const isMarkdown = (draft: Draft): boolean =>
    draft.merge.strategy === 'markdown' || /\.(md|markdown)$/i.test(draft.merge.path ?? '');

  const rowKey = rowKeys('merge');

  /* ---------- Saved change by change, after a typing rest ---------- */

  const SAVE_REST_MS = 900;
  let saveTimer: ReturnType<typeof setTimeout> | undefined;

  /** An entry not yet named has nowhere to be written; the save waits for it. */
  const nameless = $derived(drafts.some((draft) => (draft.merge.path ?? '').trim() === ''));

  function queueSave(): void {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      if (disabled || !changed || malformed >= 0 || nameless) return;
      onSave(wanted, payload);
    }, SAVE_REST_MS);
  }

  function setWanted(next: boolean | null): void {
    wanted = next;
    queueSave();
  }

  /* The receipt keys off the save the parent runs: shown when a save this
     pane queued lands without a problem. */
  let savedOn = $state(false);
  let savedTimer: ReturnType<typeof setTimeout> | undefined;
  let wasSaving = false;

  $effect(() => {
    if (saving) {
      wasSaving = true;
      return;
    }
    if (!wasSaving) return;
    wasSaving = false;
    if (saveProblem !== null) return;
    savedOn = true;
    clearTimeout(savedTimer);
    savedTimer = setTimeout(() => (savedOn = false), 1400);
  });
</script>

<section class="sync-pane card group-card">
  <div class="group-head">
    <h3 class="group-name">File sync</h3>
    <span class="save-whisper" class:is-on={savedOn} role="status"
      ><Icon name="check" size={12} /><span class="t">Saved</span></span
    >
  </div>
  <p class="group-note">
    How this repository takes the organization's files. Everything here narrows what sync writes -
    the templates themselves live on the workspace's
    {#if filesHref !== null}<a href={filesHref}>Files</a> page{:else}Files page{/if}
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

  <div class="policy-rows">
    <div class="policy-row">
      <span class="setting-say">
        <span class="setting-name">File sync</span>
        <span class="setting-why"
          >Whether the organization's files are written in this repository at all</span
        >
      </span>
      {#if wanted === null}
        <span class="policy-value">
          <span class="setting-unmanaged">Follows the installation</span>
        </span>
        <button
          class="setting-clear"
          title="Answer for this repository"
          {disabled}
          onclick={() => setWanted(true)}
        >
          <Icon name="plus" size={10} />
        </button>
      {:else}
        <span class="policy-value">
          <span class="value-word" class:is-on={wanted}>{wanted ? 'On' : 'Off'}</span>
          <Switch checked={wanted} label="File sync" {disabled} onToggle={setWanted} />
        </span>
        <button
          class="setting-clear"
          title="Stop answering - follow the installation"
          {disabled}
          onclick={() => setWanted(null)}
        >
          <Icon name="close" size={10} />
        </button>
      {/if}
    </div>
    <div class="policy-row policy-block">
      <span class="setting-say">
        <span class="setting-name">Files to leave alone here</span>
        <span class="setting-why"
          >Patterns, where * stands for any run of characters. A file named here is never written or
          removed in this repository</span
        >
      </span>
      <div class="pattern-line">
        <PatternEntries
          patterns={excludes}
          readOnly={disabled}
          onChange={(next) => {
            excludes = next;
            queueSave();
          }}
        />
      </div>
    </div>
  </div>

  {#if drafts.length === 0}
    <p class="form-note-line">This repository takes every file as the organization writes it.</p>
  {/if}

  {#each drafts as draft, index (rowKey(index))}
    <article class="entry-card">
      <div class="entry-row">
        <label class="entry-field entry-grow">
          <span class="entry-label">File</span>
          <input
            class="text-inline is-wide mono-input"
            type="text"
            value={draft.merge.path}
            {disabled}
            placeholder="renovate.json"
            onchange={(event) => patch(index, { path: event.currentTarget.value })}
          />
        </label>

        {#if !isMarkdown(draft)}
          <SegmentedControl
            name="repository-sync-strategy-{index}"
            label="How {draft.merge.path || 'this file'} is composed"
            compact
            options={STRATEGIES}
            value={draft.merge.strategy ?? ''}
            {disabled}
            onSelect={(selection) => patch(index, { strategy: selection })}
          />
        {/if}

        {#if !readOnly}
          <Button tone="stop-quiet" {disabled} onclick={() => remove(index)}>Remove</Button>
        {/if}
      </div>

      {#if !isMarkdown(draft)}
        <label class="entry-field">
          <span class="entry-label">What this repository sets</span>
          <textarea
            class="entry-code sync-merge-overrides"
            rows="5"
            {disabled}
            aria-describedby="repository-sync-overrides-note"
            value={draft.text}
            placeholder={'{\n  "timezone": "Europe/Warsaw"\n}'}
            onchange={(event) => setText(index, event.currentTarget.value)}></textarea>
        </label>

        {#each rulesOf(draft) as rule, at (at)}
          <div class="entry-row rule-row">
            <label class="entry-field">
              <span class="entry-label">List</span>
              <input
                class="text-inline mono-input"
                type="text"
                value={rule.path}
                {disabled}
                placeholder="packageRules"
                onchange={(event) => patchRule(index, at, { path: event.currentTarget.value })}
              />
            </label>
            <SegmentedControl
              name="repository-sync-list-{index}-{at}"
              label="How the {rule.path || 'list'} entries combine"
              compact
              options={LIST_STRATEGIES}
              value={rule.strategy}
              {disabled}
              onSelect={(selection) => patchRule(index, at, { strategy: selection })}
            />
            {#if !readOnly}
              <button
                class="setting-clear"
                title="Drop this list rule - the list goes back to being replaced"
                {disabled}
                onclick={() => removeRule(index, at)}
              >
                <Icon name="close" size={10} />
              </button>
            {/if}
          </div>
        {/each}

        {#if rulesOf(draft).length > 0}
          <div class="entry-row">
            <span class="entry-spacer"></span>
            <span class="entry-label-solo">Drop repeated entries</span>
            <Switch
              checked={draft.merge.deduplicate === true}
              bare
              label="Drop repeated entries"
              {disabled}
              onToggle={(next) => patch(index, { deduplicate: next ? true : undefined })}
            />
          </div>
        {/if}

        {#if !readOnly}
          <button class="add-chip" {disabled} onclick={() => addRule(index)}>
            <Icon name="plus" size={12} />
            <span class="t">Add a list rule</span>
          </button>
        {/if}
      {:else}
        {#each sectionsOf(draft) as section, at (at)}
          <div class="section-card">
            <div class="entry-row">
              <SegmentedControl
                name="repository-sync-section-{index}-{at}"
                label="What this does to the section"
                compact
                options={SECTION_ACTIONS}
                value={SECTION_ACTIONS.some((held) => held.value === section.action)
                  ? section.action
                  : 'replace'}
                {disabled}
                onSelect={(selection) => patchSection(index, at, { action: selection })}
              />
              <label class="entry-field entry-grow">
                <span class="entry-label">Heading</span>
                <input
                  class="text-inline"
                  type="text"
                  value={section.heading ?? ''}
                  {disabled}
                  placeholder="## Releasing"
                  onchange={(event) =>
                    patchSection(index, at, { heading: event.currentTarget.value })}
                />
              </label>
              {#if !readOnly}
                <button
                  class="setting-clear"
                  title="Drop this section change"
                  {disabled}
                  onclick={() => removeSection(index, at)}
                >
                  <Icon name="close" size={10} />
                </button>
              {/if}
            </div>
            {#if section.action !== 'delete'}
              <label class="entry-field">
                <span class="entry-label">What this repository writes</span>
                <textarea
                  class="entry-code"
                  rows="4"
                  {disabled}
                  value={section.content ?? ''}
                  onchange={(event) =>
                    patchSection(index, at, { content: event.currentTarget.value })}></textarea>
              </label>
            {/if}
          </div>
        {/each}
        {#if !readOnly}
          <button class="add-chip" {disabled} onclick={() => addSection(index)}>
            <Icon name="plus" size={12} />
            <span class="t">Add a section change</span>
          </button>
        {/if}
      {/if}
    </article>
  {/each}

  <p class="form-note-line" id="repository-sync-overrides-note">
    A JSON object, merged onto the organization's template. <code>null</code> removes a key.
  </p>

  {#if malformed >= 0}
    <p class="form-error" role="alert">
      What this repository sets for {drafts[malformed]?.merge.path || 'a file'} is not a JSON object.
    </p>
  {/if}

  {#if !readOnly}
    <button class="add-chip add-entry" type="button" {disabled} onclick={add}>
      <Icon name="plus" size={12} />
      <span class="t">Adjust another file</span>
    </button>
  {/if}
</section>

<style>
  .sync-pane.card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    display: block;
    padding: var(--space-5);
  }

  .group-head {
    align-items: end;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-2);
  }

  .group-name {
    font-size: var(--font-size-title);
    font-weight: 600;
    margin: 0;
    min-block-size: 12px;
    text-box: trim-both cap alphabetic;
  }

  .save-whisper {
    align-items: center;
    background: var(--success-tint);
    block-size: 20px;
    border-radius: var(--radius-chip);
    color: var(--success);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 4px;
    margin-inline-start: auto;
    opacity: 0;
    padding: 0 0.5rem;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .save-whisper.is-on {
    opacity: 1;
  }

  .save-whisper .t {
    text-box: trim-both cap alphabetic;
  }

  .group-note {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    line-height: round(1.5em, 1px);
    margin: 0 0 var(--space-2);
    max-width: 72ch;
  }

  .group-note a {
    color: var(--brand-action-text);
  }

  .policy-rows {
    display: grid;
    margin-bottom: var(--space-2);
  }

  .policy-row {
    align-items: center;
    display: grid;
    gap: var(--space-2) var(--space-4);
    grid-template-columns: 1fr auto auto;
    margin-inline: calc(var(--space-2) * -1);
    min-block-size: 48px;
    /* The air around a drawn hairline is the card's own padding, on both
       sides; the edge rows shed it where no line follows. */
    padding: var(--space-5) var(--space-2);
    position: relative;
  }

  .policy-row:first-child {
    padding-block-start: var(--space-2);
  }

  .policy-row:last-child {
    padding-block-end: var(--space-2);
  }

  .policy-row:not(:last-child)::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
  }

  .setting-say {
    display: grid;
    gap: var(--space-3);
  }

  .setting-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  .setting-why {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .policy-value {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-self: end;
  }

  .value-word {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    min-inline-size: 1.9rem;
    text-align: end;
    text-box: trim-both cap alphabetic;
  }

  .value-word.is-on {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .setting-unmanaged {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    font-style: normal;
    /* Ink-true, so the padding around the hairlines measures to the glyphs
       rather than to the line box's leading. */
    text-box: trim-both cap alphabetic;
  }

  /* These notes sit directly under the control they describe, and the sliver
     of side inset lines them up with the field's own text. */
  .form-note-line {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: 0.25rem 0.125rem var(--space-3);
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

  .entry-field {
    display: grid;
    gap: 0.5rem;
  }

  .entry-grow {
    flex: 1;
    min-width: 12rem;
  }

  .entry-label {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    font-weight: 600;
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .entry-label-solo {
    font-size: var(--font-size-meta);
    font-weight: 600;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  .entry-spacer {
    flex: 1;
  }

  .pattern-line {
    grid-column: 1 / -1;
    margin-block: var(--space-1) 0;
  }

  .entry-row {
    align-items: flex-end;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .rule-row {
    align-items: flex-end;
  }

  .entry-card {
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-3);
    margin-block-end: var(--space-3);
    padding: var(--space-4);
  }

  .section-card {
    display: grid;
    gap: var(--space-3);
  }

  .section-card + .section-card {
    border-top: 1px solid var(--border-subtle);
    padding-top: var(--space-3);
  }

  .text-inline {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-size: var(--font-size-control);
    min-block-size: 30px;
    padding-inline: 0.55rem;
    width: 11rem;
  }

  .text-inline:focus {
    border-color: var(--focus);
    outline: 2px solid var(--focus);
    outline-offset: -1px;
  }

  .text-inline.is-wide {
    flex: 1;
    min-width: 14rem;
    width: auto;
  }

  .mono-input {
    font-family: var(--mono);
  }

  .setting-clear {
    align-items: center;
    background: transparent;
    block-size: 26px;
    border: 0;
    border-radius: 50%;
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    inline-size: 26px;
    justify-content: center;
    margin-block-end: 2px;
    padding: 0;
  }

  .setting-clear:hover {
    background: var(--interactive-hover-layer);
    color: var(--text-primary);
  }

  .setting-clear:active {
    background: var(--interactive-pressed);
  }

  .add-chip {
    align-items: center;
    background: var(--control-bg);
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-chip);
    color: var(--text-secondary);
    cursor: pointer;
    display: inline-flex;
    font-size: var(--font-size-compact);
    font-weight: 500;
    gap: 0.35rem;
    justify-self: start;
    min-block-size: 30px;
    padding-block: 0;
    padding-inline: 0.7rem;
  }

  .add-chip:hover {
    background: var(--control-bg-hover);
    border-style: solid;
    color: var(--text-primary);
  }

  .add-chip:active {
    background: var(--control-bg-pressed);
  }

  .add-chip .t {
    text-box: trim-both cap alphabetic;
  }
</style>
