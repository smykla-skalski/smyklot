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
  import type { SyncFileMerge, SyncFileOverride, SyncOverride } from '#lib/types.js';

  import InheritControl from './InheritControl.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const {
    stored,
    readOnly,
    saving,
    problem = null,
    onSave,
  }: {
    stored: SyncOverride;
    readOnly: boolean;
    saving: boolean;
    problem?: string | null;
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

  /* Derived from what is saved and written over as somebody edits, so a save
     landing from anywhere reseeds it. */
  let merges = $derived<SyncFileMerge[]>(storedMerges(stored.document));
  let excludes = $derived<string[]>(storedExcludes(stored.document));
  let wanted = $derived<boolean | null>(stored.enabled);

  /* The overrides are arbitrary JSON, so they are edited as JSON. Kept as text
     rather than as a parsed value: a half-typed object is not an object, and a
     form that reparsed on every keystroke would blank the box the moment a
     brace was opened. */
  let texts = $derived<string[]>(storedMerges(stored.document).map(overridesText));

  const disabled = $derived(saving || readOnly || stored.unreadable);

  /** The first adjustment whose overrides are not JSON, or nothing. */
  const malformed = $derived(texts.findIndex((text) => parsed(text) === undefined));

  const payload = $derived(asDocument());
  const untouched = $derived(JSON.stringify(stored.document ?? {}));
  const changed = $derived(
    wanted !== stored.enabled || JSON.stringify(payload) !== untouched || textsDiffer(),
  );

  function asDocument(): Record<string, unknown> {
    const document: SyncFileOverride = {};

    if (merges.length > 0) {
      document.merges = merges.map((merge, index) => withOverrides(merge, texts[index]));
    }
    if (excludes.length > 0) {
      document.excludes = excludes;
    }

    return document as Record<string, unknown>;
  }

  /**
   * Whether the boxes say something the saved document does not. Compared as
   * text rather than through the payload, because an unparsed box contributes
   * nothing to the payload and would otherwise read as no change at all.
   */
  function textsDiffer(): boolean {
    const saved = storedMerges(stored.document).map(overridesText);

    return texts.length !== saved.length || texts.some((text, at) => text !== saved[at]);
  }

  function withOverrides(merge: SyncFileMerge, text: string): SyncFileMerge {
    const value = parsed(text);
    if (value !== undefined && Object.keys(value).length > 0) {
      return { ...merge, overrides: value };
    }

    // An empty box sets nothing, which is the absence of the key rather than an
    // empty object: the two mean the same thing to the merge and only one of
    // them reads that way in the stored document.
    const rest = { ...merge };
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

  function overridesText(merge: SyncFileMerge): string {
    return merge.overrides === undefined ? '' : JSON.stringify(merge.overrides, null, 2);
  }

  function storedMerges(from: Record<string, unknown>): SyncFileMerge[] {
    return Array.isArray(from?.merges) ? (from.merges as SyncFileMerge[]) : [];
  }

  function storedExcludes(from: Record<string, unknown>): string[] {
    return Array.isArray(from?.excludes) ? (from.excludes as string[]) : [];
  }

  function lines(values: readonly string[]): string {
    return values.join('\n');
  }

  function asList(text: string): string[] {
    return text
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line !== '');
  }

  function patch(index: number, change: Partial<SyncFileMerge>): void {
    merges = merges.map((merge, at) => (at === index ? { ...merge, ...change } : merge));
  }

  function setText(index: number, text: string): void {
    texts = texts.map((current, at) => (at === index ? text : current));
  }

  function add(): void {
    merges = [...merges, { path: '' }];
    texts = [...texts, ''];
  }

  function remove(index: number): void {
    merges = merges.filter((_, at) => at !== index);
    texts = texts.filter((_, at) => at !== index);
  }

  function rowKey(index: number): string {
    return `merge-${index}`;
  }

  function enablementValue(): string | null {
    if (wanted === null) return null;

    return wanted ? 'enabled' : 'disabled';
  }
</script>

<section class="sync-pane">
  <p class="sync-pane-lead">
    Whether the organization's files are kept in step here, and what this repository changes about
    them. Nothing reaches GitHub until a plan is approved
  </p>

  {#if problem !== null}
    <p class="form-error" role="alert">{problem}</p>
  {/if}

  {#if stored.unreadable}
    <p class="sync-pane-notice" role="alert">
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

  <label class="sync-pane-field">
    <span class="sync-pane-field-label">Files to leave alone here</span>
    <textarea
      rows="2"
      {disabled}
      aria-describedby="repository-sync-excludes-note"
      value={lines(excludes)}
      placeholder="renovate.json"
      onchange={(event) => (excludes = asList(event.currentTarget.value))}></textarea>
  </label>
  <p class="sync-pane-note" id="repository-sync-excludes-note">
    One path or pattern per line. These narrow what the installation synchronizes; they never widen
    it.
  </p>

  {#if merges.length === 0}
    <p class="sync-pane-note">This repository takes every file as the organization writes it.</p>
  {/if}

  {#each merges as merge, index (rowKey(index))}
    <article class="sync-merge">
      <div class="sync-pane-row">
        <label class="sync-merge-path">
          <span class="sync-pane-field-label">File</span>
          <input
            type="text"
            value={merge.path}
            {disabled}
            placeholder="renovate.json"
            onchange={(event) => patch(index, { path: event.currentTarget.value })}
          />
        </label>

        <SegmentedControl
          name="repository-sync-strategy-{index}"
          label="How {merge.path || 'this file'} is composed"
          compact
          options={STRATEGIES}
          value={merge.strategy ?? ''}
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

      <label class="sync-pane-field">
        <span class="sync-pane-field-label">What this repository sets</span>
        <textarea
          class="sync-merge-overrides"
          rows="6"
          {disabled}
          aria-describedby="repository-sync-overrides-note"
          value={texts[index] ?? ''}
          placeholder={'{\n  "timezone": "Europe/Warsaw"\n}'}
          onchange={(event) => setText(index, event.currentTarget.value)}></textarea>
      </label>
    </article>
  {/each}

  <p class="sync-pane-note" id="repository-sync-overrides-note">
    A JSON object, merged onto the organization's template. <code>null</code> removes a key.
  </p>

  {#if malformed >= 0}
    <p class="form-error" role="alert">
      What this repository sets for {merges[malformed]?.path || 'a file'} is not a JSON object.
    </p>
  {/if}

  {#if !readOnly}
    <div class="sync-pane-actions">
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

  .sync-pane-lead,
  .sync-pane-note {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0;
    max-width: 60ch;
  }

  .sync-pane-note {
    margin: 0.25rem 0.125rem 0;
  }

  .sync-pane-notice {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
    padding: var(--space-2) var(--space-3);
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

  .sync-pane-field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    margin-top: var(--space-4);
  }

  .sync-merge .sync-pane-field {
    margin-top: 0;
  }

  .sync-pane-field-label {
    font-size: var(--font-size-micro);
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .sync-merge {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin-top: var(--space-4);
    padding: var(--space-3);
  }

  .sync-merge-path {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 12rem;
  }

  /* JSON is read as code, so it is shown as code. */
  .sync-merge-overrides {
    font-family: var(--mono);
    font-size: var(--font-size-meta);
  }

  .sync-pane-actions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-top: var(--space-5);
  }
</style>
