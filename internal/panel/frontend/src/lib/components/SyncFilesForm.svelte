<script lang="ts">
  /**
   * The files an installation expects its repositories to carry, as a list of
   * named things.
   *
   * The template is here rather than in a repository somewhere. The tool this
   * replaces kept them in one repository and fetched each of them per
   * repository per run, and when a fetch failed the file was skipped with a
   * warning while the run reported success.
   *
   * Deletion is a named list of retired paths and nothing else. There is no
   * switch: the tool this replaces published one promising to delete every file
   * not in the central configuration - which is every file in the repository -
   * documented it as dangerous, and never implemented it. Naming a path is the
   * only way to have it removed, and naming it is the consent.
   *
   * Adding one goes through the finder rather than an empty box, because typing
   * a path from memory is guessing at a string that has to match, character for
   * character, something the reader cannot see.
   */
  import { storedList } from '#lib/form-lists.js';
  import { formatRelative } from '#lib/format.js';
  import type { SyncFile, SyncFileMerge, SyncOverrideRow } from '#lib/types.js';

  import Chip from './Chip.svelte';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import ObjectRow from './ObjectRow.svelte';
  import PathFinder, { type KnownPath } from './PathFinder.svelte';
  import PatternList from './PatternList.svelte';
  import Plate from './Plate.svelte';
  import PolicyRow from './PolicyRow.svelte';
  import StateMark, { type SyncState } from './StateMark.svelte';
  import SyncKindHead from './SyncKindHead.svelte';

  const {
    stored,
    enabled,
    unreadable,
    unavailable = '',
    problem = null,
    readOnly,
    saving,
    fileHref,
    adjustments = [],
    paths = [],
    repositories = 0,
    pathsPartial = false,
    now = Date.now(),
    markOf,
    onSave,
  }: {
    stored: Record<string, unknown>;
    enabled: boolean;
    unreadable: boolean;
    unavailable?: string;
    problem?: string | null;
    readOnly: boolean;
    saving: boolean;
    /** Where one file's own page is. */
    fileHref: (path: string) => string;
    /** Every repository's answer about files, so a row can say who adjusts it. */
    adjustments?: readonly SyncOverrideRow[];
    /** Every path this installation's repositories are known to hold. */
    paths?: readonly KnownPath[];
    /** How many repositories the installation syncs, which the finder's counts are of. */
    repositories?: number;
    /** Whether GitHub declined to list one of them whole, so the list is short. */
    pathsPartial?: boolean;
    /** One clock for every relative time on the page. */
    now?: number;
    markOf?: (path: string) => { state: SyncState; label?: string } | undefined;
    onSave: (enabled: boolean, document: Record<string, unknown>) => void;
  } = $props();

  const disabled = $derived(saving || readOnly || unreadable);

  const files = $derived(storedList<SyncFile>(stored, 'files'));
  const retired = $derived(storedList<string>(stored, 'retired'));
  const excludes = $derived(storedList<string>(stored, 'excludes'));

  let adding = $state(false);
  let draft = $state('');

  /* The whole stored document rather than the keys with controls, so a key a
     newer version of the service wrote is sent back rather than dropped by a
     browser running an older build of this page. */
  function write(change: Record<string, unknown>): void {
    onSave(enabled, { ...stored, ...change });
  }

  function add(path: string): void {
    adding = false;
    draft = '';
    if (path === '' || files.some((file) => file.path === path)) return;
    write({ files: [...files, { path, content: '' }] });
  }

  /**
   * Every repository adjustment, gathered by the path it adjusts.
   *
   * One pass over the answers rather than a scan per question: each row asks
   * two - what shape the file arrives in, and how many repositories adjust it -
   * and each scan walked every adjustment of every repository.
   */
  const merges = $derived(
    /* `Map.groupBy` builds it, so there is no Map here that this component
       writes into after building - which is state Svelte cannot see, and what
       `prefer-svelte-reactivity` is watching for. */
    Map.groupBy(
      adjustments.flatMap((row) =>
        storedList<SyncFileMerge>(row.document, 'merges').map((merge) => ({
          repository: row.repository_name,
          merge,
        })),
      ),
      (one) => one.merge.path,
    ),
  );

  /** Shared, because every miss returns one and none of them is written to. */
  const NO_MERGES: readonly { repository: string; merge: SyncFileMerge }[] = [];

  /** Every repository adjustment of one path, from the answers already read. */
  function mergesOf(path: string): readonly { repository: string; merge: SyncFileMerge }[] {
    return merges.get(path) ?? NO_MERGES;
  }

  /**
   * How a file arrives, in the two words that separate the cases: a template
   * nobody adjusts is written whole, and one somebody adjusts is composed.
   *
   * Read from the repositories rather than from the template, because that is
   * where a merge strategy is decided - the installation says what the file
   * should say, and a repository says how its own differs.
   */
  function shapeOf(path: string): string {
    const merges = mergesOf(path);
    if (merges.length === 0) return 'replaces';
    const strategies = [
      ...new Set(merges.map(({ merge }) => merge.strategy).filter((one) => one !== undefined)),
    ];

    return strategies.length === 0 ? 'merges' : `merges · ${strategies.join(', ')}`;
  }

  function summaryOf(path: string): string {
    const count = mergesOf(path).length;

    return count === 0
      ? 'no adjustments'
      : `${count} ${count === 1 ? 'repository adjusts' : 'repositories adjust'} it`;
  }
</script>

<SyncKindHead
  title="Shared files"
  lead="What every repository should carry, and what it should say. A file that differs arrives as a pull request the repository can merge or close"
  noun="files"
  {enabled}
  {unreadable}
  {unavailable}
  {problem}
  {readOnly}
  {saving}
  onToggle={(next) => onSave(next, stored)}
/>

<Plate label="{files.length} {files.length === 1 ? 'template' : 'templates'}">
  {#snippet status()}
    {#if !readOnly && !adding}
      <!-- A Button rather than an `.add-chip`: this is the section's own
           action, and every control in a content pane is 34px. An add-chip is
           for the "+" that sits IN a row beside the chips it adds to - which is
           what "Add a path" below still is. Worn here it made the one action on
           the page the shortest control on it. -->
      <Button {disabled} onclick={() => (adding = true)}>
        {#snippet icon()}<Icon name="plus" size={14} strokeWidth={2} />{/snippet}
        Add a file
      </Button>
    {/if}
  {/snippet}

  {#if adding}
    <!-- The box answers with what exists: the same file across twenty-five
         repositories is one row carrying a count, because that is the thing
         being configured rather than twenty-five separate facts. -->
    <div class="files-add">
      <PathFinder
        bind:value={draft}
        {paths}
        {repositories}
        partial={pathsPartial}
        label="Path of the file to manage"
        onChoose={add}
      />
      <!-- The finder's own confirm, so it stands at the finder's height. As an
           `.add-chip` it was 24px beside a 34px input in one row. -->
      <Button tone="brand" onclick={() => add(draft)}>Manage this path</Button>
    </div>
  {/if}

  {#if files.length === 0}
    <p class="empty-note files-empty">
      No templates yet. A path named here is written to every repository this installation syncs
    </p>
  {:else}
    <div class="object-list ruled-rows">
      {#each files as file (file.path)}
        {@const fleet = markOf?.(file.path)}
        <ObjectRow name={file.path} href={fileHref(file.path)} summary={summaryOf(file.path)}>
          {#snippet pill()}
            <Chip tone="neutral" small>{shapeOf(file.path)}</Chip>
          {/snippet}
          {#snippet mark()}
            {#if fleet !== undefined}
              <StateMark state={fleet.state} label={fleet.label} />
            {/if}
          {/snippet}
        </ObjectRow>
      {/each}
    </div>
  {/if}
</Plate>

<Plate label="Paths this list does not write">
  <div class="file-settings">
    <PolicyRow
      name="Paths to remove"
      why="Deleted wherever a repository still has them. This is the only thing here that deletes anything"
    >
      {#snippet control()}
        <PatternList
          values={retired}
          label="Paths to remove"
          addLabel="Add a path"
          placeholder=".github/stale.yml"
          {disabled}
          onChange={(next) => write({ retired: next })}
        />
      {/snippet}
    </PolicyRow>

    <PolicyRow
      name="Paths to leave alone"
      why="Path or pattern, where * stands for any run of characters. Neither written nor removed, whatever the lists say"
    >
      {#snippet control()}
        <PatternList
          values={excludes}
          label="Paths to leave alone"
          addLabel="Add a pattern"
          placeholder="LICENSE-*"
          {disabled}
          onChange={(next) => write({ excludes: next })}
        />
      {/snippet}
    </PolicyRow>
  </div>

  <p class="files-note">
    <code>{'{{DEFAULT_BRANCH}}'}</code> is filled in with whatever each repository calls its default branch.
    Anything else in braces is refused, so a template cannot reach a repository with a placeholder nobody
    fills in
  </p>
  {#if adjustments.length > 0}
    <p class="files-note">
      Last adjusted {formatRelative(
        adjustments
          .map((row) => row.updated_at ?? '')
          .filter((at) => at !== '')
          .sort()
          .at(-1) ?? '',
        now,
      )}
    </p>
  {/if}
</Plate>

<style>
  .files-add {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
  }

  .object-list {
    display: grid;
  }

  /* The rule between two rows, and its manners around a hover, are
     `.ruled-rows` in app.css. An `.object-row` pads by `--space-2`, so that is
     where its rule stops. */
  .object-list {
    --row-rule-inset: var(--space-2);
  }

  .file-settings {
    display: grid;
  }

  .file-settings > :global(.policy-row + .policy-row) {
    border-top: 1px solid var(--border-subtle);
  }

  /* `.files-note` is the same copy in a different place, so it takes what
     `.empty-note` gives an empty list; both want a little air above. */
  .files-note {
    color: var(--dim);
    font-size: var(--font-size-meta);
    max-inline-size: 66ch;
  }

  .files-empty,
  .files-note {
    margin: var(--space-3) 0 0;
  }
</style>
