<script lang="ts">
  import { patchedAt, rowKeys, storedList, withoutAt } from '#lib/form-lists.js';
  import { formatRelative } from '#lib/format.js';
  import { asArrayStrategy } from '#lib/merge.js';
  import {
    buildSyncOverrideEditorEnvelope,
    type SyncOverrideControlId,
    type SyncOverrideEditorEnvelope,
  } from '#lib/repository-sync-override-settings.js';
  import type {
    SyncArrayRule,
    SyncFileMerge,
    SyncOverride,
    SyncPatch,
    SyncSection,
  } from '#lib/types.js';

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Switch from './Switch.svelte';

  const {
    stored,
    repositoryId,
    envelope = undefined,
    readOnly,
    now,
    dirtyEnabled = false,
    dirtyDocument = false,
    onChange = () => {},
  }: {
    stored: SyncOverride;
    repositoryId: string;
    envelope?: SyncOverrideEditorEnvelope | undefined;
    readOnly: boolean;
    /**
     * The list's clock, so a refusal can say how long ago it was found. Passed
     * rather than read here, because a second timer in a dialog the list
     * already ticks for would say a different thing on the same screen.
     */
    now: number;
    dirtyEnabled?: boolean;
    dirtyDocument?: boolean;
    onChange?: (next: SyncOverrideEditorEnvelope, control: SyncOverrideControlId) => void;
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

  /**
   * What each action needs, which is both what a section shows and what
   * `setAction` drops when one is chosen. Written once: the fact that
   * appending addresses the document rather than a heading was otherwise
   * stated in the label, in the drop, and in two predicates.
   */
  const SECTION_SHAPE: Record<string, { heading: boolean; content: boolean; patches: boolean }> = {
    after: { heading: true, content: true, patches: false },
    before: { heading: true, content: true, patches: false },
    replace: { heading: true, content: true, patches: false },
    delete: { heading: true, content: false, patches: false },
    patch: { heading: true, content: false, patches: true },
    append: { heading: false, content: true, patches: false },
    prepend: { heading: false, content: true, patches: false },
  };

  /* An action a newer service wrote reads as the ordinary one, which is what
     the predicates this replaces already did with an unknown action. */
  const shapeOf = (action: string) => SECTION_SHAPE[action] ?? SECTION_SHAPE.after;

  /* What each mode is allowed to write. The engine refuses a spec holding
     both, so which keys belong to the other one is stated here rather than
     inferred from which `delete` sits under which return. */
  const MARKDOWN_KEYS = ['sections'] as const;
  const STRUCTURED_KEYS = ['overrides', 'arrays', 'deduplicate'] as const;

  /** The extensions the engine edits by heading, spelled the same way. */
  const MARKDOWN_PATH = /\.(?:md|markdown)$/i;

  /* And the ones it can merge at all. Everything else is ErrUnsupportedFormat,
     which the pane would otherwise offer a complete editor for. */
  const MERGEABLE_PATH = /\.(?:json|ya?ml|md|markdown)$/i;

  /* Two lines rather than one, because what goes in the box is a fragment of a
     document and the heading it opens with is the part people get wrong. */
  const SECTION_CONTENT_PLACEHOLDER = '### Prerequisites\n\nRun `mise install`';

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

  const controlledEnvelope = $derived(
    stored.unreadable
      ? ({
          enabled: stored.enabled,
          document: {},
          override_texts: [],
        } satisfies SyncOverrideEditorEnvelope)
      : (envelope ?? buildSyncOverrideEditorEnvelope(stored)),
  );
  /* Writable derived state keeps direct component stories useful. In the app,
     every write immediately publishes a complete envelope to the registry and
     the controlled prop becomes the same value. */
  let drafts = $derived<Draft[]>(editorDrafts(controlledEnvelope));
  let excludes = $derived<string[]>(storedList<string>(controlledEnvelope.document, 'excludes'));
  let wanted = $derived<boolean | null>(controlledEnvelope.enabled);

  const disabled = $derived(readOnly || stored.unreadable);

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
  /* A Markdown row's box is not read by anything - `composed` deletes the key
     and the refusal check never reaches it - so it is not parsed either. An
     inert `{}` rather than a parse keeps `malformed` a single question. */
  const values = $derived(
    drafts.map((draft) => (editsMarkdown(draft.merge) ? {} : parsed(draft.text))),
  );

  /** The first adjustment whose overrides are not JSON, or nothing. */
  const malformed = $derived(values.findIndex((value) => value === undefined));

  /**
   * The first adjustment the engine would refuse for a reason this form can
   * already see, written the way somebody reading the row would say it.
   *
   * Every merge is validated on save - `orgsync.FileOverride.ValidateAgainst`
   * calls `filemerge.Spec.Validate` for each one - so without this the answer
   * to a half-filled row is a round trip and one flat sentence at the top of a
   * pane that can hold several files. `Spec.Empty()` does not rescue an empty
   * row either: the short circuit for it lives in `Apply`, not on the save
   * path, so a row naming only a file is refused rather than ignored.
   *
   * What is left to the server is what the pane cannot know: whether the file
   * is one the workspace actually synchronizes.
   */
  const incomplete = $derived.by(() => {
    for (const [at, draft] of drafts.entries()) {
      const problem = refusalIn(draft, values[at]);

      if (problem !== null) return problem;
    }

    return null;
  });

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
    const document: Record<string, unknown> = { ...controlledEnvelope.document };

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
    const markdown = editsMarkdown(merge);

    // The other mode's keys never travel, whichever mode this is.
    for (const key of markdown ? STRUCTURED_KEYS : MARKDOWN_KEYS) delete merge[key];

    if (markdown) {
      if (!merge.sections?.length) delete merge.sections;

      return merge;
    }

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
    if (!merge.arrays?.length) {
      delete merge.arrays;
      delete merge.deduplicate;
    } else if (merge.deduplicate !== true) {
      delete merge.deduplicate;
    }

    return merge;
  }

  function refusalIn(draft: Draft, value: Record<string, unknown> | undefined): string | null {
    const { path } = draft.merge;
    const named = path === '' ? 'an adjustment' : path;

    if (path === '') return 'One adjustment names no file';

    if (!MERGEABLE_PATH.test(path)) {
      return `${path} has no extension this can merge; JSON, YAML and Markdown can`;
    }

    if (drafts.filter((other) => other.merge.path === path).length > 1) {
      return `${path} is adjusted twice`;
    }

    if (editsMarkdown(draft.merge)) return refusalInSections(named, draft.merge.sections ?? []);

    const rules = draft.merge.arrays ?? [];

    if ((value === undefined || Object.keys(value).length === 0) && rules.length === 0) {
      return `${named} sets nothing and has no list rule, so nothing would be merged`;
    }

    for (const [at, rule] of rules.entries()) {
      const which = `List rule ${at + 1} of ${named}`;
      const read = pathKeys(rule.path);

      if ('refusal' in read) return `${which} ${read.refusal}`;

      if (rules.filter((other) => other.path === rule.path).length > 1) {
        return `${named} has two rules for ${rule.path}`;
      }

      // A shallow merge replaces a top-level key with the override's value
      // whole, so nothing below one is ever merged.
      if (draft.merge.strategy === 'shallow-merge' && read.keys.length > 1) {
        return `${rule.path} is below the top level, and a shallow merge replaces top-level keys whole`;
      }

      // A rule says what to do with the repository's list where the template
      // has one, so a rule whose path the overrides do not set has no list to
      // work with - for every template, always. The engine refuses it; the
      // pane holds both documents, so it can say so under the box instead.
      if (value === undefined) continue;

      const target = valueAt(value, read.keys);

      if (target === undefined) {
        return `No override sets ${rule.path}, so ${named} has no list to ${rule.strategy}`;
      }

      if (!Array.isArray(target)) return `The override at ${rule.path} is not a list`;
    }

    return null;
  }

  /**
   * The keys a list-rule path names, or why it names none.
   *
   * The reading `parsePath` does: `$` for the document, a dot for each level
   * below it, and a backslash escaping the character after it - so a key
   * holding a dot is written `$.example\.com`.
   */
  function pathKeys(path: string): { keys: string[] } | { refusal: string } {
    if (path === '') return { refusal: 'names no list' };
    if (path[0] !== '$') return { refusal: `names ${path}, which does not start with $` };
    if (path.length === 1) return { refusal: 'names the whole document, which is never a list' };
    if (path[1] !== '.') return { refusal: `names ${path}, which needs a . after the $` };

    const keys: string[] = [];
    let key = '';
    let escaped = false;

    for (const character of path.slice(2)) {
      if (escaped) {
        key += character;
        escaped = false;
      } else if (character === '\\') {
        escaped = true;
      } else if (character === '.') {
        keys.push(key);
        key = '';
      } else {
        key += character;
      }
    }

    keys.push(key);

    if (keys.some((one) => one === '')) return { refusal: `names ${path}, which has an empty key` };

    return { keys };
  }

  /** What a decoded document holds at those keys, or nothing. */
  function valueAt(document: Record<string, unknown>, keys: string[]): unknown {
    let current: unknown = document;

    for (const key of keys) {
      if (current === null || typeof current !== 'object' || Array.isArray(current)) {
        return undefined;
      }

      const level = current as Record<string, unknown>;

      if (!(key in level)) return undefined;

      current = level[key];
    }

    return current;
  }

  function refusalInSections(named: string, sections: SyncSection[]): string | null {
    if (sections.length === 0) {
      return `${named} is edited by its headings, and no section says how`;
    }

    for (const [at, section] of sections.entries()) {
      const shape = shapeOf(section.action);
      const which = `Section ${at + 1} of ${named}`;

      if (shape.heading && (section.heading ?? '') === '') {
        return `${which} needs the heading it addresses, written with its # marks`;
      }

      if (shape.content && (section.content ?? '') === '') {
        return `${which} needs the content it writes`;
      }

      if (shape.patches) {
        const patches = section.patches ?? [];

        if (patches.length === 0) return `${which} substitutes nothing`;

        const empty = patches.findIndex((pair) => pair.find === '');

        if (empty >= 0) return `${which} has a substitution that finds nothing`;
      }
    }

    return null;
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

  function editorDrafts(from: SyncOverrideEditorEnvelope): Draft[] {
    return storedList<SyncFileMerge>(from.document, 'merges').map((merge, index) => ({
      merge,
      text:
        from.override_texts[index] ??
        (merge.overrides === undefined ? '' : JSON.stringify(merge.overrides, null, 2)),
    }));
  }

  function controlId(which: 'enabled' | 'document'): SyncOverrideControlId {
    return `repositories.${repositoryId}.sync.files.${which}`;
  }

  function currentEnvelope(): SyncOverrideEditorEnvelope {
    return {
      enabled: wanted,
      document: asDocument() as SyncOverrideEditorEnvelope['document'],
      override_texts: drafts.map(({ text }) => text),
    };
  }

  function stageDocument(): void {
    onChange(currentEnvelope(), controlId('document'));
  }

  function patch(index: number, change: Partial<SyncFileMerge>): void {
    drafts = patchedAt(drafts, index, {
      merge: { ...drafts[index].merge, ...change },
    });
    stageDocument();
  }

  function setText(index: number, text: string): void {
    drafts = patchedAt(drafts, index, { text });
    stageDocument();
  }

  function add(): void {
    drafts = [...drafts, { merge: { path: '' }, text: '' }];
    stageDocument();
  }

  function remove(index: number): void {
    drafts = withoutAt(drafts, index);
    stageDocument();
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

  /* A control hands back a string; a rule holds one of three words. An
     unrecognised one leaves the rule alone rather than storing something the
     engine refuses - it cannot arrive from ARRAY_STRATEGIES, and that is the
     point: nothing here has to stay true for the model to. */
  function strategyChange(selection: string): Partial<SyncArrayRule> {
    const strategy = asArrayStrategy(selection);

    return strategy === undefined ? {} : { strategy };
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

  /**
   * The path, and the strategy where the new path contradicts it.
   *
   * A strategy is only meaningful for the sort of document it edits, and the
   * engine refuses the pair rather than ignoring it: a Markdown strategy on a
   * `.json` path, or a deep merge on a `.md` one, is `ErrInvalidSpec`. The
   * strategy control cannot offer the wrong pair, but retyping the path can
   * arrive at it from the other side - so the strategy the new extension
   * contradicts is dropped here rather than saved and refused.
   *
   * Cleared rather than translated. What a row repointed at another kind of
   * file should do is a question only the person retyping the path can answer,
   * and `By extension` is the answer that asks it.
   */
  function setPath(index: number, path: string): void {
    const merge = { ...drafts[index].merge, path };

    if (
      merge.strategy !== undefined &&
      merge.strategy !== '' &&
      (merge.strategy === 'markdown') !== MARKDOWN_PATH.test(path)
    ) {
      delete merge.strategy;
    }

    drafts = patchedAt(drafts, index, { merge });
    stageDocument();
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

    if (!shapeOf(action).heading) {
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

  const rowKey = rowKeys('merge');

  function setWanted(next: boolean | null): void {
    wanted = next;
    onChange(currentEnvelope(), controlId('enabled'));
  }
</script>

<!--
@component
What one repository says about the files the organization keeps in step.

Two answers, and they are one row: whether the sync runs here at all, and
what this repository adjusts about it. A repository knows things the
template cannot - one of them ignores a directory the others do not - and
this is where that is written down.

Against the repository rather than keyed by name in the workspace's own
document, so a rename cannot orphan an adjustment. A file sync that quietly
stopped applying one would write the plain template over exactly the
customization it described.
-->

<section class="sync-pane card group-card">
  <div class="group-head">
    <h3 class="group-name">File sync</h3>
  </div>
  <p class="group-note">
    Whether the organization's files are kept in step here, and what this repository changes about
    them. Nothing reaches GitHub until a plan is approved
  </p>

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
    <div
      class={['policy-row', { 'is-unsaved': dirtyEnabled }]}
      data-unsaved={dirtyEnabled || undefined}
    >
      <span class="setting-say">
        <span class="setting-name">File sync</span>
        <span class="setting-why"
          >Whether the workspace's shared files are written in this repository at all</span
        >
      </span>
      {#if wanted === null}
        <span class="policy-value">
          <span class="setting-unmanaged">From the workspace</span>
        </span>
        <button
          class="setting-clear"
          title="Answer for this repository"
          {disabled}
          onclick={() => setWanted(true)}
        >
          <Icon name="plus" size="micro" />
        </button>
      {:else}
        <span class="policy-value">
          <span class="value-word" class:is-on={wanted}>{wanted ? 'On' : 'Off'}</span>
          <Switch checked={wanted} label="File sync" {disabled} onToggle={setWanted} />
        </span>
        <button
          class="setting-clear"
          title="Stop answering - take the value from the workspace"
          {disabled}
          onclick={() => setWanted(null)}
        >
          <Icon name="close" size="micro" />
        </button>
      {/if}
    </div>
    <div
      class={['policy-row', { 'is-unsaved': dirtyDocument }]}
      data-unsaved={dirtyDocument || undefined}
    >
      <span class="setting-say">
        <span class="setting-name">Ignored in this repository</span>
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
            stageDocument();
          }}
        />
      </div>
    </div>
  </div>

  {#if drafts.length === 0}
    <p class="form-note">This repository takes every file as the organization writes it</p>
  {/if}

  {#each drafts as draft, index (rowKey(index))}
    <article
      class={['entry-card sync-merge', { 'is-unsaved': dirtyDocument }]}
      data-unsaved={dirtyDocument || undefined}
    >
      <div class="sync-pane-row">
        <label class="sync-merge-path">
          <span class="entry-field-label">File</span>
          <input
            class="text-input"
            type="text"
            value={draft.merge.path}
            {disabled}
            placeholder="renovate.json"
            oninput={(event) => setPath(index, event.currentTarget.value)}
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
          <Button tone="quiet" {disabled} onclick={() => remove(index)}>Remove</Button>
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
                <Button tone="quiet" {disabled} onclick={() => removeSection(index, at)}
                  >Remove</Button
                >
              {/if}
            </div>

            {#if shapeOf(section.action).heading}
              <div class="sync-pane-row">
                <label class="sync-merge-heading">
                  <span class="entry-field-label">Heading</span>
                  <input
                    class="text-input"
                    type="text"
                    value={section.heading ?? ''}
                    {disabled}
                    placeholder="### Prerequisites"
                    oninput={(event) =>
                      patchSection(index, at, { heading: event.currentTarget.value })}
                  />
                </label>

                <label class="entry-field sync-merge-occurrence">
                  <span class="entry-field-label">Which one</span>
                  <input
                    class="text-input"
                    type="number"
                    min="1"
                    value={section.occurrence ?? ''}
                    {disabled}
                    oninput={(event) => setOccurrence(index, at, event.currentTarget.value)}
                  />
                </label>
              </div>
            {/if}

            {#if shapeOf(section.action).content}
              <label class="entry-field">
                <span class="entry-field-label">What this repository writes</span>
                <textarea
                  class="entry-code"
                  rows="5"
                  {disabled}
                  value={section.content ?? ''}
                  placeholder={SECTION_CONTENT_PLACEHOLDER}
                  oninput={(event) =>
                    patchSection(index, at, { content: event.currentTarget.value })}></textarea>
              </label>
            {/if}

            {#if shapeOf(section.action).patches}
              {#each section.patches ?? [] as substitution, which (`${rowKey(index)}-patch-${at}-${which}`)}
                <div class="sync-pane-row">
                  <label class="sync-merge-find">
                    <span class="entry-field-label">Find</span>
                    <input
                      class="text-input"
                      type="text"
                      value={substitution.find}
                      {disabled}
                      placeholder="make check"
                      oninput={(event) =>
                        patchSubstitution(index, at, which, { find: event.currentTarget.value })}
                    />
                  </label>

                  <label class="sync-merge-find">
                    <span class="entry-field-label">Replace with</span>
                    <input
                      class="text-input"
                      type="text"
                      value={substitution.replace}
                      {disabled}
                      placeholder="mise run check"
                      oninput={(event) =>
                        patchSubstitution(index, at, which, { replace: event.currentTarget.value })}
                    />
                  </label>

                  {#if !readOnly}
                    <Button
                      tone="quiet"
                      {disabled}
                      onclick={() => removeSubstitution(index, at, which)}>Remove</Button
                    >
                  {/if}
                </div>
              {/each}

              {#if !readOnly}
                <Button tone="quiet" {disabled} onclick={() => addSubstitution(index, at)}
                  >Add a substitution</Button
                >
              {/if}
            {/if}
          </div>
        {/each}

        {#if !readOnly}
          <Button tone="quiet" {disabled} onclick={() => addSection(index)}>Edit a section</Button>
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
            oninput={(event) => setText(index, event.currentTarget.value)}></textarea>
        </label>

        {#each draft.merge.arrays ?? [] as rule, at (`${rowKey(index)}-rule-${at}`)}
          <div class="sync-pane-row">
            <label class="sync-merge-list">
              <span class="entry-field-label">List</span>
              <input
                class="text-input"
                type="text"
                value={rule.path}
                {disabled}
                placeholder="$.packageRules"
                oninput={(event) => patchRule(index, at, { path: event.currentTarget.value })}
              />
            </label>

            <SegmentedControl
              name="repository-sync-array-{index}-{at}"
              label="What happens to {rule.path || 'this list'}"
              compact
              options={ARRAY_STRATEGIES}
              value={rule.strategy}
              {disabled}
              onSelect={(selection) => patchRule(index, at, strategyChange(selection))}
            />

            {#if !readOnly}
              <Button tone="quiet" {disabled} onclick={() => removeRule(index, at)}>Remove</Button>
            {/if}
          </div>
        {/each}

        <!-- Offered only beside a list rule, because a list with no rule is
             replaced whole and there is nothing left to deduplicate: the engine
             refuses that pair rather than ignoring the flag. -->
        {#if draft.merge.arrays?.length}
          <div class="sync-pane-row">
            <span class="sync-form-label">Drop repeated entries</span>
            <span class="sync-pane-spacer"></span>
            <Switch
              checked={draft.merge.deduplicate === true}
              bare
              label="Drop repeated entries from {draft.merge.path || 'this file'}"
              {disabled}
              onToggle={(next) => patch(index, { deduplicate: next ? true : undefined })}
            />
          </div>
        {/if}

        {#if !readOnly}
          <Button tone="quiet" {disabled} onclick={() => addRule(index)}>Add a list rule</Button>
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
  {:else if incomplete !== null}
    <p class="form-error" role="alert">{incomplete}</p>
  {/if}

  {#if !readOnly}
    <button class="add-chip add-entry" type="button" {disabled} onclick={add}>
      <Icon name="plus" size="xs" />
      <span class="t">Adjust a file</span>
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

  .policy-rows {
    margin-bottom: var(--space-2);
  }

  /* The pane's rows stand on a raised surface, so the unsaved tint mixes into that rather
     than into whatever is behind it. */
  .policy-row.is-unsaved,
  .entry-card.is-unsaved {
    background: color-mix(in srgb, var(--brand-action-tint) 45%, var(--surface-raised));
    box-shadow: inset 2px 0 var(--brand-action);
  }

  /* A block row keeps its sentence on the first line and lays the entries on a full-width
     second one. `flex-basis: 100%` is what takes that line under the row law - the old
     `grid-column: 1 / -1` addressed a grid the row no longer is. */
  .pattern-line {
    flex-basis: 100%;
    margin-block: var(--space-1) 0;
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

  .add-entry {
    margin-top: var(--space-3);
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
    color: var(--text-muted);
  }

  .sync-pane-row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    padding-block: 0.7rem;
  }

  /* The control sits at the end of its row rather than at the end of the pane:
     the spacer collapses when the row wraps, which puts the control under its
     own name at a narrow width. */
  .sync-pane-spacer {
    flex: 1;
  }

  /* Narrower than the shared-files form's, because an adjustment names a path
     the workspace already lists rather than one somebody is typing out.
     The boxes beside it share the shape and not the name: `.sync-merge-path`
     is the file this row adjusts, and a selector reaching for that must not
     also find a list rule's path or a substitution. */
  .sync-merge-path,
  /* Three named fields on one line, and the width a named field reads at is
     what decides when the line breaks rather than a guess per row. */
  .sync-merge-heading,
  .sync-merge-find,
  .sync-merge-list {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 0.25rem;
    min-width: var(--entry-say-min);
  }

  /* Wide enough for a count and no wider: it holds a small ordinal, and a box
     sized like the heading beside it would read as somewhere to type words.
     The column and its gap come from `.entry-field`, whose margin the card
     already zeroes. */
  .sync-merge-occurrence {
    width: 6rem;
  }

  /* A hairline between sections rather than a card around each: they are steps
     in one document's edit, and boxing every one of them turned a file with six
     into six files. Drawn between rather than around, so the first sits flush
     against the strategy row above it. */
  .sync-merge-section + .sync-merge-section {
    border-top: 1px solid var(--border-subtle);
    margin-top: var(--space-3);
    padding-top: var(--space-3);
  }

  /* On a phone the head's parts cannot share one line, the say keeps the
     line and the control moves under it, and the text fields lose their
     12rem floor rather than holding the page wide. */
  @media (max-width: 30rem) {
    .sync-pane.card {
      box-sizing: border-box;
      inline-size: 100%;
      max-inline-size: 100%;
      min-inline-size: 0;
      padding: var(--space-4);
    }

    .entry-card,
    .sync-pane-row {
      inline-size: 100%;
      max-inline-size: 100%;
      min-inline-size: 0;
    }

    .group-head {
      flex-wrap: wrap;
    }

    .sync-merge-path,
    .sync-merge-heading,
    .sync-merge-find,
    .sync-merge-list {
      flex-basis: 100%;
      min-width: 0;
    }

    .sync-merge-occurrence {
      width: 100%;
    }

    .sync-pane-row {
      align-items: stretch;
      flex-direction: column;
    }

    .sync-pane-row .text-input,
    .sync-pane-row :global(fieldset) {
      box-sizing: border-box;
      inline-size: 100%;
      max-inline-size: 100%;
      min-inline-size: 0;
    }

    .sync-pane-row > :global(.btn) {
      align-self: start;
    }

    .sync-pane-spacer {
      display: none;
    }

    .form-note {
      overflow-wrap: anywhere;
    }
  }
</style>
