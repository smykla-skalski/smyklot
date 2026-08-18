<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import NightPage from '#lib/components/NightPage.svelte';

  const BUILD = { version: '1.37.0', serviceHost: 'smyklot.com' };

  const { Story } = defineMeta({
    title: 'Decor/NightPage',
    component: NightPage,
    argTypes: { busy: { control: 'boolean' } },
    args: {
      title: 'Access invitation',
      documentTitle: 'Access Invitation',
      build: BUILD,
      busy: false,
    },
    parameters: { bleed: true, layout: 'fullscreen' },
  });
</script>

<!--
  The page every signed-out address uses: the sky, a card on it, and the footer. The
  card is a straight-through quiet zone - nothing crosses behind it.
-->
<Story name="Default">
  {#snippet template({ children, ...args })}
    <NightPage {...args}>
      <p>Whatever the page has to say goes in the card.</p>
    </NightPage>
  {/snippet}
</Story>

<Story name="Busy" args={{ busy: true }}>
  {#snippet template({ children, ...args })}
    <NightPage {...args}>
      <p>Waiting on an answer.</p>
    </NightPage>
  {/snippet}
</Story>
