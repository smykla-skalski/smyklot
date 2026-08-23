<script module lang="ts">
  import type { CodeLang } from '../code-tokens';
  import type { SyncFile as TemplateFile } from '../types';

  /** The language a path's extension says it is written in. */
  export function langOf(path: string): CodeLang {
    if (/\.(json|json5)$/i.test(path)) return 'json';
    if (/\.(md|markdown)$/i.test(path)) return 'markdown';
    return 'yaml';
  }

  /** Replaces one template while keeping the strict service-owned file shape. */
  export function templateDocumentWithContent(
    document: Record<string, unknown>,
    path: string,
    content: string,
  ): Record<string, unknown> {
    const files = Array.isArray(document.files) ? (document.files as TemplateFile[]) : [];
    return {
      ...document,
      files: files.map((file) => ({
        path: file.path,
        content: file.path === path ? content : file.content,
      })),
    };
  }
</script>

<script lang="ts">
  /**
   * One template's own page: what it says, and what each adjusting
   * repository turns it into. The composed copy is the surface a reader
   * studies - overridden lines wear the managed gutter bar, the keys an
   * adjustment writes stand on the patch strip with the x that removes the
   * override (never "writes the default"), and the one question a merge
   * cannot answer itself - what happens to a list both sides set - is asked
   * where it arises.
   */
  import { untrack } from 'svelte';

  import { unifiedDiff } from '../code-tokens';
  import { arrayRulePath, mergeSummary, type ArrayRule, type FileMergeSpec } from '../filemerge';
  import { composeMergedText, deriveMerge } from '../jsontext';
  import { formatRelative } from '../format';
  import { formatJson, parseJson, type JsonValue } from '../merge';
  import type {
    SyncOverrideControlId,
    SyncOverrideEditorEnvelope,
  } from '../repository-sync-override-settings';
  import type {
    SyncConfig,
    SyncFile,
    SyncFileMergeEntry,
    SyncFilesContext,
    SyncOverride,
  } from '../types';
  import type { SyncSection } from '../routes';

  import Button from './Button.svelte';
  import CodeBlock from './CodeBlock.svelte';
  import Icon from './Icon.svelte';
  import FormError from './FormError.svelte';
  import CodeEditor from './CodeEditor.svelte';
  import PanePath from './PanePath.svelte';

  const {
    config,
    savedDocument = {},
    context,
    path,
    nowMs,
    readOnly,
    problem = null,
    sectionHref,
    onOpenSection,
    onChangeDocument,
    dirtyDocument = false,
    dirtyControls = [],
    fetchOverride,
    onChangeOverride,
  }: {
    config: SyncConfig | null;
    savedDocument?: Record<string, unknown>;
    context: SyncFilesContext | null;
    /** Which template the address names. */
    path: string;
    nowMs: number;
    readOnly: boolean;
    problem?: string | null;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    onChangeDocument: (document: Record<string, unknown>) => boolean;
    dirtyDocument?: boolean;
    dirtyControls?: readonly string[];
    fetchOverride: (repositoryId: string) => Promise<{
      stored: SyncOverride;
      envelope: SyncOverrideEditorEnvelope | null;
    }>;
    onChangeOverride: (
      repositoryId: string,
      stored: SyncOverride,
      next: SyncOverrideEditorEnvelope,
      controlId: SyncOverrideControlId,
    ) => boolean;
  } = $props();

  const stored = $derived(config?.document ?? {});
  const frozen = $derived(readOnly || config?.unreadable === true || config === null);

  const files = $derived(Array.isArray(stored.files) ? (stored.files as SyncFile[]) : []);
  const file = $derived(files.find((held) => held.path === path) ?? null);
  const savedFiles = $derived(
    Array.isArray(savedDocument.files) ? (savedDocument.files as SyncFile[]) : [],
  );
  const savedFile = $derived(savedFiles.find((held) => held.path === path) ?? null);
  const templateDirty = $derived(
    dirtyDocument && (file?.content ?? '') !== (savedFile?.content ?? ''),
  );
  const lang = $derived(langOf(path));

  const merges = $derived((context?.merges ?? []).filter((entry) => entry.path === path));

  const freshness = $derived.by(() => {
    const at = config?.updated_at;
    if (at === undefined) return '';
    const by = config?.updated_by ?? '';
    const when = formatRelative(at, nowMs);
    return by === '' ? ` · updated ${when}` : ` · updated ${when} by ${by}`;
  });

  const strategyPill = $derived.by(() => {
    if (merges.length === 0) return 'replaces';
    const strategy = merges[0]?.merge.strategy;
    if (strategy === 'markdown') return 'merges · sections';
    if (strategy === 'shallow-merge') return 'merges · shallow';
    return 'merges · deep';
  });

  /* ---------- The template, editable in place ---------- */

  /* Null while untouched, so a save elsewhere refreshing the config never
     fights an edit in progress. */
  let templateDraft = $state<string | null>(null);
  let templateSource = untrack(() => file?.content ?? '');
  let pendingTemplateText: string | null = null;
  let templateUndoDepth = $state(0);
  let templateEditor = $state<CodeEditor | null>(null);
  const templateText = $derived(templateDraft ?? file?.content ?? '');

  function stageTemplate(text: string): void {
    templateDraft = text;
    if (file === null || frozen) return;
    pendingTemplateText = text;
    onChangeDocument(templateDocumentWithContent(stored, path, text));
  }

  $effect(() => {
    const source = file?.content ?? '';
    if (source === templateSource) return;
    templateSource = source;
    if (source === pendingTemplateText) {
      pendingTemplateText = null;
      return;
    }
    pendingTemplateText = null;
    untrack(() => {
      templateDraft = null;
      templateUndoDepth = 0;
    });
  });

  /* ---------- One adjustment open at a time ---------- */

  let openRepo = $state<string | null>(null);
  let sideBySide = $state(false);
  let showStored = $state(false);
  /** The open repository's whole override, fetched for its revision. */
  let held = $state<SyncOverride | null>(null);
  /** Its registry overlay, including text that is not valid JSON yet. */
  let heldEnvelope = $state<SyncOverrideEditorEnvelope | null>(null);
  let holdProblem = $state<string | null>(null);
  let rawOverrideOnly = $state(false);
  const COMPOSED_DRAFT_PREFIX = '// Smyklot composed file draft\n';

  /* The composed copy is the editable surface. Edits stage here, and the
     shared application composer writes the complete installation batch. */
  let editedText = $state<string | null>(null);
  /** The list answers given so far - stored rules, then the ask cards. */
  let answers = $state<ArrayRule[]>([]);

  /* What this page staged, keyed by repository, layered over a canonical
     context the parent has not re-read yet - null is a removed adjustment. */
  let draftMerges = $state<Record<string, FileMergeSpec | null>>({});
  let overrideFetchGeneration = 0;

  const adjusters = $derived(
    merges
      .filter((entry) => draftMerges[entry.repository_id] !== null)
      .map((entry) => {
        const kept = draftMerges[entry.repository_id];
        return kept === undefined || kept === null
          ? entry
          : { ...entry, merge: kept as SyncFileMergeEntry['merge'] };
      }),
  );

  const anyOverrideDirty = $derived(
    dirtyControls.some(
      (control) => control.startsWith('repositories.') && control.endsWith('.sync.files.document'),
    ),
  );

  function overrideDocumentControl(repositoryId: string): SyncOverrideControlId {
    return `repositories.${repositoryId}.sync.files.document`;
  }

  function overrideDirty(repositoryId: string): boolean {
    return dirtyControls.includes(overrideDocumentControl(repositoryId));
  }

  function seedEdits(merge: FileMergeSpec): void {
    editedText = file === null ? null : composeMergedText(file.content, merge);
    answers = merge.arrays ?? [];
  }

  async function toggleRow(entry: SyncFileMergeEntry): Promise<void> {
    if (openRepo === entry.repository_id) {
      overrideFetchGeneration += 1;
      openRepo = null;
      held = null;
      heldEnvelope = null;
      editedText = null;
      return;
    }
    const generation = (overrideFetchGeneration += 1);
    const repositoryId = entry.repository_id;
    openRepo = repositoryId;
    sideBySide = false;
    showStored = false;
    held = null;
    heldEnvelope = null;
    holdProblem = null;
    rawOverrideOnly = false;
    seedEdits(entry.merge as FileMergeSpec);
    try {
      const loaded = await fetchOverride(repositoryId);
      if (generation !== overrideFetchGeneration || openRepo !== repositoryId) return;
      held = loaded.stored;
      heldEnvelope = loaded.envelope;
      if (loaded.envelope === null) {
        holdProblem = loaded.stored.unreadable
          ? 'This repository file override is stored in a form this version cannot safely edit'
          : 'This repository file override is unavailable';
        return;
      }
      const rows = envelopeMerges(loaded.envelope);
      const index = rows.findIndex((merge) => merge.path === path);
      if (index < 0) {
        draftMerges = { ...draftMerges, [repositoryId]: null };
        openRepo = null;
        editedText = null;
        return;
      }
      const merge = rows[index];
      draftMerges = { ...draftMerges, [repositoryId]: merge };
      const text = loaded.envelope.override_texts[index] ?? '';
      if (text.startsWith(COMPOSED_DRAFT_PREFIX)) {
        editedText = text.slice(COMPOSED_DRAFT_PREFIX.length);
        answers = merge.arrays ?? [];
      } else if (text !== '' && !validOverrideText(text)) {
        editedText = null;
        answers = merge.arrays ?? [];
        rawOverrideOnly = true;
        holdProblem = 'Finish this incomplete raw override from the repository Sync page';
      } else {
        seedEdits(merge);
      }
    } catch (cause) {
      if (generation !== overrideFetchGeneration || openRepo !== repositoryId) return;
      holdProblem = cause instanceof Error ? cause.message : String(cause);
    }
  }

  function summaryWord(entry: SyncFileMergeEntry): string {
    const merge = entry.merge as FileMergeSpec;
    if (merge.strategy === 'markdown') {
      const sections = Array.isArray(merge.sections) ? merge.sections.length : 0;
      return `${sections} section ${sections === 1 ? 'change' : 'changes'}`;
    }
    const summary = mergeSummary(merge);
    const parts: string[] = [];
    if (summary.changed.length > 0) {
      parts.push(
        `changes ${summary.changed.length} ${summary.changed.length === 1 ? 'key' : 'keys'} - ${summary.changed.join(', ')}`,
      );
    }
    for (const listed of summary.listed) {
      parts.push(
        `${listed.strategy}s ${listed.entries} ${listed.entries === 1 ? 'entry' : 'entries'} to ${listed.key}`,
      );
    }
    if (summary.removed.length > 0) parts.push(`removes ${summary.removed.join(', ')}`);
    return parts.length === 0 ? 'no changes' : parts.join(' · ');
  }

  const openEntry = $derived(adjusters.find((entry) => entry.repository_id === openRepo) ?? null);
  const openMerge = $derived((openEntry?.merge ?? null) as FileMergeSpec | null);

  /** The override the edited copy amounts to, live as the text changes. */
  const staged = $derived.by(() => {
    if (file === null || editedText === null || openMerge === null) return null;
    return deriveMerge(file.content, editedText, openMerge.strategy ?? 'deep-merge', answers);
  });

  function specOf(overrides: Record<string, unknown>, arrays: ArrayRule[]): FileMergeSpec {
    return {
      ...openMerge,
      overrides,
      ...(arrays.length > 0 ? { arrays } : { arrays: undefined }),
    };
  }

  const openSummary = $derived(
    staged === null ? null : mergeSummary(specOf(staged.overrides, staged.arrays)),
  );

  /* A trailing comma is the seam an inserted neighbour leaves on a line
     whose content did not change - the gutter should not mark it. A comma
     alone can never be a real edit: it would not parse. */
  function unseamed(text: string): string {
    return text
      .split('\n')
      .map((line) => line.replace(/,\s*$/u, ''))
      .join('\n');
  }

  /** Which lines of the composed copy an adjustment rewrote, 1-indexed. */
  const overriddenLines = $derived.by(() => {
    if (file === null || editedText === null) return null;
    /* Both sides are the template's own bytes now - the composed copy is an
       in-place edit, so the diff marks what changed and nothing reflowed. */
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- built whole, replaced never mutated
    const marked = new Set<number>();
    let at = 0;
    for (const line of unifiedDiff(unseamed(file.content), unseamed(editedText))) {
      if (line.op === '-') continue;
      at += 1;
      if (line.op === '+') marked.add(at);
    }
    return marked;
  });

  /* ---------- Staging the open override ---------- */

  function envelopeMerges(envelope: SyncOverrideEditorEnvelope): FileMergeSpec[] {
    return Array.isArray(envelope.document.merges)
      ? (envelope.document.merges as FileMergeSpec[])
      : [];
  }

  function validOverrideText(text: string): boolean {
    if (text.trim() === '') return true;
    try {
      const value: unknown = JSON.parse(text);
      return typeof value === 'object' && value !== null && !Array.isArray(value);
    } catch {
      return false;
    }
  }

  function isJsonRecord(value: JsonValue | undefined): value is Record<string, JsonValue> {
    return (
      typeof value === 'object' &&
      value !== null &&
      !Array.isArray(value) &&
      !(typeof JSON.isRawJSON === 'function' && JSON.isRawJSON(value))
    );
  }

  function sameJson(left: unknown, right: unknown): boolean {
    if (Object.is(left, right)) return true;
    if (typeof JSON.isRawJSON === 'function' && (JSON.isRawJSON(left) || JSON.isRawJSON(right))) {
      const leftNumber = JSON.isRawJSON(left) ? Number(left.rawJSON) : left;
      const rightNumber = JSON.isRawJSON(right) ? Number(right.rawJSON) : right;
      return Object.is(leftNumber, rightNumber);
    }
    if (Array.isArray(left) && Array.isArray(right)) {
      return (
        left.length === right.length && left.every((value, index) => sameJson(value, right[index]))
      );
    }
    if (typeof left === 'object' && left !== null && typeof right === 'object' && right !== null) {
      const leftRecord = left as Record<string, unknown>;
      const rightRecord = right as Record<string, unknown>;
      const keys = Object.keys(leftRecord);
      return (
        keys.length === Object.keys(rightRecord).length &&
        keys.every(
          (key) => Object.hasOwn(rightRecord, key) && sameJson(leftRecord[key], rightRecord[key]),
        )
      );
    }
    return false;
  }

  /** Keep literal numbers from the prior override or the edited copy. */
  function rawOverrideValue(
    derived: unknown,
    composed: JsonValue | undefined,
    previous: JsonValue | undefined,
    at: string[],
    strategy: string,
    rules: readonly ArrayRule[],
  ): JsonValue {
    if (previous !== undefined && sameJson(derived, previous)) return previous;
    if (derived === null) return null;
    if (Array.isArray(derived) && Array.isArray(composed)) {
      const rule = rules.find((candidate) => candidate.path === arrayRulePath(at));
      if (rule?.strategy === 'append') {
        return derived.length === 0 ? [] : composed.slice(-derived.length);
      }
      if (rule?.strategy === 'prepend') return composed.slice(0, derived.length);
      return composed;
    }
    if (
      typeof derived === 'object' &&
      derived !== null &&
      !Array.isArray(derived) &&
      isJsonRecord(composed)
    ) {
      if (strategy === 'shallow-merge' && at.length > 0) return composed;
      const previousRecord = isJsonRecord(previous) ? previous : {};
      return Object.fromEntries(
        Object.entries(derived as Record<string, unknown>).map(([key, value]) => [
          key,
          rawOverrideValue(
            value,
            composed[key],
            previousRecord[key],
            [...at, key],
            strategy,
            rules,
          ),
        ]),
      );
    }
    return (composed ?? derived) as JsonValue;
  }

  function overrideText(
    derived: { overrides: Record<string, unknown>; arrays: ArrayRule[] },
    text: string,
    envelope: SyncOverrideEditorEnvelope,
    merge: FileMergeSpec,
  ): string {
    const index = envelopeMerges(envelope).findIndex((row) => row.path === path);
    const previousText = index < 0 ? '' : (envelope.override_texts[index] ?? '');
    const composed = parseJson(text);
    const previous = parseJson(previousText);
    if (isJsonRecord(composed)) {
      return formatJson(
        rawOverrideValue(
          derived.overrides,
          composed,
          previous,
          [],
          merge.strategy ?? 'deep-merge',
          derived.arrays,
        ) as Record<string, JsonValue>,
      ).trimEnd();
    }
    if (isJsonRecord(previous) && sameJson(derived.overrides, previous)) return previousText;
    return formatJson(derived.overrides as JsonValue).trimEnd();
  }

  /**
   * Replace this file's merge and its raw text together. Keeping both arrays
   * indexed in step preserves every other adjustment's literal text.
   */
  function envelopeWithMerge(
    envelope: SyncOverrideEditorEnvelope,
    next: FileMergeSpec | null,
    rawText: string,
  ): SyncOverrideEditorEnvelope {
    const rows = envelopeMerges(envelope);
    const texts = [...envelope.override_texts];
    const index = rows.findIndex((merge) => merge.path === path);
    const merges = [...rows];
    if (index >= 0) {
      if (next === null) {
        merges.splice(index, 1);
        texts.splice(index, 1);
      } else {
        merges[index] = next;
        texts[index] = rawText;
      }
    } else if (next !== null) {
      merges.push(next);
      texts.push(rawText);
    }
    const document = { ...envelope.document };
    if (merges.length === 0) delete document.merges;
    else {
      document.merges = merges as unknown as SyncOverrideEditorEnvelope['document'][string];
    }
    return { ...envelope, document, override_texts: texts };
  }

  function stageMergeText(text: string, nextAnswers: ArrayRule[] = answers): boolean {
    const entry = openEntry;
    const current = held;
    const envelope = heldEnvelope;
    const merge = openMerge;
    editedText = text;
    answers = nextAnswers;
    if (
      entry === null ||
      current === null ||
      envelope === null ||
      merge === null ||
      frozen ||
      current.unreadable ||
      file === null
    )
      return false;
    const derived = deriveMerge(file.content, text, merge.strategy ?? 'deep-merge', nextAnswers);
    const empty = derived !== null && Object.keys(derived.overrides).length === 0;
    const next =
      derived === null ? merge : empty ? null : specOf(derived.overrides, derived.arrays);
    /* An unfinished composed document is intentionally invalid override text.
       The registry can persist it, and the shared serializer then blocks Save
       until the editor becomes valid again. */
    const rawText =
      derived === null
        ? `${COMPOSED_DRAFT_PREFIX}${text}`
        : next === null
          ? ''
          : overrideText(derived, text, envelope, merge);
    const nextEnvelope = envelopeWithMerge(envelope, next, rawText);
    holdProblem = null;
    if (
      !onChangeOverride(
        entry.repository_id,
        current,
        nextEnvelope,
        overrideDocumentControl(entry.repository_id),
      )
    ) {
      holdProblem = 'This repository file adjustment could not be staged';
      return false;
    }
    heldEnvelope = nextEnvelope;
    draftMerges = { ...draftMerges, [entry.repository_id]: next };
    if (next === null && openRepo === entry.repository_id) {
      openRepo = null;
      editedText = null;
    }
    return true;
  }

  const mergeFrozen = $derived(
    frozen || held === null || held.unreadable || heldEnvelope === null || rawOverrideOnly,
  );

  let resultUndoDepth = $state(0);
  let resultEditor = $state<CodeEditor | null>(null);
  let pendingResultText: string | null = null;

  function stageProgrammaticText(text: string, nextAnswers: ArrayRule[]): void {
    if (stageMergeText(text, nextAnswers)) pendingResultText = text;
  }

  function stageEditorText(text: string): void {
    if (text === pendingResultText) {
      pendingResultText = null;
      return;
    }
    stageMergeText(text);
  }

  /** The x on a patch chip: the edited copy takes those lines back. */
  function dropKey(key: string): void {
    if (staged === null || file === null) return;
    const overrides = { ...staged.overrides };
    delete overrides[key];
    const path = arrayRulePath([key]);
    const keeps = (rule: ArrayRule): boolean =>
      rule.path !== path && !rule.path.startsWith(`${path}.`);
    const arrays = staged.arrays.filter(keeps);
    answers = answers.filter(keeps);
    const next = composeMergedText(file.content, specOf(overrides, arrays));
    if (next !== null) stageProgrammaticText(next, answers);
  }

  function setListRule(key: string, strategy: string): void {
    const kept = answers.filter((rule) => rule.path !== key);
    const next = strategy === 'replace' ? kept : [...kept, { path: key, strategy }];
    if (editedText !== null) stageMergeText(editedText, next);
  }

  const RULE_CHOICES = [
    { value: 'append', title: 'Append', why: "The repository's entries follow the template's" },
    { value: 'prepend', title: 'Prepend', why: "The repository's entries come first" },
    { value: 'replace', title: 'Replace', why: "The repository's list stands alone" },
  ];

  function askable(question: { canAppend: boolean; canPrepend: boolean }, value: string): boolean {
    if (value === 'append') return question.canAppend;
    if (value === 'prepend') return question.canPrepend;
    return true;
  }
</script>

<div class="view-frame">
  <PanePath
    segments={[
      { label: 'Sync', href: sectionHref('overview'), onSelect: () => onOpenSection('overview') },
      { label: 'Files', href: sectionHref('files'), onSelect: () => onOpenSection('files') },
    ]}
  />

  <header class="object-head">
    <h2 class="mono-title">{path}</h2>
    <p class="object-sub">
      {#if file === null}
        No template at this path - it may have been renamed or removed
      {:else}
        In {context?.covered ?? 0} of {context?.repositories ?? 0} repositories{freshness}
      {/if}
    </p>
  </header>

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#if file !== null}
    <div class="card" class:is-unsaved={templateDirty} data-unsaved={templateDirty || undefined}>
      <div class="card-head">
        <h3 class="card-title">Template</h3>
        <div class="head-tools">
          {#if templateUndoDepth > 0}
            <Button onclick={() => templateEditor?.undoEdit()}>
              {#snippet icon()}<Icon name="undo" size={13} />{/snippet}
              Undo
            </Button>
          {/if}
          <span class="pill pill-neutral"><span class="t">{strategyPill}</span></span>
        </div>
      </div>
      <CodeEditor
        bind:this={templateEditor}
        value={templateText}
        {lang}
        readOnly={frozen}
        onChange={stageTemplate}
        onHistory={(depth) => (templateUndoDepth = depth)}
      />
    </div>

    <div
      class="card"
      class:is-unsaved={anyOverrideDirty}
      data-unsaved={anyOverrideDirty || undefined}
    >
      <div class="card-head">
        <h3 class="card-title">Repository adjustments</h3>
        <span class="object-sum"
          >{adjusters.length} of {context?.repositories ?? 0}
          {adjusters.length === 1 ? 'repository changes' : 'repositories change'} this file</span
        >
      </div>

      {#if adjusters.length === 0}
        <p class="sync-empty">Every repository takes this file as the organization writes it</p>
      {/if}

      {#each adjusters as entry (entry.repository_id)}
        <div
          class="adjuster"
          class:is-unsaved={overrideDirty(entry.repository_id)}
          data-unsaved={overrideDirty(entry.repository_id) || undefined}
        >
          <button
            type="button"
            class="object-row"
            class:is-open={openRepo === entry.repository_id}
            aria-expanded={openRepo === entry.repository_id}
            onclick={() => void toggleRow(entry)}
          >
            <span class="object-main">
              <span class="object-name-row"><span class="file-path">{entry.repository}</span></span>
              <span class="object-sum">{summaryWord(entry)}</span>
            </span>
            <span class="object-side">
              <span class="row-chev"><Icon name="chevron-right" size={12} /></span>
            </span>
          </button>

          {#if openRepo === entry.repository_id}
            <div class="merge-result">
              {#if holdProblem !== null}
                <FormError message={holdProblem} />
              {/if}
              <div class="merge-pane-title">
                <span class="t">What {entry.repository} ends up with</span>
                <span class="pane-tools">
                  {#if editedText !== null && resultUndoDepth > 0}
                    <Button onclick={() => resultEditor?.undoEdit()}>
                      {#snippet icon()}<Icon name="undo" size={13} />{/snippet}
                      Undo
                    </Button>
                  {/if}
                  {#if editedText !== null}
                    <Button tone="quiet" onclick={() => (sideBySide = !sideBySide)}>
                      {sideBySide ? 'Hide the template' : 'Show the template beside it'}
                    </Button>
                  {/if}
                </span>
              </div>
              {#if editedText === null}
                <p class="sync-empty">
                  This copy cannot compose a {openMerge?.strategy ?? 'deep-merge'} adjustment of a
                  {lang} template - the stored override below is the whole of it
                </p>
                <CodeBlock text={JSON.stringify(openMerge, null, 2)} lang="json" />
              {:else if sideBySide}
                <div class="merge-two">
                  <div>
                    <div class="merge-pane-title"><span class="t">The template</span></div>
                    <CodeBlock text={file.content} {lang} />
                  </div>
                  <div>
                    <div class="merge-pane-title">
                      <span class="t">{entry.repository}'s copy</span>
                    </div>
                    <CodeEditor
                      bind:this={resultEditor}
                      value={editedText}
                      readOnly={mergeFrozen}
                      overridden={overriddenLines}
                      onChange={stageEditorText}
                      onHistory={(depth) => (resultUndoDepth = depth)}
                    />
                  </div>
                </div>
              {:else}
                <CodeEditor
                  bind:this={resultEditor}
                  value={editedText}
                  readOnly={mergeFrozen}
                  overridden={overriddenLines}
                  onChange={stageEditorText}
                  onHistory={(depth) => (resultUndoDepth = depth)}
                />
              {/if}
              {#if editedText !== null && staged === null}
                <p class="sync-empty">
                  Not JSON yet - the override picks the edit up when it parses again
                </p>
              {/if}

              {#if openSummary !== null && (openSummary.changed.length > 0 || openSummary.removed.length > 0 || openSummary.listed.length > 0)}
                <div class="patch-strip">
                  <span class="patch-word">This repository changes</span>
                  {#each openSummary.changed as key (key)}
                    <span class="patch-key"
                      ><span class="t">{key}</span>
                      <button
                        aria-label="Stop changing {key}"
                        disabled={mergeFrozen}
                        onclick={() => dropKey(key)}><Icon name="close" size={8} /></button
                      ></span
                    >
                  {/each}
                  {#each openSummary.removed as key (key)}
                    <span class="patch-key is-removal"
                      ><span class="t">{key}</span>
                      <button
                        aria-label="Stop removing {key}"
                        disabled={mergeFrozen}
                        onclick={() => dropKey(key)}><Icon name="close" size={8} /></button
                      ></span
                    >
                  {/each}
                  <span class="patch-word push-end">
                    <Button tone="quiet" onclick={() => (showStored = !showStored)}>
                      {showStored ? 'Hide the stored override' : 'Open the stored override'}
                    </Button>
                  </span>
                </div>
              {/if}

              {#if showStored}
                <CodeBlock text={JSON.stringify(openMerge, null, 2)} lang="json" />
              {/if}

              {#each staged?.questions ?? [] as question (question.path)}
                <div class="list-ask">
                  <span class="list-ask-word"
                    ><strong>Both set <code>{question.path}</code>.</strong> A merge cannot know how two
                    lists should combine, so this is the one question it asks:</span
                  >
                  <div class="choice-cards ask-cards">
                    {#each RULE_CHOICES as option (option.value)}
                      <label
                        class="choice-card"
                        class:is-chosen={question.chosen === option.value}
                        class:is-unaskable={!askable(question, option.value)}
                      >
                        <input
                          type="radio"
                          name="listrule-{entry.repository_id}-{question.path}"
                          checked={question.chosen === option.value}
                          disabled={mergeFrozen || !askable(question, option.value)}
                          onchange={() => setListRule(question.path, option.value)}
                        />
                        <span class="choice-dot"></span>
                        <span class="choice-title">{option.title}</span>
                        <span class="choice-why">{option.why}</span>
                      </label>
                    {/each}
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .view-frame {
    margin-inline: auto;
    max-width: var(--content-max);
  }

  .object-head {
    display: grid;
    gap: var(--space-2);
    margin-bottom: var(--space-4);
  }

  .mono-title {
    font-family: var(--mono);
    font-size: 1.375rem;
    letter-spacing: -0.01em;
    margin: 0;
  }

  .object-sub {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: round(1.5em, 1px);
    margin: 0;
    max-width: 64ch;
  }

  .card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--space-5);
  }

  .card.is-unsaved {
    border-color: color-mix(in srgb, var(--brand-action) 55%, var(--border-subtle));
    box-shadow: inset 2px 0 var(--brand-action);
  }

  .card + .card {
    margin-top: var(--space-4);
  }

  .card-title {
    font-size: var(--font-size-card-title);
    font-weight: 600;
    margin: 0;
    min-block-size: 13px;
    text-box: trim-both cap alphabetic;
  }

  .card-head {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .head-tools {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  .sync-empty {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: 0 0 var(--space-2);
  }

  .pill {
    align-items: center;
    block-size: 20px;
    border-radius: var(--radius-chip);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 0.25rem;
    line-height: 1;
    padding: 0 0.5rem;
  }

  .pill .t {
    display: block;
  }

  .pill-neutral {
    background: var(--surface-inset);
    color: var(--text-secondary);
  }

  /* ---------- The adjuster rows ---------- */

  /* One list, a hairline between neighbours - the rows read as one table
     rather than three floating cards. */
  .adjuster:not(:last-child) {
    border-bottom: 1px solid var(--border-subtle);
  }

  /* The hover pill has rounded corners; a hairline crossing its edge reads
     as a crack in it. The hovered row hides the separator under it and the
     one its neighbour would draw over it. */
  .adjuster:has(> .object-row:hover:not(:disabled)),
  .adjuster:has(+ .adjuster > .object-row:hover:not(:disabled)) {
    border-bottom-color: transparent;
  }

  .adjuster > .merge-result {
    padding-block: var(--space-2) var(--space-4);
  }

  .adjuster.is-unsaved > .object-row {
    background: color-mix(in srgb, var(--brand-action) 5%, transparent);
    box-shadow: inset 2px 0 var(--brand-action);
  }

  /* The same row the Files list stands its templates on, as a button: the
     whole surface opens the adjustment, the chevron says which way. */
  .object-row {
    align-items: center;
    background: none;
    border: 0;
    border-radius: var(--r-ctl);
    color: inherit;
    cursor: pointer;
    display: grid;
    font: inherit;
    gap: var(--space-4);
    grid-template-columns: 1fr auto;
    margin-inline: calc(var(--space-3) * -1);
    padding: 0.75rem var(--space-3);
    position: relative;
    text-align: start;
    width: calc(100% + (var(--space-3) * 2));
  }

  .object-row:hover:not(:disabled) {
    background: var(--table-row-hover);
  }

  .row-chev {
    display: inline-flex;
    transition: transform var(--duration-fast) var(--ease-out);
  }

  .object-row.is-open .row-chev {
    transform: rotate(90deg);
  }

  .object-main {
    display: grid;
    gap: var(--space-1);
  }

  .object-name-row {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-block-size: 20px;
  }

  .file-path {
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-weight: 500;
  }

  .object-sum {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .object-side {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    gap: var(--space-3);
  }

  /* ---------- The adjustment, opened ---------- */

  .merge-result {
    display: grid;
    gap: var(--space-2);
  }

  .merge-pane-title {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: var(--space-2);
    justify-content: space-between;
    letter-spacing: 0.07em;
    margin-bottom: var(--space-2);
    /* One declared height whether or not the pane carries tools, so paired
       panes start their code at the same pixel. */
    min-block-size: 28px;
    text-transform: uppercase;
  }

  .merge-pane-title .t {
    text-box: trim-both cap alphabetic;
  }

  .pane-tools {
    display: flex;
    gap: var(--space-2);
    letter-spacing: 0;
    text-transform: none;
  }

  .merge-two {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: 1fr 1fr;
  }

  @media (max-width: 64rem) {
    .merge-two {
      grid-template-columns: 1fr;
    }
  }

  .patch-strip {
    align-items: center;
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    display: flex;
    flex-wrap: wrap;
    font-size: var(--font-size-compact);
    gap: var(--space-2) var(--space-3);
    margin-top: var(--space-3);
    padding: var(--space-2) var(--space-3);
  }

  .patch-strip .patch-word {
    color: var(--text-secondary);
  }

  .push-end {
    margin-inline-start: auto;
  }

  .patch-key {
    align-items: center;
    background: var(--brand-action-tint);
    block-size: 20px;
    border-radius: var(--r-chip);
    box-sizing: border-box;
    color: var(--brand-action-text);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    gap: 0.25rem;
    line-height: 1;
    padding: 0 7px;
  }

  .patch-key.is-removal {
    background: var(--danger-tint);
    color: var(--danger);
  }

  /* A 20px disc folded around an 8px glyph - exactly the chip's height, so
     the hover fill never pokes past the pill. The same disc the ruleset
     page's condition chips wear. */
  .patch-key button {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 50%;
    color: inherit;
    cursor: pointer;
    display: inline-flex;
    margin: -0.375rem;
    opacity: 0.7;
    padding: 0.375rem;
  }

  .patch-key button:hover {
    background: var(--interactive-hover-layer);
    opacity: 1;
  }

  .patch-key button:active {
    background: var(--interactive-pressed);
  }

  /* The one question a merge cannot answer itself, asked where it arises.
     The waiting-question mark is the same inset bar managed rows wear, in
     the warning ink. */
  .list-ask {
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-3);
    padding: var(--space-3);
    position: relative;
  }

  .list-ask::before {
    background: var(--warning);
    border-radius: 2px;
    content: '';
    inset-block: var(--space-2);
    inset-inline-start: 0;
    position: absolute;
    width: 3px;
  }

  .list-ask-word {
    color: var(--text-primary);
    font-size: var(--font-size-compact);
    line-height: round(1.5em, 1px);
  }

  .list-ask-word code {
    color: var(--code-key);
    font-family: var(--mono);
    /* The mono face's taller metrics raised the line box 1.5px over the
       sans text around it; the words set the line, the key rides it. */
    line-height: 1;
  }

  .choice-cards {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(3, 1fr);
  }

  @media (max-width: 52rem) {
    .choice-cards {
      grid-template-columns: 1fr;
    }
  }

  .choice-card {
    align-content: start;
    align-items: center;
    background: var(--surface-base);
    border: 1px solid var(--control-border);
    border-radius: var(--r-strip);
    cursor: pointer;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: auto 1fr;
    padding: var(--space-3) var(--space-4);
  }

  .choice-card:hover {
    background: var(--surface-raised);
    border-color: var(--control-border-hover);
  }

  .choice-card input {
    opacity: 0;
    pointer-events: none;
    position: absolute;
  }

  .choice-dot {
    border: 1px solid var(--border-strong);
    border-radius: 50%;
    block-size: 15px;
    inline-size: 15px;
    position: relative;
  }

  .choice-card.is-chosen {
    background: var(--brand-action-tint);
    border-color: var(--brand-action);
  }

  .choice-card.is-chosen .choice-dot {
    border-color: var(--brand-action);
  }

  .choice-card.is-chosen .choice-dot::after {
    background: var(--brand-action);
    border-radius: 50%;
    content: '';
    inset: 3px;
    position: absolute;
  }

  /* An answer the edited list cannot express any more - the template's
     entries are no longer intact inside it - stays visible but cannot be
     chosen. */
  .choice-card.is-unaskable {
    cursor: default;
    opacity: 0.5;
  }

  .choice-card.is-unaskable:hover {
    background: var(--surface-base);
    border-color: var(--control-border);
  }

  .choice-title {
    font-size: var(--font-size-meta);
    font-weight: 600;
    text-box: trim-both cap alphabetic;
  }

  .choice-why {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    grid-column: 2;
  }

  @media (max-width: 36rem) {
    .card {
      padding: var(--space-4);
    }

    .card-head {
      align-items: start;
      flex-wrap: wrap;
    }

    .card-head > .object-sum {
      flex-basis: 100%;
      overflow-wrap: anywhere;
    }

    .object-main,
    .object-sum {
      min-inline-size: 0;
    }

    .object-sum,
    .file-path {
      overflow-wrap: anywhere;
    }

    .merge-pane-title {
      align-items: start;
      flex-direction: column;
    }

    .pane-tools {
      flex-wrap: wrap;
    }

    .patch-strip {
      align-items: start;
    }

    .push-end {
      margin-inline-start: 0;
    }
  }
</style>
