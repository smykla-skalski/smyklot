<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Plate from '#lib/components/Plate.svelte';
  import Chip from '#lib/components/Chip.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Plate',
    component: Plate,
    argTypes: {
      tone: { control: 'select', options: ['plain', 'lead', 'alarm'] },
    },
    args: { label: 'Repositories', tone: 'plain' },
  });
</script>

<Story name="Plain">
  {#snippet template(args)}
    <Plate {...args}>
      <p>The plate is the panel's one surface. Everything a page says sits on one.</p>
    </Plate>
  {/snippet}
</Story>

<Story name="Lead" args={{ label: 'Queue', tone: 'lead' }}>
  {#snippet template(args)}
    <Plate {...args}>
      <p>The plate the page is here for. One per page, or the weight means nothing.</p>
    </Plate>
  {/snippet}
</Story>

<Story name="Alarm" args={{ label: 'Problem', tone: 'alarm' }}>
  {#snippet template(args)}
    <Plate {...args}>
      <p>Something went wrong and the reader has to do something about it.</p>
    </Plate>
  {/snippet}
</Story>

<Story name="With status" args={{ label: 'Service' }}>
  {#snippet template(args)}
    <Plate {...args}>
      {#snippet status()}
        <Chip tone="signal" dot>Healthy</Chip>
      {/snippet}
      <p>The right-hand side of the header carries the plate's current state.</p>
    </Plate>
  {/snippet}
</Story>

<Story name="Header only" args={{ label: 'Nothing to show' }}>
  {#snippet template(args)}
    <Plate {...args} />
  {/snippet}
</Story>
