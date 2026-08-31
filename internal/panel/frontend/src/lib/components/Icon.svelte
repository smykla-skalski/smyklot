<script module lang="ts">
  export type IconName =
    | 'admin'
    | 'alert'
    | 'ban'
    | 'book'
    | 'branch'
    | 'calendar'
    | 'chevron-down'
    | 'chevron-left'
    | 'chevron-right'
    | 'chevron-up'
    | 'chevrons-up-down'
    | 'circle-dashed'
    | 'circle-slash'
    | 'close'
    | 'database'
    | 'editor'
    | 'failure'
    | 'file'
    | 'file-bypassed'
    | 'file-invalid'
    | 'file-missing'
    | 'file-valid'
    | 'filter'
    | 'gauge'
    | 'gear'
    | 'github'
    | 'globe'
    | 'history'
    | 'undo'
    | 'info'
    | 'link'
    | 'link-off'
    | 'lock'
    | 'mail'
    | 'more'
    | 'moon'
    | 'minus-circle'
    | 'no-access'
    | 'notifications'
    | 'organization'
    | 'owner'
    | 'pending'
    | 'plan'
    | 'plus'
    | 'repositories'
    | 'refresh'
    | 'search'
    | 'server'
    | 'settings'
    | 'shield'
    | 'shield-slash'
    | 'sidebar-collapse'
    | 'sidebar-expand'
    | 'sign-out'
    | 'sliders'
    | 'sort'
    | 'check'
    | 'success'
    | 'sun'
    | 'sun-moon'
    | 'system'
    | 'tag'
    | 'trash'
    | 'user'
    | 'user-plus'
    | 'users'
    | 'viewer'
    | 'warning';

  /**
   * How far each glyph's outline sits inside its own 24-unit box, as [start, end] when the two
   * differ. Measured by rendering every icon and reading its geometry, not eyeballed.
   *
   * This is what makes a symbol beside a word look wrong: a button pads both edges equally, the word
   * puts ink almost immediately, and the symbol does not. `plus` keeps 5 units clear before its
   * stroke begins - at 13px that is 2.2px of dead space the eye reads as extra padding on that side
   * alone. The stroke is drawn centred on the outline, so half of it spills back out; that half is
   * subtracted at render time because stroke width is a prop.
   */
  const INK_BEARING: Record<IconName, number | [number, number]> = {
    admin: [4, 3],
    alert: 3.5,
    ban: 3.5,
    book: 4,
    branch: 5,
    calendar: 4,
    check: 4,
    'chevron-down': 6,
    'chevron-left': 9,
    'chevron-right': 9,
    'chevron-up': 6,
    'chevrons-up-down': 7,
    'circle-dashed': 3.5,
    'circle-slash': 3.5,
    close: 6,
    database: 4.5,
    editor: [4, 4.9],
    failure: 3.5,
    // The whole family shares one document outline, `M6 3h8l4 4v14H6z`; the mark inside it is
    // always narrower, so the outline is what sets the bearing.
    file: 6,
    'file-bypassed': 6,
    'file-invalid': 6,
    'file-missing': 6,
    'file-valid': 6,
    filter: 3.5,
    // The dial's arc is drawn the long way round, so it bulges past its own endpoints.
    gauge: 2.5,
    gear: 2.8,
    /* Octicons draw to the edge of the box where this set keeps a margin, so the
       mark is nearly full-bleed. Left as it is: another product's logo is
       reproduced or it is not used. */
    github: 0.5,
    globe: 3,
    history: [4, 3.48],
    undo: [4, 3.48],
    info: 3.5,
    link: 2,
    'link-off': 2,
    lock: 5,
    mail: 3,
    'minus-circle': 3.5,
    moon: [5.1, 4],
    more: 10.35,
    'no-access': 2.5,
    notifications: 3.5,
    organization: 4,
    owner: 5,
    pending: 3.5,
    plan: 5,
    plus: 5,
    refresh: [3.34, 3.35],
    repositories: 4,
    search: 4,
    server: 4,
    settings: 4,
    shield: 5,
    'shield-slash': 3.5,
    // One pane with a divider; only the chevron inside it turns, so both states measure the same.
    'sidebar-collapse': 3,
    'sidebar-expand': 3,
    'sign-out': [5, 6],
    sliders: 2,
    sort: 8,
    success: 3.5,
    sun: 2.5,
    'sun-moon': 2.5,
    system: 3,
    tag: [3.5, 2.46],
    trash: 4,
    user: 4.5,
    'user-plus': [2.5, 2],
    users: 3.5,
    viewer: 2.5,
    warning: 2.8,
  };

  /**
   * THE ICON SCALE, and the whole of it.
   *
   * Six drawn sizes, in px because an icon does not grow with the reader's font size -
   * the units law puts it on the px side. They are named rather than numbered because a
   * literal at the call site is a decision taken 204 times: the sheet held eight
   * different sizes, and 13 and 14 were one tier spelled two ways - a button glyph at
   * 13 beside a tile glyph at 14.
   *
   * A number is still allowed, and only for what is NOT an icon: an avatar, an empty
   * state's illustration, a mark. Those do not belong to this scale and pretending they
   * do would mean adding a tier per drawing.
   */
  export type IconSize = 'nano' | 'micro' | 'xs' | 'sm' | 'base' | 'md';
</script>

<script lang="ts">
  const {
    name,
    size = 'base',
    strokeWidth = 1.75,
    class: className,
  }: {
    name: IconName;
    /** A tier of the icon scale, or a number for something that is not an icon. */
    size?: IconSize | number;
    strokeWidth?: number;
    class?: string;
  } = $props();

  /**
   * The drawn size as a CSS length, so the scale is READ rather than copied.
   *
   * A tier resolves to its token; the escape hatch resolves to its own px. The svg is
   * then sized in CSS instead of through `width`/`height` attributes, which is what
   * lets the tokens be the single place the six numbers live - an attribute takes a
   * number and cannot take a `var()`.
   */
  const length = $derived(typeof size === 'number' ? `${size}px` : `var(--icon-${size})`);

  /**
   * The dead space before the ink starts, per edge, as a calc rather than a figure.
   *
   * It has to be arithmetic now: the size is a custom property, so its value is not a
   * number this component ever holds. The bearing is the same expression it always was
   * - the glyph's dead units, less half the stroke that overhangs them, as a fraction
   * of the 24-unit grid - just computed where the length is known.
   */
  const bearing = $derived.by(() => {
    const declared = INK_BEARING[name];
    const [start, end] = Array.isArray(declared) ? declared : [declared, declared];
    const ink = (units: number): string =>
      `max(0px, calc((${units} - ${strokeWidth} / 2) / 24 * ${length}))`;
    return { start: ink(start), end: ink(end) };
  });
</script>

<!--
@component
One drawn set, one stroke, and a closed list of names. Every glyph is built on the same
24-unit grid at the same 1.75 stroke, which is what stops two icons beside each other
disagreeing about weight - the thing a mixed icon library cannot promise and the reason
this is a union type rather than a string.

`size` names a tier of the icon scale - six drawn sizes, in px, because an icon does not
grow with the reader's font size. It used to be a bare number, on the reasoning that a
chip, a row, a heading and a rail tile do not agree on a step; what that produced was
eight sizes across 204 call sites, with 13 and 14 as one tier spelled two ways. A number
is still accepted and is for what is NOT an icon: an avatar, an illustration, a mark.

It is drawn, never announced. An icon repeats something the words already say, or it
stands inside a control that carries its own accessible name; either way a screen
reader reading it too is a repetition rather than a help. A glyph that would be the
only name for something means the control is missing its label.
-->

<svg
  class={className}
  data-icon={name}
  style:--icon-ink-start={bearing.start}
  style:--icon-ink-end={bearing.end}
  style:--icon-drawn={length}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width={strokeWidth}
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {#if name === 'settings'}
    <path d="M4 7h10M18 7h2M4 17h2M10 17h10M4 12h4M12 12h8" />
    <circle cx="16" cy="7" r="2" />
    <circle cx="8" cy="17" r="2" />
    <circle cx="10" cy="12" r="2" />
  {:else if name === 'repositories'}
    <!-- Shifted up 1 from the shape as drawn. The outline ran 5.5 to 20.5 - the
         tab at the top, then eleven units of body and a 2-unit corner at the
         bottom - so its middle sat at 13 in a box whose middle is 12, and the
         glyph rode a unit low of every label beside it. Same correction, and the
         same reason, as `refresh` below: these numbers centre the ink. -->
    <path d="M4 4.5h6l2 2h8v11a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z" />
    <path d="M8 12h8M8 15.5h5" />
  {:else if name === 'users'}
    <circle cx="9" cy="8" r="3" />
    <circle cx="17" cy="9" r="2.5" />
    <path d="M3.5 19c.5-3.2 2.4-5 5.5-5s5 1.8 5.5 5M14 14.5c3.5-.5 5.6 1 6.5 4.5" />
  {:else if name === 'undo'}
    <path d="M4 5v5h5" />
    <path d="M5.4 15.5A8 8 0 1 0 6 7.2L4 10" />
  {:else if name === 'history'}
    <path d="M4 5v5h5" />
    <path d="M5.4 15.5A8 8 0 1 0 6 7.2L4 10" />
    <path d="M12 8v4l2.75 1.75" />
  {:else if name === 'system'}
    <!-- Lifted half a unit: the screen and its stand ran 4 to 21, so the middle
         sat at 12.5 in a box whose middle is 12. Same correction, and the same
         reason, as `repositories` above. `tests/browser/icon-geometry.test.ts`
         measures the whole set now, so this stays true. -->
    <rect x="3" y="3.5" width="18" height="13" rx="2" />
    <path d="M8 20.5h8M12 16.5v4" />
  {:else if name === 'search'}
    <circle cx="11" cy="11" r="7" />
    <path d="m20 20-3.5-3.5" />
  {:else if name === 'filter'}
    <path d="M3.5 5h17l-6.7 7.3v5.4l-3.6 1.8v-7.2z" />
  {:else if name === 'github'}
    <!-- primer/octicons `mark-github-24`, verbatim: another product's mark is
         reproduced or it is not used, and an approximation of this one is
         recognisably wrong. It is a filled shape, so it carries its own fill and
         cancels the stroke this component sets - without that the svg's
         `fill="none" stroke="currentColor"` would draw it as an empty outline. -->
    <path
      fill="currentColor"
      stroke="none"
      d="M10.226 17.284c-2.965-.36-5.054-2.493-5.054-5.256 0-1.123.404-2.336 1.078-3.144-.292-.741-.247-2.314.09-2.965.898-.112 2.111.36 2.83 1.01.853-.269 1.752-.404 2.853-.404 1.1 0 1.999.135 2.807.382.696-.629 1.932-1.1 2.83-.988.315.606.36 2.179.067 2.942.72.854 1.101 2 1.101 3.167 0 2.763-2.089 4.852-5.098 5.234.763.494 1.28 1.572 1.28 2.807v2.336c0 .674.561 1.056 1.235.786 4.066-1.55 7.255-5.615 7.255-10.646C23.5 6.188 18.334 1 11.978 1 5.62 1 .5 6.188.5 12.545c0 4.986 3.167 9.12 7.435 10.669.606.225 1.19-.18 1.19-.786V20.63a2.9 2.9 0 0 1-1.078.224c-1.483 0-2.359-.808-2.987-2.313-.247-.607-.517-.966-1.034-1.033-.27-.023-.359-.135-.359-.27 0-.27.45-.471.898-.471.652 0 1.213.404 1.797 1.235.45.651.921.943 1.483.943.561 0 .92-.202 1.437-.719.382-.381.674-.718.944-.943"
    />
  {:else if name === 'globe'}
    <circle cx="12" cy="12" r="8.5" />
    <path
      d="M3 12h18M12 3c2.4 2.4 3.6 5.4 3.6 9S14.4 18.6 12 21M12 3C9.6 5.4 8.4 8.4 8.4 12s1.2 6.6 3.6 9"
    />
  {:else if name === 'organization'}
    <path d="M4 21V7l8-4 8 4v14M8 21v-4h8v4" />
    <path d="M8 9h1M11.5 9h1M15 9h1M8 13h1M11.5 13h1M15 13h1" />
  {:else if name === 'user'}
    <!-- Lifted half a unit, like `system`: head and shoulders ran 4 to 21. -->
    <circle cx="12" cy="7.5" r="4" />
    <path d="M4.5 20.5c.6-4.3 3.1-6.5 7.5-6.5s6.9 2.2 7.5 6.5" />
  {:else if name === 'branch'}
    <circle cx="7" cy="5" r="2" />
    <circle cx="17" cy="8" r="2" />
    <circle cx="7" cy="19" r="2" />
    <path d="M7 7v10M9 8h3a5 5 0 0 1 5 5v-3" />
  {:else if name === 'link'}
    <path d="M9 17H7A5 5 0 0 1 7 7h2" />
    <path d="M15 7h2a5 5 0 1 1 0 10h-2" />
    <path d="M8 12h8" />
  {:else if name === 'link-off'}
    <path d="m18.84 12.25 1.72-1.71a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
    <path d="m5.17 11.75-1.71 1.71a5 5 0 0 0 7.07 7.07l1.71-1.71" />
    <path d="M8 2v3M2 8h3M16 19v3M19 16h3" />
  {:else if name === 'lock'}
    <!-- Dropped half a unit: shackle and body ran 3 to 20. -->
    <rect x="5" y="10.5" width="14" height="10" rx="2" />
    <path d="M8 10.5V7.5a4 4 0 0 1 8 0v3" />
  {:else if name === 'user-plus'}
    <circle cx="9" cy="8" r="3.5" />
    <path d="M2.5 20c.7-3.2 3.3-5 6.5-5s5.8 1.8 6.5 5" />
    <path d="M19 7v6M16 10h6" />
  {:else if name === 'more'}
    <circle cx="12" cy="5" r="1.65" fill="currentColor" stroke="none" />
    <circle cx="12" cy="12" r="1.65" fill="currentColor" stroke="none" />
    <circle cx="12" cy="19" r="1.65" fill="currentColor" stroke="none" />
  {:else if name === 'trash'}
    <path d="M4 7h16M9 3h6l1 4H8zM6.5 7l.8 14h9.4l.8-14M10 11v6M14 11v6" />
  {:else if name === 'ban'}
    <circle cx="12" cy="12" r="8.5" />
    <path d="m6 6 12 12" />
  {:else if name === 'minus-circle'}
    <circle cx="12" cy="12" r="8.5" />
    <path d="M8 12h8" />
  {:else if name === 'circle-slash'}
    <circle cx="12" cy="12" r="8.5" />
    <path d="m6 6 12 12" />
  {:else if name === 'circle-dashed'}
    <!-- A ring that is drawn but not closed: the file that should be there and
         is not. Dash length is the mock's, not a guess - a coarser dash reads
         as a decorative border at 14px. -->
    <circle cx="12" cy="12" r="8.5" stroke-dasharray="3.4 3.4" />
  {:else if name === 'refresh'}
    <!-- One arc with a single arrowhead, the approved shape. The two-arrow
         cycle it replaced read as a pair of chevrons at 14px. Used by the sync
         button, the retryable failure mark, and the history retry row - one
         glyph, so all three say the same thing.
         Shifted -0.35,+0.8 from the shape as drawn: the arc plus its arrowhead
         are not symmetric about the viewBox, so as authored the ink sat 0.35
         right and 0.8 high of centre and the glyph carried a visible left
         margin inside its own box. These numbers centre the INK. -->
    <path d="M20.65 4.8v5h-5" />
    <path d="M20.15 13.8a8.5 8.5 0 1 1-2-7.5L20.65 9.8" />
  {:else if name === 'shield'}
    <path d="M12 3 19 6v5c0 4.7-2.8 8.2-7 10-4.2-1.8-7-5.3-7-10V6z" />
  {:else if name === 'sun-moon'}
    <path d="M12 8a2.8 2.8 0 0 0 4 4 4 4 0 1 1-4-4" />
    <path
      d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M18.7 5.3l-1.4 1.4M6.7 17.3l-1.4 1.4"
    />
  {:else if name === 'shield-slash'}
    <path
      d="M12 3 19 6v5c0 2.1-.6 4-1.7 5.6M14.8 19.3c-.8.7-1.8 1.2-2.8 1.7-4.2-1.8-7-5.3-7-10V6l3.1-1.3"
    />
    <path d="M3.5 3.5 20.5 20.5" />
  {:else if name === 'sign-out'}
    <path d="M10 5H5v14h5M14 8l4 4-4 4M8 12h10" />
  {:else if name === 'sun'}
    <circle cx="12" cy="12" r="3.5" />
    <path
      d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M18.7 5.3l-1.4 1.4M6.7 17.3l-1.4 1.4"
    />
  {:else if name === 'moon'}
    <!-- Dropped 0.55: the crescent's own extrema, not its endpoints, put the
         middle at 11.45. Only the two absolute coordinates move; the arc after
         them is relative and follows. -->
    <path d="M20 15.75A8 8 0 0 1 8.8 4.55a8.1 8.1 0 1 0 11.2 11.2Z" />
  {:else if name === 'check'}
    <!-- Dropped half a unit: the tick ran 6 to 17, so it rode half a unit high
         of every word beside it - and it is the mark on every selected menu row
         and every confirmation in the product. -->
    <path d="M20 6.5 9 17.5l-5-5" />
  {:else if name === 'success'}
    <circle cx="12" cy="12" r="8.5" />
    <path d="m8 12 2.5 2.5L16.5 9" />
  {:else if name === 'pending'}
    <circle cx="12" cy="12" r="8.5" />
    <path d="M12 7v5l3 2" />
  {:else if name === 'notifications'}
    <path d="M6 9a6 6 0 0 1 12 0c0 6 2.5 6.5 2.5 6.5h-17S6 15 6 9" />
    <path d="M9.5 19a2.8 2.8 0 0 0 5 0" />
  {:else if name === 'warning'}
    <!-- Dropped half a unit: the triangle ran 3 to 20. -->
    <path d="M12 3.5 2.8 20.5h18.4z" />
    <path d="M12 9.5v4M12 17.5h.01" />
  {:else if name === 'alert'}
    <circle cx="12" cy="12" r="8.5" />
    <path d="M12 8v4.5M12 15.8h.01" />
  {:else if name === 'sliders'}
    <path d="M5 21v-6M5 11V3M12 21v-9M12 8V3M19 21v-4M19 13V3" />
    <path d="M2 15h6M9 8h6M16 17h6" />
  {:else if name === 'gauge'}
    <!-- Dropped 0.36: the dial's arc is drawn the long way round, so its own
         extrema - not its endpoints - put the middle at 11.64. -->
    <path d="m12 14.86 3.6-3.6" />
    <path d="M4.34 19.56a9.5 9.5 0 1 1 15.32 0" />
  {:else if name === 'book'}
    <path d="M4 19.5V5.5A2.5 2.5 0 0 1 6.5 3H20v18H6.5A2.5 2.5 0 0 1 4 19.5Z" />
    <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
  {:else if name === 'tag'}
    <!-- Lifted 0.75: the label runs 3.5 to 21.5 as drawn, so its middle sat at
         12.75 in a box whose middle is 12. Only the two absolute coordinates
         move; everything after them is relative and follows. -->
    <path
      d="M12.6 2.75H5a1.5 1.5 0 0 0-1.5 1.5v7.6c0 .4.16.78.44 1.06l7.9 7.9a1.5 1.5 0 0 0 2.12 0l7.14-7.14a1.5 1.5 0 0 0 0-2.12l-7.9-7.9a1.5 1.5 0 0 0-1.06-.44Z"
    />
    <path d="M8.3 7.55h.01" />
  {:else if name === 'plan'}
    <!-- Lifted 1.25: three lines and the plus below them ran 6.5 to 20, so the
         middle sat at 13.25 in a box whose middle is 12. -->
    <path d="M5 5.25h14M5 10.75h14M5 16.25h7" />
    <path d="M16.5 13.75v5M14 16.25h5" />
  {:else if name === 'gear'}
    <circle cx="12" cy="12" r="3.2" />
    <path
      d="M12 2.8v3M12 18.2v3M2.8 12h3M18.2 12h3M5.5 5.5l2.1 2.1M16.4 16.4l2.1 2.1M18.5 5.5l-2.1 2.1M7.6 16.4l-2.1 2.1"
    />
  {:else if name === 'calendar'}
    <rect x="4" y="5.5" width="16" height="15" rx="2.5" />
    <path d="M4 10.5h16M8.5 3.5v4M15.5 3.5v4" />
  {:else if name === 'server'}
    <rect x="4" y="4.5" width="16" height="6.5" rx="2" />
    <rect x="4" y="13" width="16" height="6.5" rx="2" />
    <path d="M7.5 7.75h.01M7.5 16.25h.01" />
  {:else if name === 'database'}
    <ellipse cx="12" cy="5.5" rx="7.5" ry="2.9" />
    <path d="M4.5 5.5v13c0 1.6 3.36 2.9 7.5 2.9s7.5-1.3 7.5-2.9v-13" />
    <path d="M4.5 12c0 1.6 3.36 2.9 7.5 2.9s7.5-1.3 7.5-2.9" />
  {:else if name === 'mail'}
    <rect x="3" y="5" width="18" height="14" rx="2" />
    <path d="m3 7.5 9 6 9-6" />
  {:else if name === 'failure'}
    <circle cx="12" cy="12" r="8.5" />
    <path d="m9 9 6 6M15 9l-6 6" />
  {:else if name === 'info'}
    <circle cx="12" cy="12" r="8.5" />
    <path d="M12 11v5M12 7.8h.01" />
  {:else if name === 'sidebar-collapse' || name === 'sidebar-expand'}
    <!-- The pane and its divider stay put; only the chevron turns, so the
         button reads as one control in two states rather than two icons. -->
    <rect x="3" y="3" width="18" height="18" rx="2.5" />
    <path d="M9 3v18" />
    <path d={name === 'sidebar-collapse' ? 'm16.5 15-3-3 3-3' : 'm13.5 9 3 3-3 3'} />
  {:else if name === 'chevron-down'}
    <path d="m6 9 6 6 6-6" />
  {:else if name === 'chevron-left'}
    <path d="m15 6-6 6 6 6" />
  {:else if name === 'chevron-right'}
    <path d="m9 6 6 6-6 6" />
  {:else if name === 'chevron-up'}
    <path d="m6 15 6-6 6 6" />
  {:else if name === 'chevrons-up-down'}
    <path d="m7 15 5 5 5-5M7 9l5-5 5 5" />
  {:else if name === 'close'}
    <path d="m6 6 12 12M18 6 6 18" />
  {:else if name === 'plus'}
    <path d="M12 5v14M5 12h14" />
  {:else if name === 'sort'}
    <path d="m8 9 4-4 4 4M16 15l-4 4-4-4" />
  {:else if name === 'owner'}
    <path d="M12 3 19 6v5c0 4.7-2.8 8.2-7 10-4.2-1.8-7-5.3-7-10V6z" />
    <path d="m9 12 2 2 4-4" />
  {:else if name === 'admin'}
    <circle cx="8" cy="12" r="4" />
    <path d="M12 12h9M17 12v3M20 12v2" />
  {:else if name === 'editor'}
    <!-- Lifted 0.45: the pencil ran 4.35 to 20.55, tip to nib. -->
    <path d="m4 19.55 4.2-1 10.9-10.9-3.2-3.2L5 15.35zM14.8 5.55l3.2 3.2" />
  {:else if name === 'viewer'}
    <path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6" />
    <circle cx="12" cy="12" r="2.5" />
  {:else if name === 'no-access'}
    <path
      d="M3.5 3.5 20.5 20.5M9.6 6.4A10.8 10.8 0 0 1 12 6c6 0 9.5 6 9.5 6a16 16 0 0 1-2.2 2.9M6.3 7.3A17.2 17.2 0 0 0 2.5 12s3.5 6 9.5 6a10 10 0 0 0 3.2-.5"
    />
  {:else if name === 'file' || name.startsWith('file-')}
    <path d="M6 3h8l4 4v14H6zM14 3v5h4" />
    {#if name === 'file-valid'}
      <path d="m9 14 2 2 4-4" />
    {:else if name === 'file-invalid'}
      <path d="m10 12 4 4M14 12l-4 4" />
    {:else if name === 'file-missing'}
      <path d="M9 14h6" stroke-dasharray="2 2" />
    {:else if name === 'file-bypassed'}
      <path d="M10 12h4l-4 4h4" />
    {/if}
  {/if}
</svg>

<style>
  svg {
    /* Sized here rather than through `width`/`height`, which take a number and cannot
       take a `var()` - which is why the six sizes used to live at the call sites
       instead of in the scale. */
    block-size: var(--icon-drawn);
    display: block;
    flex: none;
    inline-size: var(--icon-drawn);
    overflow: visible;
  }
</style>
