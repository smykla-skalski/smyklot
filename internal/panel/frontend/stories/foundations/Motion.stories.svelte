<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import TokenTable from '../support/TokenTable.svelte';

  const MOTION = [
    { name: 'duration-fast', note: 'A colour or a border changing under the pointer' },
    { name: 'duration-normal', note: 'A layout that moves - the sidebar collapsing' },
    { name: 'duration-press', note: 'The scale a press applies, and its release' },
    { name: 'ease-out', note: 'Something arriving' },
    { name: 'ease-standard', note: 'Something moving from one place to another' },
    { name: 'press-scale', note: 'How far a pressed control shrinks' },
    { name: 'press-scale-compact', note: 'The same, for a control inside a row' },
    { name: 'press-scale-disc', note: 'For a round control, where the same ratio reads larger' },
    { name: 'press-scale-surface', note: 'For a whole row, where it reads larger still' },
  ];

  const { Story } = defineMeta({ title: 'Foundations/Motion' });
</script>

<!--
  Motion is 3/10 in this product: it says something moved, and then gets out of the
  way. There is one blanket `prefers-reduced-motion` rule in `app.css` that shortens
  every animation to 0.01ms and takes the press transform off, so a component does not
  answer that question for itself.

  The press scale is four values rather than one because the same ratio reads larger
  on a bigger shape - a disc and a whole row need less of it than a button does.
-->
<Story name="Durations, easings and press">
  {#snippet template()}
    <TokenTable tokens={MOTION} swatch={false} />
  {/snippet}
</Story>
