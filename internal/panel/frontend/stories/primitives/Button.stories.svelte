<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import Button, { type ButtonTone } from '#lib/components/Button.svelte';
  import Icon from '#lib/components/Icon.svelte';

  const TONES: ButtonTone[] = [
    'default',
    'signal',
    'ghost',
    'stop',
    'stop-quiet',
    'brand',
    'quiet',
  ];

  const { Story } = defineMeta({
    title: 'Primitives/Button',
    component: Button,
    argTypes: {
      tone: { control: 'select', options: TONES },
      row: { control: 'boolean' },
      disabled: { control: 'boolean' },
    },
    args: { tone: 'default', row: false, disabled: false, onclick: fn() },
  });
</script>

<Story name="Playground">
  {#snippet template({ children, ...args })}
    <Button {...args}>Try again</Button>
  {/snippet}
</Story>

<!--
  Every tone side by side, which no page in the panel shows: they are spread over
  twenty-four files and no single view carries more than three. Two of them -
  `stop-quiet` and `brand` - were declared inside single components until the
  extraction, so until now they could not be seen next to the tones they belong with.

  `brand` is the one to check in both consoles: it is tinted from `--brand-action`,
  which is teal in the panel and violet on the Root console.
-->
<Story name="All tones">
  {#snippet template()}
    <div class="row">
      {#each TONES as tone (tone)}
        <Button {tone}>{tone}</Button>
      {/each}
    </div>
  {/snippet}
</Story>

<Story name="Disabled">
  {#snippet template()}
    <div class="row">
      {#each TONES as tone (tone)}
        <Button {tone} disabled>{tone}</Button>
      {/each}
    </div>
  {/snippet}
</Story>

<!--
  The icon's optical bearing is corrected by `app.css`, keyed on the icon being the
  button's own first child rather than on a prop - so it holds here without the story
  asking for it. Worth looking at against the plain tones above: the word should start
  at the same place in both.
-->
<Story name="With icon">
  {#snippet template()}
    <div class="row">
      <Button tone="signal">
        {#snippet icon()}<Icon name="user-plus" size={14} strokeWidth={2} />{/snippet}
        Add user
      </Button>
      <Button>
        {#snippet icon()}<Icon name="refresh" size={14} strokeWidth={2} />{/snippet}
        Refresh
      </Button>
      <Button tone="stop">
        {#snippet icon()}<Icon name="trash" size={14} strokeWidth={2} />{/snippet}
        Remove
      </Button>
    </div>
  {/snippet}
</Story>

<Story name="In a row" args={{ row: true }}>
  {#snippet template({ children, ...args })}
    <div class="row">
      <Button {...args}>Configure</Button>
      <Button {...args} tone="quiet">Dismiss</Button>
    </div>
  {/snippet}
</Story>

<style>
  .row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>
