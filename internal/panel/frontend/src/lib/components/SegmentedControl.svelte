<script lang="ts">
  import { tick } from 'svelte';

  import Icon, { type IconName } from './Icon.svelte';

  type SegmentTone = 'default' | 'accent' | 'on' | 'off';

  interface SegmentOption {
    value: string;
    label: string;
    /**
     * What this option means once it is the selection, which tints the thumb rather
     * than repainting it: the selection is the accent everywhere, and a tone that
     * replaced it would make one control's selection a different material from the
     * next one's.
     */
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
    fluid = false,
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
    /** Fits every option inside the width supplied by the containing row. */
    fluid?: boolean;
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
   * How many visual pixels one layout pixel is drawn as, here.
   *
   * `zoom` multiplies down the tree and a `transform` scales what is under it, and no
   * single API reports either as an effective figure - so the chain is walked and the
   * two are multiplied. Only the horizontal factor: this positions a thumb along one
   * axis, and a rotation would make the question a different one.
   */
  function scaleOf(node: HTMLElement): number {
    let scale = 1;
    for (
      let element: HTMLElement | null = node;
      element !== null;
      element = element.parentElement
    ) {
      const style = getComputedStyle(element);
      const zoom = Number.parseFloat(style.zoom);
      if (Number.isFinite(zoom) && zoom > 0) scale *= zoom;
      if (style.transform !== 'none' && style.transform !== '') {
        const factor = new DOMMatrixReadOnly(style.transform).a;
        if (Number.isFinite(factor) && factor > 0) scale *= factor;
      }
    }
    return scale === 0 ? 1 : scale;
  }

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
      const optionBox = option.getBoundingClientRect();
      if (optionBox.width === 0) {
        collapse();
        return;
      }

      /* Fluid controls divide their width at subpixel boundaries. `offsetLeft` and `offsetWidth`
         round those boundaries to whole CSS pixels, which leaves the track showing through as a
         differently coloured seam around the thumb's curved corners. Keep the same coordinate
         system as the absolute indicator while retaining the fractional geometry; scrollLeft
         restores the content coordinate when a narrow control is horizontally scrolled. */
      const nodeBox = node.getBoundingClientRect();
      /* A rect is in VISUAL pixels and a custom property is spent as LAYOUT pixels, and
         inside a scaled subtree those are not the same unit: the scale lands once on
         the measurement and again when the thumb is drawn. Under a page at `zoom: 3` a
         re-measure put a 291px option under an 874px thumb, 605px off to the right.

         Read from the chain rather than inferred from a ratio. `rect.width /
         offsetWidth` looks like the same number and is not: `offsetWidth` ROUNDS to a
         whole pixel, so on a 323.4px control it reports 323 and the ratio comes out
         1.0012 at no zoom at all - which put the thumb 0.078px out and failed the
         checks that exist to hold this geometry to a twentieth of a pixel. Multiplying
         the `zoom` and the horizontal transform of every ancestor is exact at every
         scale, including none. */
      const scale = scaleOf(node);
      const left = (optionBox.left - nodeBox.left) / scale - node.clientLeft + node.scrollLeft;
      /* Read before the write, because the give below is owed to a MOVE and this runs
         for every re-measurement: a resize, a popover opening, a label changing width.
         Animating those would put a squeeze on the page where nothing travelled. */
      const moved = node.style.getPropertyValue('--segment-left') !== `${left}px`;
      const width = `${optionBox.width / scale}px`;
      const resized = node.style.getPropertyValue('--segment-width') !== width;
      /* A CONTROL THAT RE-MEASURES ITSELF HAS NOT MOVED, so it is placed again rather
         than animated to. The thumb is where it was and the option under it has changed
         width - a count arriving in a label, a webfont landing, a pane taking its own
         width - and easing to the new number draws a thumb growing under a reader who
         did nothing. On the queue page the counts arrive about a second in, and the
         thumb grew 18px on every load. Taking `selection-ready` off is the mechanism
         that is already here for a first placement, and this is one.

         BOTH TERMS, because a re-measure that finds NOTHING changed must not touch the
         class at all: this runs on every observer tick, one of which lands while a
         travel is still in flight, and dropping `selection-ready` there cancels the
         transition mid-way and the thumb arrives in a jump. */
      if (!moved && resized) node.classList.remove('selection-ready');
      node.style.setProperty('--segment-left', `${left}px`);
      node.style.setProperty('--segment-width', width);

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
      } else if (moved) {
        /* A move rather than a first placement, so the thumb gives along the way. The
           class is taken off and put back with a layout read between, which is what
           restarts an animation that may still be running from the last move - without
           it a reader clicking quickly along the options sees the give once. */
        const thumb = node.querySelector('.selection-indicator');
        if (thumb !== null) {
          thumb.classList.remove('is-travelling');
          thumb.getBoundingClientRect();
          thumb.classList.add('is-travelling');
        }
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
    const resize =
      typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => measure());
    resize?.observe(node);
    for (const option of node.querySelectorAll('label')) resize?.observe(option);

    void scheduleMove();

    return {
      update(nextSelection: string) {
        if (nextSelection === currentSelection) return;
        currentSelection = nextSelection;
        void scheduleMove();
      },
      destroy() {
        resize?.disconnect();
      },
    };
  }
</script>

<!--
@component
One choice from a set a reader can see all of at once, with the selection marked by a
thumb that slides between them. Two to five options: past that the words stop fitting
and the choice belongs in a `Select`, which is the same decision made in less width.

Glass, not paint: the track is a veil over a frosted pane, hover and press are deeper
veils of the same ink, and the selection is the one saturated material - a solid accent
carrying the accent's own inverse ink.

The thumb is the highest thing drawn in the track, so it never gives way - rounded on
all four corners, exactly the width of the option it marks, and held one control inset
off the track on every side. Everything else answers to it: a hover fill squares the
edge it faces, a crescent drawn over the thumb's near edge fills the wedge its curve
leaves, and an option with a boundary of its own paints the glass so it does not read
as a hole. A fill facing nothing keeps its curve, and a control with nothing selected
squares nothing at all.

`value` of `null` selects nothing and the thumb does not render, which is a real state
rather than a missing one - it is how a control says the value is inherited rather
than chosen here, and the option that would apply is drawn with `outline` instead.
`preview` offers what a move would look like before it is taken.

Its geometry is measured rather than declared: CSS cannot say "be the size of that
sibling", so an action reads the checked option's box and the thumb is placed from it.
That is the only reason there is script in this file, and it is what CSS anchor
positioning will replace once it is portable.
-->

<fieldset
  class={[
    align === 'end' && 'align-end',
    compact && 'compact',
    fluid && 'fluid',
    variant === 'navigation' && 'navigation',
    surface === 'sidebar' && 'on-sidebar',
    surface === 'night' && 'on-night',
  ]}
  class:selected-on={selectedTone === 'on'}
  class:selected-off={selectedTone === 'off'}
  class:previewing={preview !== null}
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
          <Icon name={option.icon} size="sm" />
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
    /* Every surface the control paints reads through these, so a different ground is one
       block of overrides rather than a second copy of the component. */
    --seg-track: var(--segment-track);
    --seg-hover: var(--segment-hover);
    --seg-pressed: var(--segment-pressed);
    /* The glass the veils are laid over, and the hairline that frames it. Both come
       from the control's own material rather than the page's, so a control inside a
       popover frosts the popover. */
    --seg-glass: color-mix(in srgb, var(--surface-base) 72%, transparent);
    /* THE MATERIAL A BOUNDED OPTION'S GROUND IS MADE OF, at full strength. The same
       stuff as the control's glass and not a veil of it: the fill of the option beside
       this one runs on underneath by a radius, and through a 72% ground that bleed
       shone through and tinted the cell whenever a neighbour was hovered or held. A
       drawn box has to be a box. */
    --seg-cell: var(--surface-base);
    --seg-frame: color-mix(in srgb, var(--text-primary) 14%, transparent);
    --seg-frame-width: 1px;
    /* A drop shadow seats the pane on the track, and that is the whole of it. There is
       no top reflection: a light 1px line inside the thumb's own top edge reads as
       more gutter than there is, so the thumb looked as though it sat lower in the
       track than it does - a highlight that lies about the geometry. */
    --seg-thumb-shadow: 0 1px 3px color-mix(in srgb, var(--shadow-color) 72%, transparent);
    /* A SURFACE PUSHED INTO A WELL IS OCCLUDED ON EVERY EDGE, not only the top. The
       rim above it casts the deepest shadow, because that is where the light is; the
       other three cast a shallower one all round, which is what was missing - the
       leading and trailing edges kept their resting colour and the pane read as
       sliding rather than sinking. The bottom rim catches the sliver of light the
       recess lets past, which is the cue that says "in" rather than "dark".

       Black rather than the ink: a recess is darker on every palette, and on the dark
       ones --text-primary is near-white, so an ink-mixed crease lit the hollow up. */
    --seg-thumb-shadow-pressed:
      inset 0 2px 4px rgb(0 0 0 / 26%), inset 0 0 3px 1px rgb(0 0 0 / 16%),
      inset 0 -1px 0 rgb(255 255 255 / 10%);
    /* The same well as the thumb's, shallower: an unselected option is a wash rather
       than a pane, so it sinks less far. One recipe at two depths, not two ideas.

       Its THREE PARTS are named rather than the whole, because one of them has to be
       placed per side and a custom property substitutes where it is DECLARED - a
       finished shadow written here would resolve the bleed against this element, which
       is two levels above the one that knows it, and quietly land every rim at zero. */
    --seg-well-head: rgb(0 0 0 / 16%);
    --seg-well-rim: rgb(0 0 0 / 10%);
    --seg-well-foot: rgb(255 255 255 / 8%);
    /* How far a fill runs on under the thumb: exactly the thumb's own corner radius,
       which is the width of the wedge its curve leaves. */
    --seg-bleed: var(--seg-inner-radius);
    /* THE THUMB IS INSET FROM THE INSIDE OF THE FRAME, not from the track's outer
       edge - which is what the inset meant while the frame was a border, and what it
       has to keep meaning now the frame is a ring drawn on the same edge. Written as
       the sum so moving either term moves the gap: dropping the frame's width silently
       took a pixel off every side of the thumb and closed the track up around it.
       The curve follows: the track's own, less what stands inside it. */
    --seg-gutter: calc(var(--seg-frame-width) + var(--control-inset));
    --seg-inner-radius: calc(var(--r-ctl) - var(--seg-gutter));
    /* Secondary, not muted: the veiled track holds muted ink at only 3.9:1 - under
       the 4.5 text floor. Rest secondary, hover and selected primary, so the ink
       ladder climbs alongside the surface one. */
    --seg-muted: var(--text-secondary);
    --seg-text: var(--text-primary);
    /* THE SELECTION IS THE ACCENT, TINTED - never repainted. A tone says what the
       chosen option means; it does not get to decide what material a selection is
       made of, or one control's thumb would be a different object from the next
       one's. So the accent stays the bulk of the mix and the tone is a share of it,
       and at rest the thumb holds one colour: only a press moves it.

       The press advances on the palette's own accent ramp, stated per palette
       because it has to darken on a light ground and brighten on a dark one. Mixing
       an ink into the fill instead cannot do that: on the dark palettes the thumb's
       ink is near-black, so one recipe sent hover and press in opposite directions.
       The same tone rides the pressed colour, so the press is one step along the
       ramp rather than a jump to a different hue. */
    --selected-tone: var(--segment-thumb);
    --selected-share: 0%;
    --selected-bg: color-mix(
      in srgb,
      var(--selected-tone) var(--selected-share),
      var(--segment-thumb)
    );
    --selected-hover: color-mix(
      in srgb,
      var(--selected-tone) var(--selected-share),
      var(--brand-action-hover)
    );
    --selected-pressed: color-mix(
      in srgb,
      var(--selected-tone) var(--selected-share),
      var(--brand-action-pressed)
    );
    /* The badge rides the thumb, so it takes the same tone at the same share off the
       accent's own tint pair - otherwise a toned selection carries a chip in the
       untinted accent and the two disagree about what the option means. */
    --selected-tint: color-mix(
      in srgb,
      var(--selected-tone) var(--selected-share),
      var(--brand-action-tint)
    );
    --selected-tint-ink: color-mix(
      in srgb,
      var(--selected-tone) var(--selected-share),
      var(--brand-action-text)
    );
    --selected-text: var(--on-brand-action);

    /* THE TRACK IS A VEIL OVER GLASS, not a flat inset surface. Both layers are
       translucent, so the control reads as a pane laid on the page and lands the
       same step on canvas, on a card and inside a popover - a track mixed into one
       named surface is a colour decided against one ground and wrong on the others.
       A `linear-gradient` of a single colour is how a flat tint is laid over another
       background inside one `background` shorthand. */
    backdrop-filter: blur(6px);
    background: linear-gradient(var(--seg-track), var(--seg-track)), var(--seg-glass);
    /* No border: the frame is an inset ring, which cannot take part in the box's own
       size and so leaves the track's inner radius one term simpler. */
    border: 0;
    border-radius: var(--r-ctl);
    box-shadow:
      inset 0 0 0 1px var(--seg-frame),
      0 1px 2px color-mix(in srgb, var(--shadow-color) 14%, transparent);
    box-sizing: border-box;
    display: inline-flex;
    flex: none;
    height: var(--local-control-height, var(--control-height-compact));
    /* Wide enough for its options and no wider, whoever it is dropped into. `flex: none`
       is not enough on its own: a flex item's `display: inline-flex` is blockified to
       `flex`, and a cross size left `auto` is then stretched by any COLUMN flex parent -
       which is what a two-option control spanning the whole content column was. An
       explicit inline size is not `auto`, so `align-self: stretch` has nothing to
       stretch. `fluid` overrides it below, which is what that prop is for. */
    inline-size: fit-content;
    isolation: isolate;
    margin: 0;
    min-width: 0;
    overflow: clip;
    padding: var(--seg-gutter);
    position: relative;
  }

  /* A sidebar popover carries its own surfaces, so a control inside one follows the popover rather
     than the page behind it. */
  fieldset.on-sidebar {
    --seg-track: var(--sidebar-seg-track);
    --seg-hover: var(--sidebar-seg-hover);
    --seg-pressed: var(--sidebar-seg-pressed);
    --seg-glass: color-mix(in srgb, var(--sidebar-popover-bg) 72%, transparent);
    --seg-cell: var(--sidebar-popover-bg);
    /* The popover's own darkness, not the page's. The Root menu is dark inside a light
       page, where `--shadow-color` is a faint ink wash that seats nothing. */
    --seg-thumb-shadow: 0 1px 3px rgb(0 0 0 / 34%);
    --seg-frame: color-mix(in srgb, var(--sidebar-menu-text) 14%, transparent);
    --seg-sheen: color-mix(in srgb, var(--sidebar-popover-bg) 72%, transparent);
    --seg-thumb-shadow-pressed:
      inset 0 2px 5px color-mix(in srgb, var(--sidebar-menu-text) 18%, transparent),
      inset 0 1px 0 color-mix(in srgb, var(--sidebar-popover-bg) 42%, transparent);
    --seg-muted: var(--sidebar-menu-muted);
    /* The menu's ink, not the page's. The Root console's sidebar is dark in both themes while the
       page behind it is not, so `--seg-text` inherited from the base rule put the light theme's
       near-black label on the menu's near-black track: 1.09:1, which is a label that is not there.
       Every other ink in this block already comes from the sidebar family - this one was missed. */
    --seg-text: var(--sidebar-menu-text);
  }

  /* Standing on the invitation page's sky, which is night in both themes, so this
     one is the same control in light mode as in dark. The glass is the sky itself:
     nothing is laid under the veils, so the stars carry on beneath the control
     instead of stopping at its edge. Its selection is a white pane rather than the
     accent - there is no page palette out here for an accent to belong to. */
  fieldset.on-night {
    --seg-track: var(--night-seg-track);
    --seg-hover: var(--night-seg-hover);
    --seg-pressed: var(--night-seg-pressed);
    --seg-glass: transparent;
    /* The one ground where a bounded option cannot be opaque - a solid cell on the sky
       is a hole in it - and the one where it need not be, because nothing bleeds here. */
    --seg-cell: var(--night-seg-track);
    /* No bleed out here. This thumb is a veil rather than a fill, so a fill running on
       underneath it would show straight through instead of being covered - and the
       wedge it exists to close is the one thing a translucent pane does not leave. */
    --seg-bleed: 0px;
    --seg-frame: var(--night-seg-border);
    --seg-thumb-shadow: var(--night-seg-shadow);
    /* White INTO the veil, so each step raises its alpha: the night thumb is a wash
       rather than a fill, and its ramp climbs the way every other dark ground's does. */
    --selected-hover: color-mix(in srgb, #ffffff 5%, var(--night-seg-thumb));
    --selected-pressed: color-mix(in srgb, #ffffff 10%, var(--night-seg-thumb));
    --seg-muted: var(--night-seg-muted);
    --seg-text: var(--night-seg-text);
    --selected-bg: var(--night-seg-thumb);
    --selected-text: var(--night-seg-text);
  }

  /* A third of the way toward the tone and no further: past that the mix stops being
     the accent with something said about it and starts being a second selection
     colour. `accent` and `default` name no tone at all - the accent is already what
     the thumb is. */
  fieldset.selected-on {
    --selected-tone: var(--success);
    --selected-share: 30%;
  }

  /* Off is a safe state, not an error: the thumb quiets toward the shell's neutral
     rather than reaching for a warning colour. */
  fieldset.selected-off {
    --selected-tone: var(--text-muted);
    --selected-share: 30%;
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

  fieldset.fluid {
    flex: 1 1 auto;
    /* AND IT SCROLLS WHEN THE WORDS WILL NOT FIT. The track clips, which is right for a
       thumb sliding inside it and wrong for the one case where the options themselves
       are wider than the row: under the letter spacing a reader may set for 1.4.12 these
       labels ran 5px past the track and those 5px were simply gone. `clip` on the block
       axis keeps the thumb where it belongs; `auto` on the inline one gives the words a
       way out. */
    overflow-x: auto;
    overflow-y: clip;
    width: 100%;
  }

  fieldset.fluid label {
    /* Start from each option's natural width. A zero basis made every segment equal even when its
       label was not, so longer formatting choices wrapped while shorter neighbours held unused
       room. Growing from the content basis still fills the supplied row, but divides it according
       to what each option actually needs. */
    flex: 1 1 auto;
    min-width: 0;
  }

  fieldset.fluid .segment-label {
    min-width: 0;
    padding-inline: var(--space-2);
    width: 100%;
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
    /* HOW FAR THIS OPTION'S FILL RUNS ON UNDER THE THUMB, per side. Zero unless the
       thumb is that neighbour - see the two rules under the fill. */
    --bleed-start: 0px;
    --bleed-end: 0px;

    cursor: pointer;
    display: flex;
    height: 100%;
    position: relative;
    /* A segment is a control, and a double click on one is a second choice rather than
       a request to select its word. */
    user-select: none;
  }

  /* THE FILL IS ONE BOX, AND IT IS THE WHOLE PRESSED SURFACE.
     ---------------------------------------------------------
     The thumb is rounded on all four and drawn over everything, so its curve leaves a
     wedge outside the curve and inside its own box which belongs to neither option.
     The fill closes it by running on UNDER the thumb by exactly one radius: the thumb
     is opaque, so all that survives of the bleed is the wedge the curve left.

     That is not just the simplest way to cover the wedge - it is the only way the
     PRESS can cover it. A press is an inset shadow, and an inset shadow is drawn
     against the box it is set on: cut the wedge as a second element and the crease is
     two creases that meet at a seam and stop at the wedge's far end, which is the
     colour arriving at the corner without the depth. Painting the crease as a gradient
     instead makes it seamless and costs the two side rims, because a gradient runs on
     one axis. One box takes a real `box-shadow: inset` and the crease goes round all
     four sides of the whole silhouette, corners included, with nothing to align.

     The one ground this cannot serve is the night sky's, where the thumb is a veil
     rather than a fill and a bleed would show straight through it; `--seg-bleed` is
     zero there and the fill stops square. */
  label::before {
    background: var(--fill-tint, var(--seg-hover));
    /* Rounded, like the thumb: a fill floats inside the track too, and only one is ever
       shown at a time, so there is no neighbouring fill to leave a seam against. The
       squaring below is owed to the THUMB alone. */
    border-radius: var(--seg-inner-radius);
    content: '';
    inset-block: 0;
    inset-inline-end: calc(var(--bleed-end) * -1);
    inset-inline-start: calc(var(--bleed-start) * -1);
    opacity: 0;
    pointer-events: none;
    position: absolute;
    transition: opacity var(--duration-press) var(--ease-standard);
    z-index: 1;
  }

  /* Toward any neighbour that DRAWS - the thumb, or an option carrying a boundary of
     its own. Both are rounded boxes painted over this fill, so both leave the same
     wedge and both cover the same bleed: a bounded option's ground is on its label at
     `z-index: 3` and this fill is at `1`, so all that survives underneath it is the
     corner its curve gave up. */
  label:not(:has(input:checked)):is(
      label:has(input:checked) + label,
      label.outlined + label,
      label.previewed + label
    )::before {
    --bleed-start: var(--seg-bleed);
  }

  label:not(:has(input:checked)):is(
      label:has(+ label input:checked),
      label:has(+ label.outlined),
      label:has(+ label.previewed)
    )::before {
    --bleed-end: var(--seg-bleed);
  }

  /* A fill squares the side that faces a neighbour that DRAWS - the selection, or an
     option carrying a boundary of its own - and keeps its curve where it faces the
     track's end or a neighbour drawing nothing. Rounded against something drawn, it
     would leave a crescent of bare track at the join a reader is looking straight at;
     rounded against nothing, there is no join to be flush with.

     Never the SELECTED option's own fill, which is what `:not(:has(input:checked))`
     holds off: that fill lies under the thumb and the thumb covers it exactly.

     Logical corners throughout, so the same rule holds in RTL. */
  label:not(:has(input:checked)):is(
      label:has(input:checked) + label,
      label.outlined + label,
      label.previewed + label
    )::before {
    border-end-start-radius: 0;
    border-start-start-radius: 0;
  }

  label:not(:has(input:checked)):is(
      label:has(+ label input:checked),
      label:has(+ label.outlined),
      label:has(+ label.previewed)
    )::before {
    border-end-end-radius: 0;
    border-start-end-radius: 0;
  }

  /* A BOUNDED OPTION KEEPS ALL FOUR CORNERS AND WRAPS NOTHING. It is a drawn box, and
     a drawn box beside the thumb is two objects that each have their own outline - so
     it neither squares its ground against the thumb's curve nor runs a wedge into it.
     It used to do both, and each fix needed the next: square the cell, then wedge the
     notch that squaring opened, then repaint the wedge's material so it matched the
     cell, and the cell's own ground still ended up outside its own boundary once the
     thumb moved. It stays a neighbour that DRAWS, so the plain fill beside it still
     squares and wedges against it - that join is between a fill and a box, and only
     one of the two has an outline to keep. */

  /* THE SELECTED OPTION DRAWS NO STATE FILL OF ITS OWN. Its ground is the thumb, and
     the thumb carries its states; a fill underneath one is a second answer to the same
     pointer. It stayed invisible for as long as the thumb covered it exactly - and then
     the press began shrinking the thumb, which uncovered a band of hover fill down both
     sides of it and read as the pane leaking. */
  label:not(:has(input:checked)):hover:not(:has(input:disabled))::before {
    opacity: 1;
  }

  /* The press goes one step further down the same ramp, so hover and press are the same gesture at
     two depths rather than two different ideas - and the option is pressed INTO the track, which
     is the crease. The crease is a real inset shadow on the fill's own box, and that box IS the
     whole pressed surface, bleed included, so it runs round all four sides and through the wedge
     the thumb leaves rather than stopping at a seam. */
  label:not(:has(input:checked)):active:not(:has(input:disabled))::before {
    --fill-tint: var(--seg-pressed);

    /* THE SIDE RIMS ARE DISPLACED BY THE BLEED. An inset shadow is drawn against the
       box, and on a side that bleeds the box's edge is one radius further out, hidden
       under the neighbour - so the rim was drawn where nobody could see it and the
       edge a reader does see had none. Offsetting each side term by its own bleed puts
       it back on the visible edge. One bleed and not two: at twice the offset the
       band's full depth lands on the edge and the option reads as walled in rather
       than sunk, which is a heavier gesture than the one the top rim is making. Two
       terms rather than one symmetric one, because the two sides can bleed by
       different amounts; each collapses to a plain rim where its side does not bleed.

       The head and foot need no such thing: they run the full width of the box, so
       they cross the bleed and carry on through the corner the neighbour's curve gave
       up - which is the whole reason this surface is one box. */
    box-shadow:
      inset 0 2px 4px var(--seg-well-head),
      inset var(--bleed-start) 0 3px var(--seg-well-rim),
      inset calc(var(--bleed-end) * -1) 0 3px var(--seg-well-rim),
      inset 0 -1px 0 var(--seg-well-foot);
    opacity: 1;
  }

  input {
    height: 1px;
    opacity: 0;
    position: absolute;
    width: 1px;
  }

  .segment-label {
    align-items: center;
    /* The transparent border is load-bearing, and not for width: `.outlined` and
       `.previewed` draw their boundary by colouring THIS border, so a label with no
       border box has nothing to draw one on. Declared at rest so taking the state
       moves no pixel. */
    border: 1px solid transparent;
    border-radius: var(--seg-inner-radius);
    color: var(--seg-muted);
    display: flex;
    font-size: var(--font-size-compact);
    font-weight: 600;
    gap: var(--space-1);
    height: 100%;
    justify-content: center;
    line-height: var(--leading-flat);
    padding: 0 0.75rem;
    position: relative;
    transition:
      color var(--duration-fast) var(--ease-standard),
      scale var(--duration-press) var(--ease-standard);
    /* A segment's word does not wrap. Left to itself a long option broke into two
       lines inside a 28px strip - which the thumb then slid under at the wrong width -
       and the track's own overflow is what handles a control too wide for its row. */
    white-space: nowrap;
    z-index: 3;
  }

  .segment-detail {
    color: var(--text-muted);
    font-weight: 500;
  }

  .segment-detail.detail-on {
    color: var(--success);
    font-weight: 600;
  }

  /* On the thumb a detail cannot keep its own tone, whichever tone that is: a success
     green on a saturated accent is a word nobody can read, and this option can be the
     selected one - inheriting is a value the control rests on. It takes the thumb's
     ink, held back so it still reads as the qualifier rather than the name. */
  input:checked ~ .segment-label .segment-detail {
    color: color-mix(in srgb, var(--selected-text) 78%, transparent);
    font-weight: 500;
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
    /* A VEIL, not a surface. An opaque chip sitting on the track kept its own colour
       while everything around it answered the pointer, so a hovered option had a spot
       in it that had not moved. Both layers are the control's own ink over
       transparent, which means the badge rides whatever wash the segment is wearing
       and steps with it - and follows the sidebar and night skins for free. */
    background: color-mix(in srgb, var(--seg-text) 8%, transparent);
    border: 1px solid color-mix(in srgb, var(--seg-text) 12%, transparent);
    border-radius: 0.25rem;
    color: var(--text-muted);
    display: inline-grid;
    font: 700 0.5625rem / var(--leading-flat) var(--mono);
    font-variant-numeric: tabular-nums;
    height: 0.875rem;
    justify-content: center;
    min-width: 0.875rem;
    padding: 0 0.1875rem;
    place-items: center;
  }

  input:checked ~ .segment-label .segment-badge {
    background: var(--selected-tint);
    border-color: color-mix(in srgb, var(--selected-tint-ink) 20%, var(--selected-tint));
    color: var(--selected-tint-ink);
  }

  input:checked ~ .segment-label {
    color: var(--selected-text);
    font-weight: 700;
  }

  /* `:active` as well as `:hover`, because a keyboard press lands on the pressed
     surface with no hover to have promoted the ink first. */
  label:hover input:not(:checked):not(:disabled) ~ .segment-label,
  label:active input:not(:checked):not(:disabled) ~ .segment-label {
    color: var(--seg-text);
  }

  /* THE PRESS, on the word: the ink alone. The rule above already darkens it, which is
     the whole of an unselected option's press - it has no ground of its own to repaint
     and no line of its own to crease. It used to recede on a scale as well; that scale
     is gone from the product, and on a word it was the one part of a press that reads
     as the type resizing rather than as the option being held. */

  .selection-indicator {
    /* Hidden until something is selected - see the rule below the transition. */
    display: none;

    background: var(--selected-bg);
    /* Rounded on all four, always. The thumb used to square its inner corners and round
       only the pair facing the track's end, which reads right for a thumb that reaches
       that end and wrong everywhere else: at any option but the first or last it was
       square on both sides, and on a two-option control it sat mid-track with one
       squared edge. Inset by the border and the track's padding, so the curve sits
       concentric inside the track's own. */
    border-radius: var(--seg-inner-radius);
    /* One pane of glass: a drop shadow to seat it on the track, and a single top
       reflection inside it. It needs no ring - a saturated accent on a frosted track
       is the loudest thing in the control by fill alone, which the near-white thumb
       it replaced never was. */
    backdrop-filter: blur(6px);
    box-shadow: var(--seg-thumb-shadow);
    bottom: var(--seg-gutter);
    left: var(--segment-left, var(--seg-gutter));
    pointer-events: none;
    position: absolute;
    top: var(--seg-gutter);
    transition:
      left var(--duration-normal) var(--ease-overshoot),
      width var(--duration-normal) var(--ease-overshoot),
      background-color var(--duration-fast) var(--ease-standard),
      box-shadow var(--duration-fast) var(--ease-standard),
      scale var(--duration-press) var(--ease-standard);
    width: var(--segment-width, 0);
    z-index: 2;
  }

  fieldset:not(.selection-ready) .selection-indicator {
    transition: none;
  }

  /* A box that crosses a distance gives along the way: it stretches on the axis it is
     travelling and squeezes across it, then comes back to square. Applied as an
     animation rather than folded into the transition, because a transition only knows
     where the box started and where it lands - the give happens in between.

     Only on a move. A first placement has nowhere to have travelled from, which is
     the same reason the transition itself waits for `selection-ready`. */
  .selection-indicator.is-travelling {
    animation: segment-travel var(--duration-normal) var(--ease-standard);
  }

  @keyframes segment-travel {
    0% {
      transform: scaleX(1) scaleY(1);
    }

    45% {
      transform: scaleX(var(--squash-along)) scaleY(var(--squash-across));
    }

    100% {
      transform: scaleX(1) scaleY(1);
    }
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

  /* THE THUMB ANSWERS THE POINTER ON ITS OWN RAMP - the palette's accent hover, then
     its accent press. An ink mixed into the fill cannot do that: the thumb is a
     saturated accent, and the palette is the only thing that knows which way its ramp
     runs on a dark ground, where a fill must brighten rather than darken.

     On the thumb ALONE. Nothing else moves while the pointer is here: the fills either
     side keep their resting state, which is what stopped pointing at the selection
     from appearing to light up its neighbour. */
  fieldset:has(label:hover input:checked:not(:disabled)) .selection-indicator {
    background: var(--selected-hover);
  }

  /* And the press is one more step along the same ramp, with the well beneath it. */
  fieldset:has(label:active input:checked:not(:disabled)) .selection-indicator {
    /* THE ONE PRESSED SURFACE THAT MAY NOT SINK. Everything else in the product moves a
       pixel down; this thumb is set INSIDE a shared track, so a pixel down reads as the
       option sliding within its own well rather than as a key going in. So it takes the
       other two thirds of the press and not the pixel: the ground darkens and the throw
       settles into a crease. It used to recede on a scale, which is the part of a press
       the product no longer has anywhere. */
    background: var(--selected-pressed);
    box-shadow: var(--pressed-inset), var(--seg-thumb-shadow-pressed);
  }

  /* The value the control falls back to, drawn as a boundary rather than a fill: it is where this
     lands without a choice, so it must not read as one already made. */
  /* The boundary is drawn in the option's OWN ink, whichever ink that is: this option
     can be the selected one - inheriting is a choice the control can be resting on -
     and then it stands on the thumb, where the page's secondary ink is a dark word on
     a saturated fill. The ink comes from the selected rule, so only the unselected
     case names a colour here and the dashed border follows `currentcolor` either way. */
  label.outlined .segment-label {
    border-color: color-mix(in srgb, currentcolor 55%, transparent);
    border-style: dashed;
    font-weight: 650;
  }

  label.outlined:not(:has(input:checked)) .segment-label {
    color: var(--text-secondary);
  }

  /* A bounded option paints its own ground rather than letting the track show through.
     Its boundary says it is a box, and a box you can see through is not one.

     The GLASS, not a second coat of the track's veil: the track is already a veil over
     that glass, so painting the veil again here would darken this one option twice and
     the boundary would read as a shaded cell rather than a filled one.

     Not while it is the selected one. The thumb is drawn underneath `.segment-label`,
     so a ground painted here at that moment would cover the very thing it is standing
     on and the selection would go dark. */
  label.outlined:not(:has(input:checked)) .segment-label,
  label.previewed:not(:has(input:checked)) .segment-label {
    background: var(--seg-cell);
    transition: background-color var(--duration-press) var(--ease-standard);
  }

  /* And it answers a pointer here rather than through `::before`. The fill is `z-index: 1`
     and this ground is `3`, so once this one is opaque the hover underneath it cannot be
     seen - a bounded option would take the pointer and show nothing back.

     THE SAME VEIL AT THE SAME ALPHA, laid on this option's own glass and nothing else.
     A state is a STEP, not a destination: the veil is what the step is made of, so
     wearing the identical veil is what makes a bounded option answer the pointer as
     loudly as the plain option beside it - 1.2173:1 against the plain option's 1.2138,
     measured on the composite, where a stack that also re-laid the track's veil landed
     1.3367 and shouted. The two therefore do NOT arrive at the same colour, and should
     not: a filled cell and a bare track are different grounds by design, and only one
     of "same step" and "same destination" can be had. The step is the one a reader
     perceives. */
  label.outlined:not(:has(input:checked)):hover:not(:has(input:disabled)) .segment-label,
  label.previewed:not(:has(input:checked)):hover:not(:has(input:disabled)) .segment-label {
    background: linear-gradient(var(--seg-hover), var(--seg-hover)), var(--seg-cell);
  }

  label.outlined:not(:has(input:checked)):active:not(:has(input:disabled)) .segment-label,
  label.previewed:not(:has(input:checked)):active:not(:has(input:disabled)) .segment-label {
    background: linear-gradient(var(--seg-pressed), var(--seg-pressed)), var(--seg-cell);
    /* The same well, with both rims on the box's own edges: a bounded option is a
       drawn box that bleeds nowhere, so neither side needs displacing. */
    box-shadow:
      inset 0 2px 4px var(--seg-well-head),
      inset 0 0 3px var(--seg-well-rim),
      inset 0 -1px 0 var(--seg-well-foot);
  }

  /* The offer: this is what the control will look like once the move is taken. */
  label.previewed .segment-label {
    animation: segment-preview var(--rhythm-preview) var(--ease-inout) infinite;
    border-color: color-mix(in srgb, var(--brand-action) 70%, transparent);
    border-style: dashed;
  }

  /* Same reason as the outlined option above: an offer can land on the option already
     selected, and there it stands on the thumb. */
  label.previewed:not(:has(input:checked)) .segment-label {
    color: var(--text-primary);
  }

  /* The selection steps back whenever an offer is on the table - including the case where the
     offer is the option already selected, which is a real one: an override can happen to name the
     same value it inherits. */
  fieldset.previewing .selection-indicator,
  fieldset.previewing input:checked ~ .segment-label {
    opacity: 0.4;
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

  /* Drawn just INSIDE the option's own box, so the track's clip cannot cut it and it
     stays clear of the thumb's edge. */
  input:focus-visible ~ .segment-label {
    outline: var(--focus-ring-width) solid var(--focus);
    outline-offset: var(--focus-ring-inset);
  }

  /* On the solid thumb the ring flips to the thumb's own ink: a brand ring inset in a
     brand fill is not a ring. */
  input:checked:focus-visible ~ .segment-label {
    outline-color: var(--selected-text);
  }

  input:disabled ~ .segment-label,
  fieldset:disabled .selection-indicator {
    opacity: 0.45;
  }

  fieldset:disabled label {
    cursor: default;
  }

  @media (max-width: 36rem) {
    fieldset {
      box-sizing: border-box;
      inline-size: 100%;
      max-inline-size: 100%;
      overflow-x: auto;
      overflow-y: hidden;
      scrollbar-width: none;
    }

    fieldset::-webkit-scrollbar {
      display: none;
    }

    fieldset.navigation .segment-label {
      padding-inline: var(--space-2);
    }
  }
</style>
