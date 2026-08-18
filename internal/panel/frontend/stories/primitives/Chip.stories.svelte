<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Chip, { type ChipTone } from '#lib/components/Chip.svelte';

  const TONES: ChipTone[] = ['neutral', 'clear', 'signal', 'accent', 'warning', 'stop', 'absent'];

  const { Story } = defineMeta({
    title: 'Primitives/Chip',
    component: Chip,
    argTypes: {
      tone: { control: 'select', options: TONES },
      dot: { control: 'boolean' },
      small: { control: 'boolean' },
    },
    args: { tone: 'neutral', dot: false, small: false },
  });
</script>

<!--
  `children` comes out of the destructure and is never spread back in.

  Storybook's docgen puts every required prop into `args`, and `Chip` requires a
  `children` snippet - so `<Chip {...args}>Enabled</Chip>` would set it twice and the
  markup between the tags would lose. Taking it out of the rest is the pattern every
  story for a component with required snippet props uses.
-->
<Story name="Playground">
  {#snippet template({ children, ...args })}
    <Chip {...args}>Enabled</Chip>
  {/snippet}
</Story>

<!-- Every tone at once, which is the comparison the app can never show: they are
     scattered across seven tables and no page carries more than three. -->
<Story name="All tones">
  {#snippet template()}
    <div class="row">
      {#each TONES as tone (tone)}
        <Chip {tone}>{tone}</Chip>
      {/each}
    </div>
  {/snippet}
</Story>

<Story name="Live states">
  {#snippet template()}
    <div class="row">
      {#each TONES as tone (tone)}
        <Chip {tone} dot>{tone}</Chip>
      {/each}
    </div>
  {/snippet}
</Story>

<Story name="With icon" args={{ tone: 'stop', icon: 'ban' }}>
  {#snippet template({ children, ...args })}
    <Chip {...args}>Refused</Chip>
  {/snippet}
</Story>

<Story name="Small" args={{ small: true, tone: 'accent' }}>
  {#snippet template({ children, ...args })}
    <p>A chip that sits <Chip {...args}>inside</Chip> a line of text rather than beside it.</p>
  {/snippet}
</Story>

<style>
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>
