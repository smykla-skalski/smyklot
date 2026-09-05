<script module lang="ts">
  import type { SyncLabel as SavedLabel } from '../types';

  /**
   * GitHub's own label limits: a required name of at most 50 characters, a
   * description of at most 100, nothing unprintable, no stray edge spaces.
   * An invalid value never reaches the model - immediate apply commits only
   * what would survive the API. Exported for the specs.
   */
  export function labelFieldError(kind: 'name' | 'desc', value: string): string | null {
    // eslint-disable-next-line no-control-regex -- the point is refusing them
    if (/[\u0000-\u001f\u007f]/u.test(value)) return 'Control characters cannot be used';
    if (kind === 'name') {
      if (value.trim() === '') return 'A name is required';
      if (value !== value.trim()) return 'No leading or trailing spaces';
      if (value.length > 50) return `${value.length} of 50 characters - too long`;
    } else if (value.length > 100) {
      return `${value.length} of 100 characters - too long`;
    }
    return null;
  }

  export const LABEL_LIMITS = { name: 50, desc: 100 } as const;

  export interface LabelsSaveInput {
    enabled: boolean;
    labels: SavedLabel[];
    allow_removal: boolean;
    excludes: string[];
  }
</script>

<!--
@component
The labels page: staged, per-segment editing. Pressing a name,
a description or the colour dot swaps only that piece into its edit
state, in place - the in-place input is the hover ghost made real, so
the swap moves nothing. Everything updates the workspace draft as it
commits; pressing anywhere else closes the open piece. Below, the two
decisions that shape what the list means: whether
unlisted labels are removed, and the patterns left alone either way.
-->

<script lang="ts">
  import { tick, untrack } from 'svelte';

  import { receipts } from '../receipts.svelte';
  import type { SyncConfig, SyncLabel, SyncStatus } from '../types';

  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import LabelColorPicker from './LabelColorPicker.svelte';
  import PageHeader from './PageHeader.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Switch from './Switch.svelte';
  import SyncKindFacts, { syncSwitchLabel, syncSwitchWord } from './SyncKindFacts.svelte';

  const {
    config,
    readOnly,
    problem = null,
    syncStatus = null,
    nowMs,
    onChange,
    dirtyControls = [],
  }: {
    config: SyncConfig | null;
    readOnly: boolean;
    problem?: string | null;
    /** The fleet, for how far this kind reaches. */
    syncStatus?: SyncStatus | null;
    nowMs: number;
    /** Stages one semantic labels control in the application-wide draft. */
    onChange: (
      input: LabelsSaveInput,
      controlId:
        | 'sync.labels.enabled'
        | 'sync.labels.labels'
        | 'sync.labels.allow_removal'
        | 'sync.labels.excludes',
    ) => boolean;
    dirtyControls?: readonly string[];
  } = $props();

  interface Row {
    name: string;
    desc: string;
    /** #rrggbb - the model stores bare hex, the page speaks with the #. */
    color: string;
    /** The stored label carried a description, so '' means CLEAR, not "leave". */
    hadDesc: boolean;
  }

  const toRows = (source: SyncConfig | null): Row[] =>
    (source?.labels ?? []).map((label) => ({
      name: label.name,
      desc: label.description ?? '',
      color: `#${label.color.toLowerCase()}`,
      hadDesc: label.description !== undefined,
    }));

  /* One draft per mounted page. SyncView remounts this component when the
     initial config arrives; after that, parent config advances only to carry
     the next revision. Keeping the draft in state prevents such a response
     from resetting an edit queued behind it. */
  const initialConfig = untrack(() => config);
  let rows = $state<Row[]>(toRows(initialConfig));
  let patterns = $state<string[]>([...(initialConfig?.excludes ?? [])]);
  let enabled = $state(initialConfig?.enabled ?? false);
  let allowRemoval = $state(initialConfig?.allow_removal ?? false);
  let configSignature = labelsSignature(initialConfig);
  const unreadable = $derived(config?.unreadable === true);
  const unavailable = $derived(config?.unavailable ?? '');
  const frozen = $derived(readOnly || unreadable || config === null);
  const dirtyControlSet = $derived(new Set(dirtyControls));

  /* ---------- Staging: the list is the whole truth ---------- */

  function toLabels(list: Row[]): SyncLabel[] {
    return list
      .filter((row) => row.name.trim() !== '')
      .map((row) => ({
        name: row.name,
        color: row.color.slice(1),
        ...(row.desc !== '' || row.hadDesc ? { description: row.desc } : {}),
      }));
  }

  function push(
    controlId:
      | 'sync.labels.enabled'
      | 'sync.labels.labels'
      | 'sync.labels.allow_removal'
      | 'sync.labels.excludes',
    overrides: Partial<LabelsSaveInput> = {},
  ): void {
    if (config === null) return;
    if (overrides.enabled !== undefined) enabled = overrides.enabled;
    if (overrides.allow_removal !== undefined) allowRemoval = overrides.allow_removal;
    onChange(
      {
        enabled,
        labels: toLabels(rows),
        allow_removal: allowRemoval,
        excludes: patterns.filter((pattern) => pattern.trim() !== ''),
        ...overrides,
      },
      controlId,
    );
  }

  /* ---------- One segment edits at a time, page-wide ---------- */

  let editing = $state<{ index: number; piece: 'name' | 'desc' | 'color' } | null>(null);
  let fieldError = $state<string | null>(null);
  let ghostScroll = $state(0);
  let editValue = $state('');
  let editInput: HTMLInputElement | null = $state(null);

  $effect(() => {
    const current = config;
    const signature = labelsSignature(current);
    if (signature === configSignature) return;
    configSignature = signature;
    untrack(() => {
      if (signature === localLabelsSignature()) return;
      rows = toRows(current);
      patterns = [...(current?.excludes ?? [])];
      enabled = current?.enabled ?? false;
      allowRemoval = current?.allow_removal ?? false;
      editing = null;
      fieldError = null;
    });
  });

  function labelsSignature(source: SyncConfig | null): string {
    return JSON.stringify({
      enabled: source?.enabled ?? false,
      labels: source?.labels ?? [],
      allow_removal: source?.allow_removal ?? false,
      excludes: source?.excludes ?? [],
    });
  }

  function localLabelsSignature(): string {
    return JSON.stringify({
      enabled,
      labels: toLabels(rows),
      allow_removal: allowRemoval,
      excludes: patterns.filter((pattern) => pattern.trim() !== ''),
    });
  }

  /**
   * The letter under the pointer, read before the segment re-renders.
   * caretRangeFromPoint is WebKit's spelling and caretPositionFromPoint the
   * standard one Firefox ships - the panel needs whichever is there.
   */
  function pressedOffset(event: MouseEvent): number | null {
    const target = event.currentTarget as HTMLElement;
    const within = (host: Node | null, at: number): number | null => {
      const el = host?.nodeType === Node.TEXT_NODE ? host.parentElement : (host as HTMLElement);
      return el !== null && (el === target || target.contains(el)) ? at : null;
    };
    if (typeof document.caretPositionFromPoint === 'function') {
      const pos = document.caretPositionFromPoint(event.clientX, event.clientY);
      if (pos !== null) return within(pos.offsetNode, pos.offset);
      return null;
    }
    const range = document.caretRangeFromPoint?.(event.clientX, event.clientY);
    if (range) return within(range.startContainer, range.startOffset);
    return null;
  }

  function commitText(): void {
    const open = editing;
    if (open === null || open.piece === 'color') return;
    const row = rows[open.index];
    if (row === undefined) return;
    const value = editValue;
    if (labelFieldError(open.piece, value) !== null) return;
    if (row[open.piece === 'name' ? 'name' : 'desc'] === value) return;
    rows = rows.map((held, at) =>
      at === open.index ? { ...held, [open.piece === 'name' ? 'name' : 'desc']: value } : held,
    );
    push('sync.labels.labels');
  }

  function closeSegment(): void {
    commitText();
    if (editing === null) return;
    /* A row added and never named is not a label yet - it leaves with the
       editor rather than standing as an invalid blank. */
    const open = editing;
    editing = null;
    fieldError = null;
    const row = rows[open.index];
    if (row !== undefined && row.name.trim() === '') {
      rows = rows.filter((_, at) => at !== open.index);
    }
  }

  async function openText(
    index: number,
    piece: 'name' | 'desc',
    event?: MouseEvent,
  ): Promise<void> {
    if (frozen) return;
    commitText();
    /* The press names the field AND the letter: the caret range under the
       pointer, read before the segment re-renders, becomes the selection in
       the input that takes the pressed text's place. */
    const offset = event === undefined ? null : pressedOffset(event);
    const row = rows[index];
    editing = { index, piece };
    editValue = row === undefined ? '' : piece === 'name' ? row.name : row.desc;
    fieldError = null;
    ghostScroll = 0;
    await tick();
    if (editInput !== null) {
      editInput.focus();
      const pos = Math.min(offset ?? editInput.value.length, editInput.value.length);
      editInput.setSelectionRange(pos, pos);
    }
  }

  function typed(): void {
    const open = editing;
    if (open === null || open.piece === 'color') return;
    fieldError = labelFieldError(open.piece, editValue);
    if (fieldError !== null) return;
    commitText();
  }

  function textKeys(event: KeyboardEvent): void {
    if (event.key !== 'Enter') return;
    if (fieldError !== null) return;
    closeSegment();
  }

  /* The overflow highlight: mirror the value over the field, everything up
     to the limit transparent, the excess washed. */
  const overLimit = $derived.by(() => {
    const open = editing;
    if (open === null || open.piece === 'color') return null;
    const limit = LABEL_LIMITS[open.piece];
    if (editValue.length <= limit) return null;
    return { ok: editValue.slice(0, limit), over: editValue.slice(limit) };
  });

  /* ---------- The colour dot and its picker ---------- */

  function toggleColor(index: number): void {
    if (frozen) return;
    commitText();
    if (editing !== null && editing.index === index && editing.piece === 'color') {
      closeSegment();
      return;
    }
    editing = { index, piece: 'color' };
    fieldError = null;
  }

  function applyColor(index: number, hex: string, silent: boolean): void {
    rows = rows.map((held, at) => (at === index ? { ...held, color: hex } : held));
    if (!silent) push('sync.labels.labels');
  }

  function pickColor(index: number, hex: string): void {
    rows = rows.map((held, at) => (at === index ? { ...held, color: hex } : held));
    editing = null;
    push('sync.labels.labels');
  }

  const inUse = $derived(rows.map((row) => row.color));

  /* ---------- Rows arrive and leave ---------- */

  function addLabel(): void {
    if (frozen) return;
    closeSegment();
    rows = [{ name: '', desc: '', color: '#0e8a16', hadDesc: false }, ...rows];
    editing = { index: 0, piece: 'name' };
    editValue = '';
    fieldError = labelFieldError('name', '');
    void tick().then(() => editInput?.focus());
  }

  function removeLabel(index: number): void {
    if (frozen) return;
    const gone = rows[index];
    rows = rows.filter((_, at) => at !== index);
    editing = null;
    push('sync.labels.labels');
    /* A row that leaves is the one change on this page with nothing left on screen to
       read it back from, so the receipt carries both the name and the way back. */
    receipts.say(`Removed ${gone?.name === '' || gone === undefined ? 'the label' : gone.name}`, {
      undo:
        gone === undefined
          ? undefined
          : () => {
              rows = [...rows.slice(0, index), gone, ...rows.slice(index)];
              push('sync.labels.labels');
              receipts.say(`${gone.name === '' ? 'The label' : gone.name} is back`);
            },
    });
  }

  /* ---------- Outside is the exit ---------- */

  function outside(event: MouseEvent): void {
    const target = event.target as HTMLElement;
    /* A real click drains microtasks between listeners, so a press that
       opened an editor has already re-rendered - and detached - its own
       target by the time this document listener runs. A detached target
       has no ancestors to read; it was somebody's press, never "outside". */
    if (!target.isConnected) return;
    if (editing !== null && !target.closest('.label-row, .label-add')) closeSegment();
  }

  function keys(event: KeyboardEvent): void {
    if (event.key !== 'Escape') return;
    if (editing !== null) closeSegment();
  }

  /* A press on the row's own chrome - outside every segment and outside the
     open editor's DOM - closes whatever is open. */
  function rowChrome(event: MouseEvent): void {
    const target = event.target as HTMLElement;
    if (
      target.closest('.color-pop, .lbl-field, .swatch-wrap, .label-name, .label-desc, .label-del')
    ) {
      return;
    }
    closeSegment();
  }
</script>

<svelte:document onclick={outside} onkeydown={keys} />

<div class="view-frame">
  <PageHeader
    id="sync-labels-heading"
    section="Sync"
    title="Labels"
    description="Shared labels stay consistent across repositories after you save"
    statusUnsaved={dirtyControlSet.has('sync.labels.enabled')}
  >
    {#snippet actions()}
      <Button class="label-add" disabled={frozen} onclick={addLabel}>
        {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}
        Add a label
      </Button>
    {/snippet}
    {#snippet status()}
      <SyncKindFacts
        kind="labels"
        {enabled}
        status={syncStatus}
        updatedBy={config?.updated_by ?? ''}
        updatedAt={config?.updated_at ?? ''}
        {nowMs}
      />
      <Switch
        checked={enabled}
        label={syncSwitchLabel('labels', enabled)}
        word={syncSwitchWord(enabled)}
        disabled={frozen}
        onToggle={(next) => push('sync.labels.enabled', { enabled: next })}
      />
    {/snippet}
  </PageHeader>

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#if unreadable}
    <p class="sync-notice" role="alert">
      This workspace's labels are stored in a form this version of Smyklot cannot read, so they are
      not shown and nothing here can be changed. Nothing has been lost.
    </p>
  {/if}

  {#if unavailable !== '' && enabled}
    <p class="sync-notice" role="status">
      {unavailable}. Nothing here will be planned or changed until an owner grants it on the
      workspace's page on GitHub.
    </p>
  {/if}

  <Card class="label-card" unsaved={dirtyControlSet.has('sync.labels.labels')}>
    <div class="card-head">
      <h2 class="card-title">{rows.length} {rows.length === 1 ? 'label' : 'labels'}</h2>
    </div>
    {#if rows.length === 0}
      <!-- The hint below explains how to edit a row, and there are none - so the
           card said "0 labels" over a sentence about editing and then stopped. -->
      <div class="state-panel">
        <span
          ><strong>No labels are synced here yet.</strong> Every repository keeps its own until one is
          added - then every syncing repository is held to the list</span
        >
      </div>
    {:else}
      <p class="label-hint">
        Edit any name, description or colour. Each edit enters the draft as it commits; press Escape
        to take one back.
      </p>
    {/if}
    <ul class="label-rows">
      {#each rows as row, index (index)}
        {@const open = editing !== null && editing.index === index ? editing.piece : null}
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
        <li class="label-row" onclick={rowChrome}>
          <span class="swatch-wrap">
            <button
              class="dot-btn"
              type="button"
              aria-haspopup="dialog"
              aria-expanded={open === 'color'}
              aria-label="Label colour {row.color}"
              onclick={() => toggleColor(index)}
            >
              <span class="label-swatch" style:--swatch={row.color}></span>
            </button>
            {#if open === 'color'}
              <LabelColorPicker
                color={row.color}
                {inUse}
                onApply={(hex, opts) => applyColor(index, hex, opts.silent)}
                onPick={(hex) => pickColor(index, hex)}
              />
            {/if}
          </span>
          {#snippet editor(piece: 'name' | 'desc')}
            <span class="lbl-field is-inplace lbl-field-{piece}">
              <span class="limit-box">
                <input
                  class="text-inline"
                  class:is-invalid={fieldError !== null}
                  bind:this={editInput}
                  bind:value={editValue}
                  data-name={piece === 'name' ? true : undefined}
                  aria-label="Label {piece === 'name' ? 'name' : 'description'}"
                  aria-invalid={fieldError !== null}
                  placeholder={piece === 'name' ? 'Name' : 'Description (optional)'}
                  oninput={typed}
                  onkeydown={textKeys}
                  onfocusout={commitText}
                  onscroll={(event) => (ghostScroll = event.currentTarget.scrollLeft)}
                />
                {#if overLimit !== null}
                  <span class="limit-ghost" aria-hidden="true"
                    ><span class="lg-run" style:translate="{-ghostScroll}px 0"
                      ><span class="lg-ok">{overLimit.ok}</span><span class="lg-over"
                        >{overLimit.over}</span
                      ></span
                    ></span
                  >
                {/if}
              </span>
              {#if fieldError !== null}
                <span class="field-error"
                  ><Icon name="alert" size="xs" /><span class="t">{fieldError}</span></span
                >
              {/if}
            </span>
          {/snippet}
          {#if open === 'name'}
            {@render editor('name')}
          {:else}
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
            <span class="label-name" onclick={(event) => void openText(index, 'name', event)}
              >{row.name}</span
            >
          {/if}
          {#if open === 'desc'}
            {@render editor('desc')}
          {:else}
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
            <span class="label-desc" onclick={(event) => void openText(index, 'desc', event)}
              >{row.desc}</span
            >
          {/if}
          <span class="label-tail">
            <button
              class="label-del"
              aria-label="Remove {row.name === '' ? 'label' : row.name}"
              disabled={frozen}
              onclick={() => removeLabel(index)}
            >
              <Icon name="trash" size="sm" />
            </button>
          </span>
        </li>
      {/each}
    </ul>
  </Card>

  <Card>
    <div class="setting-rows">
      <div
        class="setting-row"
        class:is-unsaved={dirtyControlSet.has('sync.labels.allow_removal')}
        data-unsaved={dirtyControlSet.has('sync.labels.allow_removal') || undefined}
      >
        <span class="setting-say">
          <span class="setting-name">Delete unlisted labels</span>
          <span class="setting-why"
            >On deletes labels missing above from every syncing repository, except ignored matches.
            Off keeps repository-only labels</span
          >
        </span>
        <Switch
          checked={allowRemoval}
          label="Delete unlisted labels"
          disabled={frozen}
          onToggle={(next) => push('sync.labels.allow_removal', { allow_removal: next })}
        />
      </div>
      <div
        class="setting-row"
        class:is-unsaved={dirtyControlSet.has('sync.labels.excludes')}
        data-unsaved={dirtyControlSet.has('sync.labels.excludes') || undefined}
      >
        <span class="setting-say">
          <span class="setting-name">Ignored labels</span>
          <span class="setting-why"
            >Patterns, where <code>*</code> stands for any run of characters. Neither written nor removed
            - ignoring wins over every list above, deletion included</span
          >
        </span>
        <span class="setting-value">
          <PatternEntries
            {patterns}
            readOnly={frozen}
            onChange={(next) => {
              patterns = next;
              push('sync.labels.excludes', { excludes: next });
            }}
          />
        </span>
      </div>
    </div>
  </Card>
</div>

<style>
  .label-card.is-unsaved {
    border-color: color-mix(in srgb, var(--brand-action) 55%, var(--border-subtle));
    box-shadow: inset 2px 0 var(--brand-action);
  }

  .card-head {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .sync-notice {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: 0 0 var(--space-4);
    padding: var(--space-2) var(--space-3);
  }

  /* The card is the container the rows answer to: the row re-lays for the
     width it actually has, not for the viewport - a collapsed sidebar gives
     the card room a media query cannot see. */
  .label-card {
    container: labels / inline-size;
  }

  /* The last row's own air (a 40px row holds its 20px line with 10 below)
     sat on the card's 20 and read as a 31px bottom gap against the 21 above
     Add. The list hands those 10 back. */
  .label-rows {
    display: grid;
    list-style: none;
    margin: 0 0 -10px;
    padding: 0;
  }

  .label-row {
    /* Centred, not baseline: name and description share one line-height so
       their baselines coincide either way, and centring is what puts the
       colour dot on the text's optical middle. */
    align-items: center;
    display: grid;
    /* 24px columns, not 12: the ghosts and their inputs grow 8px into the
       gap from BOTH sides, so a 12px gap left neighbouring fields touching
       edge to edge. 24 keeps an 8px seam between two open boxes. */
    gap: var(--space-3) var(--space-6);
    /* Names get more room as the card grows, without taking the description's
       whole measure. A valid name can still be 50 wide characters, so both
       text tracks wrap below instead of ever painting across a neighbour. */
    grid-template-columns: auto clamp(11rem, 30%, 22rem) minmax(0, 1fr) auto;
    margin-inline: calc(var(--space-2) * -1);
    min-block-size: 40px;
    padding: var(--row-pad-compact) var(--space-2);
    position: relative;
  }

  .label-row:not(:last-child)::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
  }

  .label-swatch {
    background: var(--swatch);
    border-radius: 50%;
    /* 12px whole - 1cap resolved to 11.17 and the disc edge blurred. */
    block-size: 12px;
    display: inline-block;
    flex: none;
    inline-size: 12px;
  }

  .label-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
    line-height: var(--leading-meta);
    min-inline-size: 0;
    overflow-wrap: anywhere;
  }

  .label-desc {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
    min-inline-size: 0;
    overflow-wrap: anywhere;
  }

  /* Each SEGMENT is its own editor. The hover says so per segment - the
     text wears a quiet field ghost grown from negative margins so nothing
     moves: press it and it becomes that input, same box to the pixel. */
  .label-row .label-name,
  .label-row .label-desc {
    border: 1px solid transparent;
    border-radius: var(--r-ctl);
    cursor: text;
    margin-block: -4px;
    margin-inline: -8px;
    padding: 3px 7px;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard);
  }

  /* Only the segment under the pointer answers - its neighbours stay quiet. */
  .label-row .label-name:hover,
  .label-row .label-desc:hover {
    background: var(--input-bg);
    border-color: var(--control-border);
  }

  /* An empty description only speaks when it is itself under the pointer. */
  .label-row .label-desc:empty:hover::before {
    color: var(--text-muted);
    content: 'Add a description';
  }

  .label-row .label-desc:empty {
    min-inline-size: 9rem;
  }

  /* The dot's press target: the small-target round pad the 12px disc sits in,
     taken out of the layout with negative margins so the column never moves. */
  .dot-btn {
    align-items: center;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 50%;
    block-size: var(--field-target-min);
    /* A button does not inherit ink - the UA's buttontext would ride here
       and tint anything the disc ever grows. */
    color: inherit;
    cursor: pointer;
    display: inline-flex;
    inline-size: var(--field-target-min);
    justify-content: center;
    margin: -6px;
    padding: 0;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  .dot-btn:hover,
  .dot-btn[aria-expanded='true'] {
    background: var(--input-bg);
    border-color: var(--control-border);
  }

  .dot-btn:active {
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }

  .swatch-wrap {
    display: inline-flex;
    position: relative;
  }

  /* An in-place input IS the ghost it replaces: 28px tall on the ghost's own
     negative margins, so the swap moves nothing. */
  .lbl-field {
    align-content: center;
    display: grid;
    gap: var(--space-1);
    min-inline-size: 0;
  }

  .lbl-field .text-inline {
    min-inline-size: 0;
    width: 100%;
  }

  .lbl-field.is-inplace {
    margin-block: -4px;
    margin-inline: -8px;
  }

  .lbl-field.is-inplace .text-inline {
    min-block-size: var(--tier-quiet);
    padding-inline: 7px;
  }

  .lbl-field.is-inplace .limit-ghost {
    padding-inline: 7px;
  }

  .lbl-field.is-inplace [data-name] {
    font-weight: 600;
  }

  /* The row's tail: the trash, always there - a row's delete is not a
     secret. 28px at radius 6, muted until the pointer arrives, the danger
     family answering hover and the dip-and-inset answering the press. */
  .label-tail {
    align-items: center;
    display: inline-flex;
    justify-self: end;
  }

  .label-del {
    align-items: center;
    background: transparent;
    block-size: var(--tier-quiet);
    border: 0;
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    flex: none;
    inline-size: 28px;
    justify-content: center;
    margin-block: -2px;
    padding: 0;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  .label-del:hover,
  .label-del:focus-visible {
    background: color-mix(in srgb, var(--danger) 12%, transparent);
    color: var(--danger);
  }

  .label-del:active {
    background: color-mix(in srgb, var(--danger) 20%, transparent);
    box-shadow: var(--pressed-inset);
    color: var(--danger);
    translate: 0 1px;
  }

  /* How the list edits, said once, quietly, where the eye starts. */
  .label-hint {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: calc(var(--space-2) * -1) 0 var(--space-3);
  }

  /* One ring, fused to the field: the border takes the focus colour and the
     outline overlaps it - under the global offset-2 ring an input read as
     ringed twice. */
  .text-inline {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-size: var(--font-size-control);
    min-block-size: 30px;
    padding-inline: 0.55rem;
    width: 11rem;
  }

  .text-inline:focus {
    border-color: var(--focus);
    outline: var(--focus-ring-width) solid var(--focus);
    outline-offset: var(--focus-ring-inset);
  }

  /* Wrong input: the field wears danger quietly at rest - hairline and a 4%
     wash - and fuses the same single ring in danger when focused. */
  .text-inline.is-invalid {
    background: color-mix(in srgb, var(--danger) 4%, var(--input-bg));
    border-color: var(--danger);
  }

  .text-inline.is-invalid:focus {
    border-color: var(--danger);
    outline-color: var(--danger);
  }

  .field-error {
    align-items: center;
    color: var(--danger);
    display: inline-flex;
    font-size: var(--font-size-micro);
    gap: 4px;
    line-height: var(--leading-micro);
  }

  .field-error :global(svg) {
    flex: none;
  }

  /* The letters past the limit wear the wash INSIDE the field: a mirror
     sits over the input, its ink transparent, only the overflow's highlight
     painted - the input's own glyphs stay the single text layer. */
  .limit-box {
    display: grid;
    min-inline-size: 0;
    position: relative;
  }

  .limit-ghost {
    align-items: center;
    border: 1px solid transparent;
    display: flex;
    font-size: var(--font-size-control);
    inset: 0;
    overflow: hidden;
    padding-inline: 0.55rem;
    pointer-events: none;
    position: absolute;
    white-space: pre;
  }

  .limit-ghost .lg-run {
    display: inline-block;
    white-space: pre;
  }

  .limit-ghost .lg-ok {
    color: transparent;
  }

  .limit-ghost .lg-over {
    background: color-mix(in srgb, var(--danger) 22%, transparent);
    border-radius: 3px;
    color: transparent;
  }

  /* Narrow card: the description takes the second line under the name -
     the structure is the same whichever segment is editing, so this is the
     only layout the row ever changes. */
  @container labels (max-width: 30rem) {
    .label-row {
      grid-template-columns: auto minmax(0, 1fr) auto;
    }

    .label-row .label-name,
    .label-row .lbl-field-name {
      grid-column: 2;
      grid-row: 1;
    }

    .label-row .label-desc,
    .label-row .lbl-field-desc {
      grid-column: 2;
      grid-row: 2;
    }

    .label-row .label-name,
    .label-row .label-desc {
      justify-self: start;
      max-inline-size: 100%;
    }

    .label-row .label-tail {
      grid-column: 3;
      grid-row: 1 / span 2;
    }
  }

  /* ---------- The bottom card: the two decisions ---------- */

  @media (max-width: 36rem) {
    .card-head {
      flex-wrap: wrap;
    }

    /* THE ROW RE-FORMS, like every other row does here. Four columns cannot stand
       on a 320px page: an 11rem name track and the 9rem floor an empty description
       keeps for its own prompt are 320px between them before the swatch and the
       delete are drawn, so the row set the document's width and Chrome answered by
       zooming the whole page to 80%. The description takes the line below the name,
       and an empty one asks for nothing. */
    .label-row {
      grid-template-columns: auto minmax(0, 1fr) auto;
    }

    .label-row .swatch-wrap {
      grid-column: 1;
      grid-row: 1;
    }

    .label-row .label-name {
      grid-column: 2;
      grid-row: 1;
    }

    .label-row .label-tail {
      grid-column: 3;
      grid-row: 1;
    }

    .label-row .label-desc {
      grid-column: 2 / -1;
      grid-row: 2;
    }

    .label-row .label-desc:empty {
      min-inline-size: 0;
    }
  }
</style>
