# Smyklot panel design system

This file is the visual source of truth for the Smyklot administration panel.
Page-specific files under `pages/` may override it only when the exception is
explicitly documented.

## Direction

- Product: GitHub App administration and operations dashboard
- Style: trust and authority, minimal, data-dense, dark/light parity
- Density: 8/10
- Motion: 3/10
- Primary accent: indigo, never a semantic state color
- Semantic colors: sky information, emerald success, amber warning, red danger
- Icons: one Phosphor-style outline family with regular strokes
- UI typography: Plus Jakarta Sans
- Technical data: JetBrains Mono

Gold, gradients, glows, glass surfaces, decorative motion, left-edge content
accents, and mixed icon families are not part of the panel system. The official
Smyklot avatar may retain its own colors.

## Color tokens

### Foundations

| Role | Light | Dark |
| --- | --- | --- |
| Canvas | `#F8FAFC` | `#080B12` |
| Sidebar | `#0B1220` | `#0B1220` |
| Surface | `#FFFFFF` | `#111827` |
| Elevated surface | `#FFFFFF` | `#182235` |
| Subdued surface | `#F1F5F9` | `#1E293B` |
| Selected surface | `#EEF2FF` | `#1E1B4B` |
| Divider | `#E2E8F0` | `#334155` |
| Control border | `#7C8A9E` | `#64748B` |
| Primary text | `#0F172A` | `#F8FAFC` |
| Secondary text | `#475569` | `#CBD5E1` |
| Muted text | `#5F6B7A` | `#94A3B8` |

The sidebar stays midnight in both themes so navigation remains a stable visual
anchor.

### Brand and interaction

| Role | Value |
| --- | --- |
| Primary | `#4F46E5` |
| Hover | `#4338CA` |
| Pressed | `#3730A3` |
| Dark focus | `#818CF8` |
| Light selected text | `#3730A3` |
| Dark selected text | `#C7D2FE` |
| Optional brand secondary | `#7C3AED` |

Violet is limited to official brand artwork or future data visualization. It
does not create a second control accent.

### Semantic pairs

| Meaning | Light foreground/background | Dark foreground/background |
| --- | --- | --- |
| Information/default | `#075985` / `#F0F9FF` | `#7DD3FC` / `#0C4A6E` |
| Success/on/active | `#047857` / `#ECFDF5` | `#86EFAC` / `#052E16` |
| Warning/expired/bypass | `#92400E` / `#FFFBEB` | `#FDE68A` / `#451A03` |
| Danger/off/banned | `#B91C1C` / `#FEF2F2` | `#FCA5A5` / `#450A0A` |
| Neutral/inherited | `#475569` / `#F1F5F9` | `#CBD5E1` / `#1E293B` |

Use roughly 80% neutral, 15% indigo, and at most 5% semantic color. Color never
communicates state without a label or icon. Green is success only and is never
the ordinary primary action.

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

- Sidebar is midnight in both themes
- Selected navigation uses an indigo-tinted fill, stronger text, and icon
- Do not add a second rail, edge strip, or decorative marker
- Role appears as a compact label inside the account area
- Mobile uses a top bar and labelled navigation with no more than five items

### Buttons and controls

- Primary actions are solid indigo with white text
- Secondary actions use neutral surfaces and borders
- Ghost actions use transparent backgrounds with visible hover states
- Danger is reserved for destructive workflows
- Focus replaces the normal control boundary with one two-pixel indicator
- Press feedback uses stable color/elevation and at most `scale(.98)`

### Tabs

- Use text tabs with a two-pixel indigo underline
- Count badges are neutral and compact
- Do not render top-level tabs as filled pills

### Dialogs and inspectors

- Ordinary dialogs are `560-640px` wide
- Scrim uses 50-55% black plus restrained background blur
- Use clear header, scrollable body, and sticky action footer
- Preserve Escape, outside click, focus trap, and focus restoration
- Confirm before dismissing unsaved work
- Repository and user details use a right inspector on wide screens, an overlay
  sheet on medium screens, and a full-screen detail surface on mobile

### Tables

- Toolbar contains search, one filter affordance, relevant scope, and one
  primary action
- Active filters appear as removable chips
- Sorting lives in column-header buttons with `aria-sort`
- Page-size control appears in bottom pagination
- Use neutral dividers instead of zebra striping
- Hover is neutral; selected rows use the indigo selected surface
- Status uses semantic icon-plus-label badges
- Roles use neutral outline icons and text, not multiple semantic colors
- More-actions controls keep at least a 40px desktop hit area
- User and invitation tables become structured list rows below 768px
- History becomes a compact activity list on narrow screens

## Icon vocabulary

Use SVG symbols from one Phosphor-style outline system.

| Purpose | Symbol |
| --- | --- |
| Settings | Gear or SlidersHorizontal |
| Repositories | GithubLogo or GitBranch |
| Users | Users |
| History | ClockCounterClockwise |
| Help | Lifebuoy |
| Search | MagnifyingGlass |
| Filters | Funnel |
| Global / organization / personal | Globe / Buildings / UserCircle |
| Public / private | Globe / Lock |
| Add user | UserPlus |
| More actions | DotsThree |
| Remove | Trash |
| Success / pending / warning / failure | CheckCircle / Clock / Warning / XCircle |
| Information | Info |

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

- Contrast: normal text at least 4.5:1; meaningful UI boundaries at least 3:1
- Keyboard order follows visual order
- Every custom menu, dialog, tab, sort control, and inspector is keyboard usable
- Test at 375, 768, 1024, and 1440px
- No horizontal page scroll
- No content hidden behind fixed navigation
- Loading surfaces reserve their final geometry
- Light and dark states are tested independently
