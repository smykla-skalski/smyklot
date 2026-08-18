<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Link from '#lib/components/Link.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Link',
    component: Link,
    args: { href: 'https://github.com/smykla-skalski' },
  });
</script>

<!--
  The panel's other anchors are controls wearing `.btn`, or rows, or navigation. This
  is the one that reads as a link in running text, and `app.css` carried its underline
  treatment with no component to ask for it.
-->
<Story name="Default">
  {#snippet template({ children, ...args })}
    <Link {...args}>smykla-skalski</Link>
  {/snippet}
</Story>

<Story name="In a sentence">
  {#snippet template({ children, ...args })}
    <p>
      Invited by <Link {...args} target="_blank" rel="noreferrer">@bart</Link> for the Smykla Skalski
      organization
    </p>
  {/snippet}
</Story>
