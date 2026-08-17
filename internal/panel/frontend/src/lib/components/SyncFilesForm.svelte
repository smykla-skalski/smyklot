<script lang="ts">
  /**
   * The files an installation expects its repositories to carry.
   *
   * The template is here rather than in a repository somewhere. The tool this
   * replaces kept them in one repository and fetched each of them per
   * repository per run, and when a fetch failed the file was skipped with a
   * warning while the run reported success.
   *
   * Deletion is a named list of retired paths and nothing else. There is no
   * switch: the tool this replaces published one promising to delete every file
   * not in the central configuration, which is every file in the repository,
   * documented it as dangerous, and never implemented it. Naming a path is the
   * only way to have it removed, and naming it is the consent.
   *
   * Nothing is sent until Save, and nothing reaches GitHub until a plan is
   * approved. What a repository would end up with arrives as a pull request it
   * can close.
   */
  import { canonicalStringify } from '#lib/preferences-sync.js';
  import { asList, lines, rowKeys } from '#lib/form-lists.js';
  import type { SyncFile } from '#lib/types.js';

  import SyncDocumentForm from './SyncDocumentForm.svelte';

  const {
    stored,
    enabled,
    unreadable,
    unavailable = '',
    problem = null,
    readOnly,
    saving,
    onSave,
  }: {
    stored: Record<string, unknown>;
    enabled: boolean;
    unreadable: boolean;
    unavailable?: string;
    problem?: string | null;
    readOnly: boolean;
    saving: boolean;
    onSave: (enabled: boolean, document: Record<string, unknown>) => void;
  } = $props();

  /* Derived from what is saved and written over as somebody edits, so a save
     landing from anywhere reseeds it rather than leaving the screen describing
     a document that is gone. */
  let drafts = $derived<SyncFile[]>(storedFiles(stored));
  let retired = $derived<string[]>(storedList(stored, 'retired'));
  let excludes = $derived<string[]>(storedList(stored, 'excludes'));
  let wanted = $derived(enabled);

  const disabled = $derived(saving || readOnly || unreadable);

  /* The whole document rather than the parts with controls, so a key a newer
     version of the service wrote is sent back rather than dropped by a browser
     running an older build of this page. The server refuses it by name, which
     is the point: after a rollback somebody is told the document holds
     something this version does not understand, rather than saving over it. */
  const payload = $derived(asDocument(drafts, retired, excludes));

  /* What a save would send if nobody touched anything, rather than the stored
     document. The two differ on a kind nobody has configured, where the
     document is empty and this is three keys with their defaults, and comparing
     against the wrong one offers a save the moment the page loads. */
  const untouched = $derived(
    canonicalStringify(
      asDocument(
        storedFiles(stored),
        storedList(stored, 'retired'),
        storedList(stored, 'excludes'),
      ),
    ),
  );

  const changed = $derived(wanted !== enabled || canonicalStringify(payload) !== untouched);

  /** Named asDocument rather than document, which is a global this would hide. */
  function asDocument(
    files: SyncFile[],
    retiredPaths: string[],
    excluded: string[],
  ): Record<string, unknown> {
    return { ...stored, files, retired: retiredPaths, excludes: excluded };
  }

  function storedFiles(from: Record<string, unknown>): SyncFile[] {
    return Array.isArray(from.files) ? (from.files as SyncFile[]) : [];
  }

  function storedList(from: Record<string, unknown>, key: string): string[] {
    return Array.isArray(from[key]) ? (from[key] as string[]) : [];
  }

  function patch(index: number, change: Partial<SyncFile>): void {
    drafts = drafts.map((file, at) => (at === index ? { ...file, ...change } : file));
  }

  function add(): void {
    drafts = [...drafts, { path: '', content: '' }];
  }

  function remove(index: number): void {
    drafts = drafts.filter((_, at) => at !== index);
  }

  const rowKey = rowKeys('file');
</script>

<SyncDocumentForm
  heading="Shared files"
  noun="files"
  lead="What every repository in this installation should carry, and what it should say. A file
        that differs arrives as a pull request the repository can merge or close"
  enabled={wanted}
  {unreadable}
  {unavailable}
  {problem}
  {readOnly}
  {saving}
  {changed}
  {disabled}
  onToggle={(value) => (wanted = value)}
  onSave={() => onSave(wanted, payload)}
>
  {#snippet actions()}
    <!-- Every bare word inside a button is wrapped, here and below: a button is
         a flex container, so its text sits in an anonymous box no selector can
         reach, and `text-box` on the button itself never touches it. See
         `.button-label` in `app.css`. Unwrapped, each of these sat 0.47px high. -->
    <button class="btn btn-quiet" type="button" {disabled} onclick={add}>
      <span class="button-label">Add a file</span>
    </button>
  {/snippet}

  <label class="file-field">
    <span class="file-field-label">Paths to remove</span>
    <textarea
      rows="2"
      {disabled}
      aria-describedby="files-retired-note"
      value={lines(retired)}
      placeholder=".github/workflows/sync-trigger.yml"
      onchange={(event) => (retired = asList(event.currentTarget.value))}></textarea>
  </label>
  <p class="file-note" id="files-retired-note">
    One path per line. These are deleted wherever a repository still has them, and this is the only
    thing here that deletes anything.
  </p>

  <label class="file-field">
    <span class="file-field-label">Paths to leave alone</span>
    <textarea
      rows="2"
      {disabled}
      aria-describedby="files-excludes-note"
      value={lines(excludes)}
      placeholder="LICENSE"
      onchange={(event) => (excludes = asList(event.currentTarget.value))}></textarea>
  </label>
  <p class="file-note" id="files-excludes-note">
    One path or pattern per line, where <code>*</code> stands for any run of characters. These are neither
    written nor removed, whatever the lists say.
  </p>

  {#if drafts.length === 0}
    <p class="files-empty">No files yet.</p>
  {/if}

  {#each drafts as file, index (rowKey(index))}
    <article class="file">
      <div class="file-row">
        <label class="file-path">
          <span class="file-field-label">Path</span>
          <input
            type="text"
            value={file.path}
            {disabled}
            placeholder="CONTRIBUTING.md"
            onchange={(event) => patch(index, { path: event.currentTarget.value })}
          />
        </label>

        {#if !readOnly}
          <button class="btn btn-quiet" type="button" {disabled} onclick={() => remove(index)}>
            <span class="button-label">Remove</span>
          </button>
        {/if}
      </div>

      <label class="file-field">
        <span class="file-field-label">Content</span>
        <textarea
          class="file-content"
          rows="8"
          {disabled}
          aria-describedby="files-content-note"
          value={file.content}
          placeholder="# Contributing"
          onchange={(event) => patch(index, { content: event.currentTarget.value })}></textarea>
      </label>
    </article>
  {/each}

  <p class="file-note" id="files-content-note">
    <code>{'{{DEFAULT_BRANCH}}'}</code> is filled in with whatever each repository calls its default branch.
    Anything else in braces is refused, so a template cannot reach a repository with a placeholder nobody
    fills in.
  </p>
</SyncDocumentForm>

<style>
  .file-note {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0.25rem 0.125rem 0;
    max-width: 60ch;
  }

  .files-empty {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: var(--space-4) 0 0;
  }

  /* A card per file, because a path and the whole of its contents is more than
     a row: the rulesets beside this are grouped the same way and for the same
     reason. */
  .file {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin-top: var(--space-4);
    padding: var(--space-3);
  }

  .file-row {
    align-items: flex-end;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .file-path {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 14rem;
  }

  .file-field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    margin-top: var(--space-4);
  }

  /* Inside a card the field is already spaced by the card's gap, so the margin
     above would double it. */
  .file .file-field {
    margin-top: 0;
  }

  .file-field-label {
    font-size: var(--font-size-micro);
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  /* A template is read as code, so it is shown as code: proportional type turns
     an aligned table in a Markdown file into a ragged one. */
  .file-content {
    font-family: var(--mono);
    font-size: var(--font-size-meta);
  }
</style>
