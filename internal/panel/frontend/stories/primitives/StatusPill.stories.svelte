<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import StatusPill from '#lib/components/StatusPill.svelte';
  import Icon from '#lib/components/Icon.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/StatusPill',
    component: StatusPill,
    argTypes: {
      dot: { control: 'boolean' },
      live: { control: 'boolean' },
      state: { control: 'select', options: [undefined, 'healthy', 'degraded', 'unavailable'] },
    },
    args: { dot: false, live: false },
  });
</script>

<Story name="Playground">
  {#snippet template({ children, ...args })}
    <StatusPill {...args}>Application scope</StatusPill>
  {/snippet}
</Story>

<!--
  A pill says what is true of the surface it sits on; a chip carries a row's value.
  That is the whole distinction, and it is why this one is uppercase and quieter.
-->
<Story name="Plain">
  {#snippet template()}
    <StatusPill>Application scope</StatusPill>
  {/snippet}
</Story>

<Story name="Live">
  {#snippet template()}
    <div class="row">
      <StatusPill dot live>WebSocket live</StatusPill>
      <StatusPill dot live>Changes apply live</StatusPill>
      <StatusPill dot live>Webhook driven</StatusPill>
    </div>
  {/snippet}
</Story>

<!-- The three words `databaseState` decides, and the only place a pill takes a colour. -->
<Story name="Health">
  {#snippet template()}
    <div class="row">
      <StatusPill dot state="healthy">healthy</StatusPill>
      <StatusPill dot state="degraded">degraded</StatusPill>
      <StatusPill dot state="unavailable">unavailable</StatusPill>
    </div>
  {/snippet}
</Story>

<Story name="With a symbol">
  {#snippet template()}
    <div class="row">
      <StatusPill>
        {#snippet icon()}<Icon name="shield" size={14} />{/snippet}
        Owner access
      </StatusPill>
      <StatusPill>
        {#snippet icon()}<Icon name="warning" size={14} />{/snippet}
        Elevated
      </StatusPill>
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
