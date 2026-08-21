<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import LabelColorPicker from '#lib/components/LabelColorPicker.svelte';

  /* The picker positions itself under a dot; the story stands in for one. */
  const IN_USE = ['#d73a4a', '#a2eeef', '#0e8a16', '#7057ff', '#6b7280'];

  const { Story } = defineMeta({
    title: 'Primitives/LabelColorPicker',
    component: LabelColorPicker,
    args: {
      color: '#0e8a16',
      inUse: IN_USE,
      onApply: fn(),
      onPick: fn(),
    },
  });
</script>

<!--
  The label colour picker: the saturation/value area, a horizontal hue rail,
  the hex field with its live swatch, then every colour the list already
  carries and GitHub's sixteen presets - wheel-ordered, the check riding the
  current tile. Dragging applies silently and commits on release; a tile
  press is a finished act.
-->
<Story name="Open on a green">
  {#snippet template(args)}
    <div style="position: relative; min-height: 480px;">
      <LabelColorPicker {...args} />
    </div>
  {/snippet}
</Story>

<!-- A grey keeps the hue it arrived with - the rail does not snap to red. -->
<Story name="Open on a grey" args={{ color: '#6b7280' }}>
  {#snippet template(args)}
    <div style="position: relative; min-height: 480px;">
      <LabelColorPicker {...args} />
    </div>
  {/snippet}
</Story>
