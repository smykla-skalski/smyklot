<script module lang="ts">
  /**
   * hex <-> hsv, the two directions the picker needs. Exported for the specs:
   * hue stability through greys is a promise worth holding in a test.
   */
  export interface Hsv {
    h: number;
    s: number;
    v: number;
  }

  export function hexToHsv(hex: string): Hsv {
    const n = parseInt(hex.slice(1), 16);
    const r = ((n >> 16) & 255) / 255;
    const g = ((n >> 8) & 255) / 255;
    const b = (n & 255) / 255;
    const max = Math.max(r, g, b);
    const min = Math.min(r, g, b);
    const d = max - min;
    let h = 0;
    if (d !== 0) {
      if (max === r) h = ((g - b) / d) % 6;
      else if (max === g) h = (b - r) / d + 2;
      else h = (r - g) / d + 4;
      h = (h * 60 + 360) % 360;
    }
    return { h, s: max === 0 ? 0 : d / max, v: max };
  }

  export function hsvToHex({ h, s, v }: Hsv): string {
    const f = (i: number): string => {
      const k = (i + h / 60) % 6;
      const c = v - v * s * Math.max(0, Math.min(k, 4 - k, 1));
      return Math.round(c * 255)
        .toString(16)
        .padStart(2, '0');
    };
    return `#${f(5)}${f(3)}${f(1)}`;
  }

  /** WCAG relative luminance decides the check's ink on a swatch. */
  export function inkFor(hex: string): 'dark' | 'light' {
    const n = parseInt(hex.slice(1), 16);
    const lin = (raw: number): number => {
      const c = raw / 255;
      return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    };
    const lum =
      0.2126 * lin((n >> 16) & 255) + 0.7152 * lin((n >> 8) & 255) + 0.0722 * lin(n & 255);
    return lum > 0.45 ? 'dark' : 'light';
  }

  /** With or without the #, three digits or six - or nothing at all. */
  export function parseHex(raw: string): string | null {
    const match = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(raw.trim());
    if (match === null) return null;
    let hex = (match[1] ?? '').toLowerCase();
    if (hex.length === 3) hex = [...hex].map((c) => c + c).join('');
    return `#${hex}`;
  }

  /**
   * A swatch book's order: around the wheel red-to-violet, light before dark
   * inside a hue, and the greys at the end from lightest down. The wheel opens
   * on red - a hue in the last 15 degrees is red arriving early, not violet
   * arriving late. 0.2 is the neutral cutoff, not lower: GitHub's slate
   * #6b7280 sits at s 0.16 and reads grey.
   */
  export function wheelOrder(colors: readonly string[]): string[] {
    const graded = [...new Set(colors.map((c) => c.toLowerCase()))].map((hex) => ({
      hex,
      ...hexToHsv(hex),
    }));
    const chromatic = graded.filter((c) => c.s >= 0.2 && c.v >= 0.12);
    const neutral = graded.filter((c) => c.s < 0.2 || c.v < 0.12);
    const wheel = (h: number): number => (h >= 345 ? h - 360 : h);
    chromatic.sort((a, b) => wheel(a.h) - wheel(b.h) || b.v - a.v);
    neutral.sort((a, b) => b.v - a.v);
    return [...chromatic, ...neutral].map((c) => c.hex);
  }

  /** GitHub's own default palette: a saturated row and its pale row. */
  export const LABEL_PALETTE = [
    '#b60205',
    '#d93f0b',
    '#fbca04',
    '#0e8a16',
    '#006b75',
    '#1d76db',
    '#0052cc',
    '#5319e7',
    '#e99695',
    '#f9d0c4',
    '#fef2c0',
    '#c2e0c6',
    '#bfdadc',
    '#c5def5',
    '#bfd4f2',
    '#d4c5f9',
  ] as const;
</script>

<!--
@component
The label colour picker: svelte-awesome-color-picker's anatomy - the
saturation/value area, a hue rail, the hex field - built in this app's
own material rather than skinned over the library's DOM (considered,
and traded away: the drag-commit contract, the focus hand-off and the
pixel geometry all needed shims through its scoped styles). The rail
runs horizontal because nothing in this app slides vertically. Below,
every colour the list already carries ("In use", rebuilt fresh each
open) and GitHub's sixteen presets.

Dragging tracks the pointer raw and applies silently; the release is
the commit that earns the receipt. Picking a tile is a finished act -
the caller applies, closes and whispers.
-->

<script lang="ts">
  import { tick, untrack } from 'svelte';

  import Icon from './Icon.svelte';

  const {
    color,
    inUse,
    onApply,
    onPick,
  }: {
    /** The row's colour, #rrggbb. */
    color: string;
    /** Every colour the whole list carries right now, any order. */
    inUse: readonly string[];
    /** A change from the area, the rail or the hex field - the picker stays open. */
    onApply: (hex: string, opts: { silent: boolean }) => void;
    /** A tile press: apply, close, whisper - the caller's finished act. */
    onPick: (hex: string) => void;
  } = $props();

  /* The picker owns its hsv: a grey's hex has no hue to read back, so the
     hue survives here across round-trips instead of resetting to red. */
  let hsv = $state<Hsv>({ h: 0, s: 0, v: 0 });
  let hexField = $state('');
  let hexInvalid = $state(false);
  let hexInput: HTMLInputElement | null = $state(null);
  /* What the row already holds, so leaving the untouched hex field is not a
     change - a blur must never mint a save for the colour it arrived with. */
  let applied = '';

  /* Seeded ONCE: while the picker stands open it is the colour's only
     writer, and reseeding from the prop on every applied drag would round
     the hue away through greys. */
  $effect(() => {
    untrack(() => {
      hsv = hexToHsv(color);
      hexField = color;
      applied = color;
    });
  });

  $effect(() => {
    /* The keyboard lands in the hex, fully selected: paste replaces. After
       tick, or the selection is made before the bound value arrives and
       collapses to a bare caret at the end. */
    if (hexInput === null) return;
    void tick().then(() => {
      hexInput?.focus({ preventScroll: true });
      hexInput?.select();
    });
  });

  const hueColor = $derived(hsvToHex({ h: hsv.h, s: 1, v: 1 }));
  const current = $derived(hsvToHex(hsv));
  const inUseOrdered = $derived(wheelOrder(inUse));

  function applyHsv(next: Hsv, silent: boolean): void {
    hsv = next;
    const hex = hsvToHex(next);
    if (document.activeElement !== hexInput) hexField = hex;
    hexInvalid = false;
    applied = hex;
    onApply(hex, { silent });
  }

  /* ---------- Drags: capture, raw tracking, commit on release ---------- */

  let drag: { el: HTMLElement; kind: 'area' | 'hue' } | null = null;

  function dragMove(event: PointerEvent, silent: boolean): void {
    if (drag === null) return;
    const rect = drag.el.getBoundingClientRect();
    if (drag.kind === 'hue') {
      const h = Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width)) * 360;
      applyHsv({ ...hsv, h }, silent);
    } else {
      const s = Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width));
      const v = 1 - Math.min(1, Math.max(0, (event.clientY - rect.top) / rect.height));
      applyHsv({ ...hsv, s, v }, silent);
    }
  }

  function dragStart(event: PointerEvent, kind: 'area' | 'hue'): void {
    const el = event.currentTarget as HTMLElement;
    drag = { el, kind };
    try {
      el.setPointerCapture(event.pointerId);
    } catch {
      /* synthetic pointers have no id to capture */
    }
    el.focus({ preventScroll: true });
    dragMove(event, true);
    event.preventDefault();
  }

  function dragEnd(event: PointerEvent): void {
    if (drag === null) return;
    dragMove(event, false);
    drag = null;
  }

  /* ---------- The keyboard walks both sliders ---------- */

  function areaKeys(event: KeyboardEvent): void {
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return;
    event.preventDefault();
    const step = (event.shiftKey ? 10 : 1) / 100;
    let { s, v } = hsv;
    if (event.key === 'ArrowLeft') s = Math.max(0, s - step);
    if (event.key === 'ArrowRight') s = Math.min(1, s + step);
    if (event.key === 'ArrowUp') v = Math.min(1, v + step);
    if (event.key === 'ArrowDown') v = Math.max(0, v - step);
    applyHsv({ ...hsv, s, v }, false);
  }

  function hueKeys(event: KeyboardEvent): void {
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return;
    event.preventDefault();
    const step = (event.shiftKey ? 10 : 1) * 2;
    const dir = event.key === 'ArrowRight' || event.key === 'ArrowUp' ? 1 : -1;
    applyHsv({ ...hsv, h: (hsv.h + dir * step + 360) % 360 }, false);
  }

  /* ---------- The hex field: debounced verdicts, immediate on paste ---------- */

  let hexTimer: ReturnType<typeof setTimeout> | undefined;

  function commitHex(): void {
    clearTimeout(hexTimer);
    hexTimer = undefined;
    const hex = parseHex(hexField);
    if (hex === null) {
      hexInvalid = true;
      return;
    }
    hexInvalid = false;
    hexField = hex;
    if (hex === applied) return;
    /* A grey round-trips with no hue of its own - the standing hue stays. */
    const read = hexToHsv(hex);
    hsv = { ...read, h: read.s === 0 ? hsv.h : read.h };
    applied = hex;
    onApply(hex, { silent: false });
  }

  function hexTyped(event: Event): void {
    /* Typing clears the verdict; the verdict lands when the debounce does. */
    if (hexInvalid) hexInvalid = false;
    clearTimeout(hexTimer);
    const type = (event as InputEvent).inputType;
    if (type === 'insertFromPaste' || type === 'insertFromDrop') commitHex();
    else hexTimer = setTimeout(commitHex, 500);
  }

  function hexKeys(event: KeyboardEvent): void {
    if (event.key !== 'Enter') return;
    commitHex();
  }
</script>

<div
  class="color-pop"
  role="dialog"
  aria-label="Label colour"
  style:--cp-hue-color={hueColor}
  style:--cp-color={current}
  style:--cp-x="{(hsv.s * 100).toFixed(1)}%"
  style:--cp-y="{((1 - hsv.v) * 100).toFixed(1)}%"
  style:--cp-h="{((hsv.h / 360) * 100).toFixed(1)}%"
>
  <div
    class="cp-area"
    role="slider"
    tabindex="0"
    aria-label="Saturation and brightness"
    aria-valuenow={Math.round(hsv.s * 100)}
    aria-valuetext="saturation {Math.round(hsv.s * 100)}%, brightness {Math.round(hsv.v * 100)}%"
    onpointerdown={(event) => dragStart(event, 'area')}
    onpointermove={(event) => dragMove(event, true)}
    onpointerup={dragEnd}
    onpointercancel={() => (drag = null)}
    onkeydown={areaKeys}
  >
    <span class="cp-knob"></span>
  </div>
  <div
    class="cp-hue"
    role="slider"
    tabindex="0"
    aria-label="Hue"
    aria-valuemin={0}
    aria-valuemax={360}
    aria-valuenow={Math.round(hsv.h)}
    onpointerdown={(event) => dragStart(event, 'hue')}
    onpointermove={(event) => dragMove(event, true)}
    onpointerup={dragEnd}
    onpointercancel={() => (drag = null)}
    onkeydown={hueKeys}
  >
    <span class="cp-hue-knob"></span>
  </div>
  <div class="color-custom">
    <div class="color-custom-row">
      <span class="label-swatch" style:--swatch={current}></span>
      <input
        class="text-inline is-color"
        class:is-invalid={hexInvalid}
        bind:this={hexInput}
        bind:value={hexField}
        aria-label="Custom hex colour"
        aria-invalid={hexInvalid}
        spellcheck="false"
        maxlength={7}
        oninput={hexTyped}
        onkeydown={hexKeys}
        onfocusout={commitHex}
      />
    </div>
    {#if hexInvalid}
      <span class="field-error"
        ><Icon name="alert" size="xs" /><span class="t">Hex colours look like #0e8a16</span></span
      >
    {/if}
  </div>
  <div class="menu-sep" role="none"></div>
  <p class="menu-eyebrow">In use</p>
  <div class="color-grid" role="listbox" aria-label="Colours already in use">
    {#each inUseOrdered as cell (cell)}
      <button
        class="color-cell"
        class:is-current={cell === current}
        type="button"
        role="option"
        aria-selected={cell === current}
        style:--cell={cell}
        data-ink={inkFor(cell)}
        aria-label={cell}
        onclick={() => onPick(cell)}
      >
        <Icon name="check" size="sm" />
      </button>
    {/each}
  </div>
  <p class="menu-eyebrow">Presets</p>
  <div class="color-grid" role="listbox" aria-label="Preset colours">
    {#each LABEL_PALETTE as cell (cell)}
      <button
        class="color-cell"
        class:is-current={cell === current}
        type="button"
        role="option"
        aria-selected={cell === current}
        style:--cell={cell}
        data-ink={inkFor(cell)}
        aria-label={cell}
        onclick={() => onPick(cell)}
      >
        <Icon name="check" size="sm" />
      </button>
    {/each}
  </div>
</div>

<style>
  /* The menu popover's surface and shadow, 30px controls, the 6px inner
     radius. Enters like every other popover - fast fade, 4px settle. */
  .color-pop {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: 10px;
    box-shadow:
      0 12px 32px var(--shadow-color),
      0 2px 8px var(--shadow-color);
    display: grid;
    gap: var(--space-2);
    inset-block-start: calc(100% + 4px);
    inset-inline-start: 0;
    padding: var(--cp-pad);
    position: absolute;
    width: max-content;
    z-index: var(--layer-popover);
  }

  @starting-style {
    .color-pop {
      opacity: 0;
      translate: 0 -4px;
    }
  }

  /* Saturation left-to-right, value bottom-to-top, the current hue behind:
     the classic two-gradient area. 220x150 whole, sized by the preset grid
     below it. Not a border: the 14% hairline the tiles wear read as a dark
     streak across the area's bright top edge - 7% black inside is the
     quietest thing that still keeps the white corner off the popover's
     white. */
  .cp-area {
    background:
      linear-gradient(to top, #000000, transparent),
      linear-gradient(to right, #ffffff, var(--cp-hue-color, #ff0000));
    border-radius: 6px;
    block-size: var(--cp-area-height);
    box-shadow: inset 0 0 0 1px rgb(0 0 0 / 7%);
    box-sizing: border-box;
    cursor: crosshair;
    /* Derived from the grid rather than typed beside it, so the plane and the tiles
       under it are one width by construction. */
    inline-size: var(--cp-width);
    position: relative;
    touch-action: none;
  }

  /* Both knobs are the switch thumb's material: white ring, soft throw. They
     track the pointer raw - a knob that eases behind a drag reads broken. */
  .cp-knob {
    background: var(--cp-color, #ff0000);
    border: 2px solid #ffffff;
    border-radius: 50%;
    block-size: var(--cp-knob);
    box-shadow:
      0 1px 3px rgb(0 0 0 / 35%),
      0 0 0 1px rgb(0 0 0 / 12%);
    inline-size: var(--cp-knob);
    inset-block-start: var(--cp-y, 0%);
    inset-inline-start: var(--cp-x, 100%);
    pointer-events: none;
    position: absolute;
    translate: -50% -50%;
  }

  .cp-hue {
    background: linear-gradient(
      to right,
      #ff0000,
      #ffff00,
      #00ff00,
      #00ffff,
      #0000ff,
      #ff00ff,
      #ff0000
    );
    border-radius: 999px;
    block-size: var(--cp-hue-height);
    cursor: pointer;
    margin-block: 2px;
    /* The knob's own radius, so its centre reaches both rail ends without the ring
       leaving the popover - derived, so resizing the knob moves the rail with it. */
    margin-inline: calc(var(--cp-hue-thumb) / 2);
    position: relative;
    touch-action: none;
  }

  .cp-hue-knob {
    background: var(--cp-hue-color, #ff0000);
    border: 2px solid #ffffff;
    border-radius: 50%;
    block-size: var(--cp-hue-thumb);
    box-shadow:
      0 1px 3px rgb(0 0 0 / 35%),
      0 0 0 1px rgb(0 0 0 / 12%);
    inline-size: var(--cp-hue-thumb);
    inset-block-start: 50%;
    inset-inline-start: var(--cp-h, 0%);
    pointer-events: none;
    position: absolute;
    translate: -50% -50%;
  }

  /* Keyboard walks the picker the way the pointer does. */
  .cp-area:focus-visible,
  .cp-hue:focus-visible {
    outline: var(--focus-ring-width) solid var(--text-muted);
    outline-offset: var(--focus-ring-offset);
  }

  /* Held, a knob SEATS like every raised thumb: the throw collapses to its
     ring for exactly as long as the finger is down. The knob's position is
     the value, so it never dips - the shadow is the whole press. */
  .cp-knob,
  .cp-hue-knob {
    transition: box-shadow var(--duration-press) var(--ease-standard);
  }

  .cp-area:active .cp-knob,
  .cp-hue:active .cp-hue-knob {
    box-shadow:
      0 0 0 1px rgb(0 0 0 / 22%),
      0 1px 2px rgb(0 0 0 / 24%);
  }

  .menu-eyebrow {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 600;
    letter-spacing: 0.07em;
    line-height: var(--leading-tight);
    margin: 0;
    padding: 0 2px;
    text-transform: uppercase;
  }

  .menu-sep {
    background: var(--border-subtle);
    block-size: 1px;
    margin-block: var(--space-1);
  }

  /* THE SWATCH GRID IS THE MODULE, and everything else derives from it. It wrote 8, 4
     and 24 of its own while the tokens naming those decisions sat unread, so the pop
     came out wider than the tiles it held and the grid sat left in its own column. */
  .color-grid {
    display: grid;
    gap: var(--cp-gap);
    grid-template-columns: repeat(var(--cp-cols), var(--cp-cell));
  }

  .color-cell {
    align-items: center;
    background: var(--cell);
    block-size: var(--cp-cell);
    /* The hairline keeps the pale half of the palette a tile on the popover's
       own white rather than a hole in it. */
    border: 1px solid color-mix(in srgb, var(--text-primary) 14%, transparent);
    border-radius: 6px;
    cursor: pointer;
    display: inline-flex;
    inline-size: var(--cp-cell);
    justify-content: center;
    padding: 0;
    position: relative;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard),
      outline-color var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  /* The tiles speak the established press language: the two-step veil rides
     the coloured fill (half on hover, full held), the dip and the well's
     inset land the press. */
  .color-cell::before {
    background: var(--row-pressed);
    border-radius: inherit;
    content: '';
    inset: 0;
    opacity: 0;
    pointer-events: none;
    position: absolute;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .color-cell:hover::before {
    opacity: 0.5;
  }

  .color-cell:active::before {
    opacity: 1;
  }

  .color-cell:hover,
  .color-cell:focus-visible {
    outline: var(--focus-ring-width) solid var(--control-border);
    outline-offset: var(--focus-ring-offset);
  }

  .color-cell:focus-visible {
    outline-color: var(--text-muted);
  }

  .color-cell:active {
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }

  .color-cell :global(svg) {
    block-size: 14px;
    color: #ffffff;
    display: none;
    inline-size: 14px;
  }

  .color-cell[data-ink='dark'] :global(svg) {
    color: #1b1f24;
  }

  .color-cell.is-current :global(svg) {
    display: inline-flex;
  }

  .color-custom {
    display: grid;
    gap: var(--space-1);
  }

  .color-custom-row {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  .label-swatch {
    background: var(--swatch);
    border-radius: 50%;
    block-size: 12px;
    display: inline-block;
    flex: none;
    inline-size: 12px;
  }

  .text-inline {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-size: var(--font-size-control);
    min-block-size: 30px;
    padding-inline: 0.55rem;
  }

  .text-inline.is-color {
    flex: 1;
    font-family: var(--mono);
    min-inline-size: 0;
    width: 6.5rem;
  }

  /* One ring, fused to the field: the border takes the focus colour and the
     outline overlaps it - under the global offset-2 ring an input read as
     ringed twice. */
  .text-inline:focus {
    border-color: var(--focus);
    outline: var(--focus-ring-width) solid var(--focus);
    outline-offset: var(--focus-ring-inset);
  }

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
</style>
