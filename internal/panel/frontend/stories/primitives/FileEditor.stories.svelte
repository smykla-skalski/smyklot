<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FileEditor from '#lib/components/FileEditor.svelte';
  const { Story } = defineMeta({
    title: 'Primitives/FileEditor',
    component: FileEditor,
    args: {
      value: '{"enabled":true}\n',
      output: '{\n  "enabled": true\n}\n',
      lang: 'json',
      onChange: () => {},
    },
  });
</script>

<Story name="Edit and preview" />
<Story name="Read only" args={{ readOnly: true }} />
<Story
  name="Invalid template"
  args={{ output: null, problem: 'JSON must be a complete document' }}
/>
