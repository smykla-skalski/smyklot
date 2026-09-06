<script module lang="ts">
  import type { CodeLang } from '../code-tokens';
  import { formattingOverrideCount } from '../formatting';
  import type { FormattingPatch as TemplateFormattingPatch } from '../formatting';
  import type { SyncFile as TemplateFile } from '../types';
  import { terminateTemplate } from '../template-content';

  /** The language a path's extension says it is written in. */
  export function langOf(path: string): CodeLang {
    if (/\.jsonc?$/i.test(path)) return 'json';
    if (/\.toml$/i.test(path)) return 'toml';
    if (/\.ya?ml$/i.test(path)) return 'yaml';
    if (/\.(md|markdown)$/i.test(path)) return 'markdown';
    return 'text';
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
        ...file,
        content: file.path === path ? terminateTemplate(content) : file.content,
      })),
    };
  }

  /** Replaces one template's sparse policy without rebuilding its document by hand. */
  export function templateDocumentWithFormatting(
    document: Record<string, unknown>,
    path: string,
    formatting: TemplateFormattingPatch,
  ): Record<string, unknown> {
    const files = Array.isArray(document.files) ? (document.files as TemplateFile[]) : [];
    return {
      ...document,
      files: files.map((file) => {
        if (file.path !== path) return file;
        const next = { ...file };
        if (formattingOverrideCount(formatting) === 0) delete next.formatting;
        else next.formatting = formatting;
        return next;
      }),
    };
  }
</script>

<!--
@component
One template's own page: what it says, and what each adjusting
repository turns it into. The composed copy is the surface a reader
studies - overridden lines wear the managed gutter bar, the keys an
adjustment writes stand on the patch strip with the x that removes the
override (never "writes the default"), and the one question a merge
cannot answer itself - what happens to a list both sides set - is asked
where it arises.
-->

<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { goto } from '$app/navigation';

  import { unifiedDiff } from '../code-tokens';
  import { fileFormat } from '../file-format';
  import { fileAdjustmentHref } from '../file-adjustment-link';
  import { arrayRulePath, mergeSummary, type ArrayRule, type FileMergeSpec } from '../filemerge';
  import {
    composeMergedText,
    deriveMerge,
    composedEditProblem,
    sameComposedContent,
    type MergeIntent,
  } from '../jsontext';
  import { formatRelative } from '../format';
  import { FORMATTING_FIELDS, formattingPatchValue, type FormattingPatch } from '../formatting';
  import { formatJson, parseJson, type JsonValue } from '../merge';
  import { cloneSettingsJson, type SettingsJson } from '../settings-draft-storage';
  import type {
    SyncOverrideControlId,
    SyncOverrideEditorEnvelope,
  } from '../repository-sync-override-settings';
  import {
    buildSyncOverrideEditorEnvelope,
    syncOverrideFormattingEntries,
    withSyncOverrideFormatting,
  } from '../repository-sync-override-settings';
  import type {
    SyncConfig,
    SyncFile,
    SyncFileMerge,
    SyncFileMergeEntry,
    SyncFileRepositoryPolicy,
    SyncFilesContext,
    SyncOverride,
  } from '../types';
  import type { SyncFileRenderInput, SyncFileRenderResponse } from '../sync-file-render.generated';
  import { SYNC_SECTION_LABELS, type SyncSection } from '../routes';

  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import CodeBlock from './CodeBlock.svelte';
  import Icon from './Icon.svelte';
  import FormError from './FormError.svelte';
  import CodeEditor from './CodeEditor.svelte';
  import Modal from './Modal.svelte';
  import SearchField from './SearchField.svelte';
  import EmptyState from './EmptyState.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import FormattingEditor from './FormattingEditor.svelte';
  import FileEditor from './FileEditor.svelte';
  import IconButton from './IconButton.svelte';
  import PageHeader from './PageHeader.svelte';

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
    renderFile,
    onFormattingValidity,
    onChangeOverride,
    repositoryHref = null,
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
    renderFile: (input: SyncFileRenderInput) => Promise<SyncFileRenderResponse>;
    onFormattingValidity: (control: string, valid: boolean, message: string) => void;
    onChangeOverride: (
      repositoryId: string,
      stored: SyncOverride,
      next: SyncOverrideEditorEnvelope,
      controlId: SyncOverrideControlId,
    ) => boolean;
    repositoryHref?: ((repository: string) => string) | null;
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

  const reach = $derived(
    file === null
      ? 'No template at this path - it may have been renamed or removed'
      : `In ${context?.covered ?? 0} of ${context?.repositories ?? 0} repositories${freshness}`,
  );

  /* ---------- The template, editable in place ---------- */

  /* Null while untouched, so a save elsewhere refreshing the config never
     fights an edit in progress. */
  let templateDraft = $state<string | null>(null);
  let templateSource = untrack(() => file?.content ?? '');
  let pendingTemplateText: string | null = null;

  let templateEditor = $state<FileEditor | null>(null);
  const templateText = $derived(templateDraft ?? file?.content ?? '');
  const templateFormatting = $derived(file?.formatting ?? {});
  const savedTemplateFormatting = $derived(savedFile?.formatting ?? {});
  const dirtyTemplateFormatting = $derived(
    FORMATTING_FIELDS.filter(
      (field) =>
        formattingPatchValue(templateFormatting, field) !==
        formattingPatchValue(savedTemplateFormatting, field),
    ).map((field) => field.key),
  );

  let templateRender = $state<SyncFileRenderResponse | null>(null);
  let templateRendering = $state(false);
  let templateOptionsOpen = $state(false);
  let templateOptionsTrigger = $state<HTMLElement | null>(null);
  let renderGeneration = 0;

  const templateMismatch = $derived(
    templateRender?.valid === true && !templateRender.matches_formatting,
  );
  const templateDiagnostic = $derived(
    templateRender?.diagnostics.map(({ message }) => message).join(' · ') ?? '',
  );

  function templateRenderInput(): SyncFileRenderInput {
    return {
      path,
      draft_content: templateText,
      template_formatting: templateFormatting,
    };
  }

  function repositoryRenderInput(entry: RepositoryRow): SyncFileRenderInput {
    return {
      ...templateRenderInput(),
      repository: {
        id: entry.repository_id,
        path_formatting: entry.formatting ?? {},
        ...(entry.merge === undefined ? {} : { merge: repositoryMergeInput(entry) }),
      },
    };
  }

  async function refreshTemplateRender(
    input: SyncFileRenderInput,
    generation: number,
  ): Promise<void> {
    if (generation !== renderGeneration) return;
    templateRendering = true;
    try {
      const rendered = await renderFile(input);
      if (generation !== renderGeneration) return;
      templateRender = rendered;
    } catch (cause) {
      if (generation !== renderGeneration) return;
      const message = cause instanceof Error ? cause.message : String(cause);
      templateRender = {
        valid: false,
        final_content: '',
        matches_formatting: false,
        diagnostics: [{ stage: 'request', code: 'render_failed', message }],
      };
    } finally {
      if (generation === renderGeneration) templateRendering = false;
    }
  }

  $effect(() => {
    const heldFile = file;
    void templateText;
    void templateFormatting;
    if (heldFile === null) return;
    const generation = (renderGeneration += 1);
    const input = templateRenderInput();
    const timer = setTimeout(() => void refreshTemplateRender(input, generation), 120);
    return () => {
      clearTimeout(timer);
    };
  });

  function stageTemplate(text: string): void {
    templateDraft = text;
    if (file === null || frozen) return;
    pendingTemplateText = text;
    onChangeDocument(templateDocumentWithContent(stored, path, text));
  }

  function stageTemplateFormatting(formatting: FormattingPatch): void {
    if (file === null || frozen) return;
    onChangeDocument(templateDocumentWithFormatting(stored, path, formatting));
  }

  function applyTemplateFormatting(): void {
    if (templateRender?.valid !== true || !templateMismatch || frozen || templateRendering) return;
    templateEditor?.replaceValue(templateRender.final_content);
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
    });
  });

  /* ---------- One adjustment open at a time ---------- */

  let openRepo = $state<string | null>(null);
  let showStored = $state(false);
  /** The open repository's whole override, fetched for its revision. */
  let held = $state<SyncOverride | null>(null);
  /** Its registry overlay, including text that is not valid JSON yet. */
  let heldEnvelope = $state<SyncOverrideEditorEnvelope | null>(null);
  let holdProblem = $state<string | null>(null);
  let rawOverrideOnly = $state(false);
  const COMPOSED_DRAFT_PREFIX = '// Smyklot composed file draft\n';

  /* The composed copy is the editable surface. Edits stage here, and the
     shared application composer writes the complete workspace batch. */
  let editedText = $state<string | null>(null);
  /** The list answers given so far - stored rules, then the ask cards. */
  let answers = $state<ArrayRule[]>([]);

  /* What this page staged, keyed by repository, layered over a canonical
     context the parent has not re-read yet - null is a removed adjustment. */
  let draftMerges = $state<Record<string, FileMergeSpec | null>>({});
  let draftFormats = $state<Record<string, FormattingPatch | null>>({});
  let overrideFetchGeneration = 0;
  // Restoring the opening text must also restore authored leaves that happen to
  // equal the template, and their literal spellings, so Undo leaves no dirty draft.
  type AdjustmentSnapshot = { text: string; merge: FileMergeSpec; overrideText: string };
  type ResultEditContext = { snapshot?: AdjustmentSnapshot | null; answers: ArrayRule[] };
  let initialAdjustment: AdjustmentSnapshot | null = null;
  let savedAdjustment: AdjustmentSnapshot | null = null;
  let openingMerge: FileMergeSpec | null = null;
  let openingAnswers: ArrayRule[] = [];
  let resultEditContext = $state.raw<ResultEditContext | undefined>(undefined);

  type RepositoryRow = SyncFileRepositoryPolicy & {
    path: string;
    merge?: SyncFileMergeEntry['merge'];
    formatting?: FormattingPatch;
  };

  const repositoryRows = $derived(
    (context?.repository_policies ?? []).map((repository): RepositoryRow => {
      const storedAdjustment = merges.find(
        (entry) => entry.repository_id === repository.repository_id,
      );
      const heldMerge = draftMerges[repository.repository_id];
      const heldFormatting = draftFormats[repository.repository_id];
      return {
        ...repository,
        path,
        ...(heldMerge === null ? {} : { merge: heldMerge ?? storedAdjustment?.merge }),
        ...(heldFormatting === null
          ? {}
          : { formatting: heldFormatting ?? storedAdjustment?.formatting }),
      };
    }),
  );
  const adjustedCount = $derived(
    repositoryRows.filter((entry) => entry.merge !== undefined || entry.formatting !== undefined)
      .length,
  );

  let repositorySearch = $state('');
  let showAllRepositories = $state(false);
  let repositoryTab = $state('content');
  let repositoryPreviousTab = $state('content');
  let repositoryTrigger = $state<HTMLElement | null>(null);
  let handoffBusy = $state(false);
  let handoffGeneration = 0;
  let mounted = true;
  const matchingRepositories = $derived(
    repositoryRows
      .filter((entry) =>
        entry.repository.toLowerCase().includes(repositorySearch.trim().toLowerCase()),
      )
      .toSorted(
        (a, b) =>
          Number(b.merge !== undefined || b.formatting !== undefined) -
            Number(a.merge !== undefined || a.formatting !== undefined) ||
          a.repository.localeCompare(b.repository),
      ),
  );
  const visibleRepositories = $derived(
    repositorySearch.trim() !== '' || showAllRepositories
      ? matchingRepositories
      : matchingRepositories.slice(0, 8),
  );
  function closeRepository(): void {
    handoffGeneration += 1;
    handoffBusy = false;
    if (openEntry !== null) void toggleRow(openEntry);
  }

  async function followAdjustment(event: MouseEvent): Promise<void> {
    // Modified clicks keep ordinary link behavior, including opening a new tab.
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
    const href = (event.currentTarget as HTMLAnchorElement).href;
    event.preventDefault();
    if (handoffBusy || openEntry === null) return;
    const request = ++handoffGeneration;
    const repositoryId = openEntry.repository_id;
    const templateGeneration = ++renderGeneration;
    const repositoryGeneration = ++repositoryRenderGeneration;
    handoffBusy = true;
    try {
      // Finish the visible preview before handing the file to another editor.
      // Save validation has application lifetime and continues independently.
      await Promise.all([
        refreshTemplateRender(templateRenderInput(), templateGeneration),
        ...(heldEnvelope !== null && repositoryDraftProblem === null
          ? [refreshRepositoryRender(repositoryRenderInput(openEntry), repositoryGeneration)]
          : []),
      ]);
      if (
        !mounted ||
        request !== handoffGeneration ||
        openEntry?.repository_id !== repositoryId ||
        renderGeneration !== templateGeneration ||
        repositoryRenderGeneration !== repositoryGeneration
      )
        return;
      await goto(href);
    } finally {
      if (request === handoffGeneration) handoffBusy = false;
    }
  }

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

  function adjustmentSnapshot(envelope: SyncOverrideEditorEnvelope): AdjustmentSnapshot | null {
    if (file === null) return null;
    const rows = envelopeMerges(envelope);
    const index = rows.findIndex((merge) => merge.path === path);
    const merge = rows[index];
    if (merge === undefined) return null;
    const text = composeMergedText(file.content, merge);
    return text === null
      ? null
      : { text, merge, overrideText: envelope.override_texts[index] ?? '' };
  }

  async function toggleRow(entry: RepositoryRow): Promise<void> {
    if (openRepo === entry.repository_id) {
      overrideFetchGeneration += 1;
      initialAdjustment = null;
      savedAdjustment = null;
      openingMerge = null;
      openingAnswers = [];
      resultEditContext = undefined;
      openRepo = null;
      held = null;
      heldEnvelope = null;
      editedText = null;
      return;
    }
    const generation = (overrideFetchGeneration += 1);
    const repositoryId = entry.repository_id;
    openRepo = repositoryId;
    repositoryTab = 'content';
    repositoryPreviousTab = 'content';
    showStored = false;
    held = null;
    heldEnvelope = null;
    holdProblem = null;
    rawOverrideOnly = false;
    initialAdjustment = null;
    savedAdjustment = null;
    openingMerge = null;
    openingAnswers = [];
    resultEditContext = undefined;
    if (entry.merge === undefined) editedText = null;
    else seedEdits(entry.merge as FileMergeSpec);
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
      savedAdjustment = adjustmentSnapshot(buildSyncOverrideEditorEnvelope(loaded.stored));
      const rows = envelopeMerges(loaded.envelope);
      const index = rows.findIndex((merge) => merge.path === path);
      const merge = index < 0 ? null : rows[index];
      openingMerge = merge;
      openingAnswers = (merge?.arrays ?? []).map((rule) => ({ ...rule }));
      const format =
        syncOverrideFormattingEntries(loaded.envelope).find((row) => row.path === path) ?? null;
      draftMerges = { ...draftMerges, [repositoryId]: merge };
      draftFormats = { ...draftFormats, [repositoryId]: format?.formatting ?? null };
      if (merge === null) {
        editedText = null;
        answers = [];
        return;
      }
      const text = loaded.envelope.override_texts[index] ?? '';
      if (text.startsWith(COMPOSED_DRAFT_PREFIX)) {
        editedText = text.slice(COMPOSED_DRAFT_PREFIX.length);
        answers = merge.arrays ?? [];
      } else if (text !== '' && !validOverrideText(text)) {
        editedText = null;
        answers = merge.arrays ?? [];
        rawOverrideOnly = true;
        holdProblem = "Finish this adjustment in the repository's File sync settings";
      } else {
        seedEdits(merge);
        if (editedText !== null)
          initialAdjustment = { text: editedText, merge, overrideText: text };
      }
    } catch (cause) {
      if (generation !== overrideFetchGeneration || openRepo !== repositoryId) return;
      holdProblem = cause instanceof Error ? cause.message : String(cause);
    }
  }

  function summaryWord(entry: RepositoryRow): string {
    const merge = entry.merge as FileMergeSpec | undefined;
    if (merge === undefined) {
      return entry.formatting === undefined ? 'uses the shared template' : 'changes formatting';
    }
    if (merge.strategy === 'markdown' || (!merge.strategy && fileFormat(path) === 'markdown')) {
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
    if (entry.formatting !== undefined) parts.push('changes formatting');
    return parts.length === 0 ? 'no content changes' : parts.join(' · ');
  }

  const openEntry = $derived(
    repositoryRows.find((entry) => entry.repository_id === openRepo) ?? null,
  );
  const openMerge = $derived((openEntry?.merge ?? null) as FileMergeSpec | null);
  const openPathFormatting = $derived(openEntry?.formatting ?? {});
  const savedOpenPathFormatting = $derived(
    merges.find((entry) => entry.repository_id === openRepo)?.formatting ?? {},
  );
  const dirtyOpenPathFormatting = $derived(
    FORMATTING_FIELDS.filter(
      (field) =>
        formattingPatchValue(openPathFormatting, field) !==
        formattingPatchValue(savedOpenPathFormatting, field),
    ).map((field) => field.key),
  );

  let repositoryRender = $state<SyncFileRenderResponse | null>(null);
  let repositoryRendering = $state(false);
  let repositoryRenderGeneration = 0;
  const openInheritedFormatting = $derived(repositoryRender?.formatting?.inherited_policy);

  function repositoryMergeInput(
    entry: RepositoryRow,
  ): NonNullable<SyncFileRenderInput['repository']>['merge'] | undefined {
    if (entry.merge === undefined) return undefined;
    const merge = { ...(entry.merge as unknown as SyncFileMerge) };
    delete (merge as Partial<SyncFileMerge>).path;
    return merge as Omit<SyncFileMerge, 'path'>;
  }

  async function refreshRepositoryRender(
    input: SyncFileRenderInput,
    generation: number,
  ): Promise<void> {
    if (generation !== repositoryRenderGeneration) return;
    repositoryRendering = true;
    try {
      const rendered = await renderFile(input);
      if (generation !== repositoryRenderGeneration) return;
      repositoryRender = rendered;
    } catch (cause) {
      if (generation !== repositoryRenderGeneration) return;
      const message = cause instanceof Error ? cause.message : String(cause);
      repositoryRender = {
        valid: false,
        final_content: '',
        matches_formatting: false,
        diagnostics: [{ stage: 'request', code: 'render_failed', message }],
      };
    } finally {
      if (generation === repositoryRenderGeneration) repositoryRendering = false;
    }
  }

  $effect(() => {
    const entry = openEntry;
    void templateText;
    void templateFormatting;
    if (entry === null) {
      repositoryRenderGeneration += 1;
      repositoryRender = null;
      repositoryRendering = false;
      return;
    }
    const generation = (repositoryRenderGeneration += 1);
    if (heldEnvelope === null || repositoryDraftProblem !== null) {
      repositoryRender = null;
      repositoryRendering = false;
      // The persistent serializer owns invalid raw drafts; do not render stale content.
      return;
    }
    const input = repositoryRenderInput(entry);
    const timer = setTimeout(() => void refreshRepositoryRender(input, generation), 120);
    return () => {
      clearTimeout(timer);
    };
  });

  /** The override the edited copy amounts to, live as the text changes. */
  const staged = $derived.by(() => {
    if (file === null || editedText === null || openMerge === null) return null;
    return deriveMerge(
      file.content,
      editedText,
      openMerge.strategy ?? 'deep-merge',
      answers,
      mergeIntent(resultEditContext),
    );
  });

  const repositoryDraftProblem = $derived(
    rawOverrideOnly
      ? "Finish this adjustment in the repository's File sync settings"
      : file !== null && editedText !== null && staged === null
        ? composedEditProblem(file.content, editedText, openMerge?.strategy ?? 'deep-merge')
        : null,
  );
  const rawOverrideText = $derived.by(() => {
    if (heldEnvelope === null) return '';
    const index = envelopeMerges(heldEnvelope).findIndex((merge) => merge.path === path);
    return heldEnvelope.override_texts[index] ?? '';
  });

  function mergeIntent(context?: ResultEditContext): MergeIntent {
    const text =
      initialAdjustment?.text ??
      (openingMerge === null || file === null
        ? null
        : composeMergedText(file.content, openingMerge));
    return {
      current:
        context?.snapshot !== undefined
          ? context.snapshot
          : text === null || openingMerge === null
            ? null
            : { text, merge: openingMerge },
      saved: savedAdjustment,
      deduplicate: openingMerge?.deduplicate === true,
    };
  }

  function specOf(overrides: Record<string, unknown>, arrays: ArrayRule[]): FileMergeSpec {
    // Temporary list projections can omit deduplication, but cannot change the
    // opening non-text options that must return when list rules are re-enabled.
    const next: FileMergeSpec = { ...(openingMerge ?? openMerge), overrides };
    if (arrays.length > 0) next.arrays = arrays;
    else {
      delete next.arrays;
      delete next.deduplicate;
    }
    return next;
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
    const merges = Array.isArray(envelope.document.merges)
      ? (envelope.document.merges as FileMergeSpec[])
      : [];
    return merges.map((merge, index) => {
      const overrides = parseJson(envelope.override_texts[index] ?? '');
      return isJsonRecord(overrides) ? { ...merge, overrides } : merge;
    });
  }

  function stageRepositoryFormatting(formatting: FormattingPatch): void {
    const entry = openEntry;
    const current = held;
    const envelope = heldEnvelope;
    if (entry === null || current === null || envelope === null || frozen || current.unreadable) {
      return;
    }
    const nextEnvelope = withSyncOverrideFormatting(envelope, path, formatting);
    if (
      !onChangeOverride(
        entry.repository_id,
        current,
        nextEnvelope,
        overrideDocumentControl(entry.repository_id),
      )
    ) {
      holdProblem = 'This repository formatting override could not be staged';
      return;
    }
    holdProblem = null;
    heldEnvelope = nextEnvelope;
    draftFormats = {
      ...draftFormats,
      [entry.repository_id]: formattingOverrideCount(formatting) === 0 ? null : formatting,
    };
  }

  function validOverrideText(text: string): boolean {
    return text.trim() === '' || isJsonRecord(parseJson(text));
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
      const leftNumber = JSON.isRawJSON(left)
        ? left.rawJSON
        : typeof left === 'number'
          ? JSON.stringify(left)
          : null;
      const rightNumber = JSON.isRawJSON(right)
        ? right.rawJSON
        : typeof right === 'number'
          ? JSON.stringify(right)
          : null;
      return leftNumber !== null && leftNumber === rightNumber;
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
      document.merges = cloneSettingsJson(merges as SettingsJson);
    }
    return { ...envelope, document, override_texts: texts };
  }

  function stageMergeText(
    text: string,
    nextAnswers: ArrayRule[],
    context?: ResultEditContext,
  ): boolean {
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
    const derived = deriveMerge(
      file.content,
      text,
      merge.strategy ?? 'deep-merge',
      nextAnswers,
      mergeIntent(context),
    );
    const empty = derived !== null && Object.keys(derived.overrides).length === 0;
    // A deliberate removal supplies a new restoration snapshot in the editor's
    // history. Undo restores the prior context as well as its visible text.
    const snapshots =
      context?.snapshot === undefined ? [initialAdjustment, savedAdjustment] : [context.snapshot];
    const restored =
      snapshots.find(
        (snapshot) =>
          snapshot !== null &&
          sameComposedContent(text, snapshot.text) &&
          derived !== null &&
          sameJson(specOf(derived.overrides, derived.arrays), snapshot.merge),
      ) ?? null;
    const next =
      restored !== null
        ? restored.merge
        : derived === null
          ? merge
          : empty
            ? null
            : specOf(derived.overrides, derived.arrays);
    /* An unfinished composed document is intentionally invalid override text.
       The registry can persist it, and the shared serializer then blocks Save
       until the editor becomes valid again. */
    const rawText =
      restored !== null
        ? restored.overrideText
        : derived === null
          ? `${COMPOSED_DRAFT_PREFIX}${text}`
          : next === null
            ? ''
            : formatJson(derived.overrides as JsonValue).trimEnd();
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

  function stageEditorText(text: string, opaqueContext?: unknown): void {
    // Only this component supplies context to its editor, which stores it opaquely.
    const context = opaqueContext as ResultEditContext | undefined;
    resultEditContext = context;
    stageMergeText(text, context?.answers ?? openingAnswers, context);
  }

  function replaceResult(text: string, next: FileMergeSpec, nextAnswers: ArrayRule[]): void {
    if (openMerge === null || heldEnvelope === null) return;
    const derived = { overrides: next.overrides ?? {}, arrays: next.arrays ?? [] };
    const snapshot =
      Object.keys(derived.overrides).length === 0
        ? null
        : {
            text,
            merge: next,
            overrideText: formatJson(derived.overrides as JsonValue).trimEnd(),
          };
    resultEditor?.replaceValue(text, {
      snapshot,
      answers: nextAnswers.map((rule) => ({ ...rule })),
    } satisfies ResultEditContext);
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
    const merge = specOf(overrides, arrays);
    const next = composeMergedText(file.content, merge);
    if (next !== null) replaceResult(next, merge, answers.filter(keeps));
  }

  function setListRule(key: string, strategy: string): void {
    // A temporary Replace choice keeps its authored slot in history. The
    // derived wire omits inactive rules without reordering the remaining ones.
    const next = answers.some((rule) => rule.path === key)
      ? answers.map((rule) => (rule.path === key ? { ...rule, strategy } : rule))
      : [...answers, { path: key, strategy }];
    if (editedText !== null)
      resultEditor?.replaceValue(editedText, {
        ...resultEditContext,
        answers: next.map((rule) => ({ ...rule })),
      } satisfies ResultEditContext);
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

  onDestroy(() => {
    mounted = false;
    handoffGeneration += 1;
    renderGeneration += 1;
    repositoryRenderGeneration += 1;
    onFormattingValidity('sync.files.repository-formatting', true, '');
  });
</script>

<div class="view-frame">
  <!-- One crumb, to the row this page sits under. Sync is where that row lives,
       not a second place to go back to. -->

  <PageHeader
    ancestors={[
      {
        label: SYNC_SECTION_LABELS.files,
        href: sectionHref('files'),
        onSelect: () => onOpenSection('files'),
      },
    ]}
    id="sync-file-heading"
    section="Shared file"
    title={path}
    mono
    description={reach}
  />

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#if file !== null}
    <Card unsaved={templateDirty}>
      <FileEditor
        bind:this={templateEditor}
        value={templateText}
        output={templateRender?.valid === true ? templateRender.final_content : null}
        busy={templateRendering}
        problem={templateRender?.valid === false ? templateDiagnostic : ''}
        {lang}
        readOnly={frozen}
        onChange={stageTemplate}
        onFormat={templateRender?.valid === true && templateMismatch && !templateRendering
          ? applyTemplateFormatting
          : undefined}
        onOptions={lang === 'text'
          ? undefined
          : (trigger) => {
              templateOptionsTrigger = trigger;
              templateOptionsOpen = true;
            }}
      />
    </Card>

    <Card unsaved={anyOverrideDirty} labelledby="file-repositories-heading">
      <div class="card-head">
        <h2 class="card-title" id="file-repositories-heading">Repository outputs</h2>
        <span class="card-note band-trim"
          >{visibleRepositories.length === matchingRepositories.length
            ? matchingRepositories.length
            : `${visibleRepositories.length} of ${matchingRepositories.length}`}</span
        >
        {#if matchingRepositories.length > 8 && repositorySearch.trim() === ''}
          <Button
            row
            tone="quiet"
            aria-expanded={showAllRepositories}
            aria-controls="file-repository-list"
            aria-label={showAllRepositories
              ? 'Show fewer repositories'
              : `Show all ${matchingRepositories.length} repositories`}
            onclick={() => (showAllRepositories = !showAllRepositories)}
          >
            {#snippet trailing()}<Icon
                name={showAllRepositories ? 'chevron-up' : 'chevron-down'}
                size="xs"
              />{/snippet}
            {showAllRepositories ? 'Show fewer' : 'Show all'}
          </Button>
        {/if}
      </div>
      <p class="group-note">
        {adjustedCount}
        {adjustedCount === 1 ? 'repository adds' : 'repositories add'} content or formatting adjustments
      </p>
      <div class="repository-search">
        <SearchField
          label="Find a repository output"
          placeholder="Find a repository"
          value={repositorySearch}
          onInput={(value) => (repositorySearch = value)}
        />
      </div>
      <ul class="object-list" id="file-repository-list">
        {#each visibleRepositories as entry (entry.repository_id)}
          <li>
            <div
              class="object-row"
              class:is-unsaved={overrideDirty(entry.repository_id)}
              data-unsaved={overrideDirty(entry.repository_id) || undefined}
            >
              <Button
                class="row-hit"
                aria-label={`Open output for ${entry.repository}`}
                onclick={(event) => {
                  repositoryTrigger = event.currentTarget;
                  void toggleRow(entry);
                }}><span class="visually-hidden">Open output for {entry.repository}</span></Button
              >
              <span class="object-main"
                ><span class="object-name-row"
                  ><span class="object-name file-path">{entry.repository}</span></span
                ><span class="object-sum">{summaryWord(entry)}</span></span
              >
              <span class="object-side"><Icon name="chevron-right" size="xs" /></span>
            </div>
          </li>
        {/each}
      </ul>
      {#if matchingRepositories.length === 0}
        <EmptyState
          title={repositoryRows.length === 0
            ? 'No repositories receive this file yet'
            : 'No repositories match this search'}
          description={repositoryRows.length === 0
            ? 'Enable file sync for a repository to include it here'
            : 'Try another repository name or clear the search'}
          actionLabel={repositorySearch.trim() === '' ? undefined : 'Clear the search'}
          onAction={() => (repositorySearch = '')}
        />
      {/if}
    </Card>
  {/if}
</div>

{#if templateOptionsOpen && templateRender?.formatting !== undefined}
  <Modal
    id="template-options"
    open
    title="Template options"
    description={path}
    variant="inspector"
    returnFocus={templateOptionsTrigger}
    onClose={() => (templateOptionsOpen = false)}
  >
    <div class="card-stack">
      {#if templateMismatch}
        <div class="format-action">
          <span class="setting-say"
            ><span class="setting-name">Format this template</span><span class="setting-why"
              >These rules already apply to synced files; applying them here updates the source in
              one undoable edit</span
            ></span
          >
          <Button
            disabled={frozen || templateRendering}
            onclick={() => {
              applyTemplateFormatting();
              templateOptionsOpen = false;
            }}>Apply formatting</Button
          >
        </div>
      {/if}
      <FormattingEditor
        patch={templateFormatting}
        savedPatch={savedTemplateFormatting}
        inherited={templateRender.formatting.inherited_policy}
        resolution={templateRender.formatting}
        {path}
        scope="template"
        idPrefix={path}
        disabled={frozen}
        dirtyKeys={dirtyTemplateFormatting}
        onChange={stageTemplateFormatting}
        onValidity={(valid) =>
          onFormattingValidity(
            'sync.files.template-formatting',
            valid,
            'Formatting widths must be whole numbers within their documented bounds',
          )}
      />
    </div>
    {#snippet footer()}<Button onclick={() => (templateOptionsOpen = false)}>Done</Button>{/snippet}
  </Modal>
{/if}

{#if openEntry !== null && file !== null}
  {@const entry = openEntry}
  <Modal
    id="repository-file-output"
    open
    title={entry.repository}
    description={path}
    variant="inspector"
    returnFocus={repositoryTrigger}
    onClose={closeRepository}
  >
    <div class="card-stack">
      <div class="repository-view-tools">
        {#if repositoryTab === 'formatting'}
          <Button tone="quiet" onclick={() => (repositoryTab = repositoryPreviousTab)}>
            {#snippet icon()}<Icon name="chevron-left" size="sm" />{/snippet}Back to file
          </Button>
        {:else}
          <SegmentedControl
            name="repository-output-view"
            label="Repository output view"
            value={repositoryTab}
            options={[
              { value: 'content', label: 'Content adjustments' },
              { value: 'preview', label: 'Final output' },
            ]}
            onSelect={(value) => (repositoryTab = value)}
          />
          {#if lang !== 'text'}<IconButton
              toolbar
              icon="sliders"
              label="Repository file options"
              disabled={repositoryDraftProblem !== null || heldEnvelope === null}
              onclick={() => {
                repositoryPreviousTab = repositoryTab;
                repositoryTab = 'formatting';
              }}
            />{/if}
        {/if}
      </div>
      {#if holdProblem !== null && !rawOverrideOnly}<FormError message={holdProblem} />{/if}
      {#if repositoryDraftProblem === null && repositoryRender?.valid === false}
        <FormError
          message={repositoryRender.diagnostics.map(({ message }) => message).join(' · ')}
        />
      {/if}
      {#if repositoryTab === 'preview'}
        <section class="preview-pane exact-output" aria-label="Read-only repository output">
          {#if repositoryDraftProblem !== null}
            <FormError message={repositoryDraftProblem} />
          {:else if repositoryRender?.valid === true}
            <div class:is-rendering={repositoryRendering} class="rendered-output">
              <CodeBlock text={repositoryRender.final_content} {lang} />
            </div>
            {#if repositoryRendering}
              <p class="render-note" role="status">Refreshing final output…</p>
            {/if}
          {:else if repositoryRender?.valid !== false}
            <p class="sync-note">Preparing final output…</p>
          {/if}
        </section>
      {/if}
      <div hidden={repositoryTab !== 'content'}>
        <div class="card-stack">
          <section class="preview-pane">
            <div class="merge-pane-title">
              <span class="merge-pane-label">
                <span class="t"
                  >{!rawOverrideOnly && (showStored || editedText === null)
                    ? 'Adjustment settings'
                    : 'Content adjustments'}</span
                >
                {#if showStored || editedText === null || mergeFrozen}<span
                    class="setting-unmanaged">Read only</span
                  >{/if}
              </span>
              <span class="pane-tools">
                {#if editedText !== null && resultUndoDepth > 0}
                  <Button onclick={() => resultEditor?.undoEdit()}>
                    {#snippet icon()}<Icon name="undo" size="sm" />{/snippet}
                    Undo
                  </Button>
                {/if}
              </span>
            </div>
            {#if openMerge === null}
              <p class="sync-note">No content adjustments for this repository</p>
            {:else if rawOverrideOnly}
              <CodeBlock text={rawOverrideText} lang="json" />
            {:else if editedText === null}
              <CodeBlock text={JSON.stringify(openMerge, null, 2)} lang="json" />
            {:else if held === null || heldEnvelope === null}
              <CodeBlock text={editedText} lang="json" />
            {:else}
              {#if showStored}
                <CodeBlock text={JSON.stringify(openMerge, null, 2)} lang="json" />
              {/if}
              <div hidden={showStored}>
                <CodeEditor
                  bind:this={resultEditor}
                  value={editedText}
                  readOnly={mergeFrozen}
                  overridden={overriddenLines}
                  terminalNewline
                  onChange={stageEditorText}
                  onHistory={(depth) => (resultUndoDepth = depth)}
                />
              </div>
            {/if}
            {#if repositoryDraftProblem !== null}
              <div class="editor-problem band-trim-kids">
                <FormError message={repositoryDraftProblem} />
              </div>
            {/if}
          </section>
          {#if showStored || (openSummary !== null && (openSummary.changed.length > 0 || openSummary.removed.length > 0 || openSummary.listed.length > 0))}
            <div class="patch-strip">
              <span class="patch-word"
                >{openSummary !== null &&
                (openSummary.changed.length > 0 ||
                  openSummary.removed.length > 0 ||
                  openSummary.listed.length > 0)
                  ? 'This repository changes'
                  : 'No changes to the template'}</span
              >
              {#each openSummary?.changed ?? [] as key (key)}
                <span class="patch-key"
                  ><span class="t">{key}</span>
                  <button
                    aria-label="Stop changing {key}"
                    disabled={mergeFrozen}
                    onclick={() => dropKey(key)}><Icon name="close" size="nano" /></button
                  ></span
                >
              {/each}
              {#each openSummary?.removed ?? [] as key (key)}
                <span class="patch-key is-removal"
                  ><span class="t">{key}</span>
                  <button
                    aria-label="Stop removing {key}"
                    disabled={mergeFrozen}
                    onclick={() => dropKey(key)}><Icon name="close" size="nano" /></button
                  ></span
                >
              {/each}
              <span class="patch-word push-end">
                <Button tone="quiet" onclick={() => (showStored = !showStored)}>
                  {showStored ? 'Back to content' : 'View adjustment settings'}
                </Button>
              </span>
            </div>
          {/if}

          {#each staged?.questions ?? [] as question (question.path)}
            <div class="list-ask">
              <span class="list-ask-word">Choose how to combine <code>{question.path}</code></span>
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
      </div>
      {#if repositoryTab === 'formatting'}
        {#if lang !== 'text' && openInheritedFormatting !== undefined}
          <div class="repository-formatting">
            <FormattingEditor
              patch={openPathFormatting}
              savedPatch={savedOpenPathFormatting}
              inherited={openInheritedFormatting}
              resolution={repositoryRender?.formatting}
              {path}
              scope="path"
              idPrefix={`${entry.repository_id}-${path}`}
              disabled={mergeFrozen}
              dirtyKeys={dirtyOpenPathFormatting}
              onChange={stageRepositoryFormatting}
              onValidity={(valid) =>
                onFormattingValidity(
                  'sync.files.repository-formatting',
                  valid,
                  'Formatting widths must be whole numbers within their documented bounds',
                )}
            />
          </div>
        {/if}
      {/if}
    </div>
    {#snippet footer()}
      {#if overrideDirty(entry.repository_id)}<span class="unsaved-note">Unsaved changes</span>{/if}
      {#if repositoryHref !== null && fileFormat(path) !== null}
        {#if handoffBusy}
          <Button tone="quiet" disabled aria-busy="true">Opening editor…</Button>
        {:else}
          <Button
            tone="quiet"
            href={fileAdjustmentHref(repositoryHref(entry.repository), path)}
            onclick={followAdjustment}
          >
            {readOnly || held?.unreadable ? 'Inspect adjustments' : 'Edit adjustments'}
          </Button>
        {/if}
      {/if}
      <Button onclick={closeRepository}>Done</Button>
    {/snippet}
  </Modal>
{/if}

<style>
  .repository-view-tools,
  .format-action {
    align-items: center;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
  }
  .format-action {
    flex-wrap: wrap;
  }
  .repository-view-tools {
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .format-action > .setting-say {
    flex: 1 1 16rem;
  }

  /* THE HEAD'S LINE IS ITS TITLE'S CAP, so the title-to-first-row ink never
     depends on which adornments the card happens to carry. A control in the
     head gives its own slack back rather than growing the line. */

  .card-head :global(.btn) {
    margin-block: calc((var(--card-head-line) - var(--control-height-compact)) / 2);
  }

  /* A line ABOUT a row, not a page with nothing in it - what one repository
     does with the template, why an editor cannot compose, what is being waited
     for. It was `.sync-empty`, which is how three of these came to wear the
     name of a state they are not: a page that has nothing says so in a
     `.state-panel`. */
  .sync-note {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: 0 0 var(--space-2);
  }

  /* ---------- The adjuster rows ---------- */

  /* One list, a hairline between neighbours - the rows read as one table
     rather than three floating cards. */

  /* The hover pill has rounded corners; a hairline crossing its edge reads
     as a crack in it. The hovered row hides the separator under it and the
     one its neighbour would draw over it. */

  /* The same row the Files list stands its templates on, as a button: the
     whole surface opens the adjustment, the chevron says which way. */

  .file-path {
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-weight: 500;
  }

  /* ---------- The adjustment, opened ---------- */

  .merge-pane-title {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    flex-wrap: wrap;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: var(--space-2);
    justify-content: space-between;
    letter-spacing: 0.07em;
    margin-bottom: var(--space-2);
    /* One declared height whether or not the pane carries tools, so paired
       panes start their code at the same pixel. */
    min-block-size: var(--tier-quiet);
    text-transform: uppercase;
  }

  .merge-pane-title .t {
    text-box: trim-both cap alphabetic;
  }

  .pane-tools {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    letter-spacing: 0;
    text-transform: none;
  }
  .merge-pane-label {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }
  .merge-pane-label .setting-unmanaged {
    letter-spacing: 0;
    text-transform: none;
  }

  @media (max-width: 64rem) {
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
    block-size: var(--tier-mark);
    border-radius: var(--r-chip);
    box-sizing: border-box;
    color: var(--brand-action-text);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    gap: 0.25rem;
    line-height: var(--leading-flat);
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
    line-height: var(--leading-compact);
  }

  .list-ask-word code {
    color: var(--code-key);
    font-family: var(--mono);
    /* The mono face's taller metrics raised the line box 1.5px over the
       sans text around it; the words set the line, the key rides it. */
    line-height: var(--leading-flat);
  }

  /* The group, the card, the dot and the two voices are one vocabulary in `app.css`.
     What is this page's alone is the answer it cannot offer: the edited list no longer
     holds the template's entries intact, so the option stays visible and unchoosable
     rather than disappearing and taking its explanation with it. */
  .choice-card.is-unaskable {
    cursor: default;
    opacity: 0.5;
  }

  .choice-card.is-unaskable:hover {
    background: transparent;
  }

  .preview-pane {
    min-inline-size: 0;
  }
  .editor-problem {
    display: grid;
    margin-block-start: var(--row-copy-gap);
    line-height: var(--row-copy-leading);
  }
  .rendered-output {
    transition: opacity var(--duration-fast) var(--ease-standard);
  }
  .rendered-output.is-rendering {
    opacity: 0.5;
  }
  .render-note {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin-block: var(--space-3) 0;
  }
  .repository-search {
    margin-block-end: var(--space-4);
  }
  @media (max-width: 64rem) {
  }
  .unsaved-note {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin-inline-end: auto;
  }
</style>
