<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import CopyableLinkField from '#lib/components/CopyableLinkField.svelte';

  const LINK = 'https://smyklot.com/invite/8f2c1d9e-4a7b-4f31-9c2e-0b6d5a3f81cc';

  const { Story } = defineMeta({
    title: 'Primitives/CopyableLinkField',
    component: CopyableLinkField,
    argTypes: { failed: { control: 'boolean' } },
    args: { label: 'Invitation link', value: LINK, failed: false },
  });
</script>

<Story name="Default" />

<!--
  Readonly rather than disabled: the value still has to be selectable, because
  copying it by hand is exactly what this field is for once the clipboard is gone.
-->
<Story name="Clipboard refused" args={{ failed: true }} />
