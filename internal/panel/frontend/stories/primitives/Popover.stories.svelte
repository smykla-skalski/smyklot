<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Popover from '#lib/components/Popover.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Popover',
    component: Popover,
    argTypes: {
      align: { control: 'inline-radio', options: ['start', 'center', 'end'] },
      side: { control: 'inline-radio', options: ['above', 'below', 'left', 'right'] },
      width: { control: 'inline-radio', options: ['auto', 'min-trigger', 'trigger'] },
      skin: { control: 'inline-radio', options: ['default', 'sidebar'] },
      role: { control: 'inline-radio', options: ['dialog', 'listbox', 'menu'] },
    },
    args: { align: 'start', side: 'below', width: 'auto', skin: 'default', label: 'Example' },
  });
</script>

<!--
  One layer for six things that used to hand-roll their own. Bits UI owns focus,
  keyboard and positioning; this decides the panel's own shape, skin and dismissal.

  It portals to `.app-shell`, so it inherits whichever palette is active - worth
  flipping the console toolbar on.
-->
<Story name="Playground">
  {#snippet template({ children, ...args })}
    <div class="frame">
      <Popover {...args}>
        {#snippet trigger(attributes)}
          <button {...attributes} type="button" class="btn">Open</button>
        {/snippet}
        <div class="body">
          <p>Anything can go in here.</p>
        </div>
      </Popover>
    </div>
  {/snippet}
</Story>

<!-- `trigger` matches the trigger's width; `min-trigger` is at least that wide. -->
<Story name="Matching the trigger" args={{ width: 'trigger' }}>
  {#snippet template({ children, ...args })}
    <div class="frame">
      <Popover {...args}>
        {#snippet trigger(attributes)}
          <button {...attributes} type="button" class="btn">A wider trigger than the body</button>
        {/snippet}
        <div class="body"><p>Short.</p></div>
      </Popover>
    </div>
  {/snippet}
</Story>

<style>
  .frame {
    padding-bottom: 16rem;
  }
  .body {
    min-width: 12rem;
    padding: var(--space-3);
  }
  .body p {
    margin: 0;
  }
</style>
