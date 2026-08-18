<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import AppTooltip from '#lib/components/AppTooltip.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/AppTooltip',
    component: AppTooltip,
    argTypes: {
      side: { control: 'inline-radio', options: ['top', 'right', 'bottom', 'left'] },
      align: { control: 'inline-radio', options: ['start', 'center', 'end'] },
    },
    args: {
      id: 'story-tip',
      text: 'The service reads this file on every delivery',
      side: 'top',
      align: 'center',
    },
  });
</script>

<!--
  The panel's one tooltip. It portals to `.app-shell`, which is what keeps it inside
  the active palette - hover the trigger to see it.

  The trigger is a snippet taking Bits UI's props, so anything can be a trigger
  without this component knowing what.
-->
<Story name="Playground">
  {#snippet template({ children, ...args })}
    <div class="frame">
      <AppTooltip {...args}>
        {#snippet children(props)}
          <button {...props} type="button" class="btn">Hover me</button>
        {/snippet}
      </AppTooltip>
    </div>
  {/snippet}
</Story>

<Story name="Every side">
  {#snippet template()}
    <div class="grid">
      {#each ['top', 'right', 'bottom', 'left'] as side (side)}
        <AppTooltip id={`tip-${side}`} text={`Placed ${side}`} side={side as 'top'}>
          {#snippet children(props)}
            <button {...props} type="button" class="btn">{side}</button>
          {/snippet}
        </AppTooltip>
      {/each}
    </div>
  {/snippet}
</Story>

<style>
  .frame {
    padding: var(--space-8) 0;
  }
  .grid {
    display: flex;
    gap: var(--space-4);
    padding: var(--space-8) 0;
  }
</style>
