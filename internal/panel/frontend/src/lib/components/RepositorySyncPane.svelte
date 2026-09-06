<script lang="ts">
  import { untrack } from 'svelte';
  import { fileFormat } from '#lib/file-format.js';
  import { revealFileAdjustment } from '../file-adjustment-link';
  import {
    cloneSettingsJson,
    sameSettingsJson,
    type SettingsJson,
  } from '../settings-draft-storage';
  import { patchedAt, storedList, withoutAt } from '#lib/form-lists.js';
  import { formatRelative } from '#lib/format.js';
  import { parseJson } from '#lib/merge.js';
  import {
    buildSyncOverrideEditorEnvelope,
    type SyncOverrideControlId,
    type SyncOverrideEditorEnvelope,
  } from '#lib/repository-sync-override-settings.js';
  import type { SyncFileMerge, SyncOverride, SyncPatch, SyncSection } from '#lib/types.js';

  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import FileEditor from './FileEditor.svelte';
  import FormError from './FormError.svelte';
  import IconButton from './IconButton.svelte';
  import Modal from './Modal.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Select from './Select.svelte';
  import StructuredMergeRules from './StructuredMergeRules.svelte';
  import Switch from './Switch.svelte';

  const occurrenceHelpId = $props.id();

  const {
    stored,
    repositoryId,
    envelope = undefined,
    revealPath = null,
    readOnly,
    now,
    dirtyEnabled = false,
    dirtyDocument = false,
    onChange = () => {},
  }: {
    stored: SyncOverride;
    repositoryId: string;
    envelope?: SyncOverrideEditorEnvelope | undefined;
    revealPath?: string | null;
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

  /*
   * Offered only for a Markdown path. The structured choices live in the
   * merge-rules inspector. The engine refuses either crossed over, and the engine this replaces
   * did not: it let a Markdown strategy be configured for a JSON file,
   * discovered it at apply time, and wrote the raw template over the
   * repository's copy. A choice that cannot be made is a refusal nobody has to
   * read.
   */
  const MARKDOWN_STRATEGIES = [
    { value: '', label: 'By extension' },
    { value: 'markdown', label: 'Markdown' },
  ] as const;

  /** What one section does. Which fields it needs follows from it. */
  const SECTION_ACTIONS = [
    { value: 'after', label: 'After section' },
    { value: 'before', label: 'Before section' },
    { value: 'replace', label: 'Replace section' },
    { value: 'delete', label: 'Delete section' },
    { value: 'patch', label: 'Replace text in section' },
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
  type Draft = { id: number; sectionIds: number[]; merge: SyncFileMerge; text: string };
  let nextIdentity = 0;

  const controlledEnvelope = $derived(
    stored.unreadable
      ? ({
          enabled: stored.enabled,
          document: {},
          override_texts: [],
        } satisfies SyncOverrideEditorEnvelope)
      : (envelope ?? buildSyncOverrideEditorEnvelope(stored)),
  );
  let drafts = $state<Draft[]>(untrack(() => editorDrafts(controlledEnvelope)));
  let lastDraftSnapshot = untrack(() => draftSnapshot(controlledEnvelope));
  // Registry echoes preserve the editors and their local undo history. An external
  // restore starts fresh editors, so a discarded document cannot return through Undo.
  $effect(() => {
    const source = controlledEnvelope;
    const snapshot = draftSnapshot(source);
    untrack(() => {
      if (sameSettingsJson(snapshot, lastDraftSnapshot)) return;
      lastDraftSnapshot = snapshot;
      drafts = editorDrafts(source);
    });
  });
  let excludes = $derived<string[]>(storedList<string>(controlledEnvelope.document, 'excludes'));
  let wanted = $derived<boolean | null>(controlledEnvelope.enabled);

  const disabled = $derived(readOnly || stored.unreadable);
  let locatedRequest = $state<string | null>(null);
  const revealRequest = $derived(JSON.stringify([repositoryId, revealPath]));
  const unadjustedPath = $derived(
    !stored.unreadable &&
      revealPath !== null &&
      locatedRequest !== revealRequest &&
      !drafts.some((draft) => draft.merge.path === revealPath)
      ? revealPath
      : null,
  );
  function revealExistingAdjustment(node: HTMLElement): (() => void) | undefined {
    return untrack(() => {
      // A handoff reveals once. Renaming this draft away and back must leave
      // focus in its input rather than replaying the original navigation.
      if (locatedRequest === revealRequest) return;
      locatedRequest = revealRequest;
      return revealFileAdjustment(node);
    });
  }
  let rulesDraftId = $state<number | null>(null);
  let rulesTrigger = $state<HTMLElement | null>(null);
  const rulesIndex = $derived(
    drafts.findIndex((draft) => draft.id === rulesDraftId && !editsMarkdown(draft.merge)),
  );
  const rulesDraft = $derived(rulesIndex < 0 ? null : drafts[rulesIndex]);

  // A restored or removed file has a different identity. Never let an open
  // inspector start editing whichever file now occupies its former index.
  $effect(() => {
    if (rulesDraftId !== null && rulesDraft === null) untrack(closeRules);
  });

  function openRules(id: number, trigger: HTMLElement): void {
    rulesTrigger = trigger;
    rulesDraftId = id;
  }

  function closeRules(): void {
    const trigger = rulesTrigger;
    rulesDraftId = null;
    rulesTrigger = null;
    queueMicrotask(() => {
      if (trigger?.isConnected) trigger.focus({ preventScroll: true });
    });
  }

  function changeRules(id: number, change: Partial<SyncFileMerge>): void {
    const index = drafts.findIndex((draft) => draft.id === id);
    if (index >= 0 && !disabled && !editsMarkdown(drafts[index].merge)) patch(index, change);
  }

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

    return fileFormat(merge.path) === 'markdown';
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

    if (fileFormat(path) === null) {
      return `${path} has no extension this can merge; JSON, JSONC, YAML, TOML and Markdown can`;
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
    const value = parseJson(text);
    return value !== null &&
      typeof value === 'object' &&
      !Array.isArray(value) &&
      !(typeof JSON.isRawJSON === 'function' && JSON.isRawJSON(value))
      ? (value as Record<string, unknown>)
      : undefined;
  }

  function editorDrafts(from: SyncOverrideEditorEnvelope): Draft[] {
    return storedList<SyncFileMerge>(from.document, 'merges').map((merge, index) => ({
      id: ++nextIdentity,
      sectionIds: (merge.sections ?? []).map(() => ++nextIdentity),
      merge,
      text:
        from.override_texts[index] ??
        (merge.overrides === undefined ? '' : JSON.stringify(merge.overrides, null, 2)),
    }));
  }

  function draftSnapshot(from: SyncOverrideEditorEnvelope): SettingsJson {
    return cloneSettingsJson([repositoryId, from.document.merges ?? [], from.override_texts]);
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
    publish('document');
  }

  function publish(control: 'enabled' | 'document'): void {
    const next = currentEnvelope();
    lastDraftSnapshot = draftSnapshot(next);
    onChange(next, controlId(control));
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

  function add(path = ''): void {
    if (disabled) return;
    drafts = [...drafts, { id: ++nextIdentity, sectionIds: [], merge: { path }, text: '' }];
    stageDocument();
  }

  function remove(index: number): void {
    drafts = withoutAt(drafts, index);
    stageDocument();
  }

  /* The rows inside a row. Each list is edited through the merge it belongs to,
     so every one of these ends at `patch`, and a new list rather than an edit
     in place is what makes the draft compare unequal to what is stored. */
  function sectionsOf(index: number): SyncSection[] {
    return drafts[index].merge.sections ?? [];
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
      (merge.strategy === 'markdown') !== (fileFormat(path) === 'markdown')
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
    drafts = patchedAt(drafts, index, {
      sectionIds: [...drafts[index].sectionIds, ++nextIdentity],
    });
    patch(index, { sections: [...sectionsOf(index), { action: 'after', heading: '' }] });
  }

  function removeSection(index: number, at: number): void {
    drafts = patchedAt(drafts, index, { sectionIds: withoutAt(drafts[index].sectionIds, at) });
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

  function setWanted(next: boolean | null): void {
    wanted = next;
    publish('enabled');
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

<Card class="sync-pane group-card" label="File sync" unsaved={dirtyEnabled || dirtyDocument}>
  <div class="card-head">
    <h2 class="card-title">File sync</h2>
    {#if !readOnly}
      <Button tone="quiet" {disabled} onclick={() => add()}
        >{#snippet icon()}<Icon name="plus" size="sm" />{/snippet}Adjust a file</Button
      >
    {/if}
  </div>

  {#if unadjustedPath !== null}
    <div
      class="policy-row file-adjustment-target"
      tabindex="-1"
      role="group"
      aria-label="Adjustment for {unadjustedPath}"
      {@attach revealFileAdjustment}
    >
      <span class="setting-say">
        <span class="setting-name">{unadjustedPath}</span>
        <span class="setting-why">Uses the shared template without content adjustments</span>
      </span>
      {#if !readOnly}
        <span class="policy-value"
          ><Button {disabled} onclick={() => add(unadjustedPath!)}>Add adjustment</Button></span
        >
      {/if}
    </div>
  {/if}

  <!-- What the planner made of this repository, which is the question somebody
       opening this pane came to ask. A refusal is fail-closed and correct, and
       before this the only account of it was a line in the service log. -->
  {#if notSyncing !== null}
    <p class="form-notice sync-pane-standdown" role="status">
      <strong>Shared files are not being synced here</strong>
      <span>{notSyncing.reason}</span>
      {#if notSyncing.when !== null}
        <span class="sync-pane-standdown-when">Last checked {notSyncing.when}</span>
      {/if}
    </p>
  {/if}

  {#if stored.unreadable}
    <p class="form-notice" role="alert">
      This version of Smyklot cannot read these saved file adjustments · Editing is unavailable
    </p>
  {/if}

  <div class="policy-rows" class:rows-continue={drafts.length > 0}>
    <div
      class={['policy-row', { 'is-unsaved': dirtyEnabled }]}
      data-unsaved={dirtyEnabled || undefined}
    >
      <span class="setting-say">
        <span class="setting-name">File sync</span>
        <span class="setting-why">Keep shared files up to date through automatic pull requests</span
        >
      </span>
      {#if wanted === null}
        <span class="policy-value">
          <span class="setting-unmanaged">From the workspace</span>
        </span>
        <button
          class="setting-clear"
          title="Set file sync for this repository"
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
          title="Restore the workspace default"
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
          >Matching files stay untouched · Use * to match any characters</span
        >
      </span>
      <div class="policy-value">
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
    <p class="form-note">No content adjustments for this repository</p>
  {/if}

  <span id={occurrenceHelpId} class="visually-hidden"
    >Leave blank when the heading appears once; otherwise enter its number</span
  >
  {#each drafts as draft, index (draft.id)}
    <article
      class={['sync-merge', { 'is-unsaved': dirtyDocument }]}
      data-unsaved={dirtyDocument || undefined}
      tabindex="-1"
      aria-label="Adjustment for {draft.merge.path || 'unnamed file'}"
      {@attach revealPath === draft.merge.path ? revealExistingAdjustment : undefined}
    >
      <div class="sync-pane-row file-heading">
        <label class="sync-merge-path">
          <span class="setting-name">File</span>
          <input
            class="text-input"
            type="text"
            value={draft.merge.path}
            {disabled}
            placeholder="renovate.json"
            oninput={(event) => setPath(index, event.currentTarget.value)}
          />
        </label>

        {#if !editsMarkdown(draft.merge) || !readOnly}
          <div class="file-actions">
            {#if !editsMarkdown(draft.merge)}
              <Button
                tone="quiet"
                aria-haspopup="dialog"
                aria-expanded={rulesDraftId === draft.id}
                aria-controls={`repository-merge-rules-${repositoryId}`}
                onclick={(event) => openRules(draft.id, event.currentTarget)}
              >
                {#snippet icon()}<Icon name="sliders" size="sm" />{/snippet}Merge rules
              </Button>
            {/if}
            {#if !readOnly}
              <IconButton
                toolbar
                icon="trash"
                label="Remove adjustment for {draft.merge.path || 'this file'}"
                {disabled}
                onclick={() => remove(index)}
              />
            {/if}
          </div>
        {/if}
      </div>

      {#if editsMarkdown(draft.merge)}
        <div class="sync-pane-row composition-row">
          <span class="setting-name">Combine content</span>
          <SegmentedControl
            name="repository-sync-strategy-{index}"
            label="How {draft.merge.path || 'this file'} is composed"
            compact
            options={MARKDOWN_STRATEGIES}
            value={draft.merge.strategy ?? ''}
            {disabled}
            onSelect={(selection) => patch(index, { strategy: selection })}
          />
        </div>
        <!-- Markdown is edited by its headings, so the keys-and-lists controls
             are not shown rather than shown and refused. Which one a row gets
             follows the engine's own reading of the strategy and the extension. -->
        {#each draft.merge.sections ?? [] as section, at (draft.sectionIds[at])}
          <div class="sync-merge-section">
            <div class="sync-pane-row section-action-row">
              <span class="setting-name">Section action</span>
              <div class="file-actions">
                <Select
                  aria-label="Action for section {at + 1} of {draft.merge.path || 'this file'}"
                  options={SECTION_ACTIONS}
                  value={section.action}
                  {disabled}
                  onValueChange={(selection) => setAction(index, at, selection)}
                />
                {#if !readOnly}
                  <IconButton
                    toolbar
                    icon="trash"
                    label="Remove section adjustment {at + 1} for {draft.merge.path || 'this file'}"
                    {disabled}
                    onclick={() => removeSection(index, at)}
                  />
                {/if}
              </div>
            </div>

            {#if shapeOf(section.action).heading}
              <div class="sync-pane-row">
                <label class="form-field sync-merge-heading">
                  <span class="form-label">Heading</span>
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

                <label class="form-field sync-merge-occurrence">
                  <span class="form-label">Occurrence</span>
                  <input
                    class="text-input"
                    type="number"
                    placeholder="Unique"
                    aria-describedby={occurrenceHelpId}
                    min="1"
                    value={section.occurrence ?? ''}
                    {disabled}
                    oninput={(event) => setOccurrence(index, at, event.currentTarget.value)}
                  />
                </label>
              </div>
            {/if}

            {#if shapeOf(section.action).content}
              <FileEditor
                label="What this repository writes"
                headingLevel={3}
                value={section.content ?? ''}
                lang="markdown"
                readOnly={disabled}
                terminalNewline={false}
                onChange={(content) => patchSection(index, at, { content })}
              />
            {/if}

            {#if shapeOf(section.action).patches}
              {#each section.patches ?? [] as substitution, which (`${draft.sectionIds[at]}-patch-${which}`)}
                <div class="replacement-row">
                  <label class="form-field sync-merge-find">
                    <span class="form-label">Find</span>
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

                  <div class="replacement-value">
                    <label class="form-field sync-merge-find">
                      <span class="form-label">Replace with</span>
                      <input
                        class="text-input"
                        type="text"
                        value={substitution.replace}
                        {disabled}
                        placeholder="mise run check"
                        oninput={(event) =>
                          patchSubstitution(index, at, which, {
                            replace: event.currentTarget.value,
                          })}
                      />
                    </label>

                    {#if !readOnly}
                      <IconButton
                        toolbar
                        icon="trash"
                        label="Remove replacement {which + 1} from section {at + 1} for {draft.merge
                          .path || 'this file'}"
                        {disabled}
                        onclick={() => removeSubstitution(index, at, which)}
                      />
                    {/if}
                  </div>
                </div>
              {/each}

              {#if !readOnly}
                <div class="form-row">
                  <span class="setting-name">Replacements</span>
                  <Button tone="quiet" {disabled} onclick={() => addSubstitution(index, at)}
                    >Add a replacement</Button
                  >
                </div>
              {/if}
            {/if}
          </div>
        {/each}

        {#if !readOnly}
          <div class="form-row">
            <span class="setting-name">Sections</span>
            <Button tone="quiet" {disabled} onclick={() => addSection(index)}>Edit a section</Button
            >
          </div>
        {/if}
      {:else}
        <FileEditor
          label="Content adjustments"
          description="Keys override the shared template · null removes a key"
          headingLevel={3}
          value={draft.text}
          lang="json"
          readOnly={disabled}
          terminalNewline={false}
          onChange={(text) => setText(index, text)}
        />
      {/if}
    </article>
  {/each}

  {#if rulesDraft === null && malformed >= 0}
    <p class="form-error" role="alert">
      Enter a valid JSON object for {drafts[malformed]?.merge.path || 'this file'}
    </p>
  {:else if rulesDraft === null && incomplete !== null}
    <p class="form-error" role="alert">{incomplete}</p>
  {/if}
</Card>

{#if rulesDraft !== null}
  {@const selectedDraft = rulesDraft}
  {@const rulesProblem =
    values[rulesIndex] === undefined
      ? `Enter a valid JSON object for ${rulesDraft.merge.path || 'this file'}`
      : refusalIn(rulesDraft, values[rulesIndex])}
  <Modal
    id={`repository-merge-rules-${repositoryId}`}
    open
    title="Merge rules"
    description={rulesDraft.merge.path || 'New file adjustment'}
    variant="inspector"
    returnFocus={rulesTrigger}
    onClose={closeRules}
  >
    {#snippet headerExtra()}
      <IconButton toolbar icon="close" label="Close merge rules" onclick={closeRules} />
    {/snippet}
    <StructuredMergeRules
      merge={rulesDraft.merge}
      idPrefix={`repository-merge-${repositoryId}-${rulesDraft.id}`}
      {disabled}
      {readOnly}
      onChange={(change) => changeRules(selectedDraft.id, change)}
    />
    {#if rulesProblem !== null}<FormError message={rulesProblem} />{/if}
    {#snippet footer()}<Button onclick={closeRules}>Done</Button>{/snippet}
  </Modal>
{/if}

<style>
  .sync-merge,
  .file-adjustment-target {
    scroll-margin-block-start: var(--space-4);
  }
  .sync-pane-standdown {
    display: grid;
    gap: var(--row-copy-gap);
  }
  .sync-pane-standdown-when {
    color: var(--text-muted);
  }
  .sync-merge {
    container: file-adjustment / inline-size;
    display: grid;
    gap: var(--space-4);
    min-inline-size: 0;
    padding-block: var(--space-4);
  }
  /* The card owns the outer inset, so the terminal editor sheds its separator padding. */
  .sync-merge:last-child {
    padding-block-end: 0;
  }
  .sync-merge + .sync-merge {
    border-top: 1px solid transparent;
  }
  .sync-merge:not(.is-unsaved) + .sync-merge:not(.is-unsaved) {
    border-top-color: var(--border-subtle);
  }
  .sync-pane-row {
    display: flex;
    align-items: end;
    flex-wrap: wrap;
    gap: var(--space-3);
    min-inline-size: 0;
  }
  .file-heading {
    align-items: end;
    justify-content: space-between;
  }
  .file-actions {
    align-items: center;
    display: flex;
    flex: 0 0 auto;
    gap: var(--space-2);
    margin-inline-start: auto;
    max-inline-size: 100%;
  }
  .composition-row {
    align-items: center;
    justify-content: space-between;
  }
  .section-action-row {
    align-items: center;
    justify-content: space-between;
  }
  .replacement-row {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }
  .replacement-value {
    align-items: end;
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--space-3);
  }
  .sync-merge-path,
  .sync-merge-heading,
  .sync-merge-find {
    display: grid;
    flex: 1 1 12rem;
    gap: var(--row-copy-gap);
    min-inline-size: 0;
  }
  .sync-merge-path .text-input {
    font-family: var(--mono);
    inline-size: 28ch;
    max-inline-size: 100%;
    min-inline-size: min(18ch, 100%);
  }
  .file-heading .sync-merge-path {
    flex: 0 1 auto;
    inline-size: fit-content;
    max-inline-size: 100%;
  }
  @supports (field-sizing: content) {
    .sync-merge-path .text-input {
      field-sizing: content;
      inline-size: auto;
      max-inline-size: min(40ch, 100%);
    }
  }
  .sync-merge-occurrence {
    grid-template-columns: minmax(0, 1fr);
    inline-size: 6rem;
    margin: 0;
  }
  .sync-merge-occurrence .text-input {
    inline-size: 100%;
    min-inline-size: 0;
  }
  .sync-merge-section {
    display: grid;
    gap: var(--space-4);
  }
  .sync-merge-section + .sync-merge-section {
    border-top: 1px solid var(--border-subtle);
    padding-block-start: var(--space-4);
  }
  @container file-adjustment (max-width: 25rem) {
    .section-action-row {
      align-items: start;
      flex-direction: column;
    }
    .replacement-row {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  @media (max-width: 30rem) {
    .sync-merge-heading,
    .sync-merge-find {
      flex-basis: 100%;
    }
    .sync-pane-row :global(fieldset) {
      max-inline-size: 100%;
    }
  }
</style>
