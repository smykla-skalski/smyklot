<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import TokenTable from '../support/TokenTable.svelte';

  const FOUNDATIONS = [
    { name: 'canvas', note: 'The page behind everything' },
    { name: 'sidebar-bg', note: 'The rail, one step off the canvas' },
    { name: 'surface-base', note: 'A card' },
    { name: 'surface-raised', note: 'A card that sits above another' },
    { name: 'surface-inset', note: 'A well inside a card - a track, a code block' },
    { name: 'border-subtle', note: 'The rule between rows' },
    { name: 'control-border', note: 'The edge of a control' },
    { name: 'text-primary', note: 'What the page is about' },
    { name: 'text-secondary', note: 'Explanations and meta' },
    { name: 'text-muted', note: 'Present but not being read' },
  ];

  const BRAND = [
    { name: 'accent', note: 'Restrained petrol in the panel, violet on the Root console' },
    { name: 'brand-action', note: 'The one action a view is here for' },
    { name: 'brand-action-hover', note: 'Under the pointer' },
    { name: 'brand-action-pressed', note: 'Held' },
    { name: 'brand-action-text', note: 'Brand ink on a plain ground' },
    { name: 'admin', note: 'The filled signal button' },
    { name: 'on-admin', note: 'Ink on that fill' },
    { name: 'focus', note: 'The focus ring, and never anything else' },
  ];

  const SEMANTIC = [
    { name: 'clear', note: 'Teal success' },
    { name: 'clear-tint', note: 'Its ground' },
    { name: 'warning', note: 'Amber' },
    { name: 'warning-tint', note: 'Its ground' },
    { name: 'danger', note: 'Muted rose - `--stop` is an alias of this' },
    { name: 'danger-tint', note: 'Its ground' },
    { name: 'info', note: 'Blue-grey information' },
    { name: 'info-tint', note: 'Its ground' },
  ];

  const INTERACTION = [
    { name: 'interactive-hover', note: 'The ground a row or a control takes under the pointer' },
    {
      name: 'interactive-hover-layer',
      note: 'The overlay hover is painted WITH - an image, not a colour',
    },
    { name: 'press', note: 'The overlay a press replaces the hover with' },
    { name: 'press-scale', note: 'How far a pressed control shrinks' },
  ];

  const { Story } = defineMeta({ title: 'Foundations/Colours' });
</script>

<!--
  Every value here is read from the live `.app-shell`, not copied from `MASTER.md`.
  Flip the theme and console toolbars and the numbers change with them - which is the
  point: there are FOUR palettes, not two, because `.app-shell.root-mode` re-declares
  the whole alias set rather than inheriting it.
-->
<Story name="Foundations">
  {#snippet template()}
    <TokenTable tokens={FOUNDATIONS} />
  {/snippet}
</Story>

<Story name="Brand and interaction">
  {#snippet template()}
    <TokenTable tokens={BRAND} />
  {/snippet}
</Story>

<!--
  Semantic colours come in pairs: the ink and the tint it sits on. Nothing in the
  panel is told apart by hue alone - a state always carries a glyph as well - because
  several of these pairs cannot survive one dichromacy or another.
-->
<Story name="Semantic pairs">
  {#snippet template()}
    <TokenTable tokens={SEMANTIC} />
  {/snippet}
</Story>

<!--
  Hover is a background-IMAGE, not a background-colour. Every probe that reads
  `backgroundColor` finds `rgba(0, 0, 0, 0)` here and concludes there is no hover;
  read `backgroundImage` too.
-->
<Story name="Interaction">
  {#snippet template()}
    <TokenTable tokens={INTERACTION} />
  {/snippet}
</Story>
