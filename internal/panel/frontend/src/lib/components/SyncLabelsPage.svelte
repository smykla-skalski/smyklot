<script module lang="ts">
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
</script>

<script lang="ts">
  /**
   * The labels page: immediate apply, per-segment editing. Pressing a name,
   * a description or the colour dot swaps only that piece into its edit
   * state, in place - the in-place input is the hover ghost made real, so
   * the swap moves nothing. Everything applies as it commits; the whisper
   * in the card head is the receipt; pressing anywhere else closes the open
   * piece. Below, the two decisions that shape what the list means: whether
   * unlisted labels are removed, and the patterns left alone either way.
   */
  import { tick } from 'svelte';

  import type { SyncConfig, SyncLabel } from '../types';
  import type { SyncSection } from '../routes';

  import Button from './Button.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import LabelColorPicker from './LabelColorPicker.svelte';
  import PanePath from './PanePath.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Switch from './Switch.svelte';

  export interface LabelsSaveInput {
    enabled: boolean;
    labels: SyncLabel[];
    allow_removal: boolean;
    excludes: string[];
  }

  const {
    config,
    readOnly,
    problem = null,
    sectionHref,
    onOpenSection,
    onSave,
  }: {
    config: SyncConfig | null;
    readOnly: boolean;
    problem?: string | null;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    /** Saves the whole configuration; resolves true when it landed. */
    onSave: (input: LabelsSaveInput) => Promise<boolean>;
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

  /* Derived from what is saved, then written over as somebody edits - a save
     landing from anywhere reseeds it. */
  let rows = $derived(toRows(config));
  let patterns = $derived<string[]>([...(config?.excludes ?? [])]);

  const enabled = $derived(config?.enabled ?? false);
  const allowRemoval = $derived(config?.allow_removal ?? false);
  const unreadable = $derived(config?.unreadable === true);
  const unavailable = $derived(config?.unavailable ?? '');
  const frozen = $derived(readOnly || unreadable || config === null);

  /* ---------- Saving: the list is the whole truth ---------- */

  function toLabels(list: Row[]): SyncLabel[] {
    return list
      .filter((row) => row.name.trim() !== '')
      .map((row) => ({
        name: row.name,
        color: row.color.slice(1),
        ...(row.desc !== '' || row.hadDesc ? { description: row.desc } : {}),
      }));
  }

  async function push(overrides: Partial<LabelsSaveInput> = {}): Promise<void> {
    if (config === null) return;
    const ok = await onSave({
      enabled,
      labels: toLabels(rows),
      allow_removal: allowRemoval,
      excludes: patterns.filter((pattern) => pattern.trim() !== ''),
      ...overrides,
    });
    if (ok) whisper();
  }

  /* The whisper is the save receipt: one voice in the card head, on for a
     beat after any landed save, then gone. In the head it survives every
     row re-render and holds no column space hostage. */
  let savedOn = $state(false);
  let savedTimer: ReturnType<typeof setTimeout> | undefined;

  function whisper(): void {
    savedOn = true;
    clearTimeout(savedTimer);
    savedTimer = setTimeout(() => (savedOn = false), 1400);
  }

  /* ---------- One segment edits at a time, page-wide ---------- */

  let editing = $state<{ index: number; piece: 'name' | 'desc' | 'color' } | null>(null);
  let fieldError = $state<string | null>(null);
  let ghostScroll = $state(0);
  let editValue = $state('');
  let editInput: HTMLInputElement | null = $state(null);

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

  /* Committing is DEBOUNCED off the keyboard - the model is not written per
     keystroke - but leaving the field, pasting, or Enter commits at once. */
  let textTimer: ReturnType<typeof setTimeout> | undefined;

  function commitText(): void {
    clearTimeout(textTimer);
    textTimer = undefined;
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
    void push();
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

  function typed(event: Event): void {
    const open = editing;
    if (open === null || open.piece === 'color') return;
    fieldError = labelFieldError(open.piece, editValue);
    clearTimeout(textTimer);
    if (fieldError !== null) return;
    const type = (event as InputEvent).inputType;
    if (type === 'insertFromPaste' || type === 'insertFromDrop') commitText();
    else textTimer = setTimeout(commitText, 700);
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
    if (!silent) void push();
  }

  function pickColor(index: number, hex: string): void {
    rows = rows.map((held, at) => (at === index ? { ...held, color: hex } : held));
    editing = null;
    void push();
  }

  const inUse = $derived(rows.map((row) => row.color));

  /* ---------- Rows arrive and leave ---------- */

  function addLabel(): void {
    if (frozen) return;
    commitText();
    rows = [{ name: '', desc: '', color: '#0e8a16', hadDesc: false }, ...rows];
    editing = { index: 0, piece: 'name' };
    editValue = '';
    fieldError = labelFieldError('name', '');
    void tick().then(() => editInput?.focus());
  }

  function removeLabel(index: number): void {
    if (frozen) return;
    rows = rows.filter((_, at) => at !== index);
    editing = null;
    void push();
  }

  /* ---------- Outside is the exit ---------- */

  function outside(event: MouseEvent): void {
    const target = event.target as HTMLElement;
    /* A real click drains microtasks between listeners, so a press that
       opened an editor has already re-rendered - and detached - its own
       target by the time this document listener runs. A detached target
       has no ancestors to read; it was somebody's press, never "outside". */
    if (!target.isConnected) return;
    if (editing !== null && !target.closest('.label-row')) closeSegment();
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
  <PanePath
    segments={[
      { label: 'Sync', href: sectionHref('overview'), onSelect: () => onOpenSection('overview') },
    ]}
  />

  <div class="kind-head">
    <div class="kind-head-say">
      <h2 class="card-title">Labels</h2>
      <p class="kind-head-sub">
        The labels every repository should carry. Changes here feed the next plan - nothing reaches
        GitHub until you apply one
      </p>
    </div>
    <Switch
      checked={enabled}
      label="Label sync"
      word="Syncing"
      disabled={frozen}
      onToggle={(next) => void push({ enabled: next })}
    />
  </div>

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#if unreadable}
    <p class="sync-notice" role="alert">
      This installation's labels are stored in a form this version of Smyklot cannot read, so they
      are not shown and nothing here can be changed. Nothing has been lost.
    </p>
  {/if}

  {#if unavailable !== '' && enabled}
    <p class="sync-notice" role="status">
      {unavailable}. Nothing here will be planned or changed until an owner grants it on the
      installation's page on GitHub.
    </p>
  {/if}

  <div class="card label-card">
    <div class="card-head">
      <h3 class="card-title">{rows.length} {rows.length === 1 ? 'label' : 'labels'}</h3>
      <span class="label-saved" class:is-on={savedOn} role="status"
        ><Icon name="check" size={12} /><span class="t">Saved</span></span
      >
      <Button disabled={frozen} onclick={addLabel}>
        {#snippet icon()}<Icon name="plus" size={13} />{/snippet}
        Add a label
      </Button>
    </div>
    <p class="label-hint">
      Press any name, description or colour dot to change it right here - edits save themselves as
      you go
    </p>
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
                  ><Icon name="alert" size={12} /><span class="t">{fieldError}</span></span
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
              <Icon name="trash" size={14} />
            </button>
          </span>
        </li>
      {/each}
    </ul>
  </div>

  <div class="card">
    <div class="setting-rows">
      <div class="setting-row">
        <span class="setting-say">
          <span class="setting-name">Remove labels this list does not name</span>
          <span class="setting-why"
            >Off, a repository may keep labels of its own. On, the list above is the whole truth and
            everything else is deleted</span
          >
        </span>
        <Switch
          checked={allowRemoval}
          label="Remove labels this list does not name"
          disabled={frozen}
          onToggle={(next) => void push({ allow_removal: next })}
        />
      </div>
      <div class="setting-row">
        <span class="setting-say">
          <span class="setting-name">Labels to leave alone</span>
          <span class="setting-why"
            >Patterns, where <code>*</code> stands for any run of characters. Neither written nor removed</span
          >
        </span>
        <span class="setting-value">
          <PatternEntries
            {patterns}
            readOnly={frozen}
            onChange={(next) => {
              patterns = next;
              void push({ excludes: next });
            }}
          />
        </span>
      </div>
    </div>
  </div>
</div>

<style>
  .view-frame {
    margin-inline: auto;
    max-width: var(--content-max);
  }

  .kind-head {
    align-items: start;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .kind-head-say {
    display: grid;
    gap: var(--space-2);
  }

  /* Uncapped like the setting whys: the head's own width is the measure. */
  .kind-head-sub {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: round(1.5em, 1px);
    margin: 0;
  }

  /* The tap box must not inflate the head - the hit area survives on the
     input itself. */
  .kind-head :global(.switch) {
    min-block-size: auto;
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
    grid-template-columns: auto 11rem minmax(0, 1fr) auto;
    margin-inline: calc(var(--space-2) * -1);
    min-block-size: 40px;
    padding: 0.5rem var(--space-2);
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
    line-height: 20px;
  }

  .label-desc {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: 20px;
    min-inline-size: 0;
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

  /* The dot's press target: a 24px round pad the 12px disc sits in, taken
     out of the layout with negative margins so the column never moves. */
  .dot-btn {
    align-items: center;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 50%;
    block-size: 24px;
    /* A button does not inherit ink - the UA's buttontext would ride here
       and tint anything the disc ever grows. */
    color: inherit;
    cursor: pointer;
    display: inline-flex;
    inline-size: 24px;
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
    min-block-size: 28px;
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
    block-size: 28px;
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

  /* The save receipt lives in the card head, beside Add - never inside a
     row, where it would hold column space in every line for a word that
     shows for a second. */
  .label-saved {
    align-items: center;
    color: var(--text-muted);
    display: inline-flex;
    font-size: var(--font-size-micro);
    gap: 4px;
    /* The receipt rides the right edge on its own - titles hug their text
       now instead of stretching to push it there. */
    margin-inline-start: auto;
    opacity: 0;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .label-saved.is-on {
    opacity: 1;
  }

  .label-saved :global(svg) {
    color: var(--success);
  }

  .label-saved .t {
    text-box: trim-both cap alphabetic;
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
    outline: 2px solid var(--focus);
    outline-offset: -1px;
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
    line-height: round(1.5em, 1px);
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
    }

    .label-row .label-tail {
      grid-column: 3;
      grid-row: 1 / span 2;
    }
  }

  /* ---------- The bottom card: the two decisions ---------- */

  .setting-rows {
    display: grid;
  }

  /* A card that is nothing but rows: the rows' own block padding is the
     card's edge whitespace, so unswallowed it doubled onto the card's 20px
     and the top and bottom read heavier than the sides. */
  .card > .setting-rows:only-child {
    margin-block: calc(var(--space-5) * -1);
  }

  /* Top-aligned, not centred: centring the say against a taller control put
     its half-slack on the seam. The switch is the exception: a lone toggle
     reads centred against its row, and it carries no ink the seams are
     measured to. */
  .card > .setting-rows:only-child > .setting-row {
    align-items: start;
    padding-block: var(--space-5);
  }

  /* The 44px tap box stays for the finger, not for the layout. */
  .setting-row :global(.switch) {
    align-self: center;
    margin-block: calc((20px - var(--touch-target)) / 2);
  }

  .setting-row {
    align-items: center;
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-2) var(--space-4);
    /* Auto-flow, not `1fr auto auto`: a fixed template kept an empty third
       track on two-child rows, and its 16px column gap pushed the switch
       off the row's right edge. */
    grid-auto-columns: auto;
    grid-auto-flow: column;
    grid-template-columns: 1fr;
    margin-inline: calc(var(--space-2) * -1);
    /* 12px block padding: with trimmed text the padding IS the
       ink-to-hairline distance. The floor is the 44px touch target. */
    min-block-size: var(--touch-target);
    padding: var(--space-3) var(--space-2);
    position: relative;
  }

  .setting-row:not(:last-child)::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
  }

  /* 12, not 4: with name and why both ink-trimmed the gap IS the ink
     distance - blocks must sit further apart than the lines inside them. */
  .setting-say {
    display: grid;
    gap: var(--space-3);
  }

  .setting-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  .setting-why {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .setting-why code {
    font-family: var(--mono);
  }

  /* The value may wrap but never push: min-size 0 lets its grid track
     shrink below max-content, so a new entry wraps inside the row instead
     of making the card wider. */
  .setting-value {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    justify-content: end;
    justify-self: end;
    min-inline-size: 0;
  }
</style>
