<script module lang="ts">
  import type { Language } from '#lib/syntax.js';

  /** What a path is written in, from the only thing a path says about itself. */
  function languageOf(path: string): Language {
    const lower = path.toLowerCase();
    if (lower.endsWith('.json')) return 'json';
    if (lower.endsWith('.yaml') || lower.endsWith('.yml')) return 'yaml';
    if (lower.endsWith('.toml')) return 'toml';

    return 'markdown';
  }
</script>

<script lang="ts">
  /**
   * One shared file: what it should say, and what each repository ends up with.
   *
   * The RESULT is the editable surface, never the adjustment. Somebody looking
   * at this page wants the file their repository will hold; asking them to
   * write a JSON merge patch is asking them to work out the difference in their
   * head and type that instead. So the composed file is what is shown and
   * edited, and the stored adjustment is derived back out of it - see
   * `src/lib/merge.ts`, which spells RFC 7396 the way the service composes it.
   *
   * Composing is offered for JSON and nothing else yet. YAML and Markdown are
   * merged by the service with rules this cannot reproduce in a browser, so
   * their adjustments are listed and named rather than drawn as a file that
   * would be a guess.
   */
  import { storedList } from '#lib/form-lists.js';
  import { formatRelative } from '#lib/format.js';
  import {
    composable,
    composeFile,
    composesNothing,
    deriveOverrides,
    formatJson,
    markedLines,
    parseJson,
    patchedKeys,
    sharedArrays,
    asArrayStrategy,
    type ArrayStrategy,
    type JsonValue,
    type MergeSpec,
  } from '#lib/merge.js';
  import type { SyncFile, SyncFileMerge, SyncOverrideRow } from '#lib/types.js';

  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import ChoiceCards from './ChoiceCards.svelte';
  import CodeBlock, { codeLines } from './CodeBlock.svelte';
  import CodeEditor from './CodeEditor.svelte';
  import BackLink from './BackLink.svelte';
  import ObjectRow from './ObjectRow.svelte';
  import PageHeader from './PageHeader.svelte';
  import Plate from './Plate.svelte';

  const {
    stored,
    path,
    listHref,
    adjustments = [],
    repositories = 0,
    updatedBy = '',
    updatedAt = '',
    now = Date.now(),
    readOnly,
    saving,
    unreadable,
    problem = null,
    onSave,
    onSaveAdjustment,
  }: {
    stored: Record<string, unknown>;
    /** Which file this page is, by the only thing that keys one: its path. */
    path: string;
    listHref: string;
    /** Every repository's answer about files. */
    adjustments?: readonly SyncOverrideRow[];
    /** How many repositories the installation has, which the head counts against. */
    repositories?: number;
    updatedBy?: string;
    updatedAt?: string;
    now?: number;
    readOnly: boolean;
    saving: boolean;
    unreadable: boolean;
    problem?: string | null;
    /**
     * Writes the template. `false` is a write that did not land, which is what
     * keeps the editor open over it rather than closing on the words.
     */
    onSave: (document: Record<string, unknown>) => void | Promise<boolean | void>;
    /** Writes one repository's adjustment of this file, and answers the same way. */
    onSaveAdjustment?: (
      repositoryId: string,
      document: Record<string, unknown>,
    ) => void | Promise<boolean | void>;
  } = $props();

  const disabled = $derived(saving || readOnly || unreadable);

  const files = $derived(storedList<SyncFile>(stored, 'files'));
  const at = $derived(files.findIndex((file) => file.path === path));
  const file = $derived(at === -1 ? undefined : files[at]);
  const language = $derived(languageOf(path));

  /** Which surface is open: the template, or one repository's result. */
  let editingTemplate = $state(false);
  let editingRepository = $state<string | null>(null);
  let draft = $state('');
  let refused = $state<string | null>(null);

  /** Every repository that adjusts this path, with the merge that does it. */
  const adjusting = $derived(
    adjustments
      .map((row) => ({
        row,
        merge: storedList<SyncFileMerge>(row.document, 'merges').find(
          (candidate) => candidate.path === path,
        ),
      }))
      .filter(
        (one): one is { row: SyncOverrideRow; merge: SyncFileMerge } => one.merge !== undefined,
      ),
  );

  const template = $derived(file?.content ?? '');
  const templateJson = $derived(composable(path) ? parseJson(template) : undefined);

  /**
   * A stored merge entry, as the composer reads it.
   *
   * The whole entry rather than its overrides: the strategy, the list rules and
   * `deduplicate` decide what the repository ends up with as much as the
   * overrides do, and reading only the overrides is how the panel came to draw
   * a repository's own list replacing the template's where the service appended
   * to it.
   */
  function specOf(merge: SyncFileMerge): MergeSpec {
    return {
      arrays: merge.arrays,
      deduplicate: merge.deduplicate,
      overrides: (merge.overrides ?? {}) as JsonValue,
      sections: merge.sections,
      strategy: merge.strategy,
    };
  }

  /**
   * What each adjusting repository ends up with, composed once.
   *
   * A merge is a deep walk of two documents, and the row asks three questions
   * of it - the file, the reason there is not one, and which lines this
   * repository decides - each of which used to compose the whole thing again.
   * Keyed by the repository, and rebuilt when the template or the adjustments
   * move, which is when the answer can differ.
   */
  /** One repository's answer: the file, or why there is not one. */
  type Composed = { result?: string; refusal?: string; overridden: number[] };

  const composed: Map<string, Composed> = $derived(
    /* Built from entries rather than by writing into a Map afterwards: a Map a
       component mutates is state Svelte cannot see, which is what
       `prefer-svelte-reactivity` is watching for. */
    new Map(
      templateJson === undefined
        ? []
        : adjusting.map(({ row, merge }): [string, Composed] => {
            const spec = specOf(merge);
            // A spec that says nothing is not composed at all - the service
            // hands the template's own bytes over, in the order they were
            // written.
            if (composesNothing(spec)) {
              return [row.repository_id, { result: template, overridden: [] }];
            }

            const built = composeFile(path, templateJson, spec);
            const result = built.ok ? formatJson(built.value) : undefined;

            return [
              row.repository_id,
              {
                result,
                refusal: built.ok ? undefined : built.reason,
                overridden:
                  result === undefined
                    ? []
                    : markedLines(result, (merge.overrides ?? {}) as JsonValue),
              },
            ];
          }),
    ),
  );

  /** What one repository ends up with, composed the way the service composes it. */
  function resultFor(row: SyncOverrideRow): string | undefined {
    return composed.get(row.repository_id)?.result;
  }

  /** Why there is no composed file for this repository, where there is not one. */
  function refusalFor(row: SyncOverrideRow): string | undefined {
    return composed.get(row.repository_id)?.refusal;
  }

  /** Which of the result's lines this repository decides rather than the template. */
  function overriddenLines(row: SyncOverrideRow): number[] {
    return composed.get(row.repository_id)?.overridden ?? [];
  }

  function openTemplate(): void {
    draft = template;
    refused = null;
    editingRepository = null;
    editingTemplate = true;
  }

  function openRepository(row: SyncOverrideRow): void {
    draft = resultFor(row) ?? '';
    refused = null;
    editingTemplate = false;
    editingRepository = row.repository_id;
  }

  /**
   * Write the template, and hold the editor open if the write is refused.
   *
   * The editor used to close first. A rejected save - a 409 from somebody
   * else's edit, most of all - then left the page saying why beside a surface
   * that no longer held what had been typed, and `openTemplate` reads the
   * server's copy back, so there was no way to it at all.
   */
  async function saveTemplate(): Promise<void> {
    if (at === -1 || draft === template) {
      editingTemplate = false;

      return;
    }
    const wrote = await onSave({
      ...stored,
      files: files.map((one, index) => (index === at ? { ...one, content: draft } : one)),
    });
    if (wrote === false) return;
    editingTemplate = false;
  }

  /**
   * What was typed, back as the adjustment that produces it.
   *
   * Derived through the same composer that drew the surface, and then checked
   * by composing the derivation again: a merge only has an inverse where its
   * rules have one, and a deduplicated append does not. Refused rather than
   * stored wherever the check fails, and wherever RFC 7396 cannot say it - a
   * key set to `null` means "remove this key" in a patch, so storing one would
   * mean something other than what somebody typed.
   */
  async function saveResult(row: SyncOverrideRow, merge: SyncFileMerge): Promise<void> {
    const wanted = parseJson(draft);
    if (wanted === undefined) {
      refused = 'That is not valid JSON, so nothing was stored';

      return;
    }
    if (templateJson === undefined || onSaveAdjustment === undefined) return;

    const derived = deriveOverrides(path, templateJson, specOf(merge), wanted);
    if (!derived.ok) {
      refused = derived.reason;

      return;
    }

    const merges = storedList<SyncFileMerge>(row.document, 'merges').map((one) =>
      one.path === path ? { ...one, overrides: derived.overrides as Record<string, unknown> } : one,
    );
    const wrote = await onSaveAdjustment(row.repository_id, { ...row.document, merges });
    if (wrote === false) return;
    refused = null;
    editingRepository = null;
  }

  /** Stop adjusting this file here: the template's own content returns. */
  function stopAdjusting(row: SyncOverrideRow): void {
    if (onSaveAdjustment === undefined) return;
    editingRepository = null;
    onSaveAdjustment(row.repository_id, {
      ...row.document,
      merges: storedList<SyncFileMerge>(row.document, 'merges').filter((one) => one.path !== path),
    });
  }

  /** What one repository changes, in the words its own row carries. */
  function changesOf(row: SyncOverrideRow, merge: SyncFileMerge): string {
    const refusal = refusalFor(row);
    // Said on the row, because a merge the service refuses is a repository that
    // gets no file at all, and the row is the only place this page names it.
    if (refusal !== undefined) return `cannot be composed — ${refusal.toLowerCase()}`;
    const keys = patchedKeys((merge.overrides ?? {}) as JsonValue);
    if (keys.length === 0) return 'changes nothing yet';
    const rules = (merge.arrays ?? []).length;
    const lists = rules === 0 ? '' : `, ${rules} list ${rules === 1 ? 'rule' : 'rules'}`;

    return `changes ${keys.length} ${keys.length === 1 ? 'key' : 'keys'}${lists} — ${keys.join(', ')}`;
  }

  /** The one thing a merge cannot infer, asked where it arises. */
  function listQuestion(merge: SyncFileMerge): string | undefined {
    if (templateJson === undefined) return undefined;

    return sharedArrays(templateJson, (merge.overrides ?? {}) as JsonValue)[0];
  }

  const LIST_RULES = [
    { value: 'append', title: 'Append', why: "The repository's entries follow the template's" },
    { value: 'prepend', title: 'Prepend', why: "The repository's entries come first" },
    { value: 'replace', title: 'Replace', why: "The repository's list stands alone" },
  ];

  /* Replace is what a list with no rule gets, so it is also what the control
     shows for one - the reader is choosing among three, not among three and an
     absence. */
  function ruleFor(merge: SyncFileMerge, jsonPath: string): ArrayStrategy {
    return merge.arrays?.find((rule) => rule.path === jsonPath)?.strategy ?? 'replace';
  }

  function setRule(row: SyncOverrideRow, jsonPath: string, chosen: string): void {
    const strategy = asArrayStrategy(chosen);
    if (onSaveAdjustment === undefined || strategy === undefined) return;

    const merges = storedList<SyncFileMerge>(row.document, 'merges').map((one) => {
      if (one.path !== path) return one;
      const rules = one.arrays ?? [];
      const at = rules.findIndex((rule) => rule.path === jsonPath);

      /* In place where the rule is already there. Rules are an ordered list
         because two of them on one document have to resolve the same way
         twice, so dropping the old one and pushing the new on the end would
         move a rule behind a rule it used to run before - changing a
         strategy would quietly change which rule wins. */
      return {
        ...one,
        arrays:
          at < 0
            ? [...rules, { path: jsonPath, strategy }]
            : rules.with(at, { path: jsonPath, strategy }),
      };
    });
    onSaveAdjustment(row.repository_id, { ...row.document, merges });
  }
</script>

<BackLink href={listHref} label="Files" tone="quiet" />

{#if file === undefined}
  <PageHeader
    id="file-heading"
    title={path}
    description="No template here has that path. It may have been renamed or retired since this address was written down"
  />
{:else}
  <PageHeader
    id="file-heading"
    title={path}
    description="In {repositories} {repositories === 1
      ? 'repository'
      : 'repositories'}{updatedAt === ''
      ? ''
      : ` · updated ${formatRelative(updatedAt, now)}${updatedBy === '' ? '' : ` by ${updatedBy}`}`}"
  />

  {#if problem !== null}
    <p class="file-problem" role="alert">{problem}</p>
  {/if}

  <div class="file-page">
    <Plate label="Template">
      {#snippet status()}
        <span class="head-tools">
          <Chip tone="neutral" small>{adjusting.length === 0 ? 'replaces' : 'merges'}</Chip>
          {#if !readOnly}
            {#if editingTemplate}
              <Button tone="quiet" onclick={() => (editingTemplate = false)}>Cancel</Button>
              <Button tone="signal" {disabled} onclick={saveTemplate}>Done</Button>
            {:else}
              <!-- Named rather than "Edit": the rows below carry an Edit of
                   their own, and the two open different files. -->
              <Button tone="quiet" {disabled} onclick={openTemplate}>Edit the template</Button>
            {/if}
          {/if}
        </span>
      {/snippet}

      {#if editingTemplate}
        <CodeEditor bind:value={draft} {language} label="{path} template" {disabled} />
      {:else if template === ''}
        <p class="file-note">
          Nothing yet. Whatever is typed here is written to every repository that does not adjust it
        </p>
      {:else}
        <CodeBlock lines={codeLines(template)} {language} label="{path} template" />
      {/if}
    </Plate>

    <Plate label="Repository adjustments">
      {#snippet status()}
        <span class="file-count">
          {adjusting.length} of {repositories}
          {repositories === 1 ? 'repository changes' : 'repositories change'} this file
        </span>
      {/snippet}

      {#if adjusting.length === 0}
        <p class="file-note">
          Every repository gets this file exactly as it is written above. One that needs something
          different can say so on its own page
        </p>
      {:else}
        <div class="object-list">
          {#each adjusting as { row, merge } (row.repository_id)}
            <ObjectRow name={row.repository_name} summary={changesOf(row, merge)}>
              {#snippet action()}
                {#if !readOnly}
                  {#if editingRepository === row.repository_id}
                    <Button tone="quiet" onclick={() => (editingRepository = null)}>Close</Button>
                  {:else if composable(path)}
                    <Button tone="quiet" {disabled} onclick={() => openRepository(row)}>
                      Edit
                    </Button>
                  {/if}
                  <Button tone="quiet" {disabled} onclick={() => stopAdjusting(row)}>
                    Stop adjusting
                  </Button>
                {/if}
              {/snippet}
            </ObjectRow>

            {#if editingRepository === row.repository_id}
              {@const question = listQuestion(merge)}
              <div class="merge-result">
                <div class="merge-title">
                  <span class="cap-trim"
                    >What {row.repository_name} ends up with — edit it here</span
                  >
                  <span class="merge-tools">
                    <Button tone="quiet" onclick={() => (editingRepository = null)}>Cancel</Button>
                    <Button tone="signal" {disabled} onclick={() => saveResult(row, merge)}>
                      Save
                    </Button>
                  </span>
                </div>

                <CodeEditor
                  bind:value={draft}
                  {language}
                  label="What {row.repository_name} ends up with"
                  overridden={overriddenLines(row)}
                  {disabled}
                />

                {#if refused !== null}
                  <p class="merge-refused" role="alert">{refused}</p>
                {/if}

                <div class="patch-strip">
                  <span class="patch-word">This repository changes</span>
                  {#each patchedKeys((merge.overrides ?? {}) as JsonValue) as key (key)}
                    <Chip tone="neutral" small>{key}</Chip>
                  {:else}
                    <span class="patch-word">nothing yet</span>
                  {/each}
                </div>

                {#if question !== undefined}
                  <!-- The one thing a merge cannot infer: two lists can be
                       joined three ways and none is more correct, so it is
                       asked where it arises rather than guessed. -->
                  <div class="list-ask">
                    <p class="list-ask-say">
                      Both set <code>{question.replace('$.', '')}</code>. A merge cannot know how
                      two lists should combine, so this is the one question it asks
                    </p>
                    <ChoiceCards
                      name="list-rule-{row.repository_id}"
                      label="How the two lists combine"
                      options={LIST_RULES}
                      value={ruleFor(merge, question)}
                      {disabled}
                      onSelect={(next) => setRule(row, question, next)}
                    />
                  </div>
                {/if}
              </div>
            {/if}
          {/each}
        </div>
      {/if}

      {#if !composable(path)}
        <p class="file-note">
          A {language} file is composed by rules this page cannot reproduce, so an adjustment here is
          named rather than drawn. The plan says exactly what each repository would end up with
        </p>
      {/if}
    </Plate>
  </div>
{/if}

<style>
  .file-page {
    display: grid;
    gap: var(--space-4);
    margin-top: var(--space-4);
  }

  .file-problem {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
    padding: var(--space-2) var(--space-3);
  }

  .head-tools,
  .merge-tools {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  .file-count,
  .file-note {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: 0;
    max-width: 68ch;
  }

  .file-note {
    margin-top: var(--space-2);
  }

  .object-list {
    display: grid;
  }

  /* The editor belongs to the row above it, so it is set in and ruled down the
     side rather than left to float between two rows it could belong to. */
  .merge-result {
    border-inline-start: 2px solid var(--managed-bar);
    display: grid;
    gap: var(--space-3);
    margin: 0 0 var(--space-3) var(--space-2);
    padding-inline-start: var(--space-3);
  }

  .merge-title {
    align-items: center;
    color: var(--text-secondary);
    display: flex;
    flex-wrap: wrap;
    font-size: var(--font-size-compact);
    gap: var(--space-3);
    justify-content: space-between;
  }

  .merge-refused {
    color: var(--danger);
    font-size: var(--font-size-compact);
    margin: 0;
  }

  .patch-strip {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .patch-word {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .list-ask {
    display: grid;
    gap: var(--space-3);
  }

  .list-ask-say {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    margin: 0;
    max-width: 68ch;
  }
</style>
