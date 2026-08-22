<script lang="ts">
  import { tick } from 'svelte';

  import Icon, { type IconName } from './Icon.svelte';

  type SegmentTone = 'default' | 'accent' | 'on' | 'off';

  interface SegmentOption {
    value: string;
    label: string;
    tone?: SegmentTone;
    badge?: string | number;
    /** Resolved-value suffix rendered after the label, e.g. "· Enabled". */
    detail?: { text: string; tone: 'on' | 'off' };
    /**
     * Draws this option's boundary as a dashed outline: its value is where the control lands
     * without a choice being made here, rather than something chosen.
     */
    outline?: boolean;
    /** Renders in place of the label, which stays as the accessible name. */
    icon?: IconName;
  }

  const {
    name,
    label,
    descriptionId,
    options,
    value,
    preview = null,
    disabled = false,
    align = 'start',
    compact = false,
    variant = 'default',
    surface = 'panel',
    onSelect,
  }: {
    name: string;
    label: string;
    descriptionId?: string;
    options: ReadonlyArray<SegmentOption>;
    /** `null` selects nothing, and the thumb does not render. */
    value: string | null;
    /**
     * An option the control is offering to move to before it is chosen: it takes on the look it
     * will have afterwards, and the current selection steps back so the two can be compared.
     */
    preview?: string | null;
    disabled?: boolean;
    align?: 'start' | 'end';
    compact?: boolean;
    variant?: 'default' | 'navigation';
    /**
     * Which family of surfaces to draw on. Sidebar popovers carry their own, and
     * `night` is for a control standing on the invitation page's sky rather than
     * on any of the themed surfaces.
     */
    surface?: 'panel' | 'sidebar' | 'night';
    onSelect: (value: string) => void;
  } = $props();

  const selectedTone = $derived(
    options.find((option) => option.value === value)?.tone ?? 'default',
  );

  /**
   * Where the previewed option sits relative to the selection, so the thumb can square the corner
   * they share the way a hovered neighbour does.
   */
  const previewSide = $derived.by(() => {
    if (preview === null || value === null) return undefined;
    const [selected, offered] = [
      options.findIndex((option) => option.value === value),
      options.findIndex((option) => option.value === preview),
    ];
    if (selected === -1 || offered === -1 || Math.abs(selected - offered) !== 1) return undefined;
    return offered < selected ? 'before' : 'after';
  });

  function positionSelection(node: HTMLFieldSetElement, selection: string) {
    let currentSelection = selection;

    /* Nothing to point at, so the thumb takes up no room and gives up its transition: whatever it
       is told next is a first placement, not a move from here. */
    function collapse(): void {
      node.style.setProperty('--segment-width', '0px');
      node.classList.remove('selection-ready');
    }

    function measure(): void {
      const option = node.querySelector<HTMLInputElement>('input:checked')?.closest('label');
      // Nothing checked is a real state, not a missing one: the control is showing a value it
      // inherits rather than one chosen here, so the thumb collapses instead of staying put.
      if (option === null || option === undefined) {
        collapse();
        return;
      }

      /* A control inside a closed popover has no box, and `offsetWidth` reports that as 0 exactly
         as it reports a real zero. Reading the two the same way is what left the thumb at 0x0 on
         the first open of the account menu: the one measurement this had ever taken was taken
         against `display: none`, and only choosing a different option ever asked for another. No
         box is not a position, so it collapses and waits to be measured somewhere it can be. */
      if (option.offsetWidth === 0) {
        collapse();
        return;
      }

      node.style.setProperty('--segment-left', `${option.offsetLeft}px`);
      node.style.setProperty('--segment-width', `${option.offsetWidth}px`);

      /* Landing in place rather than sliding into it, which is a real difference and not a
         precaution: the two writes above and the class below otherwise resolve in one style
         change, and a transition compares against the style *before* it. So the width would go
         from nothing to its measurement with the transition already on. Reading layout in between
         splits them in two, and by the time the class lands the geometry is the old style with
         nothing left to animate. Measured with the read taken out: the theme switch opened at 6px
         and grew to 40, and the history menu's at 11px and grew to 73. */
      if (!node.classList.contains('selection-ready')) {
        node.getBoundingClientRect();
        node.classList.add('selection-ready');
      }
    }

    /* Measured once Svelte has committed the DOM, which is the event this actually waits on: the
       action's own effect can run before the `checked` attributes the measurement reads. `tick()`
       names that moment, where a pair of animation frames only landed safely past it and spent
       about two frames of stillness getting there. Nothing is cancelled when a newer call arrives
       - `measure` captures nothing and reads the checked option each time, so an older one writes
       the same answer as the newer rather than a stale one, and there is no turn to lose. */
    async function scheduleMove(): Promise<void> {
      await tick();
      measure();
    }

    /* Gaining a box is the event this actually waits on, and no prop changes when it happens: a
       popover opens, a pane unfolds, a webfont finally arrives and every option is a little wider.
       The observer catches all of those, and it fires after layout, so it can measure on the spot.
       It cannot feed itself either - the thumb is absolutely positioned, so what this writes never
       changes the size being watched.

       It measures directly and does not go through `scheduleMove`, which is not a shortcut but the
       point: `ViewTabs` had its own thumb scheduled through a frame pair that a resize tick could
       cancel out from under it, so a click landing while the sidebar was still settling moved
       nothing at all. Nothing here is cancellable and nothing is captured - `measure` reads the
       checked option each time - so a tick arriving mid-flight writes the same answer early rather
       than taking anybody's turn. */
    const resize = new ResizeObserver(() => measure());
    resize.observe(node);

    void scheduleMove();

    return {
      update(nextSelection: string) {
        if (nextSelection === currentSelection) return;
        currentSelection = nextSelection;
        void scheduleMove();
      },
      destroy() {
        resize.disconnect();
      },
    };
  }
</script>

<fieldset
  class={[
    align === 'end' && 'align-end',
    compact && 'compact',
    variant === 'navigation' && 'navigation',
    surface === 'sidebar' && 'on-sidebar',
    surface === 'night' && 'on-night',
  ]}
  class:selected-accent={selectedTone === 'accent'}
  class:selected-on={selectedTone === 'on'}
  class:selected-off={selectedTone === 'off'}
  class:previewing={preview !== null}
  data-preview={previewSide}
  aria-describedby={descriptionId}
  use:positionSelection={value ?? ''}
  {disabled}
>
  <legend>{label}</legend>
  <span class="selection-indicator" aria-hidden="true"></span>
  {#each options as option (option.value)}
    <label class:outlined={option.outline === true} class:previewed={preview === option.value}>
      <input
        type="radio"
        {name}
        value={option.value}
        checked={value === option.value}
        onchange={(event) => onSelect(event.currentTarget.value)}
      />
      <span class="segment-label">
        {#if option.icon === undefined}
          <!-- Trimmed so the label centres on its glyph bounds within the segment. -->
          <span class="band-trim">{option.label}</span>
        {:else}
          <Icon name={option.icon} size={14} />
          <span class="visually-hidden">{option.label}</span>
        {/if}
        {#if option.detail !== undefined}
          <span class="segment-detail band-trim detail-{option.detail.tone}"
            >· {option.detail.text}</span
          >
        {/if}
        {#if option.badge !== undefined}
          <!-- A count beside its word, not a superscript of it. `sup` earned
               nothing here: in a flex row `vertical-align` does not apply, so
               the element was a plain box being shoved upward by hand. -->
          <span class="segment-badge"><span class="cap-trim">{option.badge}</span></span>
        {/if}
      </span>
    </label>
  {/each}
</fieldset>

<style>
  fieldset {
    /* Every surface the control paints reads through these five, so a different ground is one
       block of overrides rather than a second copy of the component. */
    --seg-track: var(--segment-track);
    --seg-hover: var(--segment-hover);
    --seg-pressed: var(--segment-pressed);
    /* --border-control, not the hairline: the shell redesign strengthened
       the track's edge to the control border every field wears. */
    --seg-border: var(--border-control);
    --seg-shadow: var(--segment-shadow);
    --seg-muted: var(--text-muted);
    --seg-text: var(--text);
    --selected-bg: var(--segment-thumb);
    --selected-text: var(--text-primary);
    background: var(--seg-track);
    border: 1px solid var(--seg-border);
    border-radius: var(--r-ctl);
    display: inline-flex;
    flex: none;
    height: var(--local-control-height, var(--control-height-compact));
    isolation: isolate;
    margin: 0;
    min-width: 0;
    overflow: clip;
    padding: var(--control-inset);
    position: relative;
  }

  /* A sidebar popover carries its own surfaces, so a control inside one follows the popover rather
     than the page behind it. */
  fieldset.on-sidebar {
    --seg-track: var(--sidebar-seg-track);
    --seg-hover: var(--sidebar-seg-hover);
    --seg-pressed: var(--sidebar-seg-pressed);
    --seg-border: var(--sidebar-seg-border);
    --seg-shadow: var(--sidebar-seg-shadow);
    --seg-muted: var(--sidebar-menu-muted);
    /* The menu's ink, not the page's. The Root console's sidebar is dark in both themes while the
       page behind it is not, so `--seg-text` inherited from the base rule put the light theme's
       near-black label on the menu's near-black track: 1.09:1, which is a label that is not there.
       Every other ink in this block already comes from the sidebar family - this one was missed. */
    --seg-text: var(--sidebar-menu-text);
    --selected-bg: var(--sidebar-seg-thumb);
    --selected-text: var(--sidebar-menu-text);
  }

  /* Standing on the invitation page's sky, which is night in both themes, so this
     one is the same control in light mode as in dark. It is glass rather than a
     surface: the track is white over whatever is behind it, so the stars carry on
     under the control instead of stopping at its edge. */
  fieldset.on-night {
    --seg-track: var(--night-seg-track);
    --seg-hover: var(--night-seg-hover);
    --seg-pressed: var(--night-seg-pressed);
    --seg-border: var(--night-seg-border);
    --seg-shadow: var(--night-seg-shadow);
    --seg-muted: var(--night-seg-muted);
    --seg-text: var(--night-seg-text);
    --selected-bg: var(--night-seg-thumb);
    --selected-text: var(--night-seg-text);

    backdrop-filter: blur(6px);
  }

  fieldset.selected-accent {
    --selected-text: var(--brand-action-text);
  }

  fieldset.selected-on {
    --selected-text: var(--success);
  }

  /* Off is a safe state, not an error: the thumb stays neutral. */
  fieldset.selected-off {
    --selected-text: var(--text-primary);
  }

  fieldset.align-end {
    justify-self: end;
  }

  /* Same fallback as the base rule rather than the token outright: `compact` wins
     on specificity, so naming the token here would make it override the height a
     caller had already set and leave `--local-control-height` silently ignored on
     every compact control. */
  fieldset.compact {
    height: var(--local-control-height, var(--control-height-compact));
  }

  fieldset.compact .segment-label {
    font-size: var(--font-size-compact);
    min-width: 2.25rem;
  }

  fieldset.navigation .segment-label {
    font-size: var(--font-size-meta);
    gap: 0.35rem;
    min-width: 0;
    padding: 0 var(--space-3);
  }

  legend {
    clip-path: inset(50%);
    height: 1px;
    overflow: hidden;
    position: absolute;
    white-space: nowrap;
    width: 1px;
  }

  label {
    cursor: pointer;
    display: flex;
    height: 100%;
    position: relative;
  }

  /* The hover lift, under the label and over the track. It steps *down* from the track: the thumb
     is the lighter of the two in every palette, so a lift toward it previews selection. See
     --segment-hover in app.css for the measurements. */
  label::before {
    background: var(--seg-hover);
    border-radius: calc(var(--r-ctl) - 2px);
    content: '';
    inset: 0;
    opacity: 0;
    pointer-events: none;
    position: absolute;
    transition:
      opacity 120ms ease-out,
      background-color var(--duration-press) var(--ease-standard);
    z-index: 1;
  }

  label:hover:not(:has(input:disabled))::before {
    opacity: 1;
  }

  /* The press goes one step further down the same ramp, so hover and press are the same gesture at
     two depths rather than two different ideas. */
  label:active:not(:has(input:disabled))::before {
    background: var(--seg-pressed);
    opacity: 1;
  }

  /* Hovering the option next to the selected one squares the corner on its own
     side of the join, so the fill runs flat up to the thumb's edge instead of
     curving away from it and leaving a notch. The thumb keeps its own rounding:
     the hover wraps around the outside of that curve rather than cutting it off,
     which is what makes the two read as one lit stretch of the control.

     What fills the crescent outside the curve is `.selection-indicator::after`,
     drawn on the thumb rather than by the hover. The hover used to reach a radius'
     worth of fill in *under* the thumb to cover it, which works only while the
     thumb is opaque - on the night surface it is glass, and the fill tucked behind
     it read straight through as a bright band across the selected option. Painting
     only the crescent puts nothing underneath at all, so it holds however
     see-through the thumb is. */
  label:has(input:checked) + label:hover:not(:has(input:disabled))::before {
    border-start-start-radius: 0;
    border-end-start-radius: 0;
  }

  label:hover:not(:has(input:disabled)):has(+ label input:checked)::before {
    border-start-end-radius: 0;
    border-end-end-radius: 0;
  }

  /* The crescent itself: a strip one thumb-radius wide, reaching over the thumb's
     corner square from the hovered option's own edge. It is two tiles, each a disc
     of that radius punched out of the fill - transparent where the thumb is, lit
     where it is not - and each disc is centred on the arc the thumb's corner is
     drawn with, so the two curves are the same curve and meet without a seam.

     It rides the hovered label rather than the thumb because a rule that had to
     ask "is the thumb next to something hovered" would need `:has()` inside
     `:has()`, which is invalid and drops the rule silently. From here one level
     is enough. It also sits *over* the thumb, which is safe precisely because the
     part covering the thumb is the transparent part. */
  label::after {
    --thumb-radius: calc(var(--r-ctl) - 2px);
    --crescent-fill: var(--seg-hover);

    background-repeat: no-repeat;
    background-size: var(--thumb-radius) var(--thumb-radius);
    bottom: 0;
    content: '';
    opacity: 0;
    pointer-events: none;
    position: absolute;
    top: 0;
    transition: opacity 120ms ease-out;
    width: var(--thumb-radius);
    z-index: 2;
  }

  label:has(input:checked) + label:hover:not(:has(input:disabled))::after {
    background-image:
      radial-gradient(
        circle var(--thumb-radius) at 0 100%,
        transparent 0 var(--thumb-radius),
        var(--crescent-fill) var(--thumb-radius)
      ),
      radial-gradient(
        circle var(--thumb-radius) at 0 0,
        transparent 0 var(--thumb-radius),
        var(--crescent-fill) var(--thumb-radius)
      );
    background-position:
      0 0,
      0 100%;
    opacity: 1;
    right: 100%;
  }

  label:hover:not(:has(input:disabled)):has(+ label input:checked)::after {
    background-image:
      radial-gradient(
        circle var(--thumb-radius) at 100% 100%,
        transparent 0 var(--thumb-radius),
        var(--crescent-fill) var(--thumb-radius)
      ),
      radial-gradient(
        circle var(--thumb-radius) at 100% 0,
        transparent 0 var(--thumb-radius),
        var(--crescent-fill) var(--thumb-radius)
      );
    background-position:
      0 0,
      0 100%;
    left: 100%;
    opacity: 1;
  }

  /* The crescent follows the fill it belongs to down the same ramp. */
  label:has(input:checked) + label:active:not(:has(input:disabled))::after,
  label:active:not(:has(input:disabled)):has(+ label input:checked)::after {
    --crescent-fill: var(--seg-pressed);
  }

  /* An offer on the table already squares the thumb's shared corner, so there is
     no crescent left to fill. */
  fieldset.previewing label::after {
    opacity: 0;
  }

  input {
    height: 1px;
    opacity: 0;
    position: absolute;
    width: 1px;
  }

  .segment-label {
    align-items: center;
    /* The transparent border is load-bearing: the mock's segment is
       `1px solid transparent` + 0.75rem of padding, so every segment reads 2px
       wider than padding alone would make it. Drop it and each control shrinks
       by 2px per option — the width mismatch is visible against the search
       field it shares a row with. */
    border: 1px solid transparent;
    border-radius: calc(var(--r-ctl) - 2px);
    color: var(--seg-muted);
    display: flex;
    font-size: var(--font-size-compact);
    font-weight: 600;
    gap: 0.35rem;
    height: 100%;
    justify-content: center;
    line-height: 1;
    padding: 0 0.75rem;
    position: relative;
    transition:
      color 180ms ease-out,
      transform var(--duration-press) var(--ease-standard);
    z-index: 3;
  }

  .segment-detail {
    color: var(--dim);
    font-weight: 500;
  }

  .segment-detail.detail-on {
    color: var(--success);
    font-weight: 600;
  }

  input:checked ~ .segment-label .segment-detail.detail-off {
    color: var(--text-secondary);
  }

  /* On the word's own centre line. Lifted, it hung over the top of the segment
     and met the hover fill's edge with nothing between them, and the space it
     vacated below made the option read as taller on one side than the other.
     A count belongs beside what it counts.

     Square rather than wide: at 1.125rem of minimum width a single digit sat in
     a box half again its own size, and that slack read as a gap after the badge
     that no other pair of things in the control has. */
  /* The number inside is trimmed to its own band, so `place-items: center` puts
     the digits on the badge's middle rather than their line box - a 9px numeral
     in a 14px square sat 0.78px high of it, which is a device row at 2x. */
  .segment-badge {
    align-items: center;
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: 0.25rem;
    color: var(--text-muted);
    display: inline-grid;
    font: 700 0.5625rem / 1 var(--mono);
    font-variant-numeric: tabular-nums;
    height: 0.875rem;
    justify-content: center;
    min-width: 0.875rem;
    padding: 0 0.1875rem;
    place-items: center;
  }

  input:checked ~ .segment-label .segment-badge {
    background: var(--brand-action-tint);
    border-color: color-mix(in srgb, var(--brand-action-text) 20%, var(--brand-action-tint));
    color: var(--brand-action-text);
  }

  input:checked ~ .segment-label {
    color: var(--selected-text);
    font-weight: 700;
  }

  label:hover input:not(:checked):not(:disabled) ~ .segment-label {
    color: var(--seg-text);
  }

  label:active input:not(:disabled) ~ .segment-label {
    transform: scale(0.97);
  }

  .selection-indicator {
    /* Hidden until something is selected - see the rule below the transition. */
    display: none;

    background: var(--selected-bg);
    border-radius: calc(var(--r-ctl) - 2px);
    box-shadow: var(--seg-shadow);
    bottom: var(--control-inset);
    left: var(--segment-left, var(--control-inset));
    pointer-events: none;
    position: absolute;
    top: var(--control-inset);
    transition:
      left 240ms cubic-bezier(0.22, 1, 0.36, 1),
      width 240ms cubic-bezier(0.22, 1, 0.36, 1),
      background-color var(--duration-fast) var(--ease-standard),
      box-shadow var(--duration-fast) var(--ease-standard);
    width: var(--segment-width, 0);
    z-index: 2;
  }

  fieldset:not(.selection-ready) .selection-indicator {
    transition: none;
  }

  /* The thumb is drawn only when something is selected.
     --------------------------------------------------
     A control can legitimately have nothing chosen: the inherit state, where the
     value comes from the account and the option that would apply is drawn as a
     dashed outline instead. The thumb was still in the page for it, measured to
     `--segment-width: 0px`.

     Zero width is not invisible. The box keeps its background - which paints
     nothing at zero - and its shadow, which does not: `0 0 0 0.5px` is a ring
     around the box's edge, and around a box 0px wide and 28px tall that ring is
     a 1px vertical line. It stood just inside the control's left edge on every
     row of the repositories table that inherits its enablement.

     Written as "hidden, then shown" rather than as `:not(:has(input:checked))`,
     which is the way round it reads. The compiler drops that one: it prunes
     selectors it cannot prove this component's own markup can match, and a
     `:has()` inside a `:not()` defeats that analysis, so the rule never reached
     the stylesheet at all - measured, not guessed. */
  fieldset:has(input:checked) .selection-indicator {
    display: block;
  }

  /* 2.5% and 5%, not 8% and 16%. At the larger pair the thumb's own states measured 4.07 and 8.27
     dE00 against white, and the 8.27 was 2.7x the 3.08 that says the option is selected at all:
     acknowledging a pointer shouted over the state it was acknowledging.

     The window either side is narrower than it looks, because this one pair has to hold on three
     thumbs across four palettes, each mixing a different ink. Below 2.5% the sidebar popover's
     hover falls under a just-noticeable difference (0.98, mixing a grey menu ink into white);
     above it the panel's press passes the 3.08 fill it must stay under. 2.5/5 is the only pair
     that clears both ends everywhere. Pressing the option that is already selected is also the one
     press that changes nothing, so the faintest feedback on the control is the honest one for it. */
  fieldset:has(label:hover input:checked:not(:disabled)) .selection-indicator {
    background: color-mix(in srgb, var(--selected-text) 2.5%, var(--selected-bg));
  }

  fieldset:has(label:active input:checked:not(:disabled)) .selection-indicator {
    background: color-mix(in srgb, var(--selected-text) 5%, var(--selected-bg));
  }

  /* The value the control falls back to, drawn as a boundary rather than a fill: it is where this
     lands without a choice, so it must not read as one already made. */
  label.outlined .segment-label {
    border-color: color-mix(in srgb, currentcolor 55%, transparent);
    border-style: dashed;
    color: var(--text-secondary);
    font-weight: 650;
  }

  /* The offer: this is what the control will look like once the move is taken. */
  label.previewed .segment-label {
    animation: segment-preview 1.6s ease-in-out infinite;
    border-color: color-mix(in srgb, var(--brand-action) 70%, transparent);
    border-style: dashed;
    color: var(--text-primary);
  }

  /* The selection steps back whenever an offer is on the table - including the case where the
     offer is the option already selected, which is a real one: an override can happen to name the
     same value it inherits. */
  fieldset.previewing .selection-indicator,
  fieldset.previewing input:checked ~ .segment-label {
    opacity: 0.4;
  }

  /* Squaring the shared corner only applies when the two sit next to each other, so it is keyed on
     that separately rather than on the offer existing. */

  fieldset[data-preview='before'] .selection-indicator {
    border-end-start-radius: 0;
    border-start-start-radius: 0;
  }

  fieldset[data-preview='after'] .selection-indicator {
    border-end-end-radius: 0;
    border-start-end-radius: 0;
  }

  @keyframes segment-preview {
    0%,
    100% {
      box-shadow: 0 0 0 0 color-mix(in srgb, var(--brand-action) 38%, transparent);
    }

    50% {
      box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand-action) 10%, transparent);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    label.previewed .segment-label {
      animation: none;
      box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand-action) 25%, transparent);
    }
  }

  .visually-hidden {
    clip-path: inset(50%);
    height: 1px;
    overflow: hidden;
    position: absolute;
    white-space: nowrap;
    width: 1px;
  }

  input:focus-visible ~ .segment-label {
    outline: 2px solid var(--brand);
    outline-offset: -2px;
  }

  input:disabled ~ .segment-label,
  fieldset:disabled .selection-indicator {
    opacity: 0.45;
  }

  fieldset:disabled label {
    cursor: default;
  }

  @media (max-width: 36rem) {
    fieldset.navigation .segment-label {
      padding-inline: var(--space-2);
    }
  }
</style>
