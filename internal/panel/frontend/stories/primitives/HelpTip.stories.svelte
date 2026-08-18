<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import HelpTip from '#lib/components/HelpTip.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/HelpTip',
    component: HelpTip,
    argTypes: { align: { control: 'inline-radio', options: ['start', 'end'] } },
    args: {
      id: 'story-help',
      label: 'About the configuration file',
      text: 'Smyklot reads .smyklot.toml from the default branch on every delivery',
      align: 'end',
    },
  });
</script>

<!--
  The question mark beside a control, and the sentence it holds. The tip itself is
  `AppTooltip` - this used to wire up its own and paint it in reverse, against a token
  nothing declared, so every help tip in the panel was a black rectangle with its
  sentence in black inside it. Nobody had ever read one.
-->
<Story name="Beside a label">
  {#snippet template(args)}
    <p class="row">
      <span>Configuration file</span>
      <HelpTip {...args} />
    </p>
  {/snippet}
</Story>

<style>
  .row {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    margin: var(--space-8) 0;
  }
</style>
