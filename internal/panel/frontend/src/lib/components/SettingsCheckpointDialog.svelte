<script lang="ts">
  import { untrack } from 'svelte';

  import { formatDateTime, formatTimestamp } from '#lib/format.js';
  import {
    settingsCheckpointActionLabel,
    settingsCheckpointItemLabel,
    settingsCheckpointSummary,
  } from '#lib/settings-checkpoint-summary.js';
  import type {
    SettingsCheckpoint,
    SettingsCheckpointItem,
    SettingsCheckpointRestoreSide,
    SettingsRestoreInput,
  } from '#lib/types.js';

  import Avatar from './Avatar.svelte';
  import Button from './Button.svelte';
  import FormError from './FormError.svelte';
  import Modal from './Modal.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Switch from './Switch.svelte';

  const {
    open,
    identity,
    checkpointId,
    readOnly,
    hasUnsavedDrafts,
    returnFocus = null,
    fetchCheckpoint,
    restoreCheckpoint,
    onClose,
  }: {
    open: boolean;
    identity: string;
    checkpointId: string;
    readOnly: boolean;
    hasUnsavedDrafts: boolean;
    returnFocus?: HTMLElement | null;
    fetchCheckpoint: (checkpointId: string) => Promise<SettingsCheckpoint>;
    restoreCheckpoint?: (checkpointId: string, input: SettingsRestoreInput) => Promise<void>;
    onClose: () => void;
  } = $props();

  let checkpoint = $state<SettingsCheckpoint | null>(null);
  let selected = $state<string[]>([]);
  let restoreState = $state<SettingsCheckpointRestoreSide>('after');
  let loading = $state(false);
  let restoring = $state(false);
  let confirming = $state(false);
  let showMatching = $state(false);
  let problem = $state<string | null>(null);
  let rawOpen = $state<string[]>([]);
  let generation = 0;

  const changedCount = $derived(checkpoint?.items.filter((item) => item.changed).length ?? 0);
  const differingCount = $derived(
    checkpoint?.items.filter((item) => selectedSide(item).differs).length ?? 0,
  );
  const restorableDifferenceCount = $derived(
    checkpoint?.items.filter((item) => {
      const side = selectedSide(item);
      return side.differs && side.restorable;
    }).length ?? 0,
  );
  const canRestore = $derived(!readOnly && restoreCheckpoint !== undefined);
  const visibleItems = $derived(
    checkpoint?.items.filter(
      (item) => showMatching || item.changed || selectedSide(item).differs,
    ) ?? [],
  );
  const hiddenItemCount = $derived((checkpoint?.items.length ?? 0) - visibleItems.length);
  const restoreStateOptions = $derived(
    checkpoint?.items.some((item) => item.before.available)
      ? [
          { value: 'before', label: 'Before change' },
          { value: 'after', label: 'After change' },
        ]
      : [{ value: 'after', label: 'Initial state' }],
  );

  $effect(() => {
    const wanted = open ? `${identity}/${checkpointId}` : '';
    untrack(() => {
      if (wanted !== '') void loadCheckpoint(checkpointId);
    });
  });

  async function loadCheckpoint(id: string): Promise<void> {
    const currentGeneration = (generation += 1);
    loading = true;
    problem = null;
    checkpoint = null;
    selected = [];
    rawOpen = [];
    confirming = false;
    showMatching = false;
    try {
      const loaded = await fetchCheckpoint(id);
      if (generation !== currentGeneration) return;
      checkpoint = loaded;
      restoreState = 'after';
      selected = defaultSelection(loaded, restoreState);
    } catch (cause) {
      if (generation === currentGeneration) problem = messageOf(cause);
    } finally {
      if (generation === currentGeneration) loading = false;
    }
  }

  function itemIdentity(item: SettingsCheckpointItem): string {
    return [item.kind, item.repository_id ?? '', item.sync_kind ?? ''].join(':');
  }

  function toggleItem(item: SettingsCheckpointItem, checked: boolean): void {
    const identity = itemIdentity(item);
    selected = checked
      ? [...selected.filter((held) => held !== identity), identity]
      : selected.filter((held) => held !== identity);
    confirming = false;
  }

  function selectedSide(item: SettingsCheckpointItem) {
    return item[restoreState];
  }

  function defaultSelection(
    value: SettingsCheckpoint,
    side: SettingsCheckpointRestoreSide,
  ): string[] {
    return value.items
      .filter((item) => item[side].differs && item[side].restorable)
      .map(itemIdentity);
  }

  function selectRestoreState(value: string): void {
    if (value !== 'before' && value !== 'after') return;
    restoreState = value;
    selected = checkpoint === null ? [] : defaultSelection(checkpoint, value);
    confirming = false;
  }

  function toggleRaw(item: SettingsCheckpointItem): void {
    const identity = itemIdentity(item);
    rawOpen = rawOpen.includes(identity)
      ? rawOpen.filter((held) => held !== identity)
      : [...rawOpen, identity];
  }

  function requestRestore(): void {
    if (selected.length > 0 && !hasUnsavedDrafts && canRestore) confirming = true;
  }

  async function restore(): Promise<void> {
    const held = checkpoint;
    const restoreSelected = restoreCheckpoint;
    if (
      held === null ||
      restoreSelected === undefined ||
      selected.length === 0 ||
      hasUnsavedDrafts ||
      !canRestore
    ) {
      return;
    }
    const selections = held.items.flatMap((item) =>
      selected.includes(itemIdentity(item))
        ? [
            {
              kind: item.kind,
              ...(item.repository_id === undefined ? {} : { repository_id: item.repository_id }),
              ...(item.sync_kind === undefined ? {} : { sync_kind: item.sync_kind }),
              expected_revision: item.current?.revision ?? 0,
            },
          ]
        : [],
    );
    if (selections.length !== selected.length) return;

    restoring = true;
    problem = null;
    try {
      await restoreSelected(held.id, { state: restoreState, selections });
      onClose();
    } catch (cause) {
      problem = messageOf(cause);
      confirming = false;
    } finally {
      restoring = false;
    }
  }

  function itemStatus(item: SettingsCheckpointItem): string {
    const side = selectedSide(item);
    if (!side.available) return 'Not captured';
    if (side.incompatibility !== undefined) return 'Cannot restore';
    if (side.differs) return 'Differs now';
    return 'Matches current';
  }

  function rawState(item: SettingsCheckpointItem): string {
    return JSON.stringify(
      { before: item.before.state, after: item.after.state, current: item.current },
      null,
      2,
    );
  }

  function sideSummary(
    item: SettingsCheckpointItem,
    side: SettingsCheckpointItem['before'],
  ): string {
    return side.available ? settingsCheckpointSummary(item, side.state) : 'Not captured';
  }

  function rawStateId(item: SettingsCheckpointItem): string {
    return `settings-checkpoint-raw-${itemIdentity(item)}`;
  }

  function cleanReason(reason: string): string {
    return reason.trim().replace(/\.$/, '');
  }

  function messageOf(cause: unknown): string {
    return cause instanceof Error ? cause.message : String(cause);
  }
</script>

<Modal
  id="settings-checkpoint-dialog"
  {open}
  title="Settings history"
  description="Inspect saved settings and restore only the resources you choose"
  variant="wide"
  {returnFocus}
  {onClose}
>
  {#if loading && checkpoint === null}
    <p class="checkpoint-loading" role="status">Loading settings snapshot…</p>
  {:else if checkpoint !== null}
    <section class="snapshot-provenance" aria-label="Snapshot details">
      <span class="snapshot-avatar"><Avatar account={checkpoint.actor} size={28} /></span>
      <div class="snapshot-author">
        <strong
          >{settingsCheckpointActionLabel(checkpoint.action)} by {checkpoint.actor
            .display_name}</strong
        >
        <time datetime={checkpoint.created_at} title={formatTimestamp(checkpoint.created_at)}
          >{formatDateTime(checkpoint.created_at)}</time
        >
      </div>
      <div class="snapshot-count">
        <strong>{changedCount}</strong>
        <span>{changedCount === 1 ? 'affected resource' : 'affected resources'}</span>
      </div>
    </section>

    <div class="restore-state-picker">
      <SegmentedControl
        name="settings-checkpoint-restore-state"
        label="Restore state"
        options={restoreStateOptions}
        value={restoreState}
        compact
        onSelect={selectRestoreState}
      />
      <p>
        {restoreState === 'before'
          ? 'Restore the state immediately before this history entry'
          : checkpoint.action.endsWith('.baseline')
            ? 'Restore the settings captured when history began'
            : 'Restore the state saved by this history entry'}
      </p>
    </div>

    <fieldset class="checkpoint-items">
      <legend>
        <span>Snapshot resources</span>
        <small>{selected.length} selected</small>
      </legend>
      {#each visibleItems as item (itemIdentity(item))}
        {@const side = selectedSide(item)}
        <article class:matches={!side.differs} class="checkpoint-item">
          <header>
            <div class="item-title">
              <strong>{settingsCheckpointItemLabel(item)}</strong>
              <span class:matches={!side.differs}>{itemStatus(item)}</span>
            </div>
            <Switch
              bare
              checked={selected.includes(itemIdentity(item))}
              label="Restore {settingsCheckpointItemLabel(item)}"
              disabled={!side.differs || !side.restorable || !canRestore || restoring}
              onToggle={(checked) => toggleItem(item, checked)}
            />
          </header>
          <div class="state-comparison">
            <div>
              <span>Before</span>
              <p>{sideSummary(item, item.before)}</p>
            </div>
            <div class="after-state">
              <span>After</span>
              <p>{sideSummary(item, item.after)}</p>
            </div>
          </div>
          {#if side.incompatibility !== undefined}
            <p class="incompatibility">{cleanReason(side.incompatibility.reason)}</p>
          {/if}
          <div class="raw-state">
            <button
              type="button"
              aria-label="{rawOpen.includes(itemIdentity(item))
                ? 'Hide'
                : 'View'} stored state for {settingsCheckpointItemLabel(item)}"
              aria-expanded={rawOpen.includes(itemIdentity(item))}
              aria-controls={rawStateId(item)}
              onclick={() => toggleRaw(item)}
            >
              {rawOpen.includes(itemIdentity(item)) ? 'Hide' : 'View'} stored state
            </button>
            {#if rawOpen.includes(itemIdentity(item))}
              <pre id={rawStateId(item)}>{rawState(item)}</pre>
            {/if}
          </div>
        </article>
      {/each}
      {#if visibleItems.length === 0}
        <p class="all-matching">Every resource in this snapshot matches the current settings</p>
      {/if}
      {#if hiddenItemCount > 0 || showMatching}
        <div class="matching-disclosure">
          <button
            type="button"
            aria-expanded={showMatching}
            onclick={() => (showMatching = !showMatching)}
          >
            {showMatching
              ? 'Hide matching resources'
              : `Show ${hiddenItemCount} matching ${hiddenItemCount === 1 ? 'resource' : 'resources'}`}
          </button>
        </div>
      {/if}
    </fieldset>

    {#if hasUnsavedDrafts}
      <p class="checkpoint-notice" role="status">
        Save or discard unsaved settings before restoring history
      </p>
    {:else if !canRestore}
      <p class="checkpoint-notice">You can inspect this snapshot, but cannot restore it</p>
    {:else if selected.length === 0}
      <p class="checkpoint-notice">
        {differingCount === 0
          ? 'This snapshot already matches the current settings'
          : restorableDifferenceCount === 0
            ? 'No differing resources in this snapshot can be restored'
            : 'Select at least one differing resource to restore'}
      </p>
    {/if}

    {#if confirming}
      <p class="restore-confirmation" role="alert">
        Restore {selected.length}
        {selected.length === 1 ? 'resource' : 'resources'}? This creates new active revisions and a
        new history entry
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
      {#if checkpoint !== null && canRestore}
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
  .restore-confirmation,
  .incompatibility {
    margin: 0;
  }

  .snapshot-provenance {
    align-items: center;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 3.5rem;
    padding: var(--space-2) var(--space-3);
  }

  .snapshot-avatar {
    display: inline-flex;
  }

  .snapshot-author {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
  }

  .snapshot-author strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .snapshot-author time,
  .checkpoint-items legend small,
  .item-title span {
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
  }

  .snapshot-count span {
    color: var(--dim);
    font-size: var(--font-size-micro);
    white-space: nowrap;
  }

  .restore-state-picker {
    align-items: center;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr);
  }

  .restore-state-picker p {
    color: var(--dim);
    font-size: var(--font-size-compact);
    margin: 0;
  }

  .checkpoint-items {
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    display: grid;
    margin: 0;
    min-width: 0;
    overflow: hidden;
    padding: 0;
  }

  .checkpoint-items legend {
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

  .checkpoint-items legend + * {
    clear: both;
  }

  .checkpoint-items legend > span,
  .item-title strong {
    font-size: var(--font-size-meta);
    font-weight: 700;
  }

  .checkpoint-item {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
  }

  .checkpoint-item + .checkpoint-item {
    border-top: 1px solid var(--border-subtle);
  }

  .checkpoint-item:not(.matches) {
    background: color-mix(in srgb, var(--diff-chg-ink) 3.5%, var(--surface-base));
  }

  .checkpoint-item > header {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    justify-content: space-between;
  }

  .item-title {
    align-items: baseline;
    display: flex;
    gap: var(--space-2);
    min-width: 0;
  }

  .item-title strong {
    overflow-wrap: anywhere;
  }

  .item-title span:not(.matches) {
    color: var(--diff-chg-ink);
    font-weight: 600;
  }

  .item-title span.matches {
    color: var(--success);
  }

  .state-comparison {
    display: grid;
    gap: var(--space-2);
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .state-comparison > div {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
    padding: var(--space-2);
  }

  .state-comparison span {
    color: var(--dim);
    font-size: var(--font-size-micro);
    font-weight: 600;
    letter-spacing: 0.025em;
  }

  .state-comparison p {
    font-size: var(--font-size-compact);
    margin: 0;
    overflow-wrap: anywhere;
  }

  .after-state {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
  }

  .incompatibility {
    color: var(--danger);
    font-size: var(--font-size-compact);
  }

  .all-matching {
    color: var(--dim);
    font-size: var(--font-size-compact);
    margin: 0;
    padding: var(--space-4) var(--space-3);
    text-align: center;
  }

  .matching-disclosure {
    border-top: 1px solid var(--border-subtle);
    display: flex;
    justify-content: flex-end;
    padding: var(--space-2) var(--space-3);
  }

  .raw-state > button,
  .matching-disclosure > button {
    background: transparent;
    border: 0;
    color: var(--text-secondary);
    cursor: pointer;
    font-size: var(--font-size-compact);
    padding: 0;
    width: max-content;
  }

  .raw-state > button:hover,
  .raw-state > button:focus-visible,
  .matching-disclosure > button:hover,
  .matching-disclosure > button:focus-visible {
    text-decoration: underline;
  }

  .raw-state pre {
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
    .restore-state-picker {
      align-items: start;
      gap: var(--space-2);
      grid-template-columns: minmax(0, 1fr);
    }

    .snapshot-provenance {
      align-items: start;
      grid-template-columns: auto minmax(0, 1fr);
    }

    .snapshot-count {
      border-left: 0;
      border-top: 1px solid var(--border-subtle);
      grid-column: 1 / -1;
      padding: var(--space-3) 0 0;
    }

    .checkpoint-item > header,
    .item-title {
      align-items: flex-start;
    }

    .item-title {
      display: grid;
      gap: var(--space-1);
    }

    .state-comparison {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
