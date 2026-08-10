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

Gold, gradients, glows, glass surfaces, decorative motion, left-edge content
accents, and mixed icon families are not part of the panel system. The official
Smyklot avatar and the three-pixel closing rule above the app footer may retain the
brand rainbow. The closing rule moves its seamless, full-hue repeated spectrum
slowly to the right, with reduced-motion preferences disabling the movement.

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
| Table primary | `14px / 20px` | 600 |
| Table secondary | `13px / 18px` | 400 |
| Control label | `13px / 18px` | 600 |
| Technical data | `13px / 18px` | 400-500 |

Do not use monospace for headings, navigation, ordinary labels, buttons, or
long prose. Technical values use tabular figures.

## Spacing, shape, and elevation

- Spacing scale: `4, 8, 12, 16, 24, 32px`
- Surface radius: `10px`
- Control radius: `8px`
- Chips and status badges may use a full pill radius
- Desktop controls: `40px` visual height
- Touch controls: at least `44px` hit area
- Table header: `40px`
- Table rows: `52-56px`
- Table cell padding: `16px` horizontal
- Sidebar: `240px` expanded, `72px` collapsed
- Shadows appear only on raised menus, inspectors, and dialogs

Cards are used only when content forms a real unit. Settings use sections and
dividers instead of nested cards.

## Components

### Navigation

- Sidebar uses a light neutral surface in light mode and midnight in dark mode
- Selected navigation uses a petrol-tinted fill, stronger text, and icon
- Do not add a second rail, edge strip, or decorative marker
- The bottom account area stays visually quiet and does not add a custom role strip
- The collapse control sits beside the brand when expanded and straddles the
  sidebar edge when collapsed; it appears only on sidebar hover or focus-within
- Mobile uses a top bar and labelled navigation with no more than five items

### Buttons and controls

- Primary actions are solid petrol with a theme-appropriate foreground
- Secondary actions use neutral surfaces and borders
- Ghost actions use transparent backgrounds with visible hover states
- Danger is reserved for destructive workflows
- Focus replaces the normal control boundary with one two-pixel indicator
- Press feedback uses stable color/elevation and at most `scale(.98)`

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
- Confirm before dismissing unsaved work
- Repository settings and user access history use centered wide modals with a
  blurred scrim; repository sections use one horizontal tab row at every width

### Tables

- Toolbar contains search, relevant scope, and one primary action; column filters
  live beside their column labels
- Active column filters keep a visible selected treatment on their header affordance
- Sorting lives in column-header buttons with `aria-sort`
- Large datasets use cursor-backed infinite loading with TanStack Table state and
  TanStack Virtual rendering; there is no page-size or numbered-page control
- Filter and sort requests keep current rows mounted, mark the table busy, and
  replace rows in place when the latest request completes
- Infinite tables have no footer; incremental-load failures use a temporary inline
  recovery prompt without reducing the body viewport
- Desktop tables keep the header outside the native vertical body scroller, with no
  custom scrollbar skin or permanently reserved scrollbar gutter
- Use neutral dividers instead of zebra striping
- Desktop virtualized table rows share a 65px track and every data cell draws the
  same bottom divider, so content cannot overlap or hide row boundaries
- Hover is neutral; selected rows use the petrol selected surface
- Status uses semantic icon-plus-label badges
- Roles use neutral outline icons and text, not multiple semantic colors
- More-actions controls use a heavy DotsThree glyph in a borderless ghost target
  with at least a 40px desktop hit area
- User and invitation tables become structured list rows below 768px
- History becomes a compact activity list on narrow screens
- Audit history orders columns as Actor, Target, Change, When; actor identity uses
  the same name and handle typography as Access
- Filtered empty results use a centered icon, explanation, and recovery action in
  the full table-body viewport
- The sidebar is the only organization picker. User access scope chooses only
  Global or the currently selected installation
- Users/Invitations and Add user share one tab row above the standalone search
  and filter toolbar; the Add dialog inherits and summarizes the page scope
- Table role changes use a custom top-layer listbox so menus are never clipped
  by the table or rendered with browser-native styling
- Search and filters never share the table's border
- Repository names use a descender-safe line box and optical vertical alignment;
  the first and last row controls keep equal 16px visual outer insets
- Repository detail navigation uses plain semantic count text, not count pills
- Installation and account menus are content-sized, keep search visible while
  options scroll, and omit type labels already stated by their group heading
- Light and dark share identical geometry. The compact account menu owns the
  explicit theme selector, and the choice persists as a browser-local preference
- Right-edge filter menus align their right edge to the trigger so their content
  opens toward available space instead of being clipped by the viewport

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
- Exit is faster than entry
- Animate transform and opacity only
- Never animate more than two elements for one interaction
- Every effect has a `prefers-reduced-motion` fallback

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
