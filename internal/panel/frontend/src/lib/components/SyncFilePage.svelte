<script module lang="ts">
  import type { CodeLang } from '../code-tokens';

  /** The language a path's extension says it is written in. */
  export function langOf(path: string): CodeLang {
    if (/\.(json|json5)$/i.test(path)) return 'json';
    if (/\.(md|markdown)$/i.test(path)) return 'markdown';
    return 'yaml';
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
  import { unifiedDiff } from '../code-tokens';
  import { mergeSummary, type ArrayRule, type FileMergeSpec } from '../filemerge';
  import { composeMergedText, deriveMerge } from '../jsontext';
  import { formatRelative } from '../format';
  import type {
    SyncConfig,
    SyncFile,
    SyncFileMergeEntry,
    SyncFilesContext,
    SyncOverride,
    SyncOverrideInput,
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
    context,
    path,
    nowMs,
    readOnly,
    problem = null,
    saving,
    editorLogin = '',
    sectionHref,
    onOpenSection,
    onSave,
    fetchOverride,
    saveOverride,
  }: {
    config: SyncConfig | null;
    context: SyncFilesContext | null;
    /** Which template the address names. */
    path: string;
    nowMs: number;
    readOnly: boolean;
    problem?: string | null;
    saving: boolean;
    /** Who is editing, stamped onto the template's freshness on save. */
    editorLogin?: string;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    onSave: (enabled: boolean, document: Record<string, unknown>) => void;
    fetchOverride: (repositoryId: string) => Promise<SyncOverride>;
    saveOverride: (repositoryId: string, input: SyncOverrideInput) => Promise<SyncOverride>;
  } = $props();

  const stored = $derived(config?.document ?? {});
  const enabled = $derived(config?.enabled ?? false);
  const frozen = $derived(readOnly || config?.unreadable === true || saving || config === null);

  const files = $derived(Array.isArray(stored.files) ? (stored.files as SyncFile[]) : []);
  const file = $derived(files.find((held) => held.path === path) ?? null);
  const lang = $derived(langOf(path));

  const merges = $derived((context?.merges ?? []).filter((entry) => entry.path === path));

  const freshness = $derived.by(() => {
    const at = file?.updated_at ?? config?.updated_at;
    if (at === undefined) return '';
    const by = file?.updated_by ?? config?.updated_by ?? '';
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

  const templateText = $derived(templateDraft ?? file?.content ?? '');
  const templateDirty = $derived(
    file !== null && templateDraft !== null && templateDraft !== file.content,
  );

  function saveTemplate(): void {
    const next = templateDraft;
    if (file === null || next === null || next === file.content) return;
    templateDraft = null;
    onSave(enabled, {
      ...stored,
      files: files.map((held) =>
        held.path === path
          ? {
              ...held,
              content: next,
              updated_at: new Date(nowMs).toISOString(),
              ...(editorLogin === '' ? {} : { updated_by: editorLogin }),
            }
          : held,
      ),
    });
  }

  /* ---------- One adjustment open at a time ---------- */

  let openRepo = $state<string | null>(null);
  let sideBySide = $state(false);
  let showStored = $state(false);
  /** The open repository's whole override, fetched for its revision. */
  let held = $state<SyncOverride | null>(null);
  let holdProblem = $state<string | null>(null);

  /* The composed copy is the editable surface. Edits stage here, the
     override is derived from them, and Save writes it back. */
  let editedText = $state<string | null>(null);
  /** The list answers given so far - stored rules, then the ask cards. */
  let answers = $state<ArrayRule[]>([]);
  let savingMerge = $state(false);

  /* What this page saved, keyed by repository, layered over a context the
     parent has not re-read yet - null is a removed adjustment. */
  let savedMerges = $state<Record<string, FileMergeSpec | null>>({});

  const adjusters = $derived(
    merges
      .filter((entry) => savedMerges[entry.repository_id] !== null)
      .map((entry) => {
        const kept = savedMerges[entry.repository_id];
        return kept === undefined || kept === null
          ? entry
          : { ...entry, merge: kept as SyncFileMergeEntry['merge'] };
      }),
  );

  function seedEdits(merge: FileMergeSpec): void {
    editedText = file === null ? null : composeMergedText(file.content, merge);
    answers = merge.arrays ?? [];
  }

  async function toggleRow(entry: SyncFileMergeEntry): Promise<void> {
    if (openRepo === entry.repository_id) {
      openRepo = null;
      held = null;
      editedText = null;
      return;
    }
    openRepo = entry.repository_id;
    sideBySide = false;
    showStored = false;
    held = null;
    holdProblem = null;
    seedEdits(entry.merge as FileMergeSpec);
    try {
      held = await fetchOverride(entry.repository_id);
    } catch (cause) {
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

  /** The stored adjustment's composed copy - what Discard returns to. */
  const composed = $derived(
    file === null || openMerge === null ? null : composeMergedText(file.content, openMerge),
  );

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

  const dirty = $derived.by(() => {
    if (editedText === null || composed === null) return false;
    if (editedText !== composed) return true;
    const stored = JSON.stringify(
      (openMerge?.arrays ?? []).map((rule) => [rule.path, rule.strategy]).sort(),
    );
    const staged = JSON.stringify(answers.map((rule) => [rule.path, rule.strategy]).sort());
    return stored !== staged;
  });

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

  /* ---------- Writing the open override back ---------- */

  async function saveMerge(next: FileMergeSpec | null): Promise<boolean> {
    const entry = openEntry;
    const current = held;
    if (entry === null || current === null || frozen) return false;
    const all = Array.isArray(current.document.merges)
      ? (current.document.merges as FileMergeSpec[])
      : [];
    const others = all.filter((merge) => merge.path !== path);
    const document = {
      ...current.document,
      merges: next === null ? others : [...others, next],
    };
    holdProblem = null;
    try {
      held = await saveOverride(entry.repository_id, {
        enabled: current.enabled,
        document,
        expected_revision: current.revision,
      });
      savedMerges = { ...savedMerges, [entry.repository_id]: next };
      if (next === null) {
        openRepo = null;
        editedText = null;
      }
      return true;
    } catch (cause) {
      holdProblem = cause instanceof Error ? cause.message : String(cause);
      return false;
    }
  }

  async function saveEdits(): Promise<void> {
    if (staged === null || savingMerge) return;
    const emptied = Object.keys(staged.overrides).length === 0;
    const next = emptied ? null : specOf(staged.overrides, staged.arrays);
    savingMerge = true;
    const saved = await saveMerge(next);
    savingMerge = false;
    if (saved && next !== null) seedEdits(next);
  }

  function discardEdits(): void {
    if (openMerge !== null) seedEdits(openMerge);
  }

  /** The x on a patch chip: the edited copy takes those lines back. */
  function dropKey(key: string): void {
    if (staged === null || file === null) return;
    const overrides = { ...staged.overrides };
    delete overrides[key];
    const keeps = (rule: ArrayRule): boolean =>
      rule.path !== key && !rule.path.startsWith(`${key}.`);
    const arrays = staged.arrays.filter(keeps);
    answers = answers.filter(keeps);
    editedText = composeMergedText(file.content, specOf(overrides, arrays));
  }

  function setListRule(key: string, strategy: string): void {
    const kept = answers.filter((rule) => rule.path !== key);
    answers = strategy === 'replace' ? kept : [...kept, { path: key, strategy }];
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
    <div class="card">
      <div class="card-head">
        <h3 class="card-title">Template</h3>
        <div class="head-tools">
          <span class="pill pill-neutral"><span class="t">{strategyPill}</span></span>
        </div>
      </div>
      <CodeEditor
        value={templateText}
        {lang}
        readOnly={frozen}
        onChange={(text) => (templateDraft = text)}
      />
      {#if templateDirty}
        <div class="rule-edit-foot">
          <Button tone="quiet" onclick={() => (templateDraft = null)}>Discard</Button>
          <Button tone="signal" disabled={frozen} onclick={saveTemplate}>Save</Button>
        </div>
      {/if}
    </div>

    <div class="card">
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
        <div class="adjuster">
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
                      value={editedText}
                      readOnly={frozen}
                      overridden={overriddenLines}
                      onChange={(text) => (editedText = text)}
                    />
                  </div>
                </div>
              {:else}
                <CodeEditor
                  value={editedText}
                  readOnly={frozen}
                  overridden={overriddenLines}
                  onChange={(text) => (editedText = text)}
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
                        disabled={frozen}
                        onclick={() => dropKey(key)}><Icon name="close" size={8} /></button
                      ></span
                    >
                  {/each}
                  {#each openSummary.removed as key (key)}
                    <span class="patch-key is-removal"
                      ><span class="t">{key}</span>
                      <button
                        aria-label="Stop removing {key}"
                        disabled={frozen}
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
                          disabled={frozen || !askable(question, option.value)}
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

              {#if dirty}
                <div class="rule-edit-foot">
                  <Button tone="quiet" disabled={savingMerge} onclick={discardEdits}>Discard</Button
                  >
                  <Button
                    tone="signal"
                    disabled={frozen || savingMerge || staged === null || held === null}
                    onclick={() => void saveEdits()}>Save</Button
                  >
                </div>
              {/if}
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

  .rule-edit-foot {
    border-top: 1px solid var(--border-subtle);
    display: flex;
    gap: var(--space-2);
    justify-content: flex-end;
    margin-top: var(--space-3);
    padding-top: var(--space-3);
  }

  /* ---------- The adjuster rows ---------- */

  /* One list, a hairline between neighbours - the rows read as one table
     rather than three floating cards. */
  .adjuster + .adjuster {
    border-top: 1px solid var(--border-subtle);
  }

  /* The hover pill has rounded corners; a hairline crossing its edge reads
     as a crack in it. The hovered row hides the separator above it and the
     one its neighbour would draw under it. */
  .adjuster:has(> .object-row:hover),
  .adjuster:has(> .object-row:hover) + .adjuster {
    border-top-color: transparent;
  }

  .adjuster > .merge-result {
    padding-block: var(--space-2) var(--space-4);
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

  .object-row:hover {
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
</style>
