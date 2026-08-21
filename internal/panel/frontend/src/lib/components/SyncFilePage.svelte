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
  import { mergedPreview, mergeSummary, normalizedJson, type FileMergeSpec } from '../filemerge';
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

  /* ---------- The template, staged when edited ---------- */

  let editing = $state(false);
  let draft = $state('');

  function beginEdit(): void {
    if (frozen || file === null) return;
    draft = file.content;
    editing = true;
  }

  function saveTemplate(): void {
    if (file === null) return;
    editing = false;
    if (draft === file.content) return;
    onSave(enabled, {
      ...stored,
      files: files.map((held) =>
        held.path === path
          ? {
              ...held,
              content: draft,
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

  async function toggleRow(entry: SyncFileMergeEntry): Promise<void> {
    if (openRepo === entry.repository_id) {
      openRepo = null;
      held = null;
      return;
    }
    openRepo = entry.repository_id;
    sideBySide = false;
    showStored = false;
    held = null;
    holdProblem = null;
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

  const openEntry = $derived(merges.find((entry) => entry.repository_id === openRepo) ?? null);
  const openMerge = $derived((openEntry?.merge ?? null) as FileMergeSpec | null);
  const openSummary = $derived(openMerge === null ? null : mergeSummary(openMerge));

  const preview = $derived.by(() => {
    if (file === null || openMerge === null) return null;
    return mergedPreview(file.content, openMerge);
  });

  /** Which lines of the composed copy an adjustment rewrote, 1-indexed. */
  const overriddenLines = $derived.by(() => {
    if (file === null || preview === null) return null;
    /* Like against like: the preview is a re-print, so the diff runs over
       the template re-printed the same way - or every reflowed line would
       wear a gutter bar it never earned. */
    const base = normalizedJson(file.content) ?? file.content;
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- built whole, replaced never mutated
    const marked = new Set<number>();
    let at = 0;
    for (const line of unifiedDiff(base, preview)) {
      if (line.op === '-') continue;
      at += 1;
      if (line.op === '+') marked.add(at);
    }
    return marked;
  });

  /* ---------- Writing the open override back ---------- */

  async function saveMerge(next: FileMergeSpec | null): Promise<void> {
    const entry = openEntry;
    const current = held;
    if (entry === null || current === null || frozen) return;
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
      if (next === null) openRepo = null;
    } catch (cause) {
      holdProblem = cause instanceof Error ? cause.message : String(cause);
    }
  }

  function dropKey(key: string): void {
    if (openMerge === null) return;
    const overrides = { ...(openMerge.overrides ?? {}) };
    delete overrides[key];
    const arrays = (openMerge.arrays ?? []).filter((rule) => rule.path !== key);
    const emptied = Object.keys(overrides).length === 0;
    void saveMerge(
      emptied
        ? null
        : { ...openMerge, overrides, ...(arrays.length > 0 ? { arrays } : { arrays: undefined }) },
    );
  }

  function setListRule(key: string, strategy: string): void {
    if (openMerge === null) return;
    const arrays = (openMerge.arrays ?? []).filter((rule) => rule.path !== key);
    if (strategy !== 'replace') arrays.push({ path: key, strategy });
    void saveMerge({ ...openMerge, ...(arrays.length > 0 ? { arrays } : { arrays: undefined }) });
  }

  /**
   * The lists whose combining was decided out loud: each explicit array rule
   * stands as the answered question, still one press from a different
   * answer. A list without one is an ordinary changed key - RFC 7396
   * replaces it, and its chip on the strip says so.
   */
  const listQuestions = $derived.by(() => {
    if (openMerge === null) return [];
    const overrides = openMerge.overrides ?? {};
    return (openMerge.arrays ?? [])
      .filter((rule) => Array.isArray(overrides[rule.path]))
      .map((rule) => ({ key: rule.path, chosen: rule.strategy }));
  });

  const RULE_CHOICES = [
    { value: 'append', title: 'Append', why: "The repository's entries follow the template's" },
    { value: 'prepend', title: 'Prepend', why: "The repository's entries come first" },
    { value: 'replace', title: 'Replace', why: "The repository's list stands alone" },
  ];
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
          {#if !editing}
            <Button disabled={frozen} onclick={beginEdit}>Edit</Button>
          {/if}
        </div>
      </div>
      {#if editing}
        <textarea
          class="template-editor"
          rows={Math.max(8, draft.split('\n').length + 1)}
          bind:value={draft}
          aria-label="Template content"
          spellcheck="false"></textarea>
        <div class="rule-edit-foot">
          <Button tone="quiet" onclick={() => (editing = false)}>Cancel</Button>
          <Button tone="signal" onclick={saveTemplate}>Save</Button>
        </div>
      {:else}
        <CodeBlock text={file.content} {lang} />
      {/if}
    </div>

    <div class="card">
      <div class="card-head">
        <h3 class="card-title">Repository adjustments</h3>
        <span class="object-sum"
          >{merges.length} of {context?.repositories ?? 0}
          {merges.length === 1 ? 'repository changes' : 'repositories change'} this file</span
        >
      </div>

      {#if merges.length === 0}
        <p class="sync-empty">Every repository takes this file as the organization writes it</p>
      {/if}

      {#each merges as entry (entry.repository_id)}
        <div class="object-list" class:block-gap-top={entry !== merges[0]}>
          <div class="object-row plain-row">
            <span class="object-main">
              <span class="object-name-row"><span class="file-path">{entry.repository}</span></span>
              <span class="object-sum">{summaryWord(entry)}</span>
            </span>
            <span class="object-side">
              <Button tone="quiet" onclick={() => void toggleRow(entry)}>
                {openRepo === entry.repository_id ? 'Close' : 'Edit'}
              </Button>
            </span>
          </div>
        </div>

        {#if openRepo === entry.repository_id}
          <div class="merge-result">
            {#if holdProblem !== null}
              <FormError message={holdProblem} />
            {/if}
            <div class="merge-pane-title">
              <span class="t">What {entry.repository} ends up with</span>
              <span class="pane-tools">
                {#if preview !== null}
                  <Button tone="quiet" onclick={() => (sideBySide = !sideBySide)}>
                    {sideBySide ? 'Hide the template' : 'Show the template beside it'}
                  </Button>
                {/if}
              </span>
            </div>
            {#if preview === null}
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
                  <CodeBlock text={preview} {lang} overridden={overriddenLines} />
                </div>
              </div>
            {:else}
              <CodeBlock text={preview} {lang} overridden={overriddenLines} />
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
                      onclick={() => dropKey(key)}><Icon name="close" size={12} /></button
                    ></span
                  >
                {/each}
                {#each openSummary.removed as key (key)}
                  <span class="patch-key is-removal"
                    ><span class="t">{key}</span>
                    <button
                      aria-label="Stop removing {key}"
                      disabled={frozen}
                      onclick={() => dropKey(key)}><Icon name="close" size={12} /></button
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

            {#each listQuestions as question (question.key)}
              <div class="list-ask">
                <span class="list-ask-word"
                  ><strong>Both set <code>{question.key}</code>.</strong> A merge cannot know how two
                  lists should combine, so this is the one question it asks:</span
                >
                <div class="choice-cards ask-cards">
                  {#each RULE_CHOICES as option (option.value)}
                    <label class="choice-card" class:is-chosen={question.chosen === option.value}>
                      <input
                        type="radio"
                        name="listrule-{entry.repository_id}-{question.key}"
                        checked={question.chosen === option.value}
                        disabled={frozen}
                        onchange={() => setListRule(question.key, option.value)}
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

  .template-editor {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    line-height: round(1.65em, 1px);
    padding: var(--space-3);
    resize: vertical;
    width: 100%;
  }

  .template-editor:focus {
    border-color: var(--focus);
    outline: 2px solid var(--focus);
    outline-offset: -1px;
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

  .block-gap-top {
    margin-top: var(--space-6);
  }

  .object-list {
    display: grid;
  }

  .object-row {
    align-items: center;
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-4);
    grid-template-columns: 1fr auto;
    margin-inline: calc(var(--space-3) * -1);
    padding: 0.75rem var(--space-3);
    position: relative;
  }

  .plain-row {
    cursor: default;
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

  /* 24px hit target folded around a 10px glyph - the key's box never grows. */
  .patch-key button {
    background: none;
    border: 0;
    border-radius: 3px;
    color: inherit;
    cursor: pointer;
    display: inline-flex;
    margin: -7px;
    opacity: 0.7;
    padding: 7px;
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
