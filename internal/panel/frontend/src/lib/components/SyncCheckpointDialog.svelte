<script module lang="ts">
  import type { SyncConfigCheckpointState, SyncKind } from '#lib/types.js';

  export function syncCheckpointSummary(
    kind: SyncKind,
    state: SyncConfigCheckpointState | null,
  ): string {
    if (state === null) return 'Not configured';
    const prefix = state.enabled ? 'On' : 'Off';
    const document = state.document;
    switch (kind) {
      case 'labels': {
        const labels = Array.isArray(document.labels) ? document.labels.length : 0;
        const excludes = Array.isArray(document.excludes) ? document.excludes.length : 0;
        return `${prefix} · ${labels} ${labels === 1 ? 'label' : 'labels'} · ${document.allow_removal === true ? 'removal allowed' : 'removal blocked'} · ${excludes} ${excludes === 1 ? 'exclusion' : 'exclusions'}`;
      }
      case 'settings': {
        const count = Object.keys(document).length;
        return `${prefix} · ${count} managed ${count === 1 ? 'setting' : 'settings'}`;
      }
      case 'rulesets': {
        const count = Array.isArray(document.rulesets) ? document.rulesets.length : 0;
        return `${prefix} · ${count} ${count === 1 ? 'ruleset' : 'rulesets'} · ${document.allow_removal === true ? 'removal allowed' : 'removal blocked'}`;
      }
      case 'files': {
        const files = Array.isArray(document.files) ? document.files.length : 0;
        const retired = Array.isArray(document.retired) ? document.retired.length : 0;
        return `${prefix} · ${files} shared ${files === 1 ? 'file' : 'files'} · ${retired} retired`;
      }
    }
  }
</script>

<script lang="ts">
  import { untrack } from 'svelte';

  import { formatDateTime, formatTimestamp } from '#lib/format.js';
  import type {
    SyncConfigBatchResponse,
    SyncConfigCheckpoint,
    SyncConfigRestoreInput,
  } from '#lib/types.js';

  import Avatar from './Avatar.svelte';
  import Button from './Button.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import Modal from './Modal.svelte';
  import Switch from './Switch.svelte';

  const {
    open,
    targetId,
    checkpointId,
    readOnly,
    hasUnsavedDrafts,
    returnFocus = null,
    fetchCheckpoint,
    restoreCheckpoint,
    onRestored,
    onClose,
  }: {
    open: boolean;
    targetId: string;
    checkpointId: string;
    readOnly: boolean;
    hasUnsavedDrafts: boolean;
    returnFocus?: HTMLElement | null;
    fetchCheckpoint: (targetId: string, checkpointId: string) => Promise<SyncConfigCheckpoint>;
    restoreCheckpoint: (
      targetId: string,
      checkpointId: string,
      input: SyncConfigRestoreInput,
    ) => Promise<SyncConfigBatchResponse>;
    onRestored: (result: SyncConfigBatchResponse) => void;
    onClose: () => void;
  } = $props();

  let checkpoint = $state<SyncConfigCheckpoint | null>(null);
  let selected = $state<SyncKind[]>([]);
  let loading = $state(false);
  let restoring = $state(false);
  let confirming = $state(false);
  let problem = $state<string | null>(null);
  let rawOpen = $state<SyncKind[]>([]);
  let generation = 0;

  const differingCount = $derived(
    checkpoint?.kinds.filter((kind) => kind.differs_from_current).length ?? 0,
  );

  $effect(() => {
    const wanted = open ? `${targetId}/${checkpointId}` : '';
    untrack(() => {
      if (wanted !== '') void loadCheckpoint(targetId, checkpointId);
    });
  });

  async function loadCheckpoint(target: string, id: string): Promise<void> {
    const currentGeneration = (generation += 1);
    loading = true;
    problem = null;
    checkpoint = null;
    selected = [];
    rawOpen = [];
    confirming = false;
    try {
      const loaded = await fetchCheckpoint(target, id);
      if (generation !== currentGeneration) return;
      checkpoint = loaded;
      selected = loaded.kinds.filter((kind) => kind.differs_from_current).map((kind) => kind.kind);
    } catch (cause) {
      if (generation === currentGeneration) problem = messageOf(cause);
    } finally {
      if (generation === currentGeneration) loading = false;
    }
  }

  function toggleKind(kind: SyncKind, checked: boolean): void {
    selected = checked ? [...selected, kind] : selected.filter((held) => held !== kind);
    confirming = false;
  }

  function toggleRaw(kind: SyncKind): void {
    rawOpen = rawOpen.includes(kind) ? rawOpen.filter((held) => held !== kind) : [...rawOpen, kind];
  }

  function requestRestore(): void {
    if (selected.length > 0 && !hasUnsavedDrafts && !readOnly) confirming = true;
  }

  async function restore(): Promise<void> {
    const held = checkpoint;
    if (held === null || selected.length === 0 || hasUnsavedDrafts || readOnly) return;
    const kinds = selected.flatMap((kind) => {
      const state = held.kinds.find((candidate) => candidate.kind === kind);
      return state === undefined ? [] : [{ kind, expected_revision: state.current?.revision ?? 0 }];
    });
    if (kinds.length !== selected.length) return;

    restoring = true;
    problem = null;
    try {
      const result = await restoreCheckpoint(targetId, checkpointId, { kinds });
      onRestored(result);
      onClose();
    } catch (cause) {
      problem = messageOf(cause);
      confirming = false;
    } finally {
      restoring = false;
    }
  }

  function actionLabel(action: SyncConfigCheckpoint['action']): string {
    if (action === 'sync.config.restored') return 'Restored';
    if (action === 'sync.config.baseline') return 'Baseline';
    return 'Saved';
  }

  function kindLabel(kind: SyncKind): string {
    return kind[0]?.toLocaleUpperCase() + kind.slice(1);
  }

  function rawState(kind: SyncConfigCheckpoint['kinds'][number]): string {
    return JSON.stringify(
      { before: kind.before, after: kind.after, current: kind.current },
      null,
      2,
    );
  }

  function messageOf(cause: unknown): string {
    return cause instanceof Error ? cause.message : String(cause);
  }
</script>

<Modal
  id="sync-checkpoint-dialog"
  {open}
  title="Sync configuration history"
  description="Inspect the saved state and restore only the sections you choose."
  variant="wide"
  {returnFocus}
  {onClose}
>
  {#if loading && checkpoint === null}
    <p class="checkpoint-loading" role="status">Loading Sync configuration snapshot…</p>
  {:else if checkpoint !== null}
    <section class="snapshot-provenance" aria-label="Snapshot details">
      <div class="snapshot-author">
        <span class="snapshot-avatar"><Avatar account={checkpoint.actor} size={28} /></span>
        <div>
          <strong>{actionLabel(checkpoint.action)} by {checkpoint.actor.display_name}</strong>
          <time datetime={checkpoint.created_at} title={formatTimestamp(checkpoint.created_at)}
            >{formatDateTime(checkpoint.created_at)}</time
          >
        </div>
      </div>
      <div class="snapshot-count">
        <strong>{differingCount}</strong>
        <span>{differingCount === 1 ? 'section differs now' : 'sections differ now'}</span>
      </div>
    </section>

    <fieldset class="checkpoint-kinds">
      <legend>
        <span>Configuration snapshot</span>
        <small>
          <span class="selection-wide"
            >{selected.length} of {differingCount} selected to restore</span
          >
          <span class="selection-compact">{selected.length} selected</span>
        </small>
      </legend>
      {#each checkpoint.kinds as item (item.kind)}
        <article class:unchanged={!item.differs_from_current} class="checkpoint-kind">
          <header class="kind-heading">
            <strong>{kindLabel(item.kind)}</strong>
            <span class="kind-control">
              <span class:matches-current={!item.differs_from_current} class="kind-status">
                {item.differs_from_current ? 'Changed since this save' : 'Matches today'}
              </span>
              <Switch
                bare
                checked={selected.includes(item.kind)}
                label="Restore {kindLabel(item.kind)}"
                disabled={!item.differs_from_current || readOnly || restoring}
                onToggle={(checked) => toggleKind(item.kind, checked)}
              />
            </span>
          </header>
          <div class="state-comparison">
            <div>
              <span>Before save</span>
              <p>{syncCheckpointSummary(item.kind, item.before)}</p>
            </div>
            <span class="comparison-arrow" aria-hidden="true"
              ><Icon name="chevron-right" size={14} /></span
            >
            <div class="saved-state">
              <span>Saved state</span>
              <p>{syncCheckpointSummary(item.kind, item.after)}</p>
            </div>
          </div>
          <div class="raw-state">
            <button
              type="button"
              aria-expanded={rawOpen.includes(item.kind)}
              onclick={() => toggleRaw(item.kind)}
            >
              <Icon name="chevron-right" size={11} />
              <span>{rawOpen.includes(item.kind) ? 'Hide' : 'View'} stored state</span>
            </button>
            {#if rawOpen.includes(item.kind)}
              <pre>{rawState(item)}</pre>
            {/if}
          </div>
        </article>
      {/each}
    </fieldset>

    {#if hasUnsavedDrafts}
      <p class="checkpoint-notice" role="status">
        Save or discard the current Sync draft before restoring history.
      </p>
    {:else if readOnly}
      <p class="checkpoint-notice">You can inspect this snapshot, but cannot restore it.</p>
    {:else if selected.length === 0}
      <p class="checkpoint-notice">This snapshot already matches the current configuration.</p>
    {/if}

    {#if confirming}
      <p class="restore-confirmation" role="alert">
        Restore {selected.length}
        {selected.length === 1 ? 'section' : 'sections'}? This creates new active revisions and a
        new history entry. It does not change GitHub until a plan is approved.
      </p>
    {/if}
  {/if}

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#snippet footer()}
    {#if confirming}
      <Button tone="ghost" disabled={restoring} onclick={() => (confirming = false)}>Back</Button>
      <Button tone="signal" disabled={restoring} onclick={() => void restore()}>
        {restoring ? 'Restoring…' : 'Confirm restore'}
      </Button>
    {:else}
      <Button tone="ghost" disabled={restoring} onclick={onClose}>Close</Button>
      {#if checkpoint !== null && !readOnly}
        <Button
          tone="signal"
          disabled={selected.length === 0 || hasUnsavedDrafts || loading || restoring}
          onclick={requestRestore}>Restore selected</Button
        >
      {/if}
    {/if}
  {/snippet}
</Modal>

<style>
  .checkpoint-loading,
  .checkpoint-notice,
  .restore-confirmation {
    margin: 0;
  }

  .snapshot-provenance {
    align-items: center;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    display: flex;
    gap: var(--space-3);
    min-height: 3.5rem;
    padding: var(--space-2) var(--space-3);
  }

  .snapshot-author {
    align-items: center;
    display: flex;
    flex: 1;
    gap: var(--space-3);
    min-width: 0;
  }

  .snapshot-avatar {
    display: inline-flex;
  }

  .snapshot-author > div {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
  }

  .snapshot-author strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .snapshot-author time {
    color: var(--dim);
    font-size: var(--font-size-compact);
  }

  .snapshot-count {
    align-items: baseline;
    border-left: 1px solid var(--border-subtle);
    display: flex;
    gap: var(--space-2);
    padding-left: var(--space-3);
  }

  .snapshot-count strong {
    color: var(--diff-chg-ink);
    font: 700 var(--font-size-title) / 1 var(--mono);
    font-variant-numeric: tabular-nums;
  }

  .snapshot-count span {
    color: var(--dim);
    font-size: var(--font-size-micro);
    white-space: nowrap;
  }

  .checkpoint-kinds {
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    display: grid;
    margin: 0;
    min-width: 0;
    overflow: hidden;
    padding: 0;
  }

  .checkpoint-kinds legend {
    align-items: baseline;
    background: var(--surface-inset);
    box-sizing: border-box;
    display: flex;
    float: left;
    justify-content: space-between;
    min-height: 2.5rem;
    padding: var(--space-2) var(--space-3);
    width: 100%;
  }

  .checkpoint-kinds legend + * {
    clear: both;
  }

  .checkpoint-kinds legend > span {
    font-size: var(--font-size-meta);
    font-weight: 700;
  }

  .checkpoint-kinds legend small {
    color: var(--dim);
    font-size: var(--font-size-micro);
    font-weight: 500;
  }

  .selection-compact {
    display: none;
  }

  .checkpoint-kind {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
  }

  .checkpoint-kind + .checkpoint-kind {
    border-top: 1px solid var(--border-subtle);
  }

  .checkpoint-kind:not(.unchanged) {
    background: color-mix(in srgb, var(--diff-chg-ink) 3.5%, var(--surface-base));
  }

  .kind-heading {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    justify-content: space-between;
    min-height: 1.25rem;
    min-width: 0;
  }

  .kind-heading > strong {
    font-size: var(--font-size-meta);
  }

  .kind-control {
    align-items: center;
    display: flex;
    gap: var(--space-3);
  }

  .kind-status {
    color: var(--diff-chg-ink);
    font-size: var(--font-size-micro);
    font-weight: 600;
    white-space: nowrap;
  }

  .kind-status.matches-current {
    color: var(--success);
  }

  .state-comparison {
    align-items: center;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  }

  .state-comparison > div {
    align-items: baseline;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: auto minmax(0, 1fr);
    min-width: 0;
  }

  .state-comparison > div > span {
    color: var(--dim);
    display: block;
    font-size: var(--font-size-micro);
    font-weight: 600;
    letter-spacing: 0.025em;
    white-space: nowrap;
  }

  .state-comparison p {
    font-size: var(--font-size-compact);
    margin: 0;
    overflow-wrap: anywhere;
  }

  .saved-state {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    padding: var(--space-2);
  }

  .comparison-arrow {
    color: var(--dim);
    display: inline-flex;
  }

  .raw-state > button {
    align-items: center;
    background: transparent;
    border: 0;
    color: var(--text-secondary);
    cursor: pointer;
    display: inline-flex;
    font-size: var(--font-size-compact);
    gap: var(--space-1);
    padding: 0;
  }

  .raw-state > button :global(svg) {
    color: var(--dim);
    transition: rotate var(--duration-fast) var(--ease-standard);
  }

  .raw-state > button[aria-expanded='true'] :global(svg) {
    rotate: 90deg;
  }

  .raw-state > button:hover span,
  .raw-state > button:focus-visible span {
    text-decoration: underline;
  }

  .checkpoint-kind pre {
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    font: var(--font-size-compact) / 1.5 var(--mono);
    margin: var(--space-2) 0 0;
    max-height: 18rem;
    overflow: auto;
    padding: var(--space-3);
    white-space: pre-wrap;
  }

  .checkpoint-notice,
  .restore-confirmation {
    background: var(--accent-tint);
    border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    padding: var(--space-3);
  }

  @media (max-width: 34rem) {
    .selection-wide {
      display: none;
    }

    .selection-compact {
      display: inline;
    }

    .snapshot-provenance {
      align-items: flex-start;
      display: grid;
      grid-template-columns: minmax(0, 1fr);
    }

    .snapshot-author {
      align-items: flex-start;
    }

    .snapshot-count {
      border-left: 0;
      border-top: 1px solid var(--border-subtle);
      gap: var(--space-2);
      grid-column: 1 / -1;
      min-width: 0;
      padding: var(--space-3) 0 0;
    }

    .snapshot-count span {
      margin: 0;
    }

    .kind-heading {
      align-items: center;
    }

    .kind-control {
      gap: var(--space-2);
    }

    .state-comparison {
      align-items: stretch;
      grid-template-columns: minmax(0, 1fr);
    }

    .comparison-arrow {
      rotate: 90deg;
      width: min-content;
    }
  }
</style>
