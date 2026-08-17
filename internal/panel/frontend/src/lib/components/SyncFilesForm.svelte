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
  import { asList, lines, patchedAt, rowKeys, storedList, withoutAt } from '#lib/form-lists.js';
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
  let drafts = $derived<SyncFile[]>(storedList<SyncFile>(stored, 'files'));
  let retired = $derived<string[]>(storedList<string>(stored, 'retired'));
  let excludes = $derived<string[]>(storedList<string>(stored, 'excludes'));
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
        storedList<SyncFile>(stored, 'files'),
        storedList<string>(stored, 'retired'),
        storedList<string>(stored, 'excludes'),
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

  function patch(index: number, change: Partial<SyncFile>): void {
    drafts = patchedAt(drafts, index, change);
  }

  function add(): void {
    drafts = [...drafts, { path: '', content: '' }];
  }

  function remove(index: number): void {
    drafts = withoutAt(drafts, index);
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

  <label class="entry-field">
    <span class="entry-field-label">Paths to remove</span>
    <textarea
      rows="2"
      {disabled}
      aria-describedby="files-retired-note"
      value={lines(retired)}
      placeholder=".github/workflows/sync-trigger.yml"
      onchange={(event) => (retired = asList(event.currentTarget.value))}></textarea>
  </label>
  <p class="form-note file-note" id="files-retired-note">
    One path per line. These are deleted wherever a repository still has them, and this is the only
    thing here that deletes anything.
  </p>

  <label class="entry-field">
    <span class="entry-field-label">Paths to leave alone</span>
    <textarea
      rows="2"
      {disabled}
      aria-describedby="files-excludes-note"
      value={lines(excludes)}
      placeholder="LICENSE"
      onchange={(event) => (excludes = asList(event.currentTarget.value))}></textarea>
  </label>
  <p class="form-note file-note" id="files-excludes-note">
    One path or pattern per line, where <code>*</code> stands for any run of characters. These are neither
    written nor removed, whatever the lists say.
  </p>

  {#if drafts.length === 0}
    <p class="form-note files-empty">No files yet.</p>
  {/if}

  {#each drafts as file, index (rowKey(index))}
    <article class="entry-card">
      <div class="file-row">
        <label class="file-path">
          <span class="entry-field-label">Path</span>
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

      <label class="entry-field">
        <span class="entry-field-label">Content</span>
        <textarea
          class="entry-code"
          rows="8"
          {disabled}
          aria-describedby="files-content-note"
          value={file.content}
          placeholder="# Contributing"
          onchange={(event) => patch(index, { content: event.currentTarget.value })}></textarea>
      </label>
    </article>
  {/each}

  <p class="form-note file-note" id="files-content-note">
    <code>{'{{DEFAULT_BRANCH}}'}</code> is filled in with whatever each repository calls its default branch.
    Anything else in braces is refused, so a template cannot reach a repository with a placeholder nobody
    fills in.
  </p>
</SyncDocumentForm>

<style>
  /* The global rule has no margin. These notes sit directly under the control
     they describe, and the sliver of side inset lines them up with the field's
     own text. */
  .file-note {
    margin: 0.25rem 0.125rem 0;
  }

  .files-empty {
    margin: var(--space-4) 0 0;
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
</style>
