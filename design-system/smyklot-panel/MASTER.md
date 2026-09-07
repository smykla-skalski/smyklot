# Smyklot panel design system

This file is the visual source of truth for the Smyklot administration panel.
Page-specific files under `pages/` may override it only when the exception is
explicitly documented.

## Direction

- Product: GitHub App administration and operations dashboard
- Style: trust and authority, minimal, data-dense, dark/light parity
- Density: 8/10
- Motion: 3/10
- Primary accent: restrained petrol, never a destructive state color
- Semantic colors: blue-grey information, teal success, amber warning, muted rose danger
- Icons: one Phosphor-style outline family with regular strokes
- UI typography: Plus Jakarta Sans
- Technical data: JetBrains Mono

Decorative gold, gradients, glows, glass surfaces, decorative motion, left-edge content
accents, and mixed icon families are not part of the panel system. The official
Smyklot avatar and the three-pixel closing rule above the app footer may retain the
brand rainbow. The closing rule moves its seamless, full-hue repeated spectrum
slowly to the right, with reduced-motion preferences disabling the movement.
Gold has one functional exception: unsaved state and the action that resolves it,
specified under Settings interaction laws

## Color tokens

### Foundations

| Role | Light | Dark |
| --- | --- | --- |
| Canvas | `#F5F7F6` | `#0E1116` |
| Sidebar | `#EEF2F0` | `#10151B` |
| Surface | `#FFFFFF` | `#151A21` |
| Elevated surface | `#FAFBFA` | `#1B222B` |
| Subdued surface | `#F0F3F2` | `#202833` |
| Selected surface | `#E7F5F2` | `#103634` |
| Divider | `#E3E8E6` | `#2A323D` |
| Control border | `#C4CEC9` | `#3A4653` |
| Primary text | `#18211F` | `#F3F6F7` |
| Secondary text | `#4C5B57` | `#C4CDD4` |
| Muted text | `#66756F` | `#95A1AB` |

The sidebar follows the selected theme while retaining stronger surface
separation than the workspace canvas.

### Brand and interaction

| Role | Value |
| --- | --- |
| Light primary | `#0F766E` |
| Light hover | `#115E59` |
| Light pressed | `#134E4A` |
| Dark primary and focus | `#2DD4BF` |
| Light selected text | `#0F625B` |
| Dark selected text | `#99F6E4` |

Official brand artwork may retain its rainbow. It does not create a second
control accent inside the application chrome.

### Semantic pairs

| Meaning | Light foreground/background | Dark foreground/background |
| --- | --- | --- |
| Information/default | `#526B7A` / `#EDF2F4` | `#B6C5D1` / `#26323B` |
| Success/on/active | `#0F766E` / `#E7F5F2` | `#71CABC` / `#173633` |
| Warning/expired/bypass | `#8A5D17` / `#F8F0DE` | `#E8C07A` / `#382F20` |
| Danger/off/banned | `#A33A45` / `#FAECEE` | `#F0A0A6` / `#3A2429` |
| Neutral/inherited | `#4C5B57` / `#F0F3F2` | `#C4CDD4` / `#202833` |

Use roughly 85% graphite neutral, 10% petrol interaction, and at most 5%
semantic color. Color never communicates state without a label or icon. Default
is neutral, On is muted teal, and Off is muted rose.

## Typography

| Role | Size / line-height | Weight |
| --- | --- | --- |
| Page title | `28px / 34px` | 700 |
| Section title | `20px / 28px` | 700 |
| Component title | `16px / 24px` | 600 |
| Body | `15px / 22px` | 400 |
| List primary | `14px / 20px` | 600 |
| List secondary | `13px / 18px` | 400 |
| Control label | `13px / 18px` | 600 |
| Technical data | `13px / 18px` | 400-500 |

Do not use monospace for headings, navigation, ordinary labels, buttons, or
long prose. Technical values use tabular figures.

## Spacing, shape, and elevation

- Spacing scale: `4, 8, 12, 16, 20, 24, 32px`
- Surface radius: `10px`
- Control radius: `8px`
- Chips and status badges may use a full pill radius
- Standalone controls: `34px` visual height, with an explicit `40px` tier
- Touch controls: at least `44px` hit area
- Object and setting rows grow around their content at every viewport width
- Cards use the shared 20px surface inset, reduced by the shared compact token on phones
- Sidebar: `240px` expanded, `72px` collapsed
- Shadows appear only on raised menus, inspectors, and dialogs

Cards group related settings. Rows within a group use shared dividers. Advanced
settings open in the shared inspector with their own regular cards, as specified
in the disclosure laws; do not nest decorative cards inside the originating card.

## Components

### Navigation

- Sidebar uses a light neutral surface in light mode and midnight in dark mode
- Selected navigation uses the shell's paired selection background and foreground
- Do not add a second rail, edge strip, or decorative marker
- The bottom account area stays visually quiet and does not add a custom role strip
- The collapse control sits beside the brand when expanded and straddles the
  sidebar edge when collapsed; it appears only on sidebar hover or focus-within
- Mobile uses a top bar and a navigation drawer preserving desktop page order
- Selection and `aria-current` identify the displayed page, never a remembered
  workspace page while Search or Inbox is open. Personal pages retain workspace
  context and every remembered page remains reachable

### Buttons and controls

- Primary actions use the shell accent with a theme-appropriate foreground;
  pending decisions use the explicit gold exception
- Secondary actions use neutral surfaces and borders
- Ghost actions use transparent backgrounds with visible hover states
- Danger is reserved for destructive workflows
- Focus replaces the normal control boundary with one two-pixel indicator
- Press feedback uses the shared color, inset shadow, and sink motion. Hover and
  pressed surfaces are exclusive, including rows with a separate hit layer
- A value picker and its remove action form one control group. They share the
  compact height, stay on the same line, and wrap together. Once the group wraps
  below its label, it can use the full row width; do not truncate a short value
  against an empty half-row. A fixed switch/action pair keeps the copy beside it
- Adding a managed option uses the shared anchored picker, without expanding the
  page. After selection, focus moves to the added option and the picker returns
  to its prompt. Only changes to the remaining option set mark its summary dirty

### Tabs and view switches

- Use text tabs with a two-pixel petrol underline for page-level navigation
- Use one compact segmented switch for mutually exclusive views within a page
- Users/Invitations and Audit/Failures use the shared segmented-control primitive
  so hover, press, focus, and sliding-selection motion stay identical
- Users/Invitations may add compact superscript count rectangles; ordinary
  segmented switches omit count badges

### Dialogs and inspectors

- Ordinary dialogs are `560-640px` wide
- Scrim uses 50-55% black plus restrained background blur
- Use clear header, scrollable body, and sticky action footer
- Preserve Escape, outside click, focus trap, and focus restoration
- Confirm only when dismissal would discard unsaved work. Closing an inspector
  whose draft remains staged needs no confirmation
- Repository settings use an addressable page with one vertical scroll. Secondary
  file controls use the shared inspector, with content adjustments before final output
- Rule parameters use the shared inspector. Done stages one complete rule; Cancel
  intentionally discards its private edits. Escape, outside dismissal and Close
  must protect changed or invalid private input with a discard confirmation
- Check and tool names are primary-text headings. App-pinned checks show the
  resolved app name and shared avatar; missing metadata keeps the exact identifier
  secondary and must not imply that the app itself is unavailable
- Editing supported fields preserves unknown rule fields, app pins and exact
  numeric literals. Returning a control to its opening value restores the original
  field presence, and removing then restoring an entry restores its original data
- Before staging a private rule, compare its current value and presence with the
  opening snapshot. A same-rule change elsewhere blocks Done and explains the
  conflict without discarding private input; unrelated rule changes do not block it
  The final precondition runs after refreshing shared draft storage, before any
  mutation adopts its revision. Rejected staging keeps the inspector input intact

Callout symbols align with the cap-to-baseline center of the first text line,
for either tone and regardless of wrapping. Subsequent lines and adjacent
actions must not move the symbol. Use shared icon metrics and Callout alignment,
never caller-specific offsets. Visual checks compare the visible symbol with
the first line of text, not the center of the complete paragraph

Callout copy follows the row copy rhythm in both tones: use `--row-copy-leading`
for wrapped lines and `--row-copy-gap` between a heading and its description.
Verify every rendered line gap, not only the gap between separate text elements

Balance Callout space above and below the text, from its first cap edge to its
last baseline. The icon aligns with the first line but does not determine the
card's vertical padding or add height to the text block

### Lists and toolbars

- Lists use semantic `ul`/`li` and the shared `.object-row` anatomy. Do not render
  tables, column headers, or fixed-height row tracks
- Keep the whole page scrolling. Rows grow when names, descriptions, or controls
  wrap; do not clip them to preserve a uniform height
- The toolbar contains search, filters, sorting through `TableToolsMenu`, and one
  primary action. Search and filters remain outside the list card
- Large lists use cursor-backed infinite loading. Do not add page-size pickers,
  numbered pagination, or a detached “Show all” footer
- Refreshing a filter or sort preserves current rows and marks the region busy
  until the latest response arrives. A failed refresh does not erase loaded data
- Incremental-load failures use a temporary recovery action at the list boundary
- Use neutral dividers without zebra striping. Navigable rows use the shared hover,
  press, and focus states; independent switches and menus retain their own hit areas
- Status uses semantic icon-plus-label badges. Roles use neutral outline icons and
  text. State must remain understandable without color
- More-actions controls use the shared borderless ghost button and stroked icon
- History uses activity rows. Actor identities follow the same name and handle
  treatment as Access
- Empty results use the shared empty-state anatomy: a plain title, a brief
  explanation, and a relevant recovery action. Do not add a decorative icon
- The sidebar is the only organization picker. Access scope chooses Global or the
  current workspace, and Add dialogs inherit that scope
- Repository names keep descender-safe line boxes and optical vertical alignment;
  first and last rows retain the shared outer inset
- Repository detail navigation uses plain semantic counts rather than count pills
- Menus are content-sized, retain search while options scroll, and omit type labels
  already stated by group headings. Right-edge menus open toward available space
- Light and dark themes share geometry. The account menu owns the persistent theme
  selector

The responsive-shell, object-row, and browser geometry checks enforce this
anatomy. New routes must use those same primitives and participate in the sweep

## Class or component

Two questions the catalogue kept re-asking. Both were decided by looking, so neither
needs asking again.

**`cap-trim`, `band-trim` and `band-trim-stack` are utilities and stay utilities.**
They appear on 58 elements across 17 files, and what they do is trim one box to its
cap and baseline. A component cannot do that: the box that needs trimming is usually
a caller's own `<span>` inside a flex row, and wrapping each one in a component would
add an element per trimmed word — which is a box the trim then has to be applied to
anyway. `.button-label` is owned by the shared Button component, which already
renders that box.

**A surface is a class; an anatomy is a component.** `.plate` in `app.css` is a
ground, a keyline, a corner and a lift — nothing else. `Plate.svelte` is that surface
*plus* a mandatory header and body. Six call sites wear `class="plate"` by hand and
that is correct rather than a bypass: they want the surface under their own contents
and have no header to give it. The same distinction applies to the shared `.card`
surface and components with a prescribed header and body. A caller that needs
only the surface uses the class.

## Icon vocabulary

Use SVG symbols from one Phosphor-style outline system.

| Purpose | Symbol |
| --- | --- |
| Settings | Gear or SlidersHorizontal |
| Repositories | GithubLogo or GitBranch |
| Users | Users |
| History | ClockCounterClockwise |
| Search | MagnifyingGlass |
| Filters | Funnel |
| Global / organization / personal | Globe / Buildings / UserCircle |
| Public / private | Globe / Lock |
| Add user | UserPlus |
| More actions | DotsThree |
| Remove | Trash |
| Success / pending / warning / failure | CheckCircle / Clock / Warning / XCircle |
| Information | Info |

Repository file state uses CheckCircle for valid, MinusCircle for missing,
XCircle for invalid, and ShieldSlash for bypassed. The shapes communicate the
state directly instead of forcing every state into a document outline.

Icon sizes are 16px inline, 18px in controls, 20px in navigation, and 24px in
empty states. Icon-only actions always have an accessible name and a hit target
larger than the visual glyph.

## Motion

- Hover and press: `150-180ms`
- Menus and tabs: `180-220ms`
- Dialogs and inspectors: `240-280ms`

Hover, press and release form one continuous change in depth. Animate the painted
color, inset shadow and one-pixel sink with the shared motion tokens. Do not swap
non-interpolating background images between states or remove a hover layer while
the pressed surface is still moving. Neutral buttons animate one registered tint;
icon buttons and segmented choices animate their actual painted surface. Reduced
motion disables travel and transitions while retaining visible state feedback

Browser evidence must include intermediate press and release frames, not only
settled hover and pressed screenshots. Verify the rendered theme before accepting
a screenshot as light or dark

- Exit is faster than entry
- Use transform and opacity for movement and entry/exit; control state feedback also
  interpolates its color and inset shadow
- Never animate more than two elements for one interaction
- Every effect has a `prefers-reduced-motion` fallback

## Settings interaction laws

### Floating action composer exception

A floating composer that requires a decision uses a 2px border in
`--decision-accent`, an alias of the existing gold `--warning` color used by unsaved
star badges. Its Save or resolve action uses the same accent
through the shared button. This is an explicit exception to neutral panel borders
and the shell's teal or violet action color. Reserve it for pending decisions,
including invalid drafts and save conflicts. Keep informational success toasts
neutral, dismiss them after five seconds, and animate their exit with reduced-motion
support. Never auto-dismiss unsaved changes or failures

This exception permits the composer's full border and action fill, not colored
left-edge strips on settings, status messages, or rows. Configured values keep the
same text alignment as inherited values. Pending edits use `--unsaved-surface`
with paired `--unsaved-ink` and `--unsaved-secondary` text, plus
`--unsaved-control` and `--unsaved-control-border` for inputs. The dark palette
uses a deep warm neutral with pale gold text; the light palette uses a pale gold
surface with dark ochre text. Never obtain the entire treatment by washing the
accent over a neutral surface. Keep normal text contrast at least 4.5:1

This pairing follows [Atlassian's accent guidance](https://atlassian.design/foundations/color/accents):
match foreground and background by color family and emphasis, and account for
yellow turning brown when darkened. Changed panel borders use `--unsaved-border`.
Never use the shell action color to signal unsaved work

A changed row clears its own bottom separator and the separator immediately
above it. Preserve their geometry to avoid layout shifts, but set their shared
`--hairline` color to transparent. When a disclosure or other content follows the
last row, the row list uses `rows-continue` to retain its shared separator. The
following content must not paint an independent top border. Local row styles use
the shared token. Panel outlines remain intact

Adjacent changed rows leave exactly 1px of the card surface between their fills.
Use the existing separator space as a transparent border and clip the fill to
the padding box. Do not add a colored divider or shift the rows when they change
Reduced-motion styles disable transitions rather than assigning a short universal
duration, which would introduce transitions on otherwise static elements

Mark only the setting that differs from its saved value. Compare structured
settings semantically, ignoring object key order and order in set-valued lists.
Included and excluded branches are separate settings, as is each bypass actor.
Restoring a saved value removes its marker even when other settings remain dirty

The save composer stays inside the content pane's available width, clear of the
navigation rail and sidebar in both expanded and collapsed states. Its actions
wrap when that pane is narrow, even if the viewport itself is wide
Navigation and pane overlays share the same animated sidebar width, so this
boundary holds throughout a fold or breakpoint change, not just after it settles

Collection Add actions belong at the top right of the section header. A bypass
list uses one shared Add an actor button there, including when the list is empty.
That button toggles its editor and exposes its expanded state. In an expanded
card, the inset below its final action matches the inset above the header action.
Use the shared `card-action-foot` cap-height track and button margins to match
`card-head`, keeping the heading aligned with neighboring cards

Continuing lists retain their row padding instead of subtracting it from the
following form, and suggestion rows use the same left and right frame as its fields
An icon-only Remove action uses the shared square icon button at the control's
height. Never give it a text button's horizontal padding

Suggestions offer only actors that can be added. Filter configured actors by
their type and ID, including actors added during the current edit. Removing an
actor makes it available again. When every search match is already configured,
explain that state instead of displaying disabled duplicates or claiming no match

Inline editor toggles preserve their header's viewport position. Focus moves
without an automatic browser scroll; reveal an offscreen field only as far as
needed, accounting for floating actions. Closing returns focus to the trigger
without jumping the page. Respect reduced motion and cancel obsolete reveal work
when the user closes or reopens the editor

Adding a managed option focuses its new control and reveals its whole row,
including the setting name. Measure visible row geometry, not an invisible native
input. Keep it clear of the fixed header and save composer without scrolling when
the row is already visible

### Row copy rhythm

The gap from a row title to its description and between wrapped description lines
must match. Use `--row-copy-gap` for the trimmed title-to-description gap and
`--row-copy-leading` for wrapped copy. Do not inherit body leading inside a compact
row. Check rendered multiline text at narrow widths as well as single-line rows

The floating composer follows the same 8px ink gap. Trim the title and subtitle
to their cap and baseline edges, and use the shared copy leading when they wrap.
Do not add body line-box space on top of the gap

Form errors use the same trimmed 8px wrapped-copy rhythm as form help. Error
messages must not inherit the wider body-prose line height

Independently wrapping status facts use spacing to separate their roles. Do not
attach generated punctuation that can remain at the end of a wrapped line or
after the last fact. Stacked facts follow the same trimmed 8px copy rhythm
Status bands reserve an 8px inset and the shared control radius in both saved and
changed states. Their text stays aligned with the page copy when the gold fill appears

### UI copy and compact choices

UI labels, descriptions, hints, and toast messages have no trailing period. A toast
description does not repeat its title. Segmented formatting pickers occupy their
content width without stretching across the containing panel. When there is less
room than their contents need, the shared track scrolls horizontally within its
container at every viewport width. Never clip an option or widen the page

Optional explanations open in place through a quiet native disclosure. Its text
shares the card heading's left edge, its trailing chevron stays inside the hit
area, and its row shares the adjacent settings rows' width. Keep an 8px gap from
the preceding row and use the shared hover, press, and focus states. Expanded
content stays on the card surface without another frame or a colored edge

Formatting sources appear in priority order, with the current editor identified
independently of saved or unsaved state. Do not claim a source is active or
inherited without data confirming that. Omit earlier empty sources when the
backend confirms they are empty, but retain unsaved removals and the current scope

### Shared form controls survive extraction

Form controls own their appearance through shared primitives or global control
classes. A parent component's scoped CSS must never supply a child's border,
background, typography, focus state, or height. Duration fields use `DurationInput`,
which combines the shared text input and Select at the same control height. Unit
changes preserve the stored duration. Browser checks compare rendered control
styles in both themes, including focus and disabled states, and run in CI

Route styles must not globally override shared component classes. Put contextual
layout on a local wrapper, or anchor a child layout selector to that wrapper.
Shared form errors have no margins of their own; the caller supplies the 8px
code-to-error gap. Loading another route must not change that spacing

Modal forms use the shared `form-stack`, `form-field`, `form-label`, and
`form-help` anatomy. Separate fields by 16px and a field's label, control, and
help by 8px. Labels use the shared control type size and 600 weight. Duration
groups stay intrinsic and start-aligned inside a field; the containing row owns
their placement. A modal's `form-row` centers its label and switch without adding
settings-list dividers or row padding

Repeated weekly-hours controls follow the same form rhythm without a second
frame. Their Add action stays beside the group heading; each removal uses the
shared square icon button. At narrow widths, keep removal beside its day and
the two time fields together

Plain-language textareas, including reasons and notes, use the shared prose
font and control type size. Monospace is reserved for code, identifiers, and
explicit technical grammars. A textarea's native browser font must not determine
its appearance. Browser checks compare the rendered font as well as its border,
height, and focus treatment

Boolean settings use `Switch`. Consent and multi-selection use the shared
checklist anatomy with a visible check box and associated label. Native checkbox
paint, local select paint, and page-wide overrides of shared control classes are
forbidden. Verify modal controls in both themes as well as controls in page rows

A setting whose only control is a switch keeps that switch to the right of its
copy at every supported width. The copy wraps beside it, and both areas remain
vertically centered. Do not move a single switch to a separate line as though it
were a wide field group; shared row geometry owns this behavior

Compact status badges and single row actions stay beside their explanation when
both fit. At narrow widths, allow the copy to use half the row before wrapping
the value below it. A longer action still moves as a whole when it cannot fit;
never force it into clipped text or reduce its control height

Standalone row inputs, selects, popover pickers, segmented controls and buttons use
`--control-height-compact` (34px), including fields inside expanded inspectors.
Popover pickers use the shared `PickerTrigger`, with the shared stroked chevron.
A surface may opt into the 40px tier through `--local-control-height`; local paint
or content padding must not invent an intermediate height. Embedded chip removal
controls and inheritance markers retain their explicit smaller anatomy

### Settings mean differences, not interaction history

Compare each draft with its saved baseline. Changing a value and returning it to
the same value restores the original state, removes its changed marker, and clears
the composer when nothing else differs. Preserve intentional inheritance choices:
an explicit override is distinct from following a default. Compare structured JSON,
unordered exception actors, and branch patterns by their meaning while retaining
the saved representation when it is restored. Invalid input remains visible and
blocks saving; it must not silently become zero or disappear

The saved baseline survives closing an editor and reloading the page. Keep it
separate from the draft present when the editor opens. Restoring saved content
also restores its explicit overrides, including a value equal to the shared
template. Deliberately removing an override restores inheritance and must remain
removed when the editor reopens

Editing one value preserves every untouched authored override, even when it equals
the shared template. Explicit removals and list choices are part of the editor's
Undo and Redo history. Returning a list choice to its saved value restores its
original rule order and options. Compare numeric literals without floating-point
rounding, and never replace a visible edit with an older rounded value when saving

List-combination decisions belong to one shared card with ordinary policy rows.
Each row names the list path, explains the selected behavior, and uses the shared
intrinsic-width 34px segmented control. Unavailable alternatives stay visible and
disabled, with native keyboard navigation skipping them. Explain the content
constraint once at the group level. Do not use oversized radio cards, a second
frame, or a decorative side strip. Highlight only changed rows and their containing
card; restoring the saved choice or undoing it clears that state. The adjustment
summary must name lists as well as scalar keys

Load the complete saved and draft settings before starting editable history. A
loading preview must not become an undoable user edit

Durations offer seconds, minutes, and hours through `DurationInput`. Changing only
the displayed unit never stages a settings change or rounds the stored duration

### File editing and rare options

Use the shared code editor for templates and repository adjustments. There is one
code surface: content adjustments are the default, final output follows them, and
change highlighting appears in that surface. Do not introduce parallel source,
output, and diff blocks. The mode picker and adjacent action buttons have equal
heights; read-only status must not create another toolbar row. Center header actions
on the heading and use the shared 16px heading-to-editor gap. Associated help text
belongs to the editor with an 8px gap, not the 16px gap between independent fields

A read-only presentation of the current adjustment keeps a visible action back to
content, even when Undo removes every highlighted change. Label that presentation
as adjustment settings, not saved settings: it may contain an unsaved draft. Keep
the editor mounted across presentation changes so selection and Undo survive

Formatting is secondary to editing content. Keep it in the inspector or a closed
disclosure until requested. Every template and rendered file has its terminal
newline, including preserved-format output. The editor hides that terminal blank
line, excludes it from change counts, and never offers it as a formatting choice

### Rows, pairs, and compact actions

Section headings outrank their contents. A file editor nested in a settings card
uses the shared field-heading tier, not another card-title-sized heading. Field
labels, nested editor labels, and the controls below the editor follow one hierarchy

At widths that fit both sides, setting labels and explanations occupy the left and
their values or actions occupy the right. Do not force Add onto a separate row with
`flex-basis: 100%`. A collection-wide Add action belongs beside its collection heading;
nested list actions use shared settings rows. Wrap only when content needs the room

Row actions form one group: Edit and its adjacent removal control never wrap
apart. Summaries may wrap independently; when they need a separate band, the
action group stays at the right edge. A simple rule and its square off control
remain on one row, including on phones. Long names wrap within their summary
without covering actions or hiding any of the name

Inline disclosures use shared `fold-inline` geometry, with a trailing chevron,
aligned text, and the same width as neighboring rows. Use them for brief optional
explanations, not an expanding settings form beneath an active code editor

The card owns its 20px outer inset. A final nested editor sheds inter-editor padding
and the terminal row's internal half-band so neither stacks onto that inset. This
applies to collapsed disclosures, expanded controls, and Markdown section actions

A file path field fits its content within a readable bound; it does not stretch to
fill a wide editor. Keep file actions together at the trailing edge, centered with
the input. Let the field and action group wrap when they cannot fit. Undo belongs
to the code editor, separately from file-level actions

Rare controls that change how code is applied open through a named action in the
file header. Use the shared inspector, not an accordion that pushes the editor
or the following page content. Opening and closing it must preserve page geometry,
scroll position, selection, and Undo. Keep the code editor mounted while inspecting
rules. The trigger stays in the same place and receives focus when the inspector
closes. At narrow widths the inspector fills the viewport and scrolls independently

The inspector identifies the file and groups editable settings in regular cards,
using shared policy rows, controls, surface colors, and spacing. No nested settings
disclosures, detached headers, or tinted inset panels. Closing returns to editing;
it never saves, discards, or clears an invalid draft. Only the workspace save action
commits changes. Read-only readers may inspect the same rules with disabled inputs

Content and application rules remain separate. Do not introduce hidden keys inside
an override object or make users escape file content inside a configuration envelope.
A code-only alternative must have an explicit schema and preserve invalid raw drafts
before it can replace the form. Offer only formats the backend can apply, through the
shared file-capability mapping

When an inspector cannot edit a file's native adjustment, its persistent footer
offers **Edit adjustments** in the existing repository editor. Keep **Read only**
inline with the view controls; do not add another toolbar or helper row above the
code. Read-only users receive an inspection action instead. The handoff identifies
and reveals the requested file, preserves unsaved drafts, and focuses it once
without stealing focus on later edits or renames

The shared draft lifetime owns file validation, including restored drafts whose
editor has not opened. Leaving before a debounce or during a request must not
leave Save pending forever or let invalid content save. Editors invalidate their
own preview generations on leave; validation continues for the current draft.
Discard, replacement, and account changes reject late results. A successful retry
for the same current input clears a transient request error. Typed formatting
metadata must not suppress checks for that file or any other file, and file
content retains exact numeric values through validation

A validation message in the global composer links to the file that caused it,
including a repository adjustment. The message and destination share the same
validation owner; never substitute the oldest unrelated draft. Navigation and
reload retain that destination and its unsaved text without enabling Save

Returning to the same settings preserves raw invalid text, selection, and Undo;
object key order alone is not a settings change. Breadcrumbs and sidebar links
must leave a file detail for the shared-file list within the same session

Native file fixtures cover TOML, YAML, JSONC, and Markdown through the authoritative
renderer, including Markdown sections and replacements. At narrow widths, long
mode labels use the shared picker and removal actions remain square. Inputs stay
inside their field tracks; code-to-action spacing comes from the shared row

Optional settings outside an active editor may use a whole-card disclosure whose
summary is its header. Explanations and toolbars that only choose a view do not
require another card. Editable controls must never appear in a frameless inline
explanation row

Repository configuration separates editable policy from observed file state.
Bypassing a file never changes its reported validity. A file that has not been
checked says **Not checked**, and a valid empty file remains **Valid**. Inspecting
it opens the shared inspector without moving the repository settings

File priority is a semantic ordered list with a quiet, intrinsic number column
starting at the row's left text edge. One 8px gap separates the numbers and paths;
never reserve extra empty character widths. Keep a shared path column and an
observed status centered beside that path. Use regular row spacing and separators,
without tinted inset panels. Only confirmed files get
**Selected** or **Ignored** labels, using shared filled info and neutral pills
respectively. An unlabelled candidate makes no claim about
whether that path exists. Never apply `display: contents` to these list rows

Single-line facts beside setting labels, including **Last checked**, use the
shared `setting-fact` typography and cap trim. Center both visible text boxes on
the same row axis, rather than centering a trimmed label against an untrimmed
body line or introducing local pixel offsets

Navigable repository and shared-configuration rows expose a full-row link with
shared hover, pressed, and keyboard-focus states. A switch and its surrounding hit
area form a separate control; toggling it must not navigate. List expansion belongs
with the list heading and count, not a detached oversized footer action

Inherited behavior choices use the shared Button with its icon and label slots.
Expanding an override picker must retain the same control height, text alignment,
focus and press states as its collapsed action. Never rely on a style class owned
by another component to paint these choices. Check the expanded state in every
configuration scope as well as the collapsed state

A navigable row with a small switch or status badge and a direction mark keeps
that compact group beside its text on phones. Use the shared compact object-side
layout; reserve stacked actions for groups that need more room. The primary filter
and its tools button wrap together, with the search filling its own line when the
toolbar is narrow. Never strand the tools button on a line by itself

Alias pairs use the shared pair editor. Clicking or typing in an existing command
field opens its suggestions. Suggestions use the compact control tier, with arrow
keys, Enter, and Escape supported. Reset stays vertically centered beside the whole
value area, including when pairs wrap. Removal and picker affordances remain distinct

### Sync, exceptions, and truthful status

Ordinary sync proceeds automatically after saved configuration changes. Surface
blockers in context with a reason and recovery action; a separate plan approval is
not the normal workflow. Do not describe queued work as completed or stale data as
current. Monitoring copy describes measured facts without claiming a cause that
the data does not establish

A healthy status badge does not need a second panel repeating that it is ready.
Show explanatory status text when it tells the reader about a restriction, work in
progress, or a recovery step. Development-fixture wording never appears in product
copy; demo data uses the same vocabulary as real service responses

Configuration-file sync separates draft policy from saved connection status.
Toggling its switch only changes the draft; show when it starts or stops after
Save. An old observation cannot describe newly saved settings as in sync. Keep its
check time as context, and update relative time from the page's shared clock

Workspace and repository connections keep distinct identities and cache entries.
The workspace file lives in the workspace's `.github` repository and does not
replace that repository's own configuration file. Status queries use a dedicated
cache family, refreshed by saved settings and service events, so repository-detail
updates cannot treat a status response as repository data

Use the same status rows for disabled, waiting, syncing, proposed, blocked,
unavailable, and failed reads. Keep a real outstanding pull request visible with a
verified repository link. A pending operation is never described as a completed
file update, and a read failure offers a shared retry without changing policy

Configuration-file review starts with a fresh comparison scoped to the current
account, workspace, repository and authority. Unsaved work in that workspace blocks
comparison and resolution, including drafts found in storage before a cross-tab
event arrives. Closing, leaving or changing the owner invalidates late responses

The inspector defaults to **Conflicts**, a single unified comparison with both
alternatives visible before any decision. Use shared line and word highlighting,
explicit minus/plus source labels and accurate source line numbers. Fold unchanged
context; its shared Show/Hide control stays mounted to retain focus and the scroll
anchor. A changed comparison resets revealed context. **Full result** shows the
complete selected outcome in the same code slot. Switching views never applies a
choice or changes which side is kept

The intrinsic **Keep values from** picker starts without a selection. It decides
only overlapping values; independent changes on both sides remain in the result.
Keep JSON/Read only metadata inline with the view control. Preserve exact numbers
and the terminal newline while hiding its empty final display line

A deleted file shows its sole available recreation result immediately; **Recreate
file** is an explicit action. Invalid syntax, schema, scope, access or an outstanding
pull request cannot produce fictional choices. Link to an outstanding proposal so
the reader can review it on GitHub

Submit only the current backend token and explicit side. Accepted work remains
pending. A stale comparison clears the old choice and requires a fresh explicit
action. Expired authority or a blocked resolution cannot automatically repeat a write

A card header keeps its action and Read only metadata in one aligned group; the
metadata must not become an orphaned row

Merge exceptions inherit workspace defaults and allow repository overrides. Show
GitHub actor names and available avatars, with suggestions and installation status.
An unavailable app must not be silently removed from policy. Distinguish missing,
suspended, and unknown installation state. Explain the resulting restriction in
context and keep the configured choice available to fix

When a GitHub actor name cannot be resolved, keep a plain unavailable-name primary
label and show its type and exact GitHub ID as secondary recovery context. Include
this context in mode and Remove control names so unresolved actors remain distinct
to assistive technology. IDs never replace resolved names or avatars. Offer a
shared quiet name-lookup retry without changing configured permissions

Actor IDs are identities, not quantities. Preserve every digit of valid signed
64-bit IDs through lookup, row matching, draft persistence, saves, history, and
reload. Large IDs remain numeric JSON literals; never round them through a
JavaScript Number or turn them into JSON strings. Builtin role recognition and
duplicate checks use the same canonical identity comparison

Actor rows choose their layout from the card's available width. In narrow cards,
place every permission and Remove group below its identity with the shared row
gap. A permission's wording must not decide which rows wrap. Keep controls at
their intrinsic width and leave enough room to read exact identity references

### Empty states and time labels

The shared empty/error surface puts its title above its explanation with the same
8px visible copy gap as settings rows. No trailing period or joining separator is
needed after the title. Use plain wording, retain useful content during recoverable
errors, and offer a relevant next step rather than an unsupported reassurance

Day headings and the entries grouped beneath them use the same local calendar.
Never split one local day into repeated Today or Yesterday sections because its
entries cross midnight in UTC

## Enforcement

These laws apply to every route and every expanded, empty, loading, error, edited,
disabled, and read-only state. A component extraction must preserve its shared
states without depending on a caller's scoped CSS

| Contract | Automated enforcement |
| --- | --- |
| Route coverage, copy rhythm, separators, shared link paint, no left strips | `tests/browser/law-audit.test.ts` and shared `PANEL_ROUTES` |
| Callout first-line icons, wrapped copy rhythm, text-only vertical balance | `tests/browser/callout-alignment.test.ts` and operator dialogs in `duration-controls` |
| Optical centering, descenders, heights, mobile containment | `vertical-alignment`, `text-clipping`, `control-heights`, `mobile-layout` browser suites |
| Duration styles and units, explanation alignment, gold state geometry | `duration-controls` browser suite and `duration-input`, `unsaved-colors` unit suites |
| Hours-profile modal controls, square removal, responsive grouping and scroll reachability | `schedules` browser suite |
| Editor inspector layout stability, focus return, draft preservation, regular card groups | `repository-file-sync` browser suite and `repository-sync-pane` unit suite |
| Native editor handoff, delayed validation, same-session return, format variants | `native-file-handoff` browser suite and `file-adjustment-link`, `session`, `sync-file-page` unit suites |
| File validation through navigation, restored drafts, retries, and discard | `file-validation-lifetime` browser suite and `file-draft-validation` unit suite |
| File observation states, search priority, row alignment, inspector focus return | `config-migration` browser suite and `repository-control` unit suite |
| Configuration sync policy, saved observations, shared switch geometry, advancing timestamps and isolated cache ownership | `config-file-sync` browser suite and `configuration-file-sync`, `config-file-status`, `session` unit suites |
| Alias interactions and shared compact pickers | `pair-entry`, `segmented-control`, `dictionary` browser suites |
| File editor modes, exact output, hidden newline, independent row controls | `file-formatting`, `repository-file-sync` browser suites and `code-editor` unit suite |
| Compact list decisions, disabled alternatives, Undo and accessible descriptions | `file-formatting` browser suite and `sync-file-page` unit suite |
| Continuous press/release paint and movement, including the row edge | `file-formatting` browser suite and `motion` unit suite |
| Fresh conflict choices, view segments, exact highlighted alternatives, context folding and pending writes | `config-file-review` browser and unit suites, `config-file-review-mock`, `diff-block` and `diff-context` unit suites |
| Canonical drafts, save/discard, persistence, invalid input | Settings and editor unit suites, `sync-drafts`, `settings-draft-markers`, `runtime-settings` browser suites |
| Toast lifetime and decision composer | `mutation-receipt`, `settings-save-composer` unit suites and `duration-controls` browser suite |
| Empty, signed-out, and label states | `empty-states`, `signed-out-layout`, `sync-label-layout` browser suites |
| Navigation context and accurate day grouping | `sidebar-selection`, `text-clipping` browser suites |
| Bypass authorization, inheritance, lookup, installation failures, storage | Bypass policy suites in frontend, panel, gate, GitHub, and both storage engines |
| Unresolved actor references, recovered names, exact GitHub IDs through save and reload | `bypass-identities` browser suite and `bypass-policy`, `bypass-persistence`, `bypass-editors` unit suites |
| Themed pickers, keyboard and form semantics, actor suggestions, toggle scrolling, square Remove actions | `select-menus` browser suite and `select`, `shared-picker-styles`, `bypass-editors` unit suites |
| Ruleset action grouping, staged inspectors, lossless field restoration, named app pins, independent markers and 1px adjacent changed-row gap | `settings-draft-markers` browser suite and `sync-rulesets-page`, `ruleset-rule-editor` unit suites |
| Repository option groups, readable picker values, anchored management, focus and wrapped status copy | `sync-drafts` browser suite and `sync-settings-page` unit suite |

Paths above are relative to `internal/panel/frontend` except the backend suites.
CI runs these browser and unit contracts. `mise run lint:matrix` rejects a browser
suite omitted from both CI and the documented local workflow

Tests verify measurable contracts, not visual quality or all wording. Every visual
change also requires screenshot critique in both themes at 375, 768, 1024, and
1440px, including relevant interaction states. Use `SMYKLOT_VISUAL_AUDIT_DIR` with
the browser suite to retain route and empty-state screenshots and rendered copy.
Review narrow and expanded views directly; an automated pass is not design approval

The browser-test lane keeps generated SvelteKit files and Vite dependency caches
separate from the running preview. Tests must not restart the user's preview or
erase the state being reviewed

## Delivery checks

- Contrast: normal text at least 4.5:1 and focus boundaries at least 3:1;
  resting controls use labelled filled surfaces with deliberately quieter borders
- Keyboard order follows visual order
- Every custom menu, dialog, tab, sort control, and inspector is keyboard usable
- Test at 375, 768, 1024, and 1440px
- No horizontal page scroll
- No content hidden behind fixed navigation
- Loading surfaces reserve their final geometry
- Light and dark states are tested independently
